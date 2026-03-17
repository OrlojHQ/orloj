package resources

import (
	"reflect"
	"testing"
)

func TestGraphOutgoingAgentsLegacyAndRichEdges(t *testing.T) {
	node := GraphEdge{
		Next: "researcher",
		Edges: []GraphRoute{
			{To: "researcher"},
			{To: "writer"},
			{To: " reviewer "},
		},
	}

	got := GraphOutgoingAgents(node)
	want := []string{"researcher", "writer", "reviewer"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected outgoing agents: got=%v want=%v", got, want)
	}
}

func TestParseAgentSystemManifestRichGraphYAML(t *testing.T) {
	raw := []byte(`
apiVersion: orloj.dev/v1
kind: AgentSystem
metadata:
  name: report-system
spec:
  agents:
    - planner
    - reviewer
    - writer
  graph:
    planner:
      edges:
        - to: reviewer
          labels:
            lane: fast
          policy:
            retry_class: burst
        - to: writer
    writer:
      join:
        mode: quorum
        quorum_count: 1
        quorum_percent: 50
        on_failure: deadletter
`)

	system, err := ParseAgentSystemManifest(raw)
	if err != nil {
		t.Fatalf("parse agent system failed: %v", err)
	}

	planner, ok := system.Spec.Graph["planner"]
	if !ok {
		t.Fatal("expected planner graph node")
	}
	if len(planner.Edges) != 2 {
		t.Fatalf("expected 2 planner edges, got %d", len(planner.Edges))
	}
	if planner.Edges[0].To != "reviewer" {
		t.Fatalf("expected first edge to reviewer, got %q", planner.Edges[0].To)
	}
	if planner.Edges[0].Labels["lane"] != "fast" {
		t.Fatalf("expected edge label lane=fast, got %q", planner.Edges[0].Labels["lane"])
	}
	if planner.Edges[0].Policy["retry_class"] != "burst" {
		t.Fatalf("expected edge policy retry_class=burst, got %q", planner.Edges[0].Policy["retry_class"])
	}

	writer := system.Spec.Graph["writer"]
	if writer.Join.Mode != "quorum" {
		t.Fatalf("expected writer join.mode quorum, got %q", writer.Join.Mode)
	}
	if writer.Join.QuorumCount != 1 {
		t.Fatalf("expected writer join.quorum_count=1, got %d", writer.Join.QuorumCount)
	}
	if writer.Join.QuorumPercent != 50 {
		t.Fatalf("expected writer join.quorum_percent=50, got %d", writer.Join.QuorumPercent)
	}
	if writer.Join.OnFailure != "deadletter" {
		t.Fatalf("expected writer join.on_failure=deadletter, got %q", writer.Join.OnFailure)
	}
}
