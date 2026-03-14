import { useState, useEffect, useRef, useMemo } from "react";
import { useNavigate } from "react-router-dom";
import { Search, X, Network, Bot, ListTodo, Cpu, Database, Wrench, CalendarClock, Webhook } from "lucide-react";
import { useAgentSystems, useAgents, useTasks, useTaskSchedules, useTaskWebhooks, useWorkers, useModelEndpoints, useTools } from "../api/hooks";

interface SearchResult {
  kind: string;
  name: string;
  path: string;
  icon: React.ReactNode;
}

export function SearchDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [query, setQuery] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);
  const navigate = useNavigate();

  const systems = useAgentSystems();
  const agents = useAgents();
  const tasks = useTasks();
  const workers = useWorkers();
  const models = useModelEndpoints();
  const tools = useTools();
  const taskSchedules = useTaskSchedules();
  const taskWebhooks = useTaskWebhooks();

  useEffect(() => {
    if (open) {
      setQuery("");
      setTimeout(() => inputRef.current?.focus(), 50);
    }
  }, [open]);

  const allResults: SearchResult[] = useMemo(() => {
    const results: SearchResult[] = [];
    for (const s of systems.data ?? []) {
      results.push({ kind: "System", name: s.metadata.name, path: `/systems/${s.metadata.name}`, icon: <Network size={14} /> });
    }
    for (const a of agents.data ?? []) {
      results.push({ kind: "Agent", name: a.metadata.name, path: `/agents/${a.metadata.name}`, icon: <Bot size={14} /> });
    }
    for (const t of tasks.data ?? []) {
      results.push({ kind: "Task", name: t.metadata.name, path: `/tasks/${t.metadata.name}`, icon: <ListTodo size={14} /> });
    }
    for (const s of taskSchedules.data ?? []) {
      results.push({ kind: "TaskSchedule", name: s.metadata.name, path: `/task-schedules/${s.metadata.name}`, icon: <CalendarClock size={14} /> });
    }
    for (const w of taskWebhooks.data ?? []) {
      results.push({ kind: "TaskWebhook", name: w.metadata.name, path: `/task-webhooks/${w.metadata.name}`, icon: <Webhook size={14} /> });
    }
    for (const w of workers.data ?? []) {
      results.push({ kind: "Worker", name: w.metadata.name, path: `/workers`, icon: <Cpu size={14} /> });
    }
    for (const m of models.data ?? []) {
      results.push({ kind: "Model", name: m.metadata.name, path: `/models`, icon: <Database size={14} /> });
    }
    for (const t of tools.data ?? []) {
      results.push({ kind: "Tool", name: t.metadata.name, path: `/tools`, icon: <Wrench size={14} /> });
    }
    return results;
  }, [systems.data, agents.data, tasks.data, taskSchedules.data, taskWebhooks.data, workers.data, models.data, tools.data]);

  const filtered = useMemo(() => {
    if (!query.trim()) return allResults.slice(0, 20);
    const q = query.toLowerCase();
    return allResults.filter((r) => r.name.toLowerCase().includes(q) || r.kind.toLowerCase().includes(q)).slice(0, 20);
  }, [allResults, query]);

  const handleSelect = (result: SearchResult) => {
    navigate(result.path);
    onClose();
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Escape") onClose();
  };

  if (!open) return null;

  return (
    <div className="search-overlay" onClick={onClose}>
      <div className="search-dialog" onClick={(e) => e.stopPropagation()} onKeyDown={handleKeyDown}>
        <div className="search-dialog__input-row">
          <Search size={16} className="search-dialog__icon" />
          <input
            ref={inputRef}
            className="search-dialog__input"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search resources..."
            data-search
          />
          <kbd className="search-dialog__kbd">ESC</kbd>
          <button className="search-dialog__close" onClick={onClose} aria-label="Close">
            <X size={14} />
          </button>
        </div>
        <div className="search-dialog__results">
          {filtered.length === 0 && (
            <div className="search-dialog__empty">No results found</div>
          )}
          {filtered.map((r) => (
            <button
              key={`${r.kind}-${r.name}`}
              className="search-dialog__result"
              onClick={() => handleSelect(r)}
            >
              <span className="search-dialog__result-icon">{r.icon}</span>
              <span className="search-dialog__result-name">{r.name}</span>
              <span className="search-dialog__result-kind">{r.kind}</span>
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
