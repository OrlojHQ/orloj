import { useRef, type ReactNode } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { ArrowDown, ArrowUp, ArrowUpDown } from "lucide-react";
import clsx from "clsx";

export type SortOrder = "asc" | "desc";

export interface Column<T> {
  key: string;
  header: string;
  render: (row: T) => ReactNode;
  width?: string;
  /** When true, header is clickable if `onSortChange` is provided. */
  sortable?: boolean;
  /** Server/client sort field; defaults to `key`. */
  sortKey?: string;
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
  /** Controlled sort field (matches column `sortKey` / `key`). */
  sortKey?: string;
  sortOrder?: SortOrder;
  onSortChange?: (sortKey: string, sortOrder: SortOrder) => void;
  /**
   * When true and row count exceeds the threshold, virtualize the table body
   * for large lists (Tasks). Short lists stay non-virtual.
   */
  virtualize?: boolean;
  virtualizeThreshold?: number;
}

const DEFAULT_VIRTUALIZE_THRESHOLD = 50;
const ESTIMATED_ROW_HEIGHT = 41;
const VIRTUAL_VIEWPORT_MAX = 560;

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
  sortKey,
  sortOrder,
  onSortChange,
  virtualize = false,
  virtualizeThreshold = DEFAULT_VIRTUALIZE_THRESHOLD,
}: ResourceTableProps<T>) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const shouldVirtualize = virtualize && data.length > virtualizeThreshold;

  const virtualizer = useVirtualizer({
    count: shouldVirtualize ? data.length : 0,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ESTIMATED_ROW_HEIGHT,
    overscan: 8,
  });

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

  const handleSortClick = (col: Column<T>) => {
    if (!col.sortable || !onSortChange) return;
    const key = col.sortKey ?? col.key;
    if (sortKey === key) {
      onSortChange(key, sortOrder === "asc" ? "desc" : "asc");
    } else {
      onSortChange(key, "asc");
    }
  };

  const renderHeader = () => (
    <tr>
      {columns.map((col) => {
        const colSortKey = col.sortKey ?? col.key;
        const active = col.sortable && sortKey === colSortKey;
        const ariaSort = !col.sortable
          ? undefined
          : active
            ? sortOrder === "asc"
              ? "ascending"
              : "descending"
            : "none";
        return (
          <th
            key={col.key}
            style={col.width ? { width: col.width } : undefined}
            aria-sort={ariaSort}
            className={clsx(col.sortable && onSortChange && "table-th--sortable")}
          >
            {col.sortable && onSortChange ? (
              <button
                type="button"
                className="table-th__sort"
                onClick={() => handleSortClick(col)}
              >
                <span>{col.header}</span>
                {active ? (
                  sortOrder === "asc" ? <ArrowUp size={12} /> : <ArrowDown size={12} />
                ) : (
                  <ArrowUpDown size={12} className="table-th__sort-idle" />
                )}
              </button>
            ) : (
              col.header
            )}
          </th>
        );
      })}
    </tr>
  );

  const renderRow = (row: T) => (
    <tr
      key={rowKey(row)}
      onClick={onRowClick ? () => onRowClick(row) : undefined}
      className={onRowClick ? "table-row--clickable" : undefined}
    >
      {columns.map((col) => (
        <td key={col.key}>{col.render(row)}</td>
      ))}
    </tr>
  );

  const virtualItems = shouldVirtualize ? virtualizer.getVirtualItems() : [];
  const paddingTop = shouldVirtualize && virtualItems.length > 0 ? virtualItems[0].start : 0;
  const paddingBottom =
    shouldVirtualize && virtualItems.length > 0
      ? virtualizer.getTotalSize() - virtualItems[virtualItems.length - 1].end
      : 0;

  return (
    <div className="table-wrapper">
      <div
        ref={scrollRef}
        className={clsx(shouldVirtualize && "table-wrapper__scroll")}
        style={
          shouldVirtualize
            ? { maxHeight: VIRTUAL_VIEWPORT_MAX, overflow: "auto" }
            : undefined
        }
      >
        <table>
          <thead>{renderHeader()}</thead>
          <tbody>
            {shouldVirtualize ? (
              <>
                {paddingTop > 0 && (
                  <tr aria-hidden className="table-virtual-spacer">
                    <td colSpan={columns.length} style={{ height: paddingTop, padding: 0, border: 0 }} />
                  </tr>
                )}
                {virtualItems.map((item) => renderRow(data[item.index]))}
                {paddingBottom > 0 && (
                  <tr aria-hidden className="table-virtual-spacer">
                    <td colSpan={columns.length} style={{ height: paddingBottom, padding: 0, border: 0 }} />
                  </tr>
                )}
              </>
            ) : (
              data.map((row) => renderRow(row))
            )}
          </tbody>
        </table>
      </div>
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
