package crds

// CRDStatus is the common status subresource for all Orloj CRDs.
type CRDStatus struct {
	Phase              string `json:"phase,omitempty"`
	ObservedGeneration int64  `json:"observedGeneration,omitempty"`
	LastSyncedAt       string `json:"lastSyncedAt,omitempty"`
	SyncError          string `json:"syncError,omitempty"`
}
