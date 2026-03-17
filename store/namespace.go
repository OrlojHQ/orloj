package store

import (
	"strings"

	"github.com/OrlojHQ/orloj/resources"
)

func scopedName(namespace, name string) string {
	ns := resources.NormalizeNamespace(namespace)
	n := strings.TrimSpace(name)
	return ns + "/" + n
}

func scopedNameFromMeta(meta resources.ObjectMeta) string {
	return scopedName(meta.Namespace, meta.Name)
}

func normalizeLookupName(name string) string {
	n := strings.TrimSpace(name)
	if n == "" {
		return scopedName(resources.DefaultNamespace, "")
	}
	if strings.Contains(n, "/") {
		parts := strings.SplitN(n, "/", 2)
		return scopedName(parts[0], parts[1])
	}
	return scopedName(resources.DefaultNamespace, n)
}

// ScopedName builds a namespaced key expected by store Get/Delete methods.
func ScopedName(namespace, name string) string {
	return scopedName(namespace, name)
}
