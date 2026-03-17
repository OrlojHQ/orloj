package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/OrlojHQ/orloj/resources"
)

func labelSelectorFilter(r *http.Request) (map[string]string, error) {
	if r == nil {
		return nil, nil
	}
	raw := strings.TrimSpace(r.URL.Query().Get("labelSelector"))
	if raw == "" {
		raw = strings.TrimSpace(r.URL.Query().Get("labels"))
	}
	if raw == "" {
		return nil, nil
	}

	req := make(map[string]string)
	parts := strings.Split(raw, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("invalid label selector %q: expected key=value pairs", part)
		}
		key := strings.TrimSpace(kv[0])
		value := strings.TrimSpace(kv[1])
		if key == "" {
			return nil, fmt.Errorf("invalid label selector %q: key is required", part)
		}
		req[key] = value
	}
	return req, nil
}

func matchMetadataFilters(meta resources.ObjectMeta, namespace string, hasNamespace bool, labels map[string]string) bool {
	if hasNamespace && !strings.EqualFold(resources.NormalizeNamespace(meta.Namespace), namespace) {
		return false
	}
	if len(labels) == 0 {
		return true
	}
	for key, expected := range labels {
		actual, ok := meta.Labels[key]
		if !ok {
			return false
		}
		if actual != expected {
			return false
		}
	}
	return true
}
