package resources

import (
	"fmt"
	"strings"
)

// ParseManifest parses and normalizes a manifest of a supported kind. kind should be the
// value from DetectKind (any casing); it is normalized with strings.ToLower(strings.TrimSpace(kind)).
// On success, normKind is the normalized kind string, name is metadata.name, and obj is the
// typed resource suitable for json.Marshal (same shape as apply uses).
func ParseManifest(kind string, raw []byte) (normKind string, name string, obj any, err error) {
	normKind = strings.ToLower(strings.TrimSpace(kind))
	switch normKind {
	case "agent":
		o, e := ParseAgentManifest(raw)
		if e != nil {
			return "", "", nil, e
		}
		return normKind, o.Metadata.Name, o, nil
	case "agentsystem":
		o, e := ParseAgentSystemManifest(raw)
		if e != nil {
			return "", "", nil, e
		}
		return normKind, o.Metadata.Name, o, nil
	case "modelendpoint":
		o, e := ParseModelEndpointManifest(raw)
		if e != nil {
			return "", "", nil, e
		}
		return normKind, o.Metadata.Name, o, nil
	case "tool":
		o, e := ParseToolManifest(raw)
		if e != nil {
			return "", "", nil, e
		}
		return normKind, o.Metadata.Name, o, nil
	case "secret":
		o, e := ParseSecretManifest(raw)
		if e != nil {
			return "", "", nil, e
		}
		return normKind, o.Metadata.Name, o, nil
	case "memory":
		o, e := ParseMemoryManifest(raw)
		if e != nil {
			return "", "", nil, e
		}
		return normKind, o.Metadata.Name, o, nil
	case "agentpolicy":
		o, e := ParseAgentPolicyManifest(raw)
		if e != nil {
			return "", "", nil, e
		}
		return normKind, o.Metadata.Name, o, nil
	case "agentrole":
		o, e := ParseAgentRoleManifest(raw)
		if e != nil {
			return "", "", nil, e
		}
		return normKind, o.Metadata.Name, o, nil
	case "toolpermission":
		o, e := ParseToolPermissionManifest(raw)
		if e != nil {
			return "", "", nil, e
		}
		return normKind, o.Metadata.Name, o, nil
	case "toolapproval":
		o, e := ParseToolApprovalManifest(raw)
		if e != nil {
			return "", "", nil, e
		}
		return normKind, o.Metadata.Name, o, nil
	case "task":
		o, e := ParseTaskManifest(raw)
		if e != nil {
			return "", "", nil, e
		}
		return normKind, o.Metadata.Name, o, nil
	case "taskschedule":
		o, e := ParseTaskScheduleManifest(raw)
		if e != nil {
			return "", "", nil, e
		}
		return normKind, o.Metadata.Name, o, nil
	case "taskwebhook":
		o, e := ParseTaskWebhookManifest(raw)
		if e != nil {
			return "", "", nil, e
		}
		return normKind, o.Metadata.Name, o, nil
	case "worker":
		o, e := ParseWorkerManifest(raw)
		if e != nil {
			return "", "", nil, e
		}
		return normKind, o.Metadata.Name, o, nil
	case "mcpserver":
		o, e := ParseMcpServerManifest(raw)
		if e != nil {
			return "", "", nil, e
		}
		return normKind, o.Metadata.Name, o, nil
	default:
		return "", "", nil, fmt.Errorf("unsupported kind %q", normKind)
	}
}
