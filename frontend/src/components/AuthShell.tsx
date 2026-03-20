import type { ReactNode } from "react";
import { Activity, LockKeyhole, ShieldCheck } from "lucide-react";

interface AuthShellProps {
  title: string;
  subtitle: string;
  mode: "login" | "setup";
  children: ReactNode;
}

const contentByMode = {
  login: {
    heading: "Secure access to your orchestration control plane",
    description:
      "Authenticate with your local admin account to review systems, inspect task execution, and operate Orloj safely.",
  },
  setup: {
    heading: "Finish admin setup before enabling your workspace",
    description:
      "Create the first local admin account so only authorized operators can manage agents, tools, and production workflows.",
  },
} as const;

export function AuthShell({ title, subtitle, mode, children }: AuthShellProps) {
  const content = contentByMode[mode];

  return (
    <div className="auth-screen">
      <div className="auth-shell">
        <aside className="auth-shell__aside">
          <div className="auth-shell__brand">
            <span className="auth-shell__eyebrow">Orloj Control Plane</span>
            <h2 className="auth-shell__heading">{content.heading}</h2>
            <p className="auth-shell__description">{content.description}</p>
          </div>

          <ul className="auth-shell__pillars" aria-label="Security and reliability highlights">
            <li className="auth-shell__pillar">
              <span className="auth-shell__pillar-icon" aria-hidden="true">
                <ShieldCheck size={16} />
              </span>
              <div>
                <p className="auth-shell__pillar-title">Governed access</p>
                <p className="auth-shell__pillar-copy">Role-based controls with explicit permissions.</p>
              </div>
            </li>
            <li className="auth-shell__pillar">
              <span className="auth-shell__pillar-icon" aria-hidden="true">
                <LockKeyhole size={16} />
              </span>
              <div>
                <p className="auth-shell__pillar-title">Session security</p>
                <p className="auth-shell__pillar-copy">HTTP-only session cookies with configurable TTL.</p>
              </div>
            </li>
            <li className="auth-shell__pillar">
              <span className="auth-shell__pillar-icon" aria-hidden="true">
                <Activity size={16} />
              </span>
              <div>
                <p className="auth-shell__pillar-title">Operational visibility</p>
                <p className="auth-shell__pillar-copy">Inspect tasks, logs, and topology from one console.</p>
              </div>
            </li>
          </ul>
        </aside>

        <section className="auth-shell__panel" aria-label="Authentication form">
          <div className="auth-card">
            <header className="auth-card__header">
              <h1 className="auth-card__title">{title}</h1>
              <p className="auth-card__subtitle">{subtitle}</p>
            </header>
            {children}
          </div>
        </section>
      </div>
    </div>
  );
}
