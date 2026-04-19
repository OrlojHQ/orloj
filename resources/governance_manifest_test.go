package resources

import "testing"

func TestParseAgentManifestRolesYAML(t *testing.T) {
	raw := []byte(`apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: researcher
spec:
  model_ref: openai-default
  prompt: test
  roles:
    - analyst
    - analyst
`)
	agent, err := ParseAgentManifest(raw)
	if err != nil {
		t.Fatalf("parse agent failed: %v", err)
	}
	if len(agent.Spec.Roles) != 1 || agent.Spec.Roles[0] != "analyst" {
		t.Fatalf("unexpected roles: %+v", agent.Spec.Roles)
	}
}

func TestParseAgentRoleManifestYAML(t *testing.T) {
	raw := []byte(`apiVersion: orloj.dev/v1
kind: AgentRole
metadata:
  name: analyst
spec:
  permissions:
    - tool:web_search:invoke
    - capability:web.read
`)
	role, err := ParseAgentRoleManifest(raw)
	if err != nil {
		t.Fatalf("parse role failed: %v", err)
	}
	if role.Metadata.Name != "analyst" {
		t.Fatalf("unexpected role name %q", role.Metadata.Name)
	}
	if len(role.Spec.Permissions) != 2 {
		t.Fatalf("unexpected permissions: %+v", role.Spec.Permissions)
	}
}

func TestParseToolPermissionManifestYAML(t *testing.T) {
	raw := []byte(`apiVersion: orloj.dev/v1
kind: ToolPermission
metadata:
  name: web-search
spec:
  tool_ref: web_search
  action: invoke
  match_mode: all
  required_permissions:
    - tool:web_search:invoke
`)
	perm, err := ParseToolPermissionManifest(raw)
	if err != nil {
		t.Fatalf("parse tool permission failed: %v", err)
	}
	if perm.Spec.ToolRef != "web_search" {
		t.Fatalf("unexpected tool_ref %q", perm.Spec.ToolRef)
	}
	if perm.Spec.MatchMode != "all" {
		t.Fatalf("unexpected match_mode %q", perm.Spec.MatchMode)
	}
}

func TestParseAgentSystemManifestReviewCheckpointYAML(t *testing.T) {
	raw := []byte(`apiVersion: orloj.dev/v1
kind: AgentSystem
metadata:
  name: reviewed-system
spec:
  agents:
    - writer-agent
  graph:
    writer-agent:
      review:
        checkpoint_id: writer-review
        reason: Review before publish.
        ttl: 30m
  completion_review:
    checkpoint_id: final-review
    reason: Final signoff.
`)
	system, err := ParseAgentSystemManifest(raw)
	if err != nil {
		t.Fatalf("parse agent system failed: %v", err)
	}
	checkpoint := system.Spec.Graph["writer-agent"].Review
	if checkpoint == nil {
		t.Fatalf("expected writer-agent review checkpoint")
	}
	if checkpoint.CheckpointID != "writer-review" {
		t.Fatalf("unexpected checkpoint id %q", checkpoint.CheckpointID)
	}
	if system.Spec.CompletionReview == nil || system.Spec.CompletionReview.CheckpointID != "final-review" {
		t.Fatalf("unexpected completion review: %+v", system.Spec.CompletionReview)
	}
}

func TestParseTaskApprovalManifestYAML(t *testing.T) {
	raw := []byte(`apiVersion: orloj.dev/v1
kind: TaskApproval
metadata:
  name: review-1
spec:
  task_ref: default/report-task
  checkpoint_id: writer-review
  checkpoint_type: agent_output
  agent: writer-agent
  reason: Editor signoff.
  allow_request_changes: false
  max_review_cycles: 4
  review_cycle: 2
  supersedes: review-0
  output_format: text
  resume_context:
    execution_mode: sequential
`)
	approval, err := ParseTaskApprovalManifest(raw)
	if err != nil {
		t.Fatalf("parse task approval failed: %v", err)
	}
	if approval.Spec.TaskRef != "default/report-task" {
		t.Fatalf("unexpected task_ref %q", approval.Spec.TaskRef)
	}
	if approval.Spec.ReviewCycle != 2 {
		t.Fatalf("unexpected review cycle %d", approval.Spec.ReviewCycle)
	}
	if approval.Spec.AllowRequestChanges == nil || *approval.Spec.AllowRequestChanges {
		t.Fatalf("expected allow_request_changes=false, got %+v", approval.Spec.AllowRequestChanges)
	}
	if approval.Spec.MaxReviewCycles != 4 {
		t.Fatalf("unexpected max review cycles %d", approval.Spec.MaxReviewCycles)
	}
	if approval.Spec.ResumeContext["execution_mode"] != "sequential" {
		t.Fatalf("unexpected resume context: %+v", approval.Spec.ResumeContext)
	}
}
