package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/OrlojHQ/orloj/resources"
	agentruntime "github.com/OrlojHQ/orloj/runtime"
)

// ToolRuntime executes outbound A2A tool calls against remote agents.
type ToolRuntime struct {
	client       *Client
	registry     agentruntime.ToolCapabilityRegistry
	secrets      agentruntime.SecretResolver
	authInjector *agentruntime.AuthInjector
	namespace    string
}

// NewToolRuntime creates an A2A tool runtime. If secrets is non-nil, auth
// injection for tools with spec.auth is enabled automatically.
func NewToolRuntime(client *Client, registry agentruntime.ToolCapabilityRegistry, secrets agentruntime.SecretResolver) *ToolRuntime {
	var injector *agentruntime.AuthInjector
	if secrets != nil {
		injector = agentruntime.NewAuthInjector(secrets, nil)
	}
	return &ToolRuntime{
		client:       client,
		registry:     registry,
		secrets:      secrets,
		authInjector: injector,
	}
}

func (r *ToolRuntime) Call(ctx context.Context, tool string, input string) (string, error) {
	tool = strings.TrimSpace(tool)
	spec, ok := r.resolveSpec(tool)
	if !ok {
		return "", fmt.Errorf("a2a tool runtime: no spec found for tool=%s", tool)
	}

	agentURL := strings.TrimSpace(spec.A2A.AgentURL)
	if agentURL == "" {
		return "", fmt.Errorf("a2a tool runtime: spec.a2a.agent_url is required for tool=%s", tool)
	}

	// Resolve auth headers when the tool spec has auth configured.
	var authHeaders map[string]string
	if r.authInjector != nil && strings.TrimSpace(spec.Auth.SecretRef) != "" {
		authResult, err := r.authInjector.Resolve(ctx, tool, spec.Auth)
		if err != nil {
			return "", fmt.Errorf("a2a tool runtime: auth resolution failed for tool=%s: %w", tool, err)
		}
		authHeaders = authResult.Headers
	}

	// Best-effort Agent Card resolution: use the card's canonical URL if
	// available, falling back to the configured agent_url on any error.
	if card, err := r.client.FetchCard(ctx, agentURL, authHeaders); err == nil {
		if canonical := strings.TrimSpace(card.URL); canonical != "" {
			agentURL = canonical
		}
	}

	taskID := fmt.Sprintf("%s-%s", tool, generateShortID())

	params := TaskSendParams{
		ID: taskID,
		Message: TaskMessage{
			Role: "user",
			Parts: []TaskPart{{
				Type: "text",
				Text: input,
			}},
		},
	}

	result, err := r.client.SendTask(ctx, agentURL, params, authHeaders)
	if err != nil {
		return "", fmt.Errorf("a2a tool runtime: send failed for tool=%s: %w", tool, err)
	}

	if result.Status.State == TaskStateFailed || result.Status.State == TaskStateRejected {
		errMsg := "remote agent returned " + result.Status.State
		if result.Status.Message != nil {
			for _, part := range result.Status.Message.Parts {
				if part.Type == "text" {
					errMsg = part.Text
					break
				}
			}
		}
		return "", fmt.Errorf("a2a tool runtime: %s", errMsg)
	}

	if !IsTerminal(result.Status.State) {
		const maxPolls = 60
		for i := 0; i < maxPolls; i++ {
			delay := time.Duration(min(2+i, 10)) * time.Second
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(delay):
			}

			result, err = r.client.GetTask(ctx, agentURL, TaskGetParams{ID: taskID}, authHeaders)
			if err != nil {
				return "", fmt.Errorf("a2a tool runtime: poll failed: %w", err)
			}
			if IsTerminal(result.Status.State) {
				break
			}
		}
		if !IsTerminal(result.Status.State) {
			return "", fmt.Errorf("a2a tool runtime: poll timeout for tool=%s: task still in state %q after %d polls", tool, result.Status.State, maxPolls)
		}
	}

	if result.Status.State == TaskStateFailed || result.Status.State == TaskStateRejected || result.Status.State == TaskStateCanceled {
		errMsg := "remote agent returned " + result.Status.State
		if result.Status.Message != nil {
			for _, part := range result.Status.Message.Parts {
				if part.Type == "text" {
					errMsg = part.Text
					break
				}
			}
		}
		return "", fmt.Errorf("a2a tool runtime: %s", errMsg)
	}

	return formatResult(result), nil
}

func (r *ToolRuntime) resolveSpec(tool string) (resources.ToolSpec, bool) {
	if r.registry == nil {
		return resources.ToolSpec{}, false
	}
	return r.registry.Resolve(tool)
}

func formatResult(result TaskResult) string {
	var parts []string
	for _, artifact := range result.Artifacts {
		for _, part := range artifact.Parts {
			if part.Type == "text" && strings.TrimSpace(part.Text) != "" {
				parts = append(parts, part.Text)
			}
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, "\n")
	}
	if result.Status.Message != nil {
		for _, part := range result.Status.Message.Parts {
			if part.Type == "text" && strings.TrimSpace(part.Text) != "" {
				return part.Text
			}
		}
	}
	out, _ := json.Marshal(result)
	return string(out)
}

// WithRegistry returns a copy scoped to the given registry.
func (r *ToolRuntime) WithRegistry(registry agentruntime.ToolCapabilityRegistry) agentruntime.ToolRuntime {
	if r == nil {
		return nil
	}
	return &ToolRuntime{
		client:       r.client,
		registry:     registry,
		secrets:      r.secrets,
		authInjector: r.authInjector,
		namespace:    r.namespace,
	}
}

// namespaceAwareResolver matches the runtime-internal interface for secret
// resolvers that can be scoped to a Kubernetes namespace.
type namespaceAwareResolver interface {
	WithNamespace(namespace string) agentruntime.SecretResolver
}

// WithNamespace returns a copy scoped to the given namespace.
func (r *ToolRuntime) WithNamespace(namespace string) agentruntime.ToolRuntime {
	if r == nil {
		return nil
	}
	cp := ToolRuntime{
		client:       r.client,
		registry:     r.registry,
		secrets:      r.secrets,
		authInjector: r.authInjector,
		namespace:    namespace,
	}
	if aware, ok := cp.secrets.(namespaceAwareResolver); ok {
		cp.secrets = aware.WithNamespace(namespace)
		cp.authInjector = agentruntime.NewAuthInjector(cp.secrets, nil)
	}
	return &cp
}

func generateShortID() string {
	return fmt.Sprintf("%d", uint32(time.Now().UnixNano()%1000000))
}
