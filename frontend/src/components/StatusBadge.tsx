import clsx from "clsx";

type BadgeVariant =
  | "healthy"
  | "running"
  | "pending"
  | "failed"
  | "deadletter"
  | "succeeded"
  | "ready"
  | "unknown";

const PHASE_MAP: Record<string, BadgeVariant> = {
  running: "running",
  healthy: "healthy",
  ready: "ready",
  succeeded: "succeeded",
  pending: "pending",
  retrypending: "pending",
  failed: "failed",
  error: "failed",
  deadletter: "deadletter",
  degraded: "failed",
};

function resolveVariant(phase?: string): BadgeVariant {
  if (!phase) return "unknown";
  return PHASE_MAP[phase.toLowerCase()] ?? "unknown";
}

const VARIANT_STYLE: Record<BadgeVariant, string> = {
  healthy: "badge--green",
  succeeded: "badge--green",
  ready: "badge--green",
  running: "badge--blue",
  pending: "badge--yellow",
  failed: "badge--red",
  deadletter: "badge--orange",
  unknown: "badge--gray",
};

interface StatusBadgeProps {
  phase?: string;
  size?: "sm" | "md";
  pulse?: boolean;
}

export function StatusBadge({ phase, size = "sm", pulse }: StatusBadgeProps) {
  const variant = resolveVariant(phase);
  return (
    <span className={clsx("badge", VARIANT_STYLE[variant], size === "md" && "badge--md", pulse && "badge--pulse")}>
      <span className="badge__dot" />
      {phase || "Unknown"}
    </span>
  );
}
