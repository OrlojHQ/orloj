export const CRD_MANAGED_ANNOTATION = "orloj.dev/managed-by";
export const CRD_MANAGED_VALUE = "crd-sync";

export const CRD_MANAGED_EDIT_WARNING =
  "This resource is managed by a CRD. Changes saved here will be overwritten on the next operator sync. Edit via kubectl apply or your Git repo instead.";

export function isCrdManaged(metadata?: { annotations?: Record<string, string> }): boolean {
  return metadata?.annotations?.[CRD_MANAGED_ANNOTATION] === CRD_MANAGED_VALUE;
}
