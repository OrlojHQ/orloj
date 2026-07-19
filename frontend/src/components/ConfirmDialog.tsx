import { useEffect, useState, useCallback, useRef } from "react";
import { AlertTriangle } from "lucide-react";
import clsx from "clsx";

export interface ConfirmOptions {
  title: string;
  message: string;
  confirmLabel?: string;
  cancelLabel?: string;
  danger?: boolean;
}

interface PendingConfirm extends ConfirmOptions {
  resolve: (ok: boolean) => void;
}

const listeners: Set<(c: PendingConfirm) => void> = new Set();

/** Imperative, promise-based replacement for window.confirm. Requires <ConfirmDialogHost /> to be mounted. */
export function confirmDialog(opts: ConfirmOptions): Promise<boolean> {
  return new Promise((resolve) => {
    if (listeners.size === 0) {
      // Host not mounted (e.g. tests) — fall back to native confirm.
      resolve(window.confirm(`${opts.title}\n\n${opts.message}`));
      return;
    }
    listeners.forEach((fn) => fn({ ...opts, resolve }));
  });
}

export function ConfirmDialogHost() {
  const [pending, setPending] = useState<PendingConfirm | null>(null);
  const confirmRef = useRef<HTMLButtonElement>(null);

  const show = useCallback((c: PendingConfirm) => {
    setPending((prev) => {
      // Only one dialog at a time; auto-cancel any queued one.
      prev?.resolve(false);
      return c;
    });
  }, []);

  useEffect(() => {
    listeners.add(show);
    return () => { listeners.delete(show); };
  }, [show]);

  useEffect(() => {
    if (pending) confirmRef.current?.focus();
  }, [pending]);

  const close = useCallback((ok: boolean) => {
    setPending((prev) => {
      prev?.resolve(ok);
      return null;
    });
  }, []);

  useEffect(() => {
    if (!pending) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        close(false);
      }
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [pending, close]);

  if (!pending) return null;

  return (
    <div className="confirm-overlay" onClick={() => close(false)}>
      <div
        className="confirm-dialog"
        role="alertdialog"
        aria-modal="true"
        aria-labelledby="confirm-title"
        aria-describedby="confirm-message"
        onClick={(e) => e.stopPropagation()}
      >
        <div className={clsx("confirm-dialog__icon", pending.danger && "confirm-dialog__icon--danger")}>
          <AlertTriangle size={18} />
        </div>
        <div className="confirm-dialog__body">
          <h3 id="confirm-title" className="confirm-dialog__title">{pending.title}</h3>
          <p id="confirm-message" className="confirm-dialog__message">{pending.message}</p>
        </div>
        <div className="confirm-dialog__actions">
          <button className="btn-secondary" onClick={() => close(false)}>
            {pending.cancelLabel ?? "Cancel"}
          </button>
          <button
            ref={confirmRef}
            className={clsx("btn-primary", pending.danger && "confirm-dialog__confirm--danger")}
            onClick={() => close(true)}
          >
            {pending.confirmLabel ?? "Confirm"}
          </button>
        </div>
      </div>
    </div>
  );
}
