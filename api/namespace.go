package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/store"
)

// paginationParams parses optional ?limit and ?offset query parameters.
// limit defaults to 0 (meaning "use the store default"); offset defaults to 0.
func paginationParams(r *http.Request) (limit, offset int) {
	if r == nil {
		return 0, 0
	}
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if v := strings.TrimSpace(r.URL.Query().Get("offset")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	return limit, offset
}

func requestNamespace(r *http.Request) string {
	if r == nil {
		return resources.DefaultNamespace
	}
	return resources.NormalizeNamespace(r.URL.Query().Get("namespace"))
}

func namespaceFilter(r *http.Request) (string, bool) {
	if r == nil {
		return "", false
	}
	raw := strings.TrimSpace(r.URL.Query().Get("namespace"))
	if raw == "" {
		return "", false
	}
	return resources.NormalizeNamespace(raw), true
}

func scopedNameForRequest(r *http.Request, name string) string {
	return store.ScopedName(requestNamespace(r), name)
}

func applyRequestNamespace(r *http.Request, meta *resources.ObjectMeta) error {
	if meta == nil {
		return nil
	}
	ns := requestNamespace(r)
	meta.Namespace = resources.NormalizeNamespace(meta.Namespace)
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
