import { useMemo } from "react";
import { Activity, Wifi, Cpu } from "lucide-react";
import type { Task, Worker } from "../api/types";

interface SystemHealthHorizonProps {
  tasks: Task[];
  systemName?: string;
  apiReachable: boolean;
  workers: Worker[];
}

function generateSparklinePoints(tasks: Task[], width: number, height: number): string {
  if (tasks.length === 0) return "";

  const sorted = [...tasks]
    .filter((t) => t.status?.completedAt || t.status?.startedAt)
    .sort((a, b) => {
      const aTime = a.status?.completedAt ?? a.status?.startedAt ?? "";
      const bTime = b.status?.completedAt ?? b.status?.startedAt ?? "";
      return aTime.localeCompare(bTime);
    });

  if (sorted.length < 2) {
    const y = sorted.length === 1 && sorted[0].status?.phase?.toLowerCase() === "succeeded"
      ? height * 0.1
      : height * 0.5;
    return `M 0 ${y} L ${width} ${y}`;
  }

  const bucketCount = Math.min(sorted.length, 20);
  const bucketSize = Math.ceil(sorted.length / bucketCount);
  const rates: number[] = [];

  for (let i = 0; i < bucketCount; i++) {
    const bucket = sorted.slice(i * bucketSize, (i + 1) * bucketSize);
    const succeeded = bucket.filter((t) => t.status?.phase?.toLowerCase() === "succeeded").length;
    rates.push(bucket.length > 0 ? succeeded / bucket.length : 1);
  }

  const xStep = width / (rates.length - 1);
  const points = rates.map((rate, idx) => {
    const x = idx * xStep;
    const y = height - rate * height * 0.85 - height * 0.08;
    return `${x} ${y}`;
  });

  return `M ${points[0]} ${points.slice(1).map((p) => `L ${p}`).join(" ")}`;
}

export function SystemHealthHorizon({ tasks, systemName, apiReachable, workers }: SystemHealthHorizonProps) {
  const systemTasks = useMemo(
    () => systemName ? tasks.filter((t) => t.spec.system === systemName) : tasks,
    [tasks, systemName],
  );

  const totalTasks = systemTasks.length;
  const succeeded = systemTasks.filter(
    (t) => t.status?.phase?.toLowerCase() === "succeeded",
  ).length;
  const successRate = totalTasks > 0 ? Math.round((succeeded / totalTasks) * 100) : 100;

  const workersOnline = workers.filter((w) => {
    const p = (w.status?.phase ?? "").toLowerCase();
    return p === "healthy" || p === "ready";
  }).length;

  const sparkWidth = 200;
  const sparkHeight = 48;
  const sparkPath = useMemo(
    () => generateSparklinePoints(systemTasks, sparkWidth, sparkHeight),
    [systemTasks],
  );

  const apiReachCount = apiReachable ? 1 : 0;

  return (
    <div className="health-horizon">
      <div className="health-horizon__primary">
        <div className="health-horizon__metric-block">
          <span className="health-horizon__label">TASK SUCCESS RATE</span>
          <span className="health-horizon__value">{successRate}%</span>
          <span className={`health-horizon__status health-horizon__status--${successRate >= 95 ? "ok" : "warn"}`}>
            <span className="health-horizon__status-dot" />
            {successRate >= 95 ? "All systems operational" : "Degraded performance"}
          </span>
        </div>
        <div className="health-horizon__sparkline">
          <svg
            width={sparkWidth}
            height={sparkHeight}
            viewBox={`0 0 ${sparkWidth} ${sparkHeight}`}
            fill="none"
            xmlns="http://www.w3.org/2000/svg"
          >
            <defs>
              <linearGradient id="spark-fill" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor="rgba(212, 160, 74, 0.3)" />
                <stop offset="100%" stopColor="rgba(212, 160, 74, 0)" />
              </linearGradient>
            </defs>
            {sparkPath && (
              <>
                <path
                  d={`${sparkPath} L ${sparkWidth} ${sparkHeight} L 0 ${sparkHeight} Z`}
                  fill="url(#spark-fill)"
                />
                <path
                  d={sparkPath}
                  stroke="#D4A04A"
                  strokeWidth="2"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  style={{ filter: "drop-shadow(0 0 4px rgba(212, 160, 74, 0.5))" }}
                />
              </>
            )}
          </svg>
          <span className="health-horizon__time-range">Last 24 hours</span>
        </div>
      </div>

      <div className="health-horizon__secondary">
        <div className="health-horizon__stat">
          <Wifi size={16} className="health-horizon__stat-icon" />
          <div className="health-horizon__stat-content">
            <span className="health-horizon__stat-label">API REACHABILITY</span>
            <span className="health-horizon__stat-value">{apiReachCount}/1</span>
            <span className="health-horizon__stat-sub">API reachability</span>
          </div>
        </div>
        <div className="health-horizon__stat">
          <Cpu size={16} className="health-horizon__stat-icon" />
          <div className="health-horizon__stat-content">
            <span className="health-horizon__stat-label">WORKERS ONLINE</span>
            <span className="health-horizon__stat-value">{workersOnline}/{workers.length}</span>
            <span className="health-horizon__stat-sub">Workers online</span>
          </div>
        </div>
      </div>

      <div className="health-horizon__indicator">
        <Activity size={14} />
      </div>
    </div>
  );
}
