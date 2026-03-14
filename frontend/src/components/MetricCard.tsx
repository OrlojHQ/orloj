import clsx from "clsx";
import type { ReactNode } from "react";

interface MetricCardProps {
  label: string;
  value: number | string;
  icon?: ReactNode;
  variant?: "default" | "green" | "blue" | "yellow" | "red" | "orange";
  subtitle?: string;
}

export function MetricCard({ label, value, icon, variant = "default", subtitle }: MetricCardProps) {
  return (
    <div className={clsx("metric-card", `metric-card--${variant}`)}>
      <div className="metric-card__header">
        {icon && <span className="metric-card__icon">{icon}</span>}
        <span className="metric-card__label">{label}</span>
      </div>
      <div className="metric-card__value">{value}</div>
      {subtitle && <div className="metric-card__subtitle">{subtitle}</div>}
    </div>
  );
}
