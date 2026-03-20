package agentruntime

import (
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

func TestToolApprovalScopedStoreKeyStable(t *testing.T) {
	k := ToolApprovalScopedStoreKey("rr-real-tool-approval/rr-real-approval-task", "msg-1")
	if k == "" || k[0] == '/' {
		t.Fatalf("unexpected key %q", k)
	}
	if k2 := ToolApprovalScopedStoreKey("rr-real-tool-approval/rr-real-approval-task", "msg-1"); k2 != k {
		t.Fatalf("key not stable: %q vs %q", k, k2)
	}
}

func TestAuthorizerApprovedToolGrantAllowsWhenStoreApproved(t *testing.T) {
	inner := fixedVerdictAuthorizer{verdict: AuthorizeVerdictApprovalRequired}
	taskKey := "default/my-task"
	msgID := "inbox-1"
	key := ToolApprovalScopedStoreKey(taskKey, msgID)
	store := map[string]resources.ToolApproval{
		key: {
			Spec: resources.ToolApprovalSpec{Tool: "deploy"},
			Status: resources.ToolApprovalStatus{
				Phase: "Approved",
			},
		},
	}
	getter := func(k string) (resources.ToolApproval, bool) {
		a, ok := store[k]
		return a, ok
	}
	auth := NewAuthorizerWithApprovedToolGrant(inner, getter, taskKey, msgID)
	res, err := auth.Authorize("deploy", resources.ToolSpec{})
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if res == nil || res.Verdict != AuthorizeVerdictAllow {
		t.Fatalf("expected allow, got %+v", res)
	}
}

func TestAuthorizerApprovedToolGrantRespectsPending(t *testing.T) {
	inner := fixedVerdictAuthorizer{verdict: AuthorizeVerdictApprovalRequired}
	taskKey := "default/my-task"
	msgID := "inbox-1"
	key := ToolApprovalScopedStoreKey(taskKey, msgID)
	store := map[string]resources.ToolApproval{
		key: {
			Spec:   resources.ToolApprovalSpec{Tool: "deploy"},
			Status: resources.ToolApprovalStatus{Phase: "Pending"},
		},
	}
	getter := func(k string) (resources.ToolApproval, bool) {
		a, ok := store[k]
		return a, ok
	}
	auth := NewAuthorizerWithApprovedToolGrant(inner, getter, taskKey, msgID)
	res, err := auth.Authorize("deploy", resources.ToolSpec{})
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if res == nil || res.Verdict != AuthorizeVerdictApprovalRequired {
		t.Fatalf("expected approval still required, got %+v", res)
	}
}

type fixedVerdictAuthorizer struct {
	verdict string
}

func (f fixedVerdictAuthorizer) Authorize(_ string, _ resources.ToolSpec) (*AuthorizeResult, error) {
	return &AuthorizeResult{Verdict: f.verdict}, nil
}
