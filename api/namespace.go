package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/OrlojHQ/orloj/crds"
	"github.com/OrlojHQ/orloj/store"
)

func requestNamespace(r *http.Request) string {
	if r == nil {
		return crds.DefaultNamespace
	}
	return crds.NormalizeNamespace(r.URL.Query().Get("namespace"))
}

func namespaceFilter(r *http.Request) (string, bool) {
	if r == nil {
		return "", false
	}
	raw := strings.TrimSpace(r.URL.Query().Get("namespace"))
	if raw == "" {
		return "", false
	}
	return crds.NormalizeNamespace(raw), true
}

func scopedNameForRequest(r *http.Request, name string) string {
	return store.ScopedName(requestNamespace(r), name)
}

func applyRequestNamespace(r *http.Request, meta *crds.ObjectMeta) error {
	if meta == nil {
		return nil
	}
	ns := requestNamespace(r)
	meta.Namespace = crds.NormalizeNamespace(meta.Namespace)
	if strings.TrimSpace(meta.Namespace) == "" {
		meta.Namespace = ns
	}
	requested := strings.TrimSpace(r.URL.Query().Get("namespace"))
	if requested != "" && !strings.EqualFold(meta.Namespace, ns) {
		return fmt.Errorf("metadata.namespace %q does not match request namespace %q", meta.Namespace, ns)
	}
	if strings.TrimSpace(meta.Namespace) == "" {
		meta.Namespace = ns
	}
	return nil
}
