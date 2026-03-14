import { BrowserRouter, Routes, Route } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState, useEffect, useCallback } from "react";
import { useAppStore } from "./store";
import { useWatchInvalidation } from "./api/watch";
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
import { ModelEndpoints } from "./pages/ModelEndpoints";
import { Tools } from "./pages/Tools";
import { Memories } from "./pages/Memories";
import { Secrets } from "./pages/Secrets";
import { Policies } from "./pages/Policies";
import { Roles } from "./pages/Roles";
import { Permissions } from "./pages/Permissions";
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
            <Route path="/models" element={<ModelEndpoints />} />
            <Route path="/tools" element={<Tools />} />
            <Route path="/memories" element={<Memories />} />
            <Route path="/secrets" element={<Secrets />} />
            <Route path="/policies" element={<Policies />} />
            <Route path="/roles" element={<Roles />} />
            <Route path="/permissions" element={<Permissions />} />
          </Routes>
        </main>
      </div>
      <ToastContainer />
      <SearchDialog open={searchOpen} onClose={() => setSearchOpen(false)} />
    </div>
  );
}

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <BrowserRouter basename={import.meta.env.DEV ? "" : "/ui"}>
          <WatchProvider>
            <AppLayout />
          </WatchProvider>
        </BrowserRouter>
      </ThemeProvider>
    </QueryClientProvider>
  );
}
