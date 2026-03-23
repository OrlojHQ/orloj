import type { ReactNode } from "react";

export interface Column<T> {
  key: string;
  header: string;
  render: (row: T) => ReactNode;
  width?: string;
}

interface ResourceTableProps<T> {
  columns: Column<T>[];
  data: T[];
  rowKey: (row: T) => string;
  onRowClick?: (row: T) => void;
  emptyMessage?: string;
  loading?: boolean;
  /** Server-backed list: more pages available (cursor pagination). */
  hasMore?: boolean;
  onLoadMore?: () => void;
  loadingMore?: boolean;
}

export function ResourceTable<T>({
  columns,
  data,
  rowKey,
  onRowClick,
  emptyMessage = "No resources found",
  loading,
  hasMore,
  onLoadMore,
  loadingMore,
}: ResourceTableProps<T>) {
  if (loading) {
    return (
      <div className="table-skeleton">
        {Array.from({ length: 5 }).map((_, i) => (
          <div key={i} className="table-skeleton__row">
            {columns.map((col) => (
              <div key={col.key} className="table-skeleton__cell" />
            ))}
          </div>
        ))}
      </div>
    );
  }

  if (data.length === 0) {
    return <div className="table-empty">{emptyMessage}</div>;
  }

  return (
    <div className="table-wrapper">
      <table>
        <thead>
          <tr>
            {columns.map((col) => (
              <th key={col.key} style={col.width ? { width: col.width } : undefined}>
                {col.header}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {data.map((row) => (
            <tr
              key={rowKey(row)}
              onClick={onRowClick ? () => onRowClick(row) : undefined}
              className={onRowClick ? "table-row--clickable" : undefined}
            >
              {columns.map((col) => (
                <td key={col.key}>{col.render(row)}</td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
      {hasMore && onLoadMore && (
        <div className="table-load-more">
          <button
            type="button"
            className="btn-secondary"
            onClick={onLoadMore}
            disabled={loadingMore}
          >
            {loadingMore ? "Loading…" : "Load more"}
          </button>
        </div>
      )}
    </div>
  );
}
