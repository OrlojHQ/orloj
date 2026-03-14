import { NavLink } from "react-router-dom";
import { useAppStore } from "../store";
import clsx from "clsx";
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
  KeyRound,
  Lock,
  Webhook,
  PanelLeftClose,
  PanelLeftOpen,
} from "lucide-react";
import type { ReactNode } from "react";

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
  { to: "/memories", icon: <Brain size={18} />, label: "Memories", group: "Infra" },
  { to: "/secrets", icon: <Lock size={18} />, label: "Secrets", group: "Infra" },
  { to: "/policies", icon: <Shield size={18} />, label: "Policies", group: "Governance" },
  { to: "/roles", icon: <KeyRound size={18} />, label: "Roles", group: "Governance" },
  { to: "/permissions", icon: <KeyRound size={18} />, label: "Permissions", group: "Governance" },
];

export function Sidebar() {
  const collapsed = useAppStore((s) => s.sidebarCollapsed);
  const toggle = useAppStore((s) => s.toggleSidebar);

  let lastGroup: string | undefined;

  return (
    <aside className={clsx("sidebar", collapsed && "sidebar--collapsed")} role="navigation" aria-label="Main navigation">
      <div className="sidebar__logo">
        <div className="sidebar__logo-icon">
          <Network size={20} />
        </div>
        {!collapsed && <span className="sidebar__logo-text">Orloj</span>}
      </div>

      <nav className="sidebar__nav">
        {NAV_ITEMS.map((item) => {
          const showGroup = !collapsed && item.group && item.group !== lastGroup;
          lastGroup = item.group;
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
              </NavLink>
            </div>
          );
        })}
      </nav>

      <button className="sidebar__toggle" onClick={toggle} aria-label="Toggle sidebar">
        {collapsed ? <PanelLeftOpen size={16} /> : <PanelLeftClose size={16} />}
      </button>
    </aside>
  );
}
