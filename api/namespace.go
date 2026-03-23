package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/store"
)

const maxPaginationLimit = 1000

// paginationParams parses optional ?limit and ?offset query parameters.
// limit defaults to 0 (meaning "use the store default"); offset defaults to 0.
// Both are capped to prevent resource-exhaustion attacks.
func paginationParams(r *http.Request) (limit, offset int) {
	if r == nil {
		return 0, 0
	}
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > maxPaginationLimit {
		limit = maxPaginationLimit
	}
	if v := strings.TrimSpace(r.URL.Query().Get("offset")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	return limit, offset
}

// cursorParam extracts the optional ?after= cursor for keyset pagination.
func cursorParam(r *http.Request) string {
	if r == nil {
		return ""
	}
	return strings.TrimSpace(r.URL.Query().Get("after"))
}

// listContinue returns the cursor value for the next page. If the returned
// slice has exactly limit items, the last item's name is used as the continue
// token; otherwise the empty string signals "no more pages".
func listContinue(limit, count int, lastName string) string {
	if limit > 0 && count >= limit {
		return lastName
	}
	return ""
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
