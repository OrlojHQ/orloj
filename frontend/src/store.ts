import { create } from "zustand";
import { persist } from "zustand/middleware";

export type Theme = "dark" | "light";

interface AppState {
  apiBase: string;
  namespace: string;
  token: string;
  theme: Theme;
  connected: boolean;
  sidebarCollapsed: boolean;
  sidebarOpen: boolean;

  setApiBase: (v: string) => void;
  setNamespace: (v: string) => void;
  setToken: (v: string) => void;
  setTheme: (v: Theme) => void;
  toggleTheme: () => void;
  setConnected: (v: boolean) => void;
  setSidebarCollapsed: (v: boolean) => void;
  toggleSidebar: () => void;
  setSidebarOpen: (v: boolean) => void;
}

function defaultApiBase(): string {
  if (typeof window === "undefined") return "http://127.0.0.1:8080";
  return window.location.origin;
}

export const useAppStore = create<AppState>()(
  persist(
    (set) => ({
      apiBase: defaultApiBase(),
      namespace: "default",
      token: "",
      theme: "dark",
      connected: false,
      sidebarCollapsed: false,
      sidebarOpen: false,

      setApiBase: (v) => set({ apiBase: v }),
      setNamespace: (v) => set({ namespace: v }),
      setToken: (v) => set({ token: v }),
      setTheme: (v) => set({ theme: v }),
      toggleTheme: () => set((s) => ({ theme: s.theme === "dark" ? "light" : "dark" })),
      setConnected: (v) => set({ connected: v }),
      setSidebarCollapsed: (v) => set({ sidebarCollapsed: v }),
      toggleSidebar: () => set((s) => ({ sidebarCollapsed: !s.sidebarCollapsed })),
      setSidebarOpen: (v) => set({ sidebarOpen: v }),
    }),
    {
      name: "orloj-ui",
      partialize: (s) => ({
        apiBase: s.apiBase,
        namespace: s.namespace,
        theme: s.theme,
        sidebarCollapsed: s.sidebarCollapsed,
      }),
    },
  ),
);
