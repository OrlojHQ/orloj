interface LogViewerProps {
  logs: string;
  loading?: boolean;
}

export function LogViewer({ logs, loading }: LogViewerProps) {
  if (loading) {
    return <div className="log-viewer log-viewer--loading">Loading logs...</div>;
  }

  if (!logs.trim()) {
    return <div className="log-viewer log-viewer--empty">No logs available</div>;
  }

  const lines = logs.split("\n");

  return (
    <div className="log-viewer">
      <pre className="log-viewer__content">
        {lines.map((line, i) => (
          <div key={i} className="log-viewer__line">
            <span className="log-viewer__lineno">{i + 1}</span>
            <span className="log-viewer__text">{line}</span>
          </div>
        ))}
      </pre>
    </div>
  );
}
