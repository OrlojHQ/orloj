import { useMemo, useEffect, useSyncExternalStore } from "react";
import { NavLink, useLocation } from "react-router-dom";
import { useAppStore } from "../store";
import { useToolApprovals } from "../api/hooks";
import clsx from "clsx";

const mqMobile = typeof window !== "undefined" ? window.matchMedia("(max-width: 768px)") : null;
function useIsMobile() {
  return useSyncExternalStore(
    (cb) => { mqMobile?.addEventListener("change", cb); return () => mqMobile?.removeEventListener("change", cb); },
    () => mqMobile?.matches ?? false,
  );
}
import {
  LayoutDashboard,
  Network,
  Bot,
  ListTodo,
  CalendarClock,
  Cpu,
  Wrench,
  Database,
  Brain,
  Shield,
  ShieldCheck,
  KeyRound,
  Lock,
  Webhook,
  Plug,
  Sparkles,
  PanelLeftClose,
  PanelLeftOpen,
  CircleUserRound,
} from "lucide-react";
import type { ReactNode } from "react";
import orlojMark from "/orloj-mark.png?url";

interface NavItem {
  to: string;
  icon: ReactNode;
  label: string;
  group?: string;
}

const NAV_ITEMS: NavItem[] = [
  { to: "/", icon: <LayoutDashboard size={18} />, label: "Dashboard" },
  { to: "/systems", icon: <Network size={18} />, label: "Agent Systems", group: "Core" },
  { to: "/agents", icon: <Bot size={18} />, label: "Agents", group: "Core" },
  { to: "/tasks", icon: <ListTodo size={18} />, label: "Tasks", group: "Core" },
  { to: "/task-schedules", icon: <CalendarClock size={18} />, label: "Task Schedules", group: "Core" },
  { to: "/task-webhooks", icon: <Webhook size={18} />, label: "Task Webhooks", group: "Core" },
  { to: "/workers", icon: <Cpu size={18} />, label: "Workers", group: "Infra" },
  { to: "/models", icon: <Database size={18} />, label: "Model Endpoints", group: "Infra" },
  { to: "/tools", icon: <Wrench size={18} />, label: "Tools", group: "Infra" },
  { to: "/mcp-servers", icon: <Plug size={18} />, label: "MCP Servers", group: "Infra" },
  { to: "/memories", icon: <Brain size={18} />, label: "Memories", group: "Infra" },
  { to: "/secrets", icon: <Lock size={18} />, label: "Secrets", group: "Infra" },
  { to: "/policies", icon: <Shield size={18} />, label: "Policies", group: "Governance" },
  { to: "/roles", icon: <KeyRound size={18} />, label: "Roles", group: "Governance" },
  { to: "/permissions", icon: <KeyRound size={18} />, label: "Permissions", group: "Governance" },
  { to: "/approvals", icon: <ShieldCheck size={18} />, label: "Approvals", group: "Governance" },
  { to: "/capabilities", icon: <Sparkles size={18} />, label: "Capabilities", group: "System" },
];

interface SidebarProps {
  nativeAuthEnabled?: boolean;
  username?: string;
}

export function Sidebar({ nativeAuthEnabled = false, username }: SidebarProps) {
  const storeCollapsed = useAppStore((s) => s.sidebarCollapsed);
  const toggle = useAppStore((s) => s.toggleSidebar);
  const sidebarOpen = useAppStore((s) => s.sidebarOpen);
  const setSidebarOpen = useAppStore((s) => s.setSidebarOpen);
  const approvals = useToolApprovals();
  const location = useLocation();
  const isMobile = useIsMobile();
  const collapsed = isMobile ? false : storeCollapsed;

  useEffect(() => {
    setSidebarOpen(false);
  }, [location.pathname, setSidebarOpen]);

  useEffect(() => {
    const mq = window.matchMedia("(min-width: 769px)");
    const handler = () => { if (mq.matches) setSidebarOpen(false); };
    mq.addEventListener("change", handler);
    return () => mq.removeEventListener("change", handler);
  }, [setSidebarOpen]);

  const pendingCount = useMemo(() => {
    return (approvals.data ?? []).filter((a) => (a.status?.phase ?? "Pending").toLowerCase() === "pending").length;
  }, [approvals.data]);

  let lastGroup: string | undefined;

  return (
    <>
      {sidebarOpen && (
        <div className="sidebar-backdrop" onClick={() => setSidebarOpen(false)} />
      )}
      <aside className={clsx("sidebar", collapsed && "sidebar--collapsed", sidebarOpen && "sidebar--mobile-open")} role="navigation" aria-label="Main navigation">
        <div className="sidebar__logo">
          <img src={orlojMark} alt="Orloj" className="sidebar__logo-mark" />
          {!collapsed && <span className="sidebar__logo-text">Orloj</span>}
        </div>

        <nav className="sidebar__nav">
          {NAV_ITEMS.map((item) => {
            const showGroup = !collapsed && item.group && item.group !== lastGroup;
            lastGroup = item.group;
            const badge = item.to === "/approvals" && pendingCount > 0 ? pendingCount : 0;
            return (
              <div key={item.to}>
                {showGroup && <div className="sidebar__group-label">{item.group}</div>}
                <NavLink
                  to={item.to}
                  end={item.to === "/"}
                  className={({ isActive }) =>
                    clsx("sidebar__link", isActive && "sidebar__link--active")
                  }
                  title={collapsed ? item.label : undefined}
                >
                  <span className="sidebar__link-icon">{item.icon}</span>
                  {!collapsed && <span className="sidebar__link-label">{item.label}</span>}
                  {badge > 0 && <span className="sidebar__badge">{badge}</span>}
                </NavLink>
              </div>
            );
          })}
        </nav>

        {nativeAuthEnabled && (
          <div className="sidebar__account">
            <NavLink
              to="/account"
              className={({ isActive }) => clsx("sidebar__account-link", isActive && "sidebar__account-link--active")}
              title={collapsed ? "Account Settings" : undefined}
            >
              <span className="sidebar__account-avatar">
                <CircleUserRound size={16} />
              </span>
              {!collapsed && (
                <span className="sidebar__account-meta">
                  <span className="sidebar__account-label">Signed in as</span>
                  <span className="sidebar__account-name mono">{username?.trim() || "local-admin"}</span>
                </span>
              )}
            </NavLink>
          </div>
        )}

        <button className="sidebar__toggle" onClick={toggle} aria-label="Toggle sidebar">
          {collapsed ? <PanelLeftOpen size={16} /> : <PanelLeftClose size={16} />}
        </button>
      </aside>
    </>
  );
}
