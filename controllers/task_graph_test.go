package controllers

import (
	"strings"
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

func TestExecutionOrderTopologicalWithFanOut(t *testing.T) {
	system := resources.AgentSystem{
		Spec: resources.AgentSystemSpec{
			Agents: []string{"planner", "researcher", "reviewer", "writer"},
			Graph: map[string]resources.GraphEdge{
				"planner": {
					Edges: []resources.GraphRoute{
						{To: "researcher"},
						{To: "reviewer"},
					},
				},
				"researcher": {Edges: []resources.GraphRoute{{To: "writer"}}},
				"reviewer":   {Edges: []resources.GraphRoute{{To: "writer"}}},
			},
		},
	}

	got := executionOrder(system)
	want := []string{"planner", "researcher", "reviewer", "writer"}
	if len(got) != len(want) {
		t.Fatalf("unexpected order length: got=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected execution order: got=%v want=%v", got, want)
		}
	}
}

func TestValidateGraphRejectsInvalidJoinConfig(t *testing.T) {
	system := resources.AgentSystem{
		Spec: resources.AgentSystemSpec{
			Agents: []string{"planner", "writer"},
			Graph: map[string]resources.GraphEdge{
				"planner": {
					Edges: []resources.GraphRoute{{To: "writer"}},
				},
				"writer": {
					Join: resources.GraphJoin{
						Mode:          "invalid-mode",
						QuorumPercent: 120,
						OnFailure:     "unknown",
					},
				},
			},
		},
	}

	agentSet := map[string]struct{}{
		"planner": {},
		"writer":  {},
	}
	errs := validateGraph(system, agentSet)
	if len(errs) == 0 {
		t.Fatal("expected validateGraph errors")
	}

	joined := strings.Join(errs, " | ")
	if !strings.Contains(joined, "unsupported join.mode") {
		t.Fatalf("expected join.mode validation error, got %v", errs)
	}
	if !strings.Contains(joined, "invalid join.quorum_percent") {
		t.Fatalf("expected join.quorum_percent validation error, got %v", errs)
	}
	if !strings.Contains(joined, "unsupported join.on_failure") {
		t.Fatalf("expected join.on_failure validation error, got %v", errs)
	}
}
