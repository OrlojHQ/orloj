import { useAppStore } from "../store";
import { useHealthCheck } from "../api/hooks";
import { logoutLocalAuth } from "../api/client";
import { Sun, Moon, Wifi, WifiOff, Settings } from "lucide-react";
import { useState, useEffect, useCallback } from "react";
import { NamespaceSelector } from "./NamespaceSelector";

export function TopBar() {
  const namespace = useAppStore((s) => s.namespace);
  const setNamespace = useAppStore((s) => s.setNamespace);
  const theme = useAppStore((s) => s.theme);
  const toggleTheme = useAppStore((s) => s.toggleTheme);
  const setConnected = useAppStore((s) => s.setConnected);
  const connected = useAppStore((s) => s.connected);
  const apiBase = useAppStore((s) => s.apiBase);
  const setApiBase = useAppStore((s) => s.setApiBase);

  const [showSettings, setShowSettings] = useState(false);
  const health = useHealthCheck();

  useEffect(() => {
    setConnected(health.data === true);
  }, [health.data, setConnected]);

  const handleKeyDown = useCallback((e: KeyboardEvent) => {
    if ((e.metaKey || e.ctrlKey) && e.key === "k") {
      e.preventDefault();
      const el = document.querySelector<HTMLInputElement>("[data-search]");
      el?.focus();
    }
  }, []);

  useEffect(() => {
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [handleKeyDown]);

  return (
    <header className="topbar">
      <div className="topbar__left">
        <div className="topbar__breadcrumb">
          <span className="topbar__breadcrumb-muted">namespace:</span>
          <NamespaceSelector value={namespace} onChange={setNamespace} />
        </div>
      </div>

      <div className="topbar__right">
        <div className="topbar__status" title={connected ? "Connected" : "Disconnected"}>
          {connected ? (
            <Wifi size={14} className="topbar__status-icon topbar__status-icon--ok" />
          ) : (
            <WifiOff size={14} className="topbar__status-icon topbar__status-icon--err" />
          )}
          <span className="topbar__status-label">
            {connected ? "Connected" : "Disconnected"}
          </span>
        </div>

        <button
          className="topbar__icon-btn"
          onClick={() => setShowSettings(!showSettings)}
          aria-label="Settings"
        >
          <Settings size={16} />
        </button>

        <button className="topbar__icon-btn" onClick={toggleTheme} aria-label="Toggle theme">
          {theme === "dark" ? <Sun size={16} /> : <Moon size={16} />}
        </button>
      </div>

      {showSettings && (
        <div className="topbar__settings-panel">
          <label className="topbar__settings-label">
            API Base
            <input
              value={apiBase}
              onChange={(e) => setApiBase(e.target.value)}
              placeholder="http://127.0.0.1:8080"
            />
          </label>
          <label className="topbar__settings-label">
            Session
            <button
              type="button"
              className="btn-secondary"
              onClick={async () => {
                await logoutLocalAuth();
                window.location.href = "/ui/login";
              }}
            >
              Sign Out
            </button>
          </label>
        </div>
      )}
    </header>
  );
}
