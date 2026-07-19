import { isCrdManaged } from "../utils/crd";
import { confirmDialog } from "../components/ConfirmDialog";

export function useDeleteConfirm() {
  return (kind: string, name: string, metadata?: { annotations?: Record<string, string> }): Promise<boolean> => {
    if (isCrdManaged(metadata)) {
      return confirmDialog({
        title: `Delete ${kind}?`,
        message:
          `"${name}" is managed by a CRD. Deleting it from Orloj will not remove the CRD — the operator will recreate the resource on its next sync. ` +
          `To permanently delete, remove the CRD with: kubectl delete ${kind.toLowerCase()} ${name}`,
        confirmLabel: "Delete anyway",
        danger: true,
      });
    }
    return confirmDialog({
      title: `Delete ${kind}?`,
      message: `"${name}" will be permanently deleted. This cannot be undone.`,
      confirmLabel: "Delete",
      danger: true,
    });
  };
}
