package agentruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/OrlojHQ/orloj/resources"
)

// ContextAdapterHook transforms task input before any agent sees it.
// Implementations must be safe to call concurrently.
type ContextAdapterHook interface {
	AdaptContext(ctx context.Context, input map[string]string) (map[string]string, error)
}

// ToolBackedContextAdapter runs a sanitization Tool against raw task input.
type ToolBackedContextAdapter struct {
	toolRuntime ToolRuntime
	spec        resources.ContextAdapterSpec
}

// NewToolBackedContextAdapter builds a hook backed by ToolRuntime.Call(spec.ToolRef, JSON payload).
func NewToolBackedContextAdapter(
	spec resources.ContextAdapterSpec,
	toolRuntime ToolRuntime,
) *ToolBackedContextAdapter {
	return &ToolBackedContextAdapter{
		toolRuntime: toolRuntime,
		spec:        spec,
	}
}

// AdaptContext JSON-encodes input, calls the tool, and decodes the JSON output.
func (c *ToolBackedContextAdapter) AdaptContext(ctx context.Context, input map[string]string) (map[string]string, error) {
	if c == nil || c.toolRuntime == nil {
		return nil, fmt.Errorf("context adapter: runtime not configured")
	}
	toolRef := strings.TrimSpace(c.spec.ToolRef)
	if toolRef == "" {
		return nil, fmt.Errorf("context adapter: spec.tool_ref is required")
	}

	original := cloneStringMap(input)

	encoded, err := json.Marshal(input)
	if err != nil {
		return c.finishWithError(original, fmt.Errorf("context adapter: encode payload: %w", err))
	}

	rawOutput, callErr := c.toolRuntime.Call(ctx, toolRef, string(encoded))
	if callErr != nil {
		return c.finishWithError(original, fmt.Errorf("context adapter: tool %q: %w", toolRef, callErr))
	}

	rawOutput = strings.TrimSpace(rawOutput)
	var sanitized map[string]string
	if err := json.Unmarshal([]byte(rawOutput), &sanitized); err != nil {
		return c.finishWithError(original, fmt.Errorf("context adapter: decode tool output: %w", err))
	}
	if sanitized == nil {
		sanitized = map[string]string{}
	}
	return sanitized, nil
}

func (c *ToolBackedContextAdapter) finishWithError(original map[string]string, err error) (map[string]string, error) {
	if c.spec.OnError == "passthrough" {
		log.Printf("context adapter: %v — passthrough enabled, using raw task input", err)
		return original, nil
	}
	return nil, err
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
