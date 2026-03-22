package agentruntime

import (
	"strings"

	"github.com/OrlojHQ/orloj/resources"
)

// GovernedToolApprovalContext optionally lets the governed tool runtime treat an
// existing Approved ToolApproval (same key as pauseTaskForToolApproval) as a
// grant to invoke the tool, so resuming a message after approval does not loop
// on approval_required for every fresh agent worker run.
type GovernedToolApprovalContext struct {
	Getter    func(key string) (resources.ToolApproval, bool, error)
	TaskKey   string
	MessageID string
}

type approvedToolGrantAuthorizer struct {
	inner     ToolCallAuthorizer
	getter    func(key string) (resources.ToolApproval, bool, error)
	taskKey   string
	messageID string
}

// NewAuthorizerWithApprovedToolGrant wraps inner (typically AgentToolAuthorizer).
// When inner requires approval, it allows the call if the ToolApproval row for
// (taskKey, messageID) is Approved and its spec.tool matches the requested tool.
func NewAuthorizerWithApprovedToolGrant(inner ToolCallAuthorizer, getter func(key string) (resources.ToolApproval, bool, error), taskKey, messageID string) ToolCallAuthorizer {
	if inner == nil || getter == nil {
		return inner
	}
	taskKey = strings.TrimSpace(taskKey)
	messageID = strings.TrimSpace(messageID)
	if taskKey == "" || messageID == "" {
		return inner
	}
	return &approvedToolGrantAuthorizer{
		inner:     inner,
		getter:    getter,
		taskKey:   taskKey,
		messageID: messageID,
	}
}

func (a *approvedToolGrantAuthorizer) Authorize(tool string, spec resources.ToolSpec) (*AuthorizeResult, error) {
	out, err := a.inner.Authorize(tool, spec)
	if err != nil || out == nil || out.Verdict != AuthorizeVerdictApprovalRequired {
		return out, err
	}
	key := ToolApprovalScopedStoreKey(a.taskKey, a.messageID)
	row, ok, getErr := a.getter(key)
	if getErr != nil {
		return out, getErr
	}
	if !ok {
		return out, err
	}
	if !strings.EqualFold(strings.TrimSpace(row.Status.Phase), "approved") {
		return out, err
	}
	if !strings.EqualFold(normalizeToolKey(row.Spec.Tool), normalizeToolKey(tool)) {
		return out, err
	}
	return &AuthorizeResult{Verdict: AuthorizeVerdictAllow}, nil
}
