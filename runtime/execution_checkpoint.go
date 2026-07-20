package agentruntime

// AgentExecutionCheckpoint is the versioned, serializable state required to
// continue one ReAct loop from the next safe step without replaying completed
// model and tool work.
type AgentExecutionCheckpoint struct {
	Version                int               `json:"version"`
	NextStep               int               `json:"next_step"`
	Completed              bool              `json:"completed,omitempty"`
	History                []ChatMessage     `json:"history,omitempty"`
	Memory                 map[string]string `json:"memory,omitempty"`
	ToolResultCache        map[string]string `json:"tool_result_cache,omitempty"`
	ToolCalled             []string          `json:"tool_called,omitempty"`
	ContractRemaining      []string          `json:"contract_remaining,omitempty"`
	ConsecutiveModelErrors int               `json:"consecutive_model_errors,omitempty"`
	LastOutput             string            `json:"last_output,omitempty"`
	Steps                  int               `json:"steps,omitempty"`
	ToolCalls              int               `json:"tool_calls,omitempty"`
	TokensUsed             int               `json:"tokens_used,omitempty"`
	InputTokens            int               `json:"input_tokens,omitempty"`
	OutputTokens           int               `json:"output_tokens,omitempty"`
}

const AgentExecutionCheckpointVersion = 1

type ExecutionCheckpointSink func(AgentExecutionCheckpoint) error

func copyAgentExecutionCheckpoint(in AgentExecutionCheckpoint) AgentExecutionCheckpoint {
	out := in
	out.History = append([]ChatMessage(nil), in.History...)
	for i := range out.History {
		out.History[i].ToolCalls = append([]ChatToolCall(nil), in.History[i].ToolCalls...)
	}
	out.Memory = copyStringMap(in.Memory)
	out.ToolResultCache = copyStringMap(in.ToolResultCache)
	out.ToolCalled = append([]string(nil), in.ToolCalled...)
	out.ContractRemaining = append([]string(nil), in.ContractRemaining...)
	return out
}

func CheckpointFromExecutionResult(result AgentExecutionResult) AgentExecutionCheckpoint {
	return AgentExecutionCheckpoint{
		Version:    AgentExecutionCheckpointVersion,
		NextStep:   result.Steps + 1,
		Completed:  true,
		LastOutput: result.Output,
		Steps:      result.Steps,
		ToolCalls:  result.ToolCalls,
		TokensUsed: result.TokensUsed,
	}
}
