import { useState, useEffect, useRef, useMemo, useCallback } from "react";
import { Search, Copy, ArrowDown, ChevronUp, ChevronDown } from "lucide-react";
import { toast } from "./Toast";

interface LogViewerProps {
  logs: string;
  loading?: boolean;
}

function getLogLevel(line: string): string | null {
  const upper = line.toUpperCase();
  if (upper.includes("ERROR") || upper.includes("ERR ") || upper.includes("[ERROR]")) return "error";
  if (upper.includes("WARN") || upper.includes("WRN ") || upper.includes("[WARN]")) return "warn";
  if (upper.includes("INFO") || upper.includes("INF ") || upper.includes("[INFO]")) return "info";
  if (upper.includes("DEBUG") || upper.includes("DBG ") || upper.includes("[DEBUG]")) return "debug";
  return null;
}

export function LogViewer({ logs, loading }: LogViewerProps) {
  const [searchQuery, setSearchQuery] = useState("");
  const [autoScroll, setAutoScroll] = useState(false);
  const [currentMatch, setCurrentMatch] = useState(0);
  const contentRef = useRef<HTMLPreElement>(null);

  const lines = useMemo(() => (logs ? logs.split("\n") : []), [logs]);

  const matchingLines = useMemo(() => {
    if (!searchQuery.trim()) return [];
    const q = searchQuery.toLowerCase();
    return lines
      .map((line, i) => ({ index: i, match: line.toLowerCase().includes(q) }))
      .filter((l) => l.match)
      .map((l) => l.index);
  }, [lines, searchQuery]);

  useEffect(() => {
    setCurrentMatch(0);
  }, [searchQuery]);

  useEffect(() => {
    if (autoScroll && contentRef.current) {
      contentRef.current.scrollTop = contentRef.current.scrollHeight;
    }
  }, [logs, autoScroll]);

  const scrollToMatch = useCallback((idx: number) => {
    const el = contentRef.current?.querySelector(`[data-line="${matchingLines[idx]}"]`);
    el?.scrollIntoView({ block: "center", behavior: "smooth" });
  }, [matchingLines]);

  const handlePrevMatch = () => {
    if (matchingLines.length === 0) return;
    const next = currentMatch > 0 ? currentMatch - 1 : matchingLines.length - 1;
    setCurrentMatch(next);
    scrollToMatch(next);
  };

  const handleNextMatch = () => {
    if (matchingLines.length === 0) return;
    const next = currentMatch < matchingLines.length - 1 ? currentMatch + 1 : 0;
    setCurrentMatch(next);
    scrollToMatch(next);
  };

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(logs);
      toast("success", "Logs copied to clipboard");
    } catch {
      toast("error", "Failed to copy logs");
    }
  };

  if (loading) {
    return <div className="log-viewer log-viewer--loading">Loading logs...</div>;
  }

  if (!logs.trim()) {
    return <div className="log-viewer log-viewer--empty">No logs available</div>;
  }

  const searchLower = searchQuery.toLowerCase();

  return (
    <div className="log-viewer">
      <div className="log-viewer__toolbar">
        <Search size={14} className="text-muted" />
        <input
          className="log-viewer__search"
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          placeholder="Search logs..."
        />
        {matchingLines.length > 0 && (
          <>
            <span className="log-viewer__match-count">
              {currentMatch + 1}/{matchingLines.length}
            </span>
            <button className="btn-ghost btn-sm" onClick={handlePrevMatch} aria-label="Previous match">
              <ChevronUp size={14} />
            </button>
            <button className="btn-ghost btn-sm" onClick={handleNextMatch} aria-label="Next match">
              <ChevronDown size={14} />
            </button>
          </>
        )}
        {searchQuery && matchingLines.length === 0 && (
          <span className="log-viewer__match-count">No matches</span>
        )}
        <div className="log-viewer__actions">
          <button
            className={`btn-ghost btn-sm ${autoScroll ? "text-accent" : ""}`}
            onClick={() => setAutoScroll(!autoScroll)}
            title="Auto-scroll to bottom"
            aria-label="Toggle auto-scroll"
          >
            <ArrowDown size={14} />
          </button>
          <button className="btn-ghost btn-sm" onClick={handleCopy} title="Copy logs" aria-label="Copy logs">
            <Copy size={14} />
          </button>
        </div>
      </div>
      <pre className="log-viewer__content" ref={contentRef}>
        {lines.map((line, i) => {
          const level = getLogLevel(line);
          const isMatch = searchQuery && line.toLowerCase().includes(searchLower);
          const levelClass = level ? ` log-line--${level}` : "";
          const highlightClass = isMatch ? " log-line--highlight" : "";
          return (
            <div key={i} className={`log-viewer__line${levelClass}${highlightClass}`} data-line={i}>
              <span className="log-viewer__lineno">{i + 1}</span>
              <span className="log-viewer__text">{line}</span>
            </div>
          );
        })}
      </pre>
    </div>
  );
}
