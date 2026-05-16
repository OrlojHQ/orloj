package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/OrlojHQ/orloj/crds"
	"github.com/OrlojHQ/orloj/resources"
)

const (
	CRDConflictOff    = "off"
	CRDConflictWarn   = "warn"
	CRDConflictReject = "reject"
)

func normalizeCRDConflictPolicy(policy string) string {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case CRDConflictReject:
		return CRDConflictReject
	case CRDConflictOff:
		return CRDConflictOff
	default:
		return CRDConflictWarn
	}
}

// checkCRDConflict inspects the resource's annotations for the CRD-managed
// marker. In "warn" mode it adds a response header; in "reject" mode it
// writes a 409 Conflict and returns true (caller should abort).
func (s *Server) checkCRDConflict(w http.ResponseWriter, meta resources.ObjectMeta, kind string) bool {
	if s.crdConflictPolicy == CRDConflictOff {
		return false
	}
	if !crds.IsCRDManaged(meta) {
		return false
	}
	switch s.crdConflictPolicy {
	case CRDConflictReject:
		w.Header().Set("X-Orloj-CRD-Managed", "true")
		http.Error(w,
			fmt.Sprintf("%s %q is managed by a CRD — update it via kubectl apply or your Git repo, not the REST API", kind, meta.Name),
			http.StatusConflict)
		return true
	case CRDConflictWarn:
		w.Header().Set("X-Orloj-CRD-Managed", "true")
		if s.logger != nil {
			s.logger.Printf("[warn] %s %q is CRD-managed; REST write will be overwritten on next operator sync", kind, meta.Name)
		}
		return false
	}
	return false
}
