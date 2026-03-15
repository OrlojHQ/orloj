package cli

import (
	"reflect"
	"testing"

	"github.com/OrlojHQ/orloj/crds"
)

func TestSystemGraphLines(t *testing.T) {
	t.Run("declared order without graph", func(t *testing.T) {
		system := crds.AgentSystem{
			Spec: crds.AgentSystemSpec{
				Agents: []string{"planner", "researcher", "writer"},
			},
		}
		got := systemGraphLines(system)
		want := []string{
			"planner -> researcher",
			"researcher -> writer",
			"writer -> (end)",
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("unexpected graph lines: got=%v want=%v", got, want)
		}
	})

	t.Run("explicit graph", func(t *testing.T) {
		system := crds.AgentSystem{
			Spec: crds.AgentSystemSpec{
				Agents: []string{"planner", "researcher", "writer"},
				Graph: map[string]crds.GraphEdge{
					"planner":    {Next: "researcher"},
					"researcher": {Next: "writer"},
				},
			},
		}
		got := systemGraphLines(system)
		want := []string{
			"planner -> researcher",
			"researcher -> writer",
			"writer -> (end)",
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("unexpected graph lines: got=%v want=%v", got, want)
		}
	})

	t.Run("fan-out graph edges", func(t *testing.T) {
		system := crds.AgentSystem{
			Spec: crds.AgentSystemSpec{
				Agents: []string{"planner", "researcher", "reviewer", "writer"},
				Graph: map[string]crds.GraphEdge{
					"planner": {
						Edges: []crds.GraphRoute{
							{To: "researcher"},
							{To: "reviewer"},
						},
					},
					"researcher": {Edges: []crds.GraphRoute{{To: "writer"}}},
					"reviewer":   {Edges: []crds.GraphRoute{{To: "writer"}}},
				},
			},
		}
		got := systemGraphLines(system)
		want := []string{
			"planner -> researcher",
			"planner -> reviewer",
			"researcher -> writer",
			"reviewer -> writer",
			"writer -> (end)",
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("unexpected graph lines: got=%v want=%v", got, want)
		}
	})
}

func TestTaskExecutionOrder(t *testing.T) {
	t.Run("from execution_order output", func(t *testing.T) {
		task := crds.Task{
			Status: crds.TaskStatus{
				Output: map[string]string{
					"execution_order": "planner -> researcher -> writer",
				},
			},
		}
		got := taskExecutionOrder(task, nil)
		want := []string{"planner", "researcher", "writer"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("unexpected order: got=%v want=%v", got, want)
		}
	})

	t.Run("from indexed output", func(t *testing.T) {
		task := crds.Task{
			Status: crds.TaskStatus{
				Output: map[string]string{
					"agent.2.name": "researcher",
					"agent.1.name": "planner",
				},
			},
		}
		got := taskExecutionOrder(task, nil)
		want := []string{"planner", "researcher"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("unexpected order: got=%v want=%v", got, want)
		}
	})

	t.Run("falls back to system when output is absent", func(t *testing.T) {
		task := crds.Task{}
		system := &crds.AgentSystem{
			Spec: crds.AgentSystemSpec{
				Agents: []string{"planner", "researcher", "writer"},
			},
		}
		got := taskExecutionOrder(task, system)
		want := []string{"planner", "researcher", "writer"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("unexpected order: got=%v want=%v", got, want)
		}
	})
}

func TestTaskGraphMetrics(t *testing.T) {
	task := crds.Task{
		Status: crds.TaskStatus{
			Trace: []crds.TaskTraceEvent{
				{Type: "agent_start", Agent: "planner"},
				{
					Type:         "agent_end",
					Agent:        "planner",
					LatencyMS:    150,
					Tokens:       300,
					ToolCalls:    1,
					MemoryWrites: 2,
					Message:      "step=3 completed",
				},
				{Type: "agent_start", Agent: "researcher"},
				{Type: "agent_error", Agent: "researcher", Message: "timeout"},
			},
			Output: map[string]string{
				"agent.2.duration_ms":      "200",
				"agent.2.estimated_tokens": "500",
				"agent.2.tool_calls":       "2",
				"agent.2.memory_writes":    "1",
			},
		},
	}
	order := []string{"planner", "researcher"}

	got := taskGraphMetrics(task, order)

	if got["planner"].Status != "succeeded" || got["planner"].LatencyMS != 150 || got["planner"].Tokens != 300 {
		t.Fatalf("unexpected planner metrics: %+v", got["planner"])
	}
	if got["researcher"].Status != "failed" {
		t.Fatalf("expected researcher status failed, got %+v", got["researcher"])
	}
	if got["researcher"].LatencyMS != 200 || got["researcher"].Tokens != 500 || got["researcher"].ToolCalls != 2 || got["researcher"].MemoryWrites != 1 {
		t.Fatalf("expected output fallback metrics for researcher, got %+v", got["researcher"])
	}
}
