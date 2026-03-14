package crds

import "strings"

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

// NormalizeGraphJoin applies defaults and clamps unsupported values.
func NormalizeGraphJoin(join GraphJoin) GraphJoin {
	mode := strings.ToLower(strings.TrimSpace(join.Mode))
	switch mode {
	case "", "wait_for_all":
		join.Mode = "wait_for_all"
	case "quorum":
		join.Mode = "quorum"
	default:
		join.Mode = "wait_for_all"
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
		join.OnFailure = "deadletter"
	}
	return join
}
