/** Shared loading skeleton for resource detail pages. */
export function DetailSkeleton() {
  return (
    <div className="page">
      <div className="detail-skeleton" aria-hidden>
        <div className="skeleton detail-skeleton__title" />
        <div className="skeleton detail-skeleton__tabs" />
        <div className="skeleton detail-skeleton__block" />
      </div>
    </div>
  );
}
