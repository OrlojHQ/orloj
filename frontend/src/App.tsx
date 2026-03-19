import { BrowserRouter, Routes, Route, Navigate } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState, useEffect, useCallback } from "react";
import { useAppStore } from "./store";
import { useWatchInvalidation } from "./api/watch";
import { getAuthConfig, getAuthMe, type AuthConfigResponse } from "./api/client";
import { Sidebar } from "./components/Sidebar";
import { TopBar } from "./components/TopBar";
import { ToastContainer } from "./components/Toast";
import { SearchDialog } from "./components/SearchDialog";
import { Dashboard } from "./pages/Dashboard";
import { AgentSystems } from "./pages/AgentSystems";
import { AgentSystemDetail } from "./pages/AgentSystemDetail";
import { Agents } from "./pages/Agents";
import { AgentDetail } from "./pages/AgentDetail";
import { Tasks } from "./pages/Tasks";
import { TaskDetail } from "./pages/TaskDetail";
import { TaskSchedules } from "./pages/TaskSchedules";
import { TaskScheduleDetail } from "./pages/TaskScheduleDetail";
import { TaskWebhooks } from "./pages/TaskWebhooks";
import { TaskWebhookDetail } from "./pages/TaskWebhookDetail";
import { Workers } from "./pages/Workers";
import { WorkerDetail } from "./pages/WorkerDetail";
import { ModelEndpoints } from "./pages/ModelEndpoints";
import { ModelEndpointDetail } from "./pages/ModelEndpointDetail";
import { Tools } from "./pages/Tools";
import { ToolDetail } from "./pages/ToolDetail";
import { Memories } from "./pages/Memories";
import { MemoryDetail } from "./pages/MemoryDetail";
import { Secrets } from "./pages/Secrets";
import { SecretDetail } from "./pages/SecretDetail";
import { Policies } from "./pages/Policies";
import { AgentPolicyDetail } from "./pages/AgentPolicyDetail";
import { Roles } from "./pages/Roles";
import { AgentRoleDetail } from "./pages/AgentRoleDetail";
import { Permissions } from "./pages/Permissions";
import { ToolPermissionDetail } from "./pages/ToolPermissionDetail";
import { ToolApprovals } from "./pages/ToolApprovals";
import { ToolApprovalDetail } from "./pages/ToolApprovalDetail";
import { NotFound } from "./pages/NotFound";
import { Login } from "./pages/Login";
import { Setup } from "./pages/Setup";
import { ErrorBoundary } from "./components/ErrorBoundary";
import clsx from "clsx";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      staleTime: 5000,
    },
  },
});

function ThemeProvider({ children }: { children: React.ReactNode }) {
  const theme = useAppStore((s) => s.theme);
  useEffect(() => {
    document.documentElement.setAttribute("data-theme", theme);
  }, [theme]);
  return <>{children}</>;
}

function WatchProvider({ children }: { children: React.ReactNode }) {
  useWatchInvalidation();
  return <>{children}</>;
}

interface AuthBootstrapState {
  loading: boolean;
  config: AuthConfigResponse | null;
  authenticated: boolean;
}

function AppLayout() {
  const collapsed = useAppStore((s) => s.sidebarCollapsed);
  const [searchOpen, setSearchOpen] = useState(false);

  const handleKeyDown = useCallback((e: KeyboardEvent) => {
    if ((e.metaKey || e.ctrlKey) && e.key === "k") {
      e.preventDefault();
      setSearchOpen(true);
    }
  }, []);

  useEffect(() => {
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [handleKeyDown]);

  return (
    <div className={clsx("app-layout", collapsed && "app-layout--collapsed")}>
      <a href="#main-content" className="skip-link">Skip to main content</a>
      <Sidebar />
      <div className="app-layout__main">
        <TopBar />
        <main id="main-content" className="app-layout__content" role="main">
          <ErrorBoundary>
            <Routes>
              <Route path="/" element={<Dashboard />} />
              <Route path="/systems" element={<AgentSystems />} />
              <Route path="/systems/:name" element={<AgentSystemDetail />} />
              <Route path="/agents" element={<Agents />} />
              <Route path="/agents/:name" element={<AgentDetail />} />
              <Route path="/tasks" element={<Tasks />} />
              <Route path="/tasks/:name" element={<TaskDetail />} />
              <Route path="/task-schedules" element={<TaskSchedules />} />
              <Route path="/task-schedules/:name" element={<TaskScheduleDetail />} />
              <Route path="/task-webhooks" element={<TaskWebhooks />} />
              <Route path="/task-webhooks/:name" element={<TaskWebhookDetail />} />
              <Route path="/workers" element={<Workers />} />
              <Route path="/workers/:name" element={<WorkerDetail />} />
              <Route path="/models" element={<ModelEndpoints />} />
              <Route path="/models/:name" element={<ModelEndpointDetail />} />
              <Route path="/tools" element={<Tools />} />
              <Route path="/tools/:name" element={<ToolDetail />} />
              <Route path="/memories" element={<Memories />} />
              <Route path="/memories/:name" element={<MemoryDetail />} />
              <Route path="/secrets" element={<Secrets />} />
              <Route path="/secrets/:name" element={<SecretDetail />} />
              <Route path="/policies" element={<Policies />} />
              <Route path="/policies/:name" element={<AgentPolicyDetail />} />
              <Route path="/roles" element={<Roles />} />
              <Route path="/roles/:name" element={<AgentRoleDetail />} />
              <Route path="/permissions" element={<Permissions />} />
              <Route path="/permissions/:name" element={<ToolPermissionDetail />} />
              <Route path="/approvals" element={<ToolApprovals />} />
              <Route path="/approvals/:name" element={<ToolApprovalDetail />} />
              <Route path="*" element={<NotFound />} />
            </Routes>
          </ErrorBoundary>
        </main>
      </div>
      <ToastContainer />
      <SearchDialog open={searchOpen} onClose={() => setSearchOpen(false)} />
    </div>
  );
}

export function App() {
  const [refreshAuthNonce, setRefreshAuthNonce] = useState(0);
  const [auth, setAuth] = useState<AuthBootstrapState>({
    loading: true,
    config: null,
    authenticated: false,
  });

  const refreshAuth = useCallback(() => {
    setRefreshAuthNonce((n) => n + 1);
  }, []);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const config = await getAuthConfig();
        let authenticated = true;
        if (config.mode === "local" && !config.setup_required) {
          const me = await getAuthMe();
          authenticated = me.authenticated === true;
        }
        if (!cancelled) {
          setAuth({ loading: false, config, authenticated });
        }
      } catch {
        if (!cancelled) {
          setAuth({
            loading: false,
            config: { mode: "off", setup_required: false, login_methods: [] },
            authenticated: true,
          });
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [refreshAuthNonce]);

  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <BrowserRouter basename={import.meta.env.DEV ? "" : "/ui"}>
          {auth.loading ? (
            <div className="page">
              <div className="page__header">
                <h1 className="page__title">Loading</h1>
              </div>
            </div>
          ) : auth.config?.mode === "local" && auth.config.setup_required ? (
            <Routes>
              <Route path="/setup" element={<Setup onSuccess={refreshAuth} />} />
              <Route path="*" element={<Navigate to="/setup" replace />} />
            </Routes>
          ) : auth.config?.mode === "local" && !auth.authenticated ? (
            <Routes>
              <Route path="/login" element={<Login onSuccess={refreshAuth} />} />
              <Route path="*" element={<Navigate to="/login" replace />} />
            </Routes>
          ) : (
            <WatchProvider>
              <AppLayout />
            </WatchProvider>
          )}
        </BrowserRouter>
      </ThemeProvider>
    </QueryClientProvider>
  );
}
