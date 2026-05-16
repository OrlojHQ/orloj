import { isCrdManaged } from "../utils/crd";

export function useDeleteConfirm() {
  return (kind: string, name: string, metadata?: { annotations?: Record<string, string> }): boolean => {
    if (isCrdManaged(metadata)) {
      return window.confirm(
        `This resource is managed by a CRD. Deleting it from Orloj will not remove the CRD — the operator will recreate the resource on its next sync.\n\nTo permanently delete, remove the CRD with: kubectl delete ${kind.toLowerCase()} ${name}\n\nDelete from Orloj anyway?`,
      );
    }
    return window.confirm(`Delete ${kind} "${name}"?`);
  };
}
