package resources

import (
	"reflect"
	"strings"
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

func TestParseAgentSystemManifestA2AYAML(t *testing.T) {
	raw := []byte(`
apiVersion: orloj.dev/v1
kind: AgentSystem
metadata:
  name: a2a-system
spec:
  a2a:
    enabled: true
  agents:
    - planner
`)

	system, err := ParseAgentSystemManifest(raw)
	if err != nil {
		t.Fatalf("parse agent system failed: %v", err)
	}
	if !system.Spec.A2A.Enabled {
		t.Fatal("expected spec.a2a.enabled=true")
	}
}

func TestFilterRoutesForOutputNoConditionsPassThrough(t *testing.T) {
	routes := []GraphRoute{
		{To: "a"},
		{To: "b"},
	}
	got := FilterRoutesForOutput(routes, "anything")
	if len(got) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(got))
	}
}

func TestFilterRoutesForOutputContains(t *testing.T) {
	routes := []GraphRoute{
		{To: "risk", Condition: &EdgeCondition{OutputContains: "HIGH_RISK"}},
		{To: "fast", Condition: &EdgeCondition{OutputContains: "LOW_RISK"}},
	}
	got := FilterRoutesForOutput(routes, "The assessment shows HIGH_RISK levels")
	if len(got) != 1 || got[0].To != "risk" {
		t.Fatalf("expected [risk], got %v", routeNames(got))
	}
}

func TestFilterRoutesForOutputContainsCaseInsensitive(t *testing.T) {
	routes := []GraphRoute{
		{To: "a", Condition: &EdgeCondition{OutputContains: "MATCH"}},
	}
	got := FilterRoutesForOutput(routes, "this has match in lowercase")
	if len(got) != 1 {
		t.Fatalf("expected 1 route (case-insensitive), got %d", len(got))
	}
}

func TestFilterRoutesForOutputNotContains(t *testing.T) {
	routes := []GraphRoute{
		{To: "ok", Condition: &EdgeCondition{OutputNotContains: "ERROR"}},
		{To: "err", Condition: &EdgeCondition{OutputContains: "ERROR"}},
	}
	got := FilterRoutesForOutput(routes, "All good, no issues")
	if len(got) != 1 || got[0].To != "ok" {
		t.Fatalf("expected [ok], got %v", routeNames(got))
	}
}

func TestFilterRoutesForOutputRegexMatch(t *testing.T) {
	routes := []GraphRoute{
		{To: "num", Condition: &EdgeCondition{OutputMatches: `score:\s*\d+`}},
		{To: "text", Condition: &EdgeCondition{OutputNotContains: "score"}},
	}
	got := FilterRoutesForOutput(routes, "result score: 42 points")
	if len(got) != 1 || got[0].To != "num" {
		t.Fatalf("expected [num], got %v", routeNames(got))
	}
}

func TestFilterRoutesForOutputDefaultFallback(t *testing.T) {
	routes := []GraphRoute{
		{To: "a", Condition: &EdgeCondition{OutputContains: "ALPHA"}},
		{To: "b", Condition: &EdgeCondition{OutputContains: "BETA"}},
		{To: "fallback", Condition: &EdgeCondition{Default: true}},
	}
	got := FilterRoutesForOutput(routes, "nothing matches here")
	if len(got) != 1 || got[0].To != "fallback" {
		t.Fatalf("expected [fallback], got %v", routeNames(got))
	}
}

func TestFilterRoutesForOutputNoMatchNoDefault(t *testing.T) {
	routes := []GraphRoute{
		{To: "a", Condition: &EdgeCondition{OutputContains: "ALPHA"}},
		{To: "b", Condition: &EdgeCondition{OutputContains: "BETA"}},
	}
	got := FilterRoutesForOutput(routes, "nothing matches")
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", routeNames(got))
	}
}

func TestFilterRoutesForOutputMultipleMatches(t *testing.T) {
	routes := []GraphRoute{
		{To: "research", Condition: &EdgeCondition{OutputContains: "NEEDS_RESEARCH"}},
		{To: "legal", Condition: &EdgeCondition{OutputContains: "NEEDS_LEGAL"}},
		{To: "engineering", Condition: &EdgeCondition{OutputContains: "NEEDS_ENGINEERING"}},
	}
	got := FilterRoutesForOutput(routes, "NEEDS_RESEARCH and NEEDS_LEGAL review")
	if len(got) != 2 {
		t.Fatalf("expected 2 routes, got %d: %v", len(got), routeNames(got))
	}
	names := routeNames(got)
	if names[0] != "research" || names[1] != "legal" {
		t.Fatalf("expected [research, legal], got %v", names)
	}
}

func TestFilterRoutesForOutputUnconditionalWithConditional(t *testing.T) {
	routes := []GraphRoute{
		{To: "conditional", Condition: &EdgeCondition{OutputContains: "TRIGGER"}},
		{To: "always"},
	}
	got := FilterRoutesForOutput(routes, "has TRIGGER")
	if len(got) != 2 {
		t.Fatalf("expected 2 routes (conditional + unconditional), got %d: %v", len(got), routeNames(got))
	}

	got2 := FilterRoutesForOutput(routes, "no match")
	if len(got2) != 1 || got2[0].To != "always" {
		t.Fatalf("expected [always] when no condition matches, got %v", routeNames(got2))
	}
}

func TestFilterRoutesForOutputDefaultNotUsedWhenConditionMatches(t *testing.T) {
	routes := []GraphRoute{
		{To: "matched", Condition: &EdgeCondition{OutputContains: "HIT"}},
		{To: "fallback", Condition: &EdgeCondition{Default: true}},
	}
	got := FilterRoutesForOutput(routes, "HIT")
	if len(got) != 1 || got[0].To != "matched" {
		t.Fatalf("expected [matched] (default not used), got %v", routeNames(got))
	}
}

func TestFilterRoutesForOutputEmptyOutput(t *testing.T) {
	routes := []GraphRoute{
		{To: "a", Condition: &EdgeCondition{OutputContains: "X"}},
		{To: "b", Condition: &EdgeCondition{OutputNotContains: "X"}},
	}
	got := FilterRoutesForOutput(routes, "")
	if len(got) != 1 || got[0].To != "b" {
		t.Fatalf("expected [b] for empty output with not_contains, got %v", routeNames(got))
	}
}

func TestParseAgentSystemConditionYAML(t *testing.T) {
	raw := []byte(`
apiVersion: orloj.dev/v1
kind: AgentSystem
metadata:
  name: triage-system
spec:
  agents:
    - classifier
    - billing
    - tech
    - fallback
  graph:
    classifier:
      edges:
        - to: billing
          condition:
            output_contains: "BILLING"
        - to: tech
          condition:
            output_matches: "TECH|ENGINEERING"
        - to: fallback
          condition:
            default: true
`)
	system, err := ParseAgentSystemManifest(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	node := system.Spec.Graph["classifier"]
	if len(node.Edges) != 3 {
		t.Fatalf("expected 3 edges, got %d", len(node.Edges))
	}
	if node.Edges[0].Condition == nil || node.Edges[0].Condition.OutputContains != "BILLING" {
		t.Fatalf("expected billing edge with output_contains=BILLING, got %+v", node.Edges[0].Condition)
	}
	if node.Edges[1].Condition == nil || node.Edges[1].Condition.OutputMatches != "TECH|ENGINEERING" {
		t.Fatalf("expected tech edge with output_matches, got %+v", node.Edges[1].Condition)
	}
	if node.Edges[2].Condition == nil || !node.Edges[2].Condition.Default {
		t.Fatalf("expected fallback edge with default=true, got %+v", node.Edges[2].Condition)
	}
}

func TestParseAgentSystemConditionNotContainsYAML(t *testing.T) {
	raw := []byte(`
apiVersion: orloj.dev/v1
kind: AgentSystem
metadata:
  name: gate-system
spec:
  agents:
    - checker
    - proceed
    - reject
  graph:
    checker:
      edges:
        - to: proceed
          condition:
            output_not_contains: "ERROR"
        - to: reject
          condition:
            output_contains: "ERROR"
`)
	system, err := ParseAgentSystemManifest(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	node := system.Spec.Graph["checker"]
	if node.Edges[0].Condition == nil || node.Edges[0].Condition.OutputNotContains != "ERROR" {
		t.Fatalf("expected output_not_contains=ERROR, got %+v", node.Edges[0].Condition)
	}
}

func TestAgentSystemNormalizeRejectsInvalidRegex(t *testing.T) {
	system := AgentSystem{
		Kind:     "AgentSystem",
		Metadata: ObjectMeta{Name: "bad-regex"},
		Spec: AgentSystemSpec{
			Graph: map[string]GraphEdge{
				"a": {Edges: []GraphRoute{
					{To: "b", Condition: &EdgeCondition{OutputMatches: "[invalid"}},
				}},
			},
		},
	}
	err := system.Normalize()
	if err == nil {
		t.Fatal("expected error for invalid regex")
	}
	if !strings.Contains(err.Error(), "invalid regex") {
		t.Fatalf("expected 'invalid regex' in error, got: %v", err)
	}
}

func TestAgentSystemNormalizeRejectsDefaultWithOtherConditions(t *testing.T) {
	system := AgentSystem{
		Kind:     "AgentSystem",
		Metadata: ObjectMeta{Name: "bad-default"},
		Spec: AgentSystemSpec{
			Graph: map[string]GraphEdge{
				"a": {Edges: []GraphRoute{
					{To: "b", Condition: &EdgeCondition{Default: true, OutputContains: "X"}},
				}},
			},
		},
	}
	err := system.Normalize()
	if err == nil {
		t.Fatal("expected error for default with other conditions")
	}
	if !strings.Contains(err.Error(), "default edge must not have other condition fields") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAgentSystemNormalizeRejectsMultipleDefaults(t *testing.T) {
	system := AgentSystem{
		Kind:     "AgentSystem",
		Metadata: ObjectMeta{Name: "multi-default"},
		Spec: AgentSystemSpec{
			Graph: map[string]GraphEdge{
				"a": {Edges: []GraphRoute{
					{To: "b", Condition: &EdgeCondition{Default: true}},
					{To: "c", Condition: &EdgeCondition{Default: true}},
				}},
			},
		},
	}
	err := system.Normalize()
	if err == nil {
		t.Fatal("expected error for multiple defaults")
	}
	if !strings.Contains(err.Error(), "at most one default edge") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAgentSystemConditionJSON(t *testing.T) {
	raw := []byte(`{
		"apiVersion": "orloj.dev/v1",
		"kind": "AgentSystem",
		"metadata": {"name": "json-cond"},
		"spec": {
			"agents": ["a", "b", "c"],
			"graph": {
				"a": {
					"edges": [
						{"to": "b", "condition": {"output_contains": "MATCH"}},
						{"to": "c", "condition": {"default": true}}
					]
				}
			}
		}
	}`)
	system, err := ParseAgentSystemManifest(raw)
	if err != nil {
		t.Fatalf("JSON parse failed: %v", err)
	}
	node := system.Spec.Graph["a"]
	if len(node.Edges) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(node.Edges))
	}
	if node.Edges[0].Condition == nil || node.Edges[0].Condition.OutputContains != "MATCH" {
		t.Fatalf("expected output_contains=MATCH, got %+v", node.Edges[0].Condition)
	}
	if node.Edges[1].Condition == nil || !node.Edges[1].Condition.Default {
		t.Fatalf("expected default=true, got %+v", node.Edges[1].Condition)
	}
}

// --- Phase 2: JSON path condition tests ---

func TestFilterRoutesJSONPathEquals(t *testing.T) {
	routes := []GraphRoute{
		{To: "research", Condition: &EdgeCondition{OutputJSONPath: "$.route", Equals: "research"}},
		{To: "legal", Condition: &EdgeCondition{OutputJSONPath: "$.route", Equals: "legal"}},
	}
	got := FilterRoutesForOutput(routes, `{"route": "research", "confidence": 0.95}`)
	if len(got) != 1 || got[0].To != "research" {
		t.Fatalf("expected [research], got %v", routeNames(got))
	}
}

func TestFilterRoutesJSONPathNotEquals(t *testing.T) {
	routes := []GraphRoute{
		{To: "skip", Condition: &EdgeCondition{OutputJSONPath: "$.status", NotEquals: "done"}},
	}
	got := FilterRoutesForOutput(routes, `{"status": "pending"}`)
	if len(got) != 1 || got[0].To != "skip" {
		t.Fatalf("expected [skip], got %v", routeNames(got))
	}
	got2 := FilterRoutesForOutput(routes, `{"status": "done"}`)
	if len(got2) != 0 {
		t.Fatalf("expected [] when status=done, got %v", routeNames(got2))
	}
}

func TestFilterRoutesJSONPathContainsArray(t *testing.T) {
	routes := []GraphRoute{
		{To: "legal-lead", Condition: &EdgeCondition{OutputJSONPath: "$.domains", Contains: "legal"}},
	}
	got := FilterRoutesForOutput(routes, `{"domains": ["engineering", "legal", "finance"]}`)
	if len(got) != 1 || got[0].To != "legal-lead" {
		t.Fatalf("expected [legal-lead], got %v", routeNames(got))
	}
}

func TestFilterRoutesJSONPathContainsString(t *testing.T) {
	routes := []GraphRoute{
		{To: "match", Condition: &EdgeCondition{OutputJSONPath: "$.summary", Contains: "urgent"}},
	}
	got := FilterRoutesForOutput(routes, `{"summary": "This is an urgent matter"}`)
	if len(got) != 1 || got[0].To != "match" {
		t.Fatalf("expected [match], got %v", routeNames(got))
	}
}

func TestFilterRoutesJSONPathContainsArrayNoMatch(t *testing.T) {
	routes := []GraphRoute{
		{To: "legal-lead", Condition: &EdgeCondition{OutputJSONPath: "$.domains", Contains: "legal"}},
	}
	got := FilterRoutesForOutput(routes, `{"domains": ["engineering", "finance"]}`)
	if len(got) != 0 {
		t.Fatalf("expected [] when legal not in domains, got %v", routeNames(got))
	}
}

func TestFilterRoutesJSONPathGreaterThan(t *testing.T) {
	routes := []GraphRoute{
		{To: "high", Condition: &EdgeCondition{OutputJSONPath: "$.confidence", GreaterThan: "0.9"}},
		{To: "low", Condition: &EdgeCondition{OutputJSONPath: "$.confidence", LessThan: "0.5"}},
		{To: "fallback", Condition: &EdgeCondition{Default: true}},
	}
	got := FilterRoutesForOutput(routes, `{"confidence": 0.95}`)
	if len(got) != 1 || got[0].To != "high" {
		t.Fatalf("expected [high], got %v", routeNames(got))
	}
}

func TestFilterRoutesJSONPathLessThan(t *testing.T) {
	routes := []GraphRoute{
		{To: "high", Condition: &EdgeCondition{OutputJSONPath: "$.confidence", GreaterThan: "0.9"}},
		{To: "low", Condition: &EdgeCondition{OutputJSONPath: "$.confidence", LessThan: "0.5"}},
		{To: "fallback", Condition: &EdgeCondition{Default: true}},
	}
	got := FilterRoutesForOutput(routes, `{"confidence": 0.3}`)
	if len(got) != 1 || got[0].To != "low" {
		t.Fatalf("expected [low], got %v", routeNames(got))
	}
}

func TestFilterRoutesJSONPathFallback(t *testing.T) {
	routes := []GraphRoute{
		{To: "high", Condition: &EdgeCondition{OutputJSONPath: "$.confidence", GreaterThan: "0.9"}},
		{To: "low", Condition: &EdgeCondition{OutputJSONPath: "$.confidence", LessThan: "0.5"}},
		{To: "fallback", Condition: &EdgeCondition{Default: true}},
	}
	got := FilterRoutesForOutput(routes, `{"confidence": 0.7}`)
	if len(got) != 1 || got[0].To != "fallback" {
		t.Fatalf("expected [fallback] for mid-range confidence, got %v", routeNames(got))
	}
}

func TestFilterRoutesJSONPathNestedField(t *testing.T) {
	routes := []GraphRoute{
		{To: "match", Condition: &EdgeCondition{OutputJSONPath: "$.result.category", Equals: "billing"}},
	}
	got := FilterRoutesForOutput(routes, `{"result": {"category": "billing", "score": 0.8}}`)
	if len(got) != 1 || got[0].To != "match" {
		t.Fatalf("expected [match], got %v", routeNames(got))
	}
}

func TestFilterRoutesJSONPathInvalidJSON(t *testing.T) {
	routes := []GraphRoute{
		{To: "a", Condition: &EdgeCondition{OutputJSONPath: "$.route", Equals: "research"}},
		{To: "fallback", Condition: &EdgeCondition{Default: true}},
	}
	got := FilterRoutesForOutput(routes, "not json at all")
	if len(got) != 1 || got[0].To != "fallback" {
		t.Fatalf("expected [fallback] for invalid JSON, got %v", routeNames(got))
	}
}

func TestFilterRoutesJSONPathMissingKey(t *testing.T) {
	routes := []GraphRoute{
		{To: "a", Condition: &EdgeCondition{OutputJSONPath: "$.nonexistent", Equals: "x"}},
	}
	got := FilterRoutesForOutput(routes, `{"route": "research"}`)
	if len(got) != 0 {
		t.Fatalf("expected [] for missing key, got %v", routeNames(got))
	}
}

func TestFilterRoutesJSONPathNumericEquals(t *testing.T) {
	routes := []GraphRoute{
		{To: "exact", Condition: &EdgeCondition{OutputJSONPath: "$.count", Equals: "42"}},
	}
	got := FilterRoutesForOutput(routes, `{"count": 42}`)
	if len(got) != 1 || got[0].To != "exact" {
		t.Fatalf("expected [exact], got %v", routeNames(got))
	}
}

func TestFilterRoutesJSONPathBoolEquals(t *testing.T) {
	routes := []GraphRoute{
		{To: "approved", Condition: &EdgeCondition{OutputJSONPath: "$.approved", Equals: "true"}},
	}
	got := FilterRoutesForOutput(routes, `{"approved": true}`)
	if len(got) != 1 || got[0].To != "approved" {
		t.Fatalf("expected [approved], got %v", routeNames(got))
	}
}

func TestFilterRoutesJSONPathWithStringConditions(t *testing.T) {
	routes := []GraphRoute{
		{To: "json-route", Condition: &EdgeCondition{OutputJSONPath: "$.route", Equals: "alpha"}},
		{To: "string-route", Condition: &EdgeCondition{OutputContains: "BETA"}},
	}
	got := FilterRoutesForOutput(routes, `{"route": "alpha", "note": "BETA"}`)
	if len(got) != 2 {
		t.Fatalf("expected both routes to match, got %v", routeNames(got))
	}
}

func TestAgentSystemNormalizeJSONPathRequiresOperator(t *testing.T) {
	system := AgentSystem{
		Kind:     "AgentSystem",
		Metadata: ObjectMeta{Name: "bad-jsonpath"},
		Spec: AgentSystemSpec{
			Graph: map[string]GraphEdge{
				"a": {Edges: []GraphRoute{
					{To: "b", Condition: &EdgeCondition{OutputJSONPath: "$.route"}},
				}},
			},
		},
	}
	err := system.Normalize()
	if err == nil {
		t.Fatal("expected error for output_json_path without operator")
	}
	if !strings.Contains(err.Error(), "requires at least one comparison operator") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAgentSystemNormalizeOperatorRequiresJSONPath(t *testing.T) {
	system := AgentSystem{
		Kind:     "AgentSystem",
		Metadata: ObjectMeta{Name: "bad-op"},
		Spec: AgentSystemSpec{
			Graph: map[string]GraphEdge{
				"a": {Edges: []GraphRoute{
					{To: "b", Condition: &EdgeCondition{Equals: "foo"}},
				}},
			},
		},
	}
	err := system.Normalize()
	if err == nil {
		t.Fatal("expected error for operator without output_json_path")
	}
	if !strings.Contains(err.Error(), "require output_json_path") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAgentSystemNormalizeJSONPathBadPrefix(t *testing.T) {
	system := AgentSystem{
		Kind:     "AgentSystem",
		Metadata: ObjectMeta{Name: "bad-prefix"},
		Spec: AgentSystemSpec{
			Graph: map[string]GraphEdge{
				"a": {Edges: []GraphRoute{
					{To: "b", Condition: &EdgeCondition{OutputJSONPath: "route", Equals: "x"}},
				}},
			},
		},
	}
	err := system.Normalize()
	if err == nil {
		t.Fatal("expected error for output_json_path without $. prefix")
	}
	if !strings.Contains(err.Error(), "must start with") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAgentSystemNormalizeInvalidGreaterThan(t *testing.T) {
	system := AgentSystem{
		Kind:     "AgentSystem",
		Metadata: ObjectMeta{Name: "bad-gt"},
		Spec: AgentSystemSpec{
			Graph: map[string]GraphEdge{
				"a": {Edges: []GraphRoute{
					{To: "b", Condition: &EdgeCondition{OutputJSONPath: "$.score", GreaterThan: "not-a-number"}},
				}},
			},
		},
	}
	err := system.Normalize()
	if err == nil {
		t.Fatal("expected error for non-numeric greater_than")
	}
	if !strings.Contains(err.Error(), "must be a valid number") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAgentSystemNormalizeInvalidLessThan(t *testing.T) {
	system := AgentSystem{
		Kind:     "AgentSystem",
		Metadata: ObjectMeta{Name: "bad-lt"},
		Spec: AgentSystemSpec{
			Graph: map[string]GraphEdge{
				"a": {Edges: []GraphRoute{
					{To: "b", Condition: &EdgeCondition{OutputJSONPath: "$.score", LessThan: "abc"}},
				}},
			},
		},
	}
	err := system.Normalize()
	if err == nil {
		t.Fatal("expected error for non-numeric less_than")
	}
	if !strings.Contains(err.Error(), "must be a valid number") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAgentSystemNormalizeDefaultWithJSONPath(t *testing.T) {
	system := AgentSystem{
		Kind:     "AgentSystem",
		Metadata: ObjectMeta{Name: "bad-default-json"},
		Spec: AgentSystemSpec{
			Graph: map[string]GraphEdge{
				"a": {Edges: []GraphRoute{
					{To: "b", Condition: &EdgeCondition{Default: true, OutputJSONPath: "$.x", Equals: "y"}},
				}},
			},
		},
	}
	err := system.Normalize()
	if err == nil {
		t.Fatal("expected error for default with JSON path conditions")
	}
	if !strings.Contains(err.Error(), "default edge must not have other condition fields") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseAgentSystemJSONPathYAML(t *testing.T) {
	raw := []byte(`
apiVersion: orloj.dev/v1
kind: AgentSystem
metadata:
  name: json-routing
spec:
  agents:
    - classifier
    - research
    - legal
    - high-priority
  graph:
    classifier:
      edges:
        - to: research
          condition:
            output_json_path: "$.route"
            equals: "research"
        - to: legal
          condition:
            output_json_path: "$.domains"
            contains: "legal"
        - to: high-priority
          condition:
            output_json_path: "$.confidence"
            greater_than: "0.9"
`)
	system, err := ParseAgentSystemManifest(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	node := system.Spec.Graph["classifier"]
	if len(node.Edges) != 3 {
		t.Fatalf("expected 3 edges, got %d", len(node.Edges))
	}
	if node.Edges[0].Condition.OutputJSONPath != "$.route" || node.Edges[0].Condition.Equals != "research" {
		t.Fatalf("edge 0: expected json_path=$.route equals=research, got %+v", node.Edges[0].Condition)
	}
	if node.Edges[1].Condition.OutputJSONPath != "$.domains" || node.Edges[1].Condition.Contains != "legal" {
		t.Fatalf("edge 1: expected json_path=$.domains contains=legal, got %+v", node.Edges[1].Condition)
	}
	if node.Edges[2].Condition.OutputJSONPath != "$.confidence" || node.Edges[2].Condition.GreaterThan != "0.9" {
		t.Fatalf("edge 2: expected json_path=$.confidence greater_than=0.9, got %+v", node.Edges[2].Condition)
	}
}

func TestExtractJSONPath(t *testing.T) {
	tests := []struct {
		name   string
		json   string
		path   string
		expect string
		err    bool
	}{
		{"simple string", `{"route": "research"}`, "$.route", "research", false},
		{"nested", `{"a": {"b": "deep"}}`, "$.a.b", "deep", false},
		{"number", `{"score": 0.95}`, "$.score", "0.95", false},
		{"integer", `{"count": 42}`, "$.count", "42", false},
		{"bool", `{"ok": true}`, "$.ok", "true", false},
		{"missing key", `{"a": 1}`, "$.b", "", true},
		{"invalid json", `not json`, "$.a", "", true},
		{"root", `{"a": 1}`, "$", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			val, err := extractJSONPath(tc.json, tc.path)
			if tc.err {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.path == "$" {
				if val == nil {
					t.Fatal("expected non-nil root")
				}
				return
			}
			got := jsonValueToString(val)
			if got != tc.expect {
				t.Fatalf("expected %q, got %q", tc.expect, got)
			}
		})
	}
}

func TestParseDelegatesYAML(t *testing.T) {
	raw := []byte(`
apiVersion: orloj.dev/v1
kind: AgentSystem
metadata:
  name: delegation-test
spec:
  agents:
    - manager
    - worker-a
    - worker-b
    - synthesizer
  graph:
    manager:
      delegates:
        - to: worker-a
          condition:
            output_contains: "backend"
        - to: worker-b
          condition:
            output_contains: "frontend"
      delegate_join:
        mode: wait_for_all
      edges:
        - to: synthesizer
`)

	system, err := ParseAgentSystemManifest(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	mgr, ok := system.Spec.Graph["manager"]
	if !ok {
		t.Fatal("expected manager graph node")
	}
	if len(mgr.Delegates) != 2 {
		t.Fatalf("expected 2 delegates, got %d", len(mgr.Delegates))
	}
	if mgr.Delegates[0].To != "worker-a" {
		t.Fatalf("expected first delegate to worker-a, got %q", mgr.Delegates[0].To)
	}
	if mgr.Delegates[0].Condition == nil || mgr.Delegates[0].Condition.OutputContains != "backend" {
		t.Fatal("expected first delegate condition output_contains=backend")
	}
	if mgr.Delegates[1].To != "worker-b" {
		t.Fatalf("expected second delegate to worker-b, got %q", mgr.Delegates[1].To)
	}
	if mgr.DelegateJoin.Mode != "wait_for_all" {
		t.Fatalf("expected delegate_join mode wait_for_all, got %q", mgr.DelegateJoin.Mode)
	}
	if len(mgr.Edges) != 1 || mgr.Edges[0].To != "synthesizer" {
		t.Fatalf("expected 1 edge to synthesizer, got %v", mgr.Edges)
	}
}

func TestParseDelegatesJSON(t *testing.T) {
	raw := []byte(`{
		"apiVersion": "orloj.dev/v1",
		"kind": "AgentSystem",
		"metadata": {"name": "json-delegation"},
		"spec": {
			"agents": ["ceo", "vp-eng", "vp-product", "board"],
			"graph": {
				"ceo": {
					"delegates": [
						{"to": "vp-eng", "condition": {"output_json_path": "$.departments", "contains": "engineering"}},
						{"to": "vp-product"}
					],
					"delegate_join": {"mode": "quorum", "quorum_count": 1},
					"edges": [{"to": "board"}]
				}
			}
		}
	}`)
	system, err := ParseAgentSystemManifest(raw)
	if err != nil {
		t.Fatalf("JSON parse failed: %v", err)
	}
	ceo := system.Spec.Graph["ceo"]
	if len(ceo.Delegates) != 2 {
		t.Fatalf("expected 2 delegates, got %d", len(ceo.Delegates))
	}
	if ceo.DelegateJoin.Mode != "quorum" || ceo.DelegateJoin.QuorumCount != 1 {
		t.Fatalf("expected quorum mode with count=1, got mode=%q count=%d", ceo.DelegateJoin.Mode, ceo.DelegateJoin.QuorumCount)
	}
}

func TestDelegateSelfReferenceRejected(t *testing.T) {
	raw := []byte(`{
		"apiVersion": "orloj.dev/v1",
		"kind": "AgentSystem",
		"metadata": {"name": "self-ref"},
		"spec": {
			"agents": ["a"],
			"graph": {
				"a": {
					"delegates": [{"to": "a"}]
				}
			}
		}
	}`)
	_, err := ParseAgentSystemManifest(raw)
	if err == nil {
		t.Fatal("expected error for self-delegation")
	}
	if !strings.Contains(err.Error(), "cannot delegate to itself") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDelegateConditionValidation(t *testing.T) {
	raw := []byte(`{
		"apiVersion": "orloj.dev/v1",
		"kind": "AgentSystem",
		"metadata": {"name": "bad-cond"},
		"spec": {
			"agents": ["a", "b"],
			"graph": {
				"a": {
					"delegates": [{
						"to": "b",
						"condition": {"output_json_path": "$.field"}
					}]
				}
			}
		}
	}`)
	_, err := ParseAgentSystemManifest(raw)
	if err == nil {
		t.Fatal("expected error for JSON path without comparison operator")
	}
	if !strings.Contains(err.Error(), "requires at least one comparison operator") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFilterDelegatesForOutput(t *testing.T) {
	delegates := []GraphRoute{
		{To: "worker-a", Condition: &EdgeCondition{OutputContains: "backend"}},
		{To: "worker-b", Condition: &EdgeCondition{OutputContains: "frontend"}},
		{To: "worker-c"},
	}

	got := FilterRoutesForOutput(delegates, "work on backend services")
	names := routeNames(got)
	if len(names) != 2 || names[0] != "worker-a" || names[1] != "worker-c" {
		t.Fatalf("expected [worker-a, worker-c], got %v", names)
	}

	got = FilterRoutesForOutput(delegates, "work on frontend")
	names = routeNames(got)
	if len(names) != 2 || names[0] != "worker-b" || names[1] != "worker-c" {
		t.Fatalf("expected [worker-b, worker-c], got %v", names)
	}

	got = FilterRoutesForOutput(delegates, "both backend and frontend")
	names = routeNames(got)
	if len(names) != 3 {
		t.Fatalf("expected all 3 delegates, got %v", names)
	}
}

func TestDelegateJoinQuorumYAML(t *testing.T) {
	raw := []byte(`
apiVersion: orloj.dev/v1
kind: AgentSystem
metadata:
  name: quorum-delegation
spec:
  agents:
    - manager
    - w1
    - w2
    - w3
  graph:
    manager:
      delegates:
        - to: w1
        - to: w2
        - to: w3
      delegate_join:
        mode: quorum
        quorum_count: 2
`)
	system, err := ParseAgentSystemManifest(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	mgr := system.Spec.Graph["manager"]
	if len(mgr.Delegates) != 3 {
		t.Fatalf("expected 3 delegates, got %d", len(mgr.Delegates))
	}
	join, _ := NormalizeGraphJoin(mgr.DelegateJoin)
	if join.Mode != "quorum" {
		t.Fatalf("expected quorum mode, got %q", join.Mode)
	}
	if join.QuorumCount != 2 {
		t.Fatalf("expected quorum_count=2, got %d", join.QuorumCount)
	}
}

func routeNames(routes []GraphRoute) []string {
	names := make([]string, len(routes))
	for i, r := range routes {
		names[i] = r.To
	}
	return names
}

func TestNormalizeGraphJoin_InvalidMode(t *testing.T) {
	_, err := NormalizeGraphJoin(GraphJoin{Mode: "quorom"})
	if err == nil {
		t.Fatal("expected error for invalid mode 'quorom'")
	}
}

func TestNormalizeGraphJoin_InvalidOnFailure(t *testing.T) {
	_, err := NormalizeGraphJoin(GraphJoin{Mode: "wait_for_all", OnFailure: "bogus"})
	if err == nil {
		t.Fatal("expected error for invalid on_failure 'bogus'")
	}
}

func TestNormalizeGraphJoin_ValidModes(t *testing.T) {
	join, err := NormalizeGraphJoin(GraphJoin{Mode: "quorum", OnFailure: "skip"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if join.Mode != "quorum" {
		t.Fatalf("expected quorum, got %q", join.Mode)
	}
	if join.OnFailure != "skip" {
		t.Fatalf("expected skip, got %q", join.OnFailure)
	}
}

func TestUnwrapFencedCodeBlock(t *testing.T) {
	t.Parallel()

	if got := UnwrapFencedCodeBlock("plain text"); got != "plain text" {
		t.Fatalf("plain text should pass through, got %q", got)
	}
	if got := UnwrapFencedCodeBlock("```json\n{\"a\":1}\n```"); got != `{"a":1}` {
		t.Fatalf("should unwrap fenced JSON, got %q", got)
	}
	if got := UnwrapFencedCodeBlock("```\nhello\n```"); got != "hello" {
		t.Fatalf("should unwrap bare fences, got %q", got)
	}
	if got := UnwrapFencedCodeBlock("```json\n```"); got != "```json\n```" {
		t.Fatalf("two-line fence should pass through, got %q", got)
	}
	if got := UnwrapFencedCodeBlock(""); got != "" {
		t.Fatalf("empty should pass through, got %q", got)
	}
}

func TestFilterRoutesJSONPathFencedOutput(t *testing.T) {
	t.Parallel()

	routes := []GraphRoute{
		{To: "approve", Condition: &EdgeCondition{OutputJSONPath: "$.decision", Equals: "approve"}},
		{To: "decline", Condition: &EdgeCondition{OutputJSONPath: "$.decision", Equals: "decline"}},
	}
	got := FilterRoutesForOutput(routes, "```json\n{\"decision\":\"approve\"}\n```")
	if len(got) != 1 || got[0].To != "approve" {
		t.Fatalf("expected [approve] for fenced JSON, got %v", routeNames(got))
	}
}

func TestEdgeConditionRegexLengthLimit(t *testing.T) {
	longPattern := make([]byte, 600)
	for i := range longPattern {
		longPattern[i] = 'a'
	}
	routes := []GraphRoute{{
		To: "next",
		Condition: &EdgeCondition{
			OutputMatches: string(longPattern),
		},
	}}
	_, err := normalizeGraphRoutes(routes, "test", "edges")
	if err == nil {
		t.Fatal("expected error for regex exceeding 512 chars")
	}
}
