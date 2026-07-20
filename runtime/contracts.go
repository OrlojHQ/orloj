package agentruntime

import "context"

// ToolRuntime executes external tool calls for agents.
type ToolRuntime interface {
	Call(ctx context.Context, tool string, input string) (string, error)
}

// ToolClient is kept as a compatibility alias.
type ToolClient = ToolRuntime

// MemoryStore stores short-lived agent working memory.
type MemoryStore interface {
	Put(key, value string)
	Get(key string) (string, bool)
	Snapshot() map[string]string
}

// ChatMessage represents one message in a multi-turn conversation.
type ChatMessage struct {
	Role       string         `json:"role"` // "system", "user", "assistant", "tool"
	Content    string         `json:"content,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"` // role="tool": the ID of the tool call this result answers
	ToolCalls  []ChatToolCall `json:"tool_calls,omitempty"`   // role="assistant": tool calls the model made this turn
	IsError    bool           `json:"is_error,omitempty"`     // role="tool": true when this tool result represents a failure
}

// ChatToolCall captures one tool invocation from an assistant message.
type ChatToolCall struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Input        string `json:"input,omitempty"`
	ProviderName string `json:"provider_name,omitempty"`
}

// ToolSchemaInfo carries optional description and JSON Schema for a tool.
// When present, model gateways use these instead of the generic fallback.
type ToolSchemaInfo struct {
	Description string
	InputSchema map[string]any
}

// ModelRequest defines one model inference request for an agent step.
type ModelRequest struct {
	Model             string
	ModelRef          string
	FallbackModelRefs []string
	Namespace         string
	Agent             string
	Prompt            string
	Step              int
	Tools             []string
	ToolSchemas       map[string]ToolSchemaInfo
	Context           map[string]string
	Messages          []ChatMessage
	OutputSchema      map[string]any
}

// ModelResponse captures model output used by the runtime loop.
type ModelResponse struct {
	Content   string
	Done      bool
	ToolCalls []ModelToolCall
	Usage     ModelUsage
}

// ModelUsage captures provider-reported or estimated token usage for one model call.
type ModelUsage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
	Source       string
}

// ModelToolCall is one model-selected tool invocation request.
type ModelToolCall struct {
	ID           string
	Name         string
	Input        string
	ProviderName string
}

// ModelStreamEventType identifies one typed event in a streamed model response.
type ModelStreamEventType string

const (
	ModelStreamEventTextDelta  ModelStreamEventType = "text_delta"
	ModelStreamEventToolCall   ModelStreamEventType = "tool_call"
	ModelStreamEventUsage      ModelStreamEventType = "usage"
	ModelStreamEventCompletion ModelStreamEventType = "completion"
	ModelStreamEventError      ModelStreamEventType = "error"
)

// ModelStreamEvent is one incremental model response event. Fields are populated
// according to Type: Delta for text_delta, ToolCall for a complete tool_call,
// Usage for a cumulative usage snapshot, Response for completion, and Err for
// error.
type ModelStreamEvent struct {
	Type     ModelStreamEventType
	Delta    string
	ToolCall *ModelToolCall
	Usage    *ModelUsage
	Response *ModelResponse
	Err      error
}

// ModelStreamEventSink receives model stream events synchronously and in order.
type ModelStreamEventSink func(ModelStreamEvent)

// ToolSchemaResolver resolves rich tool schemas for model gateway formatting.
// Implementations that wrap tool registries (e.g. GovernedToolRuntime) can
// provide per-tool descriptions and JSON Schemas to the LLM.
type ToolSchemaResolver interface {
	ResolveToolSchemas(toolNames []string) map[string]ToolSchemaInfo
}

// ModelGateway abstracts model-provider calls for agent execution.
type ModelGateway interface {
	Complete(ctx context.Context, req ModelRequest) (ModelResponse, error)
}

// StreamingModelGateway is an optional extension implemented by gateways that
// can deliver incremental output. ModelGateway remains intentionally unchanged
// so existing provider and test implementations continue to work.
type StreamingModelGateway interface {
	Stream(ctx context.Context, req ModelRequest, sink ModelStreamEventSink) (ModelResponse, error)
}
