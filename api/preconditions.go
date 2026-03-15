package api

import (
	"fmt"
	"strings"

	"github.com/OrlojHQ/orloj/crds"
)

func requireUpdatePrecondition(headerValue string, incoming *crds.ObjectMeta, current crds.ObjectMeta) error {
	if incoming == nil {
		return fmt.Errorf("metadata is required")
	}

	ifMatch := canonicalizeIfMatch(headerValue)
	if ifMatch != "" {
		if ifMatch == "*" {
			ifMatch = current.ResourceVersion
		}
		if incoming.ResourceVersion != "" && incoming.ResourceVersion != ifMatch {
			return fmt.Errorf("metadata.resourceVersion %q does not match If-Match %q", incoming.ResourceVersion, ifMatch)
		}
		incoming.ResourceVersion = ifMatch
	}

	if strings.TrimSpace(incoming.ResourceVersion) == "" {
		return fmt.Errorf("resourceVersion precondition required for update (set metadata.resourceVersion or If-Match)")
	}
	if current.ResourceVersion == "" {
		return fmt.Errorf("current resourceVersion is missing")
	}
	return nil
}

func canonicalizeIfMatch(v string) string {
	v = strings.TrimSpace(v)
	if strings.HasPrefix(strings.ToUpper(v), "W/") {
		v = strings.TrimSpace(v[2:])
	}
	v = strings.Trim(v, `"`)
	return v
}
