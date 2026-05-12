package resources

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// GraphOutgoingRoutes returns normalized outgoing routes in deterministic order.
// Legacy next is treated as the first route and deduplicated against edges[*].to.
func GraphOutgoingRoutes(node GraphEdge) []GraphRoute {
	routes := make([]GraphRoute, 0, len(node.Edges)+1)
	seen := make(map[string]struct{}, len(node.Edges)+1)
	add := func(route GraphRoute) {
		to := strings.TrimSpace(route.To)
		if to == "" {
			return
		}
		key := strings.ToLower(to)
		if _, ok := seen[key]; ok {
			return
		}
		route.To = to
		routes = append(routes, route)
		seen[key] = struct{}{}
	}

	if legacy := strings.TrimSpace(node.Next); legacy != "" {
		add(GraphRoute{To: legacy})
	}
	for _, edge := range node.Edges {
		add(edge)
	}
	return routes
}

// GraphOutgoingAgents returns normalized outgoing target agent names.
func GraphOutgoingAgents(node GraphEdge) []string {
	routes := GraphOutgoingRoutes(node)
	out := make([]string, 0, len(routes))
	for _, route := range routes {
		out = append(out, route.To)
	}
	return out
}

// FilterRoutesForOutput applies edge conditions against the completing agent's
// output and returns only the routes that should fire. When no edges carry
// conditions the full set is returned (backward-compatible). When conditions
// are present: matched conditional edges fire first; if none match, default
// edges fire; if no defaults exist either, an empty slice is returned
// (the task completes at this node).
func FilterRoutesForOutput(routes []GraphRoute, output string) []GraphRoute {
	if len(routes) == 0 {
		return nil
	}

	hasAnyCondition := false
	for _, r := range routes {
		if r.Condition != nil {
			hasAnyCondition = true
			break
		}
	}
	if !hasAnyCondition {
		return routes
	}

	outputLower := strings.ToLower(output)

	var matched []GraphRoute
	var defaults []GraphRoute
	var unconditional []GraphRoute

	for _, r := range routes {
		if r.Condition == nil {
			unconditional = append(unconditional, r)
			continue
		}
		if r.Condition.Default {
			defaults = append(defaults, r)
			continue
		}
		if edgeConditionMatches(r.Condition, output, outputLower) {
			matched = append(matched, r)
		}
	}

	if len(matched) > 0 {
		return append(matched, unconditional...)
	}
	if len(defaults) > 0 {
		return append(defaults, unconditional...)
	}
	if len(unconditional) > 0 {
		return unconditional
	}
	return nil
}

// EdgeConditionMatchesOutput evaluates an EdgeCondition against the given output string.
func EdgeConditionMatchesOutput(c *EdgeCondition, output string) bool {
	return edgeConditionMatches(c, output, strings.ToLower(output))
}

func edgeConditionMatches(c *EdgeCondition, output, outputLower string) bool {
	if c.OutputContains != "" {
		if !strings.Contains(outputLower, strings.ToLower(c.OutputContains)) {
			return false
		}
	}
	if c.OutputNotContains != "" {
		if strings.Contains(outputLower, strings.ToLower(c.OutputNotContains)) {
			return false
		}
	}
	if c.OutputMatches != "" {
		re := c.CompiledOutputMatches
		if re == nil {
			var err error
			re, err = regexp.Compile(c.OutputMatches)
			if err != nil {
				return false
			}
		}
		if !re.MatchString(output) {
			return false
		}
	}
	if c.OutputJSONPath != "" {
		if !jsonPathConditionMatches(c, output) {
			return false
		}
	}
	return true
}

// jsonPathConditionMatches evaluates JSON path conditions against the output.
func jsonPathConditionMatches(c *EdgeCondition, output string) bool {
	val, err := extractJSONPath(output, c.OutputJSONPath)
	if err != nil {
		return false
	}
	if c.Equals != "" {
		if jsonValueToString(val) != c.Equals {
			return false
		}
	}
	if c.NotEquals != "" {
		if jsonValueToString(val) == c.NotEquals {
			return false
		}
	}
	if c.Contains != "" {
		if !jsonValueContains(val, c.Contains) {
			return false
		}
	}
	if c.GreaterThan != "" {
		threshold, err := strconv.ParseFloat(c.GreaterThan, 64)
		if err != nil {
			return false
		}
		num, ok := jsonValueToFloat(val)
		if !ok || num <= threshold {
			return false
		}
	}
	if c.LessThan != "" {
		threshold, err := strconv.ParseFloat(c.LessThan, 64)
		if err != nil {
			return false
		}
		num, ok := jsonValueToFloat(val)
		if !ok || num >= threshold {
			return false
		}
	}
	return true
}

// extractJSONPath resolves a dot-notation path (e.g. "$.route" or "$.nested.field")
// against a JSON string and returns the raw value at that path.
func extractJSONPath(jsonStr string, path string) (any, error) {
	path = strings.TrimPrefix(path, "$")
	path = strings.TrimPrefix(path, ".")

	parseJSON := func(s string) (any, error) {
		var v any
		if err := json.Unmarshal([]byte(s), &v); err != nil {
			if unwrapped := UnwrapFencedCodeBlock(s); unwrapped != s {
				if err2 := json.Unmarshal([]byte(unwrapped), &v); err2 == nil {
					return v, nil
				}
			}
			return nil, err
		}
		return v, nil
	}

	if path == "" {
		return parseJSON(jsonStr)
	}

	root, err := parseJSON(jsonStr)
	if err != nil {
		return nil, fmt.Errorf("invalid JSON output: %w", err)
	}

	segments := strings.Split(path, ".")
	current := root
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		obj, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("cannot traverse non-object at %q", seg)
		}
		val, exists := obj[seg]
		if !exists {
			return nil, fmt.Errorf("key %q not found", seg)
		}
		current = val
	}
	return current, nil
}

func jsonValueToString(val any) string {
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

func jsonValueToFloat(val any) (float64, bool) {
	switch v := val.(type) {
	case float64:
		return v, true
	case string:
		f, err := strconv.ParseFloat(v, 64)
		return f, err == nil
	default:
		return 0, false
	}
}

// jsonValueContains checks if val contains the target string. For arrays, it
// checks whether any element's string representation equals target. For strings,
// it checks substring containment (case-insensitive).
func jsonValueContains(val any, target string) bool {
	switch v := val.(type) {
	case []any:
		for _, elem := range v {
			if jsonValueToString(elem) == target {
				return true
			}
		}
		return false
	case string:
		return strings.Contains(strings.ToLower(v), strings.ToLower(target))
	default:
		return strings.Contains(strings.ToLower(jsonValueToString(val)), strings.ToLower(target))
	}
}

// NormalizeGraphJoin applies defaults and validates join configuration.
func NormalizeGraphJoin(join GraphJoin) (GraphJoin, error) {
	mode := strings.ToLower(strings.TrimSpace(join.Mode))
	switch mode {
	case "", "wait_for_all":
		join.Mode = "wait_for_all"
	case "quorum":
		join.Mode = "quorum"
	default:
		return join, fmt.Errorf("invalid join.mode %q: expected wait_for_all or quorum", join.Mode)
	}

	if join.QuorumCount < 0 {
		join.QuorumCount = 0
	}
	if join.QuorumPercent < 0 {
		join.QuorumPercent = 0
	}
	if join.QuorumPercent > 100 {
		join.QuorumPercent = 100
	}

	onFailure := strings.ToLower(strings.TrimSpace(join.OnFailure))
	switch onFailure {
	case "", "deadletter":
		join.OnFailure = "deadletter"
	case "skip":
		join.OnFailure = "skip"
	case "continue_partial":
		join.OnFailure = "continue_partial"
	default:
		return join, fmt.Errorf("invalid join.on_failure %q: expected deadletter, skip, or continue_partial", join.OnFailure)
	}
	return join, nil
}

// UnwrapFencedCodeBlock strips markdown code fences (```lang ... ```) from
// content that models sometimes wrap around JSON output. If the content is
// not fenced the original string is returned unchanged.
func UnwrapFencedCodeBlock(content string) string {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "```") {
		return content
	}
	lines := strings.Split(content, "\n")
	if len(lines) < 3 {
		return content
	}
	if !strings.HasPrefix(strings.TrimSpace(lines[0]), "```") {
		return content
	}
	last := strings.TrimSpace(lines[len(lines)-1])
	if !strings.HasPrefix(last, "```") {
		return content
	}
	body := strings.Join(lines[1:len(lines)-1], "\n")
	return strings.TrimSpace(body)
}
