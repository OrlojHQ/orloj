package resources

import "strings"

// ExecutionAgentOrder returns a topological ordering of agents in the AgentSystem suitable
// for sequential execution. If no graph is present, declaration order from spec.agents is used.
// This duplicates the controller semantics used for task reconciliation.
func ExecutionAgentOrder(system AgentSystem) []string {
	if len(system.Spec.Agents) == 0 {
		return nil
	}

	if len(system.Spec.Graph) == 0 {
		order := make([]string, len(system.Spec.Agents))
		copy(order, system.Spec.Agents)
		return order
	}

	indegree := make(map[string]int, len(system.Spec.Agents))
	for _, agent := range system.Spec.Agents {
		indegree[agent] = 0
	}
	for _, node := range system.Spec.Graph {
		for _, to := range GraphOutgoingAgents(node) {
			if _, ok := indegree[to]; ok {
				indegree[to]++
			}
		}
	}

	queue := make([]string, 0, len(system.Spec.Agents))
	queued := make(map[string]struct{}, len(system.Spec.Agents))
	for _, agent := range system.Spec.Agents {
		if indegree[agent] != 0 {
			continue
		}
		queue = append(queue, agent)
		queued[agent] = struct{}{}
	}
	if len(queue) == 0 {
		order := make([]string, len(system.Spec.Agents))
		copy(order, system.Spec.Agents)
		return order
	}

	order := make([]string, 0, len(system.Spec.Agents))
	visited := make(map[string]struct{}, len(system.Spec.Agents))
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if _, seen := visited[current]; seen {
			continue
		}
		visited[current] = struct{}{}
		order = append(order, current)

		node, ok := system.Spec.Graph[current]
		if !ok {
			continue
		}
		for _, to := range GraphOutgoingAgents(node) {
			if _, tracked := indegree[to]; !tracked {
				continue
			}
			indegree[to]--
			if indegree[to] == 0 {
				if _, alreadyQueued := queued[to]; alreadyQueued {
					continue
				}
				queue = append(queue, to)
				queued[to] = struct{}{}
			}
		}
	}

	for _, agent := range system.Spec.Agents {
		if _, seen := visited[agent]; seen {
			continue
		}
		order = append(order, agent)
	}
	return order
}

// IsFirstExecutionAgent reports whether agentName is the first agent in ExecutionAgentOrder.
func IsFirstExecutionAgent(system AgentSystem, agentName string) bool {
	name := strings.TrimSpace(agentName)
	if name == "" {
		return false
	}
	o := ExecutionAgentOrder(system)
	if len(o) == 0 {
		return false
	}
	return strings.TrimSpace(o[0]) == name
}
