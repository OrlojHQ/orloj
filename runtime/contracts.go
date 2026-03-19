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
	Role       string // "system", "user", "assistant", "tool"
	Content    string
	ToolCallID string         // role="tool": the ID of the tool call this result answers
	ToolCalls  []ChatToolCall // role="assistant": tool calls the model made this turn
}

// ChatToolCall captures one tool invocation from an assistant message.
type ChatToolCall struct {
	ID    string
	Name  string
	Input string
}

// ModelRequest defines one model inference request for an agent step.
type ModelRequest struct {
	Model     string
	ModelRef  string
	Namespace string
	Agent     string
	Prompt    string
	Step      int
	Tools     []string
	Context   map[string]string
	Messages  []ChatMessage
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
	ID    string
	Name  string
	Input string
}

// ModelGateway abstracts model-provider calls for agent execution.
type ModelGateway interface {
	Complete(ctx context.Context, req ModelRequest) (ModelResponse, error)
}
