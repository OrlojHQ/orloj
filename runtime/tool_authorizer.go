package agentruntime

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/OrlojHQ/orloj/crds"
)

type ToolCallAuthorizer interface {
	Authorize(tool string, spec crds.ToolSpec) error
}

type AgentRoleLookup interface {
	Get(name string) (crds.AgentRole, bool)
}

type ToolPermissionLookup interface {
	List() []crds.ToolPermission
}

type toolPermissionRule struct {
	Name      string
	ToolRef   string
	Action    string
	MatchMode string
	Required  []string
}

type AgentToolAuthorizer struct {
	enforceByRole bool
	permissions   map[string]struct{}
	roles         []string
	missingRoles  []string
	rules         map[string][]toolPermissionRule
}

func NewAgentToolAuthorizer(
	namespace string,
	agent crds.Agent,
	roleLookup AgentRoleLookup,
	permissionLookup ToolPermissionLookup,
) *AgentToolAuthorizer {
	if isNilLookup(roleLookup) {
		roleLookup = nil
	}
	if isNilLookup(permissionLookup) {
		permissionLookup = nil
	}
	a := &AgentToolAuthorizer{
		enforceByRole: len(agent.Spec.Roles) > 0,
		permissions:   make(map[string]struct{}),
		rules:         make(map[string][]toolPermissionRule),
	}

	for _, rawRole := range agent.Spec.Roles {
		roleName := strings.TrimSpace(rawRole)
		if roleName == "" {
			continue
		}
		a.roles = append(a.roles, roleName)
		if roleLookup == nil {
			a.missingRoles = append(a.missingRoles, roleName)
			continue
		}
		role, ok := roleLookup.Get(scopedRuntimeName(namespace, roleName))
		if !ok {
			role, ok = roleLookup.Get(roleName)
		}
		if !ok {
			a.missingRoles = append(a.missingRoles, roleName)
			continue
		}
		for _, permission := range role.Spec.Permissions {
			token := normalizePermission(permission)
			if token == "" {
				continue
			}
			a.permissions[token] = struct{}{}
		}
	}

	if permissionLookup == nil {
		return a
	}
	for _, item := range permissionLookup.List() {
		if crds.NormalizeNamespace(item.Metadata.Namespace) != crds.NormalizeNamespace(namespace) {
			continue
		}
		if !toolPermissionAppliesToAgent(item, agent.Metadata.Name) {
			continue
		}
		key := normalizeToolKey(item.Spec.ToolRef)
		if key == "" {
			continue
		}
		required := make([]string, 0, len(item.Spec.RequiredPermissions))
		for _, permission := range item.Spec.RequiredPermissions {
			token := normalizePermission(permission)
			if token == "" {
				continue
			}
			required = append(required, token)
		}
		a.rules[key] = append(a.rules[key], toolPermissionRule{
			Name:      strings.TrimSpace(item.Metadata.Name),
			ToolRef:   strings.TrimSpace(item.Spec.ToolRef),
			Action:    normalizePermissionToken(item.Spec.Action),
			MatchMode: strings.ToLower(strings.TrimSpace(item.Spec.MatchMode)),
			Required:  required,
		})
	}
	return a
}

func toolPermissionAppliesToAgent(item crds.ToolPermission, agentName string) bool {
	mode := strings.ToLower(strings.TrimSpace(item.Spec.ApplyMode))
	if mode == "" || mode == "global" {
		return true
	}
	if mode != "scoped" {
		return false
	}
	for _, target := range item.Spec.TargetAgents {
		if strings.EqualFold(strings.TrimSpace(target), strings.TrimSpace(agentName)) {
			return true
		}
	}
	return false
}

func (a *AgentToolAuthorizer) Authorize(tool string, spec crds.ToolSpec) error {
	if a == nil {
		return nil
	}
	if len(a.missingRoles) > 0 {
		missing := append([]string(nil), a.missingRoles...)
		sort.Strings(missing)
		return NewToolDeniedError(
			fmt.Sprintf("policy permission denied for tool=%s missing_roles=%s", strings.TrimSpace(tool), strings.Join(missing, ",")),
			map[string]string{
				"tool":          strings.TrimSpace(tool),
				"missing_roles": strings.Join(missing, ","),
			},
			ErrToolPermissionDenied,
		)
	}

	toolKey := normalizeToolKey(tool)
	rules := a.rules[toolKey]
	if len(rules) == 0 && !a.enforceByRole {
		return nil
	}
	if len(rules) == 0 {
		required := defaultRequiredPermissions(tool, spec, "invoke")
		if ok, missing := permissionMatchAll(a.permissions, required); !ok {
			return NewToolDeniedError(
				fmt.Sprintf("policy permission denied for tool=%s required=%s", strings.TrimSpace(tool), strings.Join(missing, ",")),
				map[string]string{
					"tool":     strings.TrimSpace(tool),
					"required": strings.Join(missing, ","),
				},
				ErrToolPermissionDenied,
			)
		}
		return nil
	}

	for _, rule := range rules {
		required := append([]string(nil), rule.Required...)
		if len(required) == 0 {
			required = defaultRequiredPermissions(tool, spec, rule.Action)
		}
		if len(required) == 0 {
			continue
		}
		matchMode := strings.ToLower(strings.TrimSpace(rule.MatchMode))
		if matchMode == "" {
			matchMode = "all"
		}
		switch matchMode {
		case "any":
			if ok, missing := permissionMatchAny(a.permissions, required); !ok {
				return NewToolDeniedError(
					fmt.Sprintf("policy permission denied for tool=%s rule=%s required_any=%s", strings.TrimSpace(tool), rule.Name, strings.Join(missing, ",")),
					map[string]string{
						"tool":         strings.TrimSpace(tool),
						"rule":         strings.TrimSpace(rule.Name),
						"required_any": strings.Join(missing, ","),
					},
					ErrToolPermissionDenied,
				)
			}
		default:
			if ok, missing := permissionMatchAll(a.permissions, required); !ok {
				return NewToolDeniedError(
					fmt.Sprintf("policy permission denied for tool=%s rule=%s required=%s", strings.TrimSpace(tool), rule.Name, strings.Join(missing, ",")),
					map[string]string{
						"tool":     strings.TrimSpace(tool),
						"rule":     strings.TrimSpace(rule.Name),
						"required": strings.Join(missing, ","),
					},
					ErrToolPermissionDenied,
				)
			}
		}
	}
	return nil
}

func defaultRequiredPermissions(tool string, spec crds.ToolSpec, action string) []string {
	action = normalizePermissionToken(action)
	if action == "" {
		action = "invoke"
	}
	out := []string{normalizePermission("tool:" + normalizePermissionToken(tool) + ":" + action)}
	for _, capability := range spec.Capabilities {
		token := normalizePermission("capability:" + normalizePermissionToken(capability))
		if token == "" {
			continue
		}
		out = append(out, token)
	}
	return dedupePermissions(out)
}

func dedupePermissions(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		normalized := normalizePermission(value)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func permissionMatchAll(granted map[string]struct{}, required []string) (bool, []string) {
	missing := make([]string, 0)
	for _, permission := range dedupePermissions(required) {
		if _, ok := granted[permission]; ok {
			continue
		}
		missing = append(missing, permission)
	}
	return len(missing) == 0, missing
}

func permissionMatchAny(granted map[string]struct{}, required []string) (bool, []string) {
	required = dedupePermissions(required)
	for _, permission := range required {
		if _, ok := granted[permission]; ok {
			return true, nil
		}
	}
	return false, required
}

func normalizePermission(value string) string {
	replacer := strings.NewReplacer(
		" ", "",
		"/", "_",
		"\\", "_",
		"-", "_",
	)
	value = replacer.Replace(strings.TrimSpace(strings.ToLower(value)))
	if value == "" {
		return ""
	}
	return value
}

func normalizePermissionToken(value string) string {
	replacer := strings.NewReplacer(
		" ", "_",
		"/", "_",
		"\\", "_",
		"-", "_",
	)
	return strings.Trim(strings.ToLower(replacer.Replace(strings.TrimSpace(value))), "_:")
}

func isNilLookup(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}
