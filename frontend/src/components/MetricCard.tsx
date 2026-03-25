import clsx from "clsx";
import { HelpCircle } from "lucide-react";
import { useId, useState, useCallback, type ReactNode } from "react";

interface MetricCardProps {
  label: string;
  value: number | string;
  icon?: ReactNode;
  variant?: "default" | "green" | "blue" | "yellow" | "red" | "orange";
  subtitle?: string;
  /** Top-right help icon with an instant CSS tooltip on hover/focus. */
  hint?: string;
}

export function MetricCard({ label, value, icon, variant = "default", subtitle, hint }: MetricCardProps) {
  const hintTooltipId = useId();
  const [hintVisible, setHintVisible] = useState(false);

  const toggleHint = useCallback(() => {
    setHintVisible((v) => !v);
  }, []);

  return (
    <div className={clsx("metric-card", `metric-card--${variant}`, hint && "metric-card--has-hint")}>
      {hint && (
        <div className="metric-card__hint-wrap">
          <button
            type="button"
            className="metric-card__hint"
            aria-describedby={hintTooltipId}
            aria-label="Metric explanation"
            onClick={toggleHint}
          >
            <HelpCircle size={14} strokeWidth={2} aria-hidden />
          </button>
          <span
            id={hintTooltipId}
            role="tooltip"
            className={clsx("metric-card__hint-tooltip", hintVisible && "metric-card__hint-tooltip--visible")}
          >
            {hint}
          </span>
        </div>
      )}
      <div className="metric-card__header">
        {icon && <span className="metric-card__icon">{icon}</span>}
        <span className="metric-card__label">{label}</span>
      </div>
      <div className="metric-card__value">{value}</div>
      {subtitle && <div className="metric-card__subtitle">{subtitle}</div>}
    </div>
  );
}
