import { isCrdManaged } from "../utils/crd";

interface CrdManagedBadgeProps {
  metadata?: { annotations?: Record<string, string> };
}

export function CrdManagedBadge({ metadata }: CrdManagedBadgeProps) {
  if (!isCrdManaged(metadata)) return null;
  return (
    <span
      className="badge badge--purple"
      title="Managed via kubectl / GitOps. Edit in your Git repo or with kubectl, not here."
    >
      CRD
    </span>
  );
}
