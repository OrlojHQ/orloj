import { useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import {
  ArrowLeft,
  CircleStop,
  GitFork,
  Pause,
  Play,
  RotateCcw,
  Send,
  ShieldCheck,
  Wifi,
  WifiOff,
  X,
} from "lucide-react";
import { useNavigate, useParams } from "react-router-dom";
import {
  useForkSessionCheckpoint,
  useReplaySessionCheckpoint,
  useRewindSessionCheckpoint,
  useSession,
  useSessionAction,
  useSessionCheckpoints,
  useSendSessionTurn,
} from "../api/hooks";
import {
  mergeSessionEvents,
  subscribeToSession,
  type SessionStreamState,
} from "../api/sessionStream";
import type { SessionCheckpoint, SessionEvent } from "../api/types";
import { AiDisclosure } from "../components/AiDisclosure";
import { confirmDialog } from "../components/ConfirmDialog";
import { DetailSkeleton } from "../components/DetailSkeleton";
import { ResourceDetailLoadError } from "../components/ResourceDetailLoadError";
import { SessionTimeline } from "../components/SessionTimeline";
import { StatusBadge } from "../components/StatusBadge";
import { toast } from "../components/Toast";
import { useDetailReturnNav } from "../hooks/useDetailReturnNav";
import { buildSessionTimeline } from "../utils/sessionTimeline";

function phaseOf(phase?: string): string {
  return phase ?? "Unknown";
}

export function SessionDetail() {
  const { name: nameParam } = useParams<{ name: string }>();
  const routeName = nameParam ?? "";
  const navigate = useNavigate();
  const { goBack } = useDetailReturnNav("/sessions");
  const sessionQuery = useSession(routeName);
  const checkpointsQuery = useSessionCheckpoints(routeName);
  const turnMutation = useSendSessionTurn();
  const actionMutation = useSessionAction();
  const replayMutation = useReplaySessionCheckpoint();
  const rewindMutation = useRewindSessionCheckpoint();
  const forkMutation = useForkSessionCheckpoint();
  const [events, setEvents] = useState<SessionEvent[]>([]);
  const [streamState, setStreamState] = useState<SessionStreamState>("connecting");
  const [streamRevision, setStreamRevision] = useState(0);
  const [content, setContent] = useState("");
  const [interrupt, setInterrupt] = useState(false);
  const [selectedCheckpointID, setSelectedCheckpointID] = useState("");
  const [replayOpen, setReplayOpen] = useState(false);
  const [rewindOpen, setRewindOpen] = useState(false);
  const [rewindResume, setRewindResume] = useState(true);
  const [rewindInterrupt, setRewindInterrupt] = useState(false);
  const [forkOpen, setForkOpen] = useState(false);
  const [forkName, setForkName] = useState("");
  const [forkResume, setForkResume] = useState(false);
  const timelineEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    setEvents([]);
    setSelectedCheckpointID("");
    setReplayOpen(false);
    setRewindOpen(false);
    setForkOpen(false);
  }, [routeName]);

  useEffect(() => {
    if (!routeName) return;
    return subscribeToSession(routeName, {
      onEvent: (event) => {
        setEvents((current) => mergeSessionEvents(current, [event]));
        if (
          event.type.startsWith("checkpoint.") ||
          event.type.startsWith("session.")
        ) {
          void sessionQuery.refetch();
          void checkpointsQuery.refetch();
        }
      },
      onStateChange: setStreamState,
    });
    // Reconnecting on every detail poll would interrupt the live stream.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [routeName, streamRevision]);

  const checkpoints = checkpointsQuery.data ?? [];
  const timeline = useMemo(
    () =>
      buildSessionTimeline(
        events,
        checkpoints,
        sessionQuery.data?.status?.lastCheckpointID,
      ),
    [events, checkpoints, sessionQuery.data?.status?.lastCheckpointID],
  );
  const selectedCheckpoint = useMemo(
    () =>
      checkpoints.find((checkpoint) => checkpoint.id === selectedCheckpointID),
    [checkpoints, selectedCheckpointID],
  );

  useEffect(() => {
    if (
      selectedCheckpointID &&
      checkpoints.some((checkpoint) => checkpoint.id === selectedCheckpointID)
    ) {
      return;
    }
    const head = sessionQuery.data?.status?.lastCheckpointID;
    const next =
      (head && checkpoints.find((checkpoint) => checkpoint.id === head)?.id) ||
      checkpoints[0]?.id ||
      "";
    setSelectedCheckpointID(next);
  }, [
    checkpoints,
    selectedCheckpointID,
    sessionQuery.data?.status?.lastCheckpointID,
  ]);

  useEffect(() => {
    timelineEndRef.current?.scrollIntoView({ behavior: "smooth", block: "nearest" });
  }, [events.length, timeline.length]);

  if (sessionQuery.isError) {
    return (
      <ResourceDetailLoadError
        title="Session"
        message={
          sessionQuery.error instanceof Error
            ? sessionQuery.error.message
            : "Failed to load session"
        }
        goBack={goBack}
      />
    );
  }

  if (sessionQuery.isLoading || !sessionQuery.data) {
    return <DetailSkeleton />;
  }

  const session = sessionQuery.data;
  const phase = phaseOf(session.status?.phase);
  const phaseLower = phase.toLowerCase();
  const paused = phaseLower === "paused";
  const terminal =
    phaseLower === "cancelled" ||
    phaseLower === "canceled" ||
    phaseLower === "completed" ||
    phaseLower === "succeeded" ||
    phaseLower === "failed" ||
    phaseLower === "expired";
  const lastError = session.status?.lastError;

  const selectCheckpoint = (checkpointID: string) => {
    setSelectedCheckpointID(checkpointID);
    setReplayOpen(false);
    replayMutation.reset();
  };

  const sendTurn = async (event: FormEvent) => {
    event.preventDefault();
    const trimmed = content.trim();
    if (!trimmed || turnMutation.isPending || terminal) return;
    try {
      await turnMutation.mutateAsync({
        name: routeName,
        content: trimmed,
        interrupt: interrupt || undefined,
        idempotencyKey: crypto.randomUUID(),
      });
      setContent("");
      setInterrupt(false);
    } catch {
      // The mutation error remains visible below the composer.
    }
  };

  const runAction = async (action: "pause" | "resume" | "cancel") => {
    if (
      action === "cancel" &&
      !(await confirmDialog({
        title: "Cancel session?",
        message:
          "Cancellation is terminal. Any active or queued turns will be stopped and cannot be resumed.",
        confirmLabel: "Cancel session",
        danger: true,
      }))
    ) {
      return;
    }
    try {
      await actionMutation.mutateAsync({ name: routeName, action });
      await sessionQuery.refetch();
      toast(
        action === "cancel" ? "warning" : "success",
        action === "resume"
          ? "Session resumed"
          : action === "pause"
            ? "Session paused"
            : "Session cancelled",
      );
    } catch {
      // The mutation error remains visible in the header.
    }
  };

  const replayCheckpoint = async (checkpoint: SessionCheckpoint) => {
    setSelectedCheckpointID(checkpoint.id);
    setReplayOpen(true);
    try {
      await replayMutation.mutateAsync({
        name: routeName,
        checkpointID: checkpoint.id,
      });
    } catch {
      // The replay error remains visible in the checkpoint inspector.
    }
  };

  const openRewind = (checkpoint: SessionCheckpoint) => {
    rewindMutation.reset();
    setSelectedCheckpointID(checkpoint.id);
    setRewindInterrupt(Boolean(session.status?.activeTurnID));
    setRewindResume(true);
    setRewindOpen(true);
  };

  const rewindCheckpoint = async (event: FormEvent) => {
    event.preventDefault();
    if (!selectedCheckpoint) return;
    try {
      await rewindMutation.mutateAsync({
        name: routeName,
        checkpointID: selectedCheckpoint.id,
        interrupt: rewindInterrupt || undefined,
        resume: rewindResume || undefined,
      });
      setRewindOpen(false);
      setReplayOpen(false);
      setStreamRevision((revision) => revision + 1);
      await Promise.all([sessionQuery.refetch(), checkpointsQuery.refetch()]);
      toast(
        "success",
        rewindResume
          ? "Session rewound and resumed"
          : "Session rewound and left paused",
      );
    } catch {
      // The rewind error remains visible in the dialog.
    }
  };

  const openFork = (checkpoint: SessionCheckpoint) => {
    forkMutation.reset();
    setSelectedCheckpointID(checkpoint.id);
    setForkName(`${session.metadata.name}-fork`);
    setForkResume(false);
    setForkOpen(true);
  };

  const forkCheckpoint = async (event: FormEvent) => {
    event.preventDefault();
    if (!selectedCheckpoint || !forkName.trim()) return;
    try {
      const result = await forkMutation.mutateAsync({
        name: routeName,
        checkpointID: selectedCheckpoint.id,
        forkName: forkName.trim(),
        resume: forkResume || undefined,
      });
      setForkOpen(false);
      toast("success", `Forked Session ${result.session.metadata.name}`);
      navigate(`/sessions/${encodeURIComponent(result.session.metadata.name)}`);
    } catch {
      // The fork error remains visible in the dialog.
    }
  };

  return (
    <div className="page session-detail">
      <div className="page__header">
        <div className="page__header-back">
          <button className="btn-ghost" onClick={goBack} aria-label="Back to sessions">
            <ArrowLeft size={16} />
          </button>
          <div>
            <h1 className="page__title">{session.metadata.name}</h1>
            <p className="page__subtitle">
              {session.spec.system ?? "No agent system"} &middot;{" "}
              {session.metadata.namespace ?? "default"}
            </p>
          </div>
          <StatusBadge phase={phase} size="md" pulse={phaseLower === "running"} />
        </div>
        <div className="page__header-actions session-detail__actions">
          <span
            className={`session-stream-state session-stream-state--${streamState}`}
            aria-live="polite"
          >
            {streamState === "open" ? <Wifi size={14} /> : <WifiOff size={14} />}
            {streamState === "open"
              ? "Live"
              : streamState === "reconnecting"
                ? "Reconnecting"
                : streamState === "closed"
                  ? "Disconnected"
                  : "Connecting"}
          </span>
          <button
            className="btn-secondary"
            disabled={terminal || actionMutation.isPending}
            onClick={() => void runAction(paused ? "resume" : "pause")}
          >
            {paused ? <Play size={14} /> : <Pause size={14} />}
            {paused ? "Resume" : "Pause"}
          </button>
          <button
            className="btn-secondary text-red"
            disabled={terminal || actionMutation.isPending}
            onClick={() => void runAction("cancel")}
          >
            <CircleStop size={14} /> Cancel
          </button>
        </div>
      </div>

      {(lastError || actionMutation.isError) && (
        <div className="list-fetch-error" role="alert">
          <p>
            {actionMutation.error instanceof Error
              ? actionMutation.error.message
              : lastError ?? "Session action failed"}
          </p>
        </div>
      )}

      <div className="session-live-layout">
        <section className="session-chat" aria-label="Session live execution timeline">
          <AiDisclosure kind="ai-interaction" />
          <div className="session-chat__timeline" aria-live="polite">
            <SessionTimeline
              items={timeline}
              selectedCheckpointID={selectedCheckpointID}
              onSelectCheckpoint={selectCheckpoint}
              endRef={timelineEndRef}
            />
          </div>

          <form className="session-composer" onSubmit={sendTurn}>
            <textarea
              rows={3}
              value={content}
              onChange={(event) => setContent(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter" && !event.shiftKey) {
                  event.preventDefault();
                  event.currentTarget.form?.requestSubmit();
                }
              }}
              placeholder={
                terminal ? "This Session has ended" : "Send steering input…"
              }
              aria-label="Session steering message"
              disabled={terminal}
            />
            <div className="session-composer__footer">
              <label className="checkbox-inline">
                <input
                  type="checkbox"
                  checked={interrupt}
                  onChange={(event) => setInterrupt(event.target.checked)}
                  disabled={terminal}
                />
                Interrupt active turn
              </label>
              <button
                type="submit"
                className="btn-primary"
                disabled={!content.trim() || turnMutation.isPending || terminal}
              >
                <Send size={14} />
                {turnMutation.isPending ? "Sending…" : "Send"}
              </button>
            </div>
            {turnMutation.isError && (
              <p className="text-red text-xs" role="alert">
                {turnMutation.error instanceof Error
                  ? turnMutation.error.message
                  : "Failed to send turn"}
              </p>
            )}
          </form>
        </section>

        <aside className="session-checkpoint-panel" aria-label="Session checkpoints">
          <div className="session-checkpoint-panel__header">
            <div>
              <h2>Session checkpoints</h2>
              <p>
                {checkpoints.length} durable safe point
                {checkpoints.length === 1 ? "" : "s"}
              </p>
            </div>
            <ShieldCheck size={18} />
          </div>

          <div className="session-runtime-summary">
            <div>
              <span>Active turn</span>
              <strong className="mono">
                {session.status?.activeTurnID?.slice(0, 12) ?? "None"}
              </strong>
            </div>
            <div>
              <span>Restored from</span>
              <strong className="mono">
                {session.status?.restoredCheckpoint?.slice(0, 12) ?? "Original"}
              </strong>
            </div>
          </div>

          {checkpointsQuery.isError && (
            <p className="text-red text-xs" role="alert">
              {checkpointsQuery.error instanceof Error
                ? checkpointsQuery.error.message
                : "Failed to load Session checkpoints"}
            </p>
          )}

          {checkpoints.length > 0 && (
            <div className="session-checkpoint-list" aria-label="Checkpoint list">
              {checkpoints.map((checkpoint) => {
                const marker = timeline.find(
                  (item) =>
                    item.kind === "checkpoint" &&
                    item.checkpointID === checkpoint.id,
                );
                return (
                  <button
                    type="button"
                    key={checkpoint.id}
                    className={
                      checkpoint.id === selectedCheckpointID
                        ? "session-checkpoint-list__item session-checkpoint-list__item--selected"
                        : "session-checkpoint-list__item"
                    }
                    onClick={() => selectCheckpoint(checkpoint.id)}
                  >
                    <span>
                      {checkpoint.agent || checkpoint.safe_point.replace(".", " ")}
                    </span>
                    <span className="mono">#{checkpoint.event_sequence}</span>
                    <span>{marker?.abandoned ? "Abandoned" : "Active"}</span>
                  </button>
                );
              })}
            </div>
          )}

          {!checkpointsQuery.isLoading &&
            !checkpointsQuery.isError &&
            checkpoints.length === 0 && (
              <div className="session-checkpoint-panel__empty">
                Checkpoints appear after an agent completes a safe execution step.
              </div>
            )}

          {selectedCheckpoint && (
            <div className="session-checkpoint-inspector">
              <div className="session-checkpoint-inspector__title">
                <strong>{selectedCheckpoint.safe_point.replace(".", " ")}</strong>
                <span className="mono">{selectedCheckpoint.id}</span>
              </div>
              <dl>
                <div>
                  <dt>Agent</dt>
                  <dd>{selectedCheckpoint.agent || "Turn boundary"}</dd>
                </div>
                <div>
                  <dt>Event</dt>
                  <dd className="mono">#{selectedCheckpoint.event_sequence}</dd>
                </div>
                <div>
                  <dt>Created</dt>
                  <dd>{new Date(selectedCheckpoint.created_at).toLocaleString()}</dd>
                </div>
                <div>
                  <dt>State hash</dt>
                  <dd className="mono" title={selectedCheckpoint.state_hash}>
                    {selectedCheckpoint.state_hash.slice(0, 16)}…
                  </dd>
                </div>
              </dl>
              <div className="session-checkpoint-inspector__actions">
                <button
                  type="button"
                  className="btn-secondary"
                  disabled={replayMutation.isPending}
                  onClick={() => void replayCheckpoint(selectedCheckpoint)}
                >
                  <ShieldCheck size={14} /> Replay
                </button>
                <button
                  type="button"
                  className="btn-secondary"
                  onClick={() => openRewind(selectedCheckpoint)}
                >
                  <RotateCcw size={14} /> Rewind
                </button>
                <button
                  type="button"
                  className="btn-secondary"
                  onClick={() => openFork(selectedCheckpoint)}
                >
                  <GitFork size={14} /> Fork
                </button>
              </div>
            </div>
          )}

          {replayOpen && (
            <div className="session-replay-panel" aria-live="polite">
              <div className="session-replay-panel__header">
                <strong>Read-only replay</strong>
                <button
                  type="button"
                  className="btn-ghost"
                  aria-label="Close replay"
                  onClick={() => setReplayOpen(false)}
                >
                  <X size={14} />
                </button>
              </div>
              {replayMutation.isPending && <p>Verifying Session checkpoint…</p>}
              {replayMutation.isError && (
                <p className="text-red" role="alert">
                  {replayMutation.error instanceof Error
                    ? replayMutation.error.message
                    : "Replay verification failed"}
                </p>
              )}
              {replayMutation.data && (
                <>
                  <div
                    className={
                      replayMutation.data.verified
                        ? "session-replay-panel__verified"
                        : "session-replay-panel__failed"
                    }
                  >
                    <ShieldCheck size={14} />
                    {replayMutation.data.verified
                      ? "State and event history verified"
                      : "Verification failed"}
                  </div>
                  <dl>
                    <div>
                      <dt>Checkpoints checked</dt>
                      <dd>{replayMutation.data.checkpoint_count}</dd>
                    </div>
                    <div>
                      <dt>Events replayed</dt>
                      <dd>{replayMutation.data.events?.length ?? 0}</dd>
                    </div>
                  </dl>
                  <details>
                    <summary>
                      Replayed events ({replayMutation.data.events?.length ?? 0})
                    </summary>
                    <div className="session-replay-events">
                      {(replayMutation.data.events ?? []).map((event) => (
                        <div key={event.event_id ?? event.seq}>
                          <span className="mono">#{event.seq}</span>
                          <span>{event.type}</span>
                        </div>
                      ))}
                    </div>
                  </details>
                </>
              )}
            </div>
          )}
        </aside>
      </div>

      {rewindOpen && selectedCheckpoint && (
        <div
          className="confirm-overlay"
          role="presentation"
          onMouseDown={() => !rewindMutation.isPending && setRewindOpen(false)}
        >
          <form
            className="session-action-dialog"
            role="dialog"
            aria-modal="true"
            aria-labelledby="rewind-session-title"
            onSubmit={rewindCheckpoint}
            onMouseDown={(event) => event.stopPropagation()}
          >
            <div className="session-action-dialog__header">
              <div>
                <h2 id="rewind-session-title">Rewind Session</h2>
                <p>Restore Session checkpoint {selectedCheckpoint.id.slice(0, 12)}.</p>
              </div>
              <button
                type="button"
                className="btn-ghost"
                aria-label="Close"
                disabled={rewindMutation.isPending}
                onClick={() => setRewindOpen(false)}
              >
                <X size={16} />
              </button>
            </div>
            <div className="session-action-dialog__body">
              <p>
                Events after #{selectedCheckpoint.event_sequence} remain visible as
                an abandoned timeline for audit.
              </p>
              <label className="checkbox-inline">
                <input
                  type="checkbox"
                  checked={rewindResume}
                  onChange={(event) => setRewindResume(event.target.checked)}
                />
                Resume from this checkpoint
              </label>
              <label className="checkbox-inline">
                <input
                  type="checkbox"
                  checked={rewindInterrupt}
                  disabled={!session.status?.activeTurnID}
                  onChange={(event) => setRewindInterrupt(event.target.checked)}
                />
                Interrupt the active turn
              </label>
              {session.status?.activeTurnID && !rewindInterrupt && (
                <p className="text-red text-xs">
                  The active turn must be interrupted before rewinding.
                </p>
              )}
              {rewindMutation.isError && (
                <p className="text-red text-xs" role="alert">
                  {rewindMutation.error instanceof Error
                    ? rewindMutation.error.message
                    : "Failed to rewind Session"}
                </p>
              )}
            </div>
            <div className="session-action-dialog__footer">
              <button
                type="button"
                className="btn-secondary"
                disabled={rewindMutation.isPending}
                onClick={() => setRewindOpen(false)}
              >
                Cancel
              </button>
              <button
                type="submit"
                className="btn-primary"
                disabled={
                  rewindMutation.isPending ||
                  Boolean(session.status?.activeTurnID && !rewindInterrupt)
                }
              >
                <RotateCcw size={14} />
                {rewindMutation.isPending
                  ? "Rewinding…"
                  : rewindResume
                    ? "Rewind and resume"
                    : "Rewind paused"}
              </button>
            </div>
          </form>
        </div>
      )}

      {forkOpen && selectedCheckpoint && (
        <div
          className="confirm-overlay"
          role="presentation"
          onMouseDown={() => !forkMutation.isPending && setForkOpen(false)}
        >
          <form
            className="session-action-dialog"
            role="dialog"
            aria-modal="true"
            aria-labelledby="fork-session-title"
            onSubmit={forkCheckpoint}
            onMouseDown={(event) => event.stopPropagation()}
          >
            <div className="session-action-dialog__header">
              <div>
                <h2 id="fork-session-title">Fork Session</h2>
                <p>Create an independent timeline from this Session checkpoint.</p>
              </div>
              <button
                type="button"
                className="btn-ghost"
                aria-label="Close"
                disabled={forkMutation.isPending}
                onClick={() => setForkOpen(false)}
              >
                <X size={16} />
              </button>
            </div>
            <div className="session-action-dialog__body">
              <label>
                <span>New Session name</span>
                <input
                  autoFocus
                  required
                  value={forkName}
                  onChange={(event) => setForkName(event.target.value)}
                />
              </label>
              <label className="checkbox-inline">
                <input
                  type="checkbox"
                  checked={forkResume}
                  onChange={(event) => setForkResume(event.target.checked)}
                />
                Resume the fork immediately
              </label>
              {forkMutation.isError && (
                <p className="text-red text-xs" role="alert">
                  {forkMutation.error instanceof Error
                    ? forkMutation.error.message
                    : "Failed to fork Session"}
                </p>
              )}
            </div>
            <div className="session-action-dialog__footer">
              <button
                type="button"
                className="btn-secondary"
                disabled={forkMutation.isPending}
                onClick={() => setForkOpen(false)}
              >
                Cancel
              </button>
              <button
                type="submit"
                className="btn-primary"
                disabled={!forkName.trim() || forkMutation.isPending}
              >
                <GitFork size={14} />
                {forkMutation.isPending ? "Forking…" : "Fork Session"}
              </button>
            </div>
          </form>
        </div>
      )}
    </div>
  );
}
