package agentruntime

import (
	"context"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/OrlojHQ/orloj/crds"
)

var stepRegex = regexp.MustCompile(`step=([0-9]+)`)                     //nolint:gochecknoglobals
var toolRegex = regexp.MustCompile(`tool=([^\s]+)`)                     //nolint:gochecknoglobals
var modelErrRegex = regexp.MustCompile(`model_error=(.+)$`)             //nolint:gochecknoglobals
var modelOutputRegex = regexp.MustCompile(`model_output=(.+)$`)         //nolint:gochecknoglobals
var toolCodeRegex = regexp.MustCompile(`tool_code=([^\s]+)`)            //nolint:gochecknoglobals
var toolReasonRegex = regexp.MustCompile(`tool_reason=([^\s]+)`)        //nolint:gochecknoglobals
var retryableRegex = regexp.MustCompile(`retryable=(true|false)`)       //nolint:gochecknoglobals
var toolContractRegex = regexp.MustCompile(`tool_contract=([^\s]+)`)    //nolint:gochecknoglobals
var toolRequestIDRegex = regexp.MustCompile(`tool_request_id=([^\s]+)`) //nolint:gochecknoglobals
var toolAttemptRegex = regexp.MustCompile(`tool_attempt=([0-9]+)`)      //nolint:gochecknoglobals
var tokenRegex = regexp.MustCompile(`tokens=([0-9]+)`)                  //nolint:gochecknoglobals
var usageSourceRegex = regexp.MustCompile(`usage_source=([^\s]+)`)      //nolint:gochecknoglobals

// AgentStepEvent is one structured runtime event emitted during agent execution.
type AgentStepEvent struct {
	Timestamp           string
	Type                string
	Step                int
	Tool                string
	Message             string
	ErrorCode           string
	ErrorReason         string
	Retryable           *bool
	ToolContractVersion string
	ToolRequestID       string
	ToolAttempt         int
	Tokens              int
	UsageSource         string
}

// AgentExecutionResult captures task-time execution details for one agent.
type AgentExecutionResult struct {
	Agent           string
	Model           string
	Steps           int
	ToolCalls       int
	MemoryWrites    int
	EstimatedTokens int
	TokensUsed      int
	TokenSource     string
	Duration        time.Duration
	Output          string
	LastEvent       string
	Events          []string
	StepEvents      []AgentStepEvent
}

type observedAgentEvent struct {
	Timestamp string
	Message   string
}

// TaskExecutor runs agents on-demand for Task execution.
type TaskExecutor struct {
	engine         ExecutionEngine
	toolRuntime    ToolRuntime
	modelGateway   ModelGateway
	newMemoryStore func() MemoryStore
	stepEvery      time.Duration
	logger         *log.Logger
}

func NewTaskExecutor(logger *log.Logger) *TaskExecutor {
	return NewTaskExecutorWithRuntime(logger, nil, nil, nil)
}

func NewTaskExecutorWithRuntime(
	logger *log.Logger,
	toolRuntime ToolRuntime,
	modelGateway ModelGateway,
	newMemoryStore func() MemoryStore,
) *TaskExecutor {
	if toolRuntime == nil {
		toolRuntime = &MockToolClient{}
	}
	if modelGateway == nil {
		modelGateway = &MockModelGateway{}
	}
	if newMemoryStore == nil {
		newMemoryStore = func() MemoryStore { return NewMemoryManager() }
	}
	const stepEvery = 25 * time.Millisecond
	engine := NewReActExecutionEngine(toolRuntime, modelGateway, newMemoryStore, stepEvery)
	return &TaskExecutor{
		engine:         engine,
		toolRuntime:    toolRuntime,
		modelGateway:   modelGateway,
		newMemoryStore: newMemoryStore,
		stepEvery:      stepEvery,
		logger:         logger,
	}
}

func (e *TaskExecutor) ExecuteAgent(ctx context.Context, agent crds.Agent, input map[string]string) (AgentExecutionResult, error) {
	return e.ExecuteAgentWithRuntime(ctx, agent, input, nil)
}

func (e *TaskExecutor) ExecuteAgentWithRuntime(
	ctx context.Context,
	agent crds.Agent,
	input map[string]string,
	override ToolRuntime,
) (AgentExecutionResult, error) {
	engine := e.engine
	if override != nil {
		engine = NewReActExecutionEngine(override, e.modelGateway, e.newMemoryStore, e.stepEvery)
	}
	result, err := engine.Execute(ctx, agent, input)
	if err != nil {
		return result, err
	}

	if e.logger != nil {
		e.logger.Printf("task-executor agent=%s steps=%d tool_calls=%d duration=%s", result.Agent, result.Steps, result.ToolCalls, result.Duration)
	}

	return result, nil
}

func observedMessages(events []observedAgentEvent) []string {
	out := make([]string, 0, len(events))
	for _, event := range events {
		out = append(out, event.Message)
	}
	return out
}

func parseAgentStepEvents(events []observedAgentEvent) []AgentStepEvent {
	out := make([]AgentStepEvent, 0, len(events))
	for _, event := range events {
		msg := strings.TrimSpace(event.Message)
		if msg == "" {
			continue
		}

		step := parseStep(msg)
		kind := classifyAgentStepEvent(msg)
		tool := parseTool(msg)
		message := msg
		code := parseToolCode(msg)
		reason := parseToolReason(msg)
		retryable := parseRetryable(msg)
		toolContractVersion := parseToolContractVersion(msg)
		toolRequestID := parseToolRequestID(msg)
		toolAttempt := parseToolAttempt(msg)
		if kind == "model_error" {
			if matches := modelErrRegex.FindStringSubmatch(msg); len(matches) == 2 {
				message = strings.TrimSpace(matches[1])
			}
		} else if kind == "model_output" {
			if matches := modelOutputRegex.FindStringSubmatch(msg); len(matches) == 2 {
				message = strings.TrimSpace(matches[1])
			}
		}
		out = append(out, AgentStepEvent{
			Timestamp:           strings.TrimSpace(event.Timestamp),
			Type:                kind,
			Step:                step,
			Tool:                tool,
			Message:             message,
			ErrorCode:           code,
			ErrorReason:         reason,
			Retryable:           retryable,
			ToolContractVersion: toolContractVersion,
			ToolRequestID:       toolRequestID,
			ToolAttempt:         toolAttempt,
			Tokens:              parseTokens(msg),
			UsageSource:         parseUsageSource(msg),
		})
	}
	return out
}

func classifyAgentStepEvent(msg string) string {
	switch {
	case strings.HasPrefix(msg, "inbox "):
		return "agent_inbox"
	case strings.HasPrefix(msg, "worker started"):
		return "agent_worker_start"
	case strings.Contains(msg, "worker stopped"):
		return "agent_worker_stop"
	case strings.Contains(msg, "max steps reached"):
		return "agent_worker_complete"
	case strings.Contains(msg, "model success"):
		return "model_call"
	case strings.Contains(msg, "model_output="):
		return "model_output"
	case strings.Contains(msg, "model_error="):
		return "model_error"
	case strings.Contains(msg, "tool=") && strings.Contains(msg, " success"):
		return "tool_call"
	case strings.Contains(strings.ToLower(msg), "tool_status=denied"),
		strings.Contains(strings.ToLower(msg), "tool_code=permission_denied"):
		return "tool_permission_denied"
	case strings.Contains(msg, "tool=") && strings.Contains(strings.ToLower(msg), "permission denied"):
		return "tool_permission_denied"
	case strings.Contains(msg, "tool=") && strings.Contains(msg, " error="):
		return "tool_error"
	case strings.Contains(msg, "no tools configured"):
		return "agent_step"
	default:
		return "agent_event"
	}
}

func parseStep(msg string) int {
	matches := stepRegex.FindStringSubmatch(msg)
	if len(matches) < 2 {
		return 0
	}
	n, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0
	}
	return n
}

func parseTool(msg string) string {
	matches := toolRegex.FindStringSubmatch(msg)
	if len(matches) < 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}

func parseToolCode(msg string) string {
	matches := toolCodeRegex.FindStringSubmatch(msg)
	if len(matches) < 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}

func parseToolReason(msg string) string {
	matches := toolReasonRegex.FindStringSubmatch(msg)
	if len(matches) < 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}

func parseRetryable(msg string) *bool {
	matches := retryableRegex.FindStringSubmatch(strings.ToLower(msg))
	if len(matches) < 2 {
		return nil
	}
	switch strings.TrimSpace(matches[1]) {
	case "true":
		value := true
		return &value
	case "false":
		value := false
		return &value
	default:
		return nil
	}
}

func parseToolContractVersion(msg string) string {
	matches := toolContractRegex.FindStringSubmatch(msg)
	if len(matches) < 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}

func parseToolRequestID(msg string) string {
	matches := toolRequestIDRegex.FindStringSubmatch(msg)
	if len(matches) < 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}

func parseToolAttempt(msg string) int {
	matches := toolAttemptRegex.FindStringSubmatch(msg)
	if len(matches) < 2 {
		return 0
	}
	value, err := strconv.Atoi(strings.TrimSpace(matches[1]))
	if err != nil {
		return 0
	}
	return value
}

func parseTokens(msg string) int {
	matches := tokenRegex.FindStringSubmatch(msg)
	if len(matches) < 2 {
		return 0
	}
	value, err := strconv.Atoi(strings.TrimSpace(matches[1]))
	if err != nil {
		return 0
	}
	if value < 0 {
		return 0
	}
	return value
}

func parseUsageSource(msg string) string {
	matches := usageSourceRegex.FindStringSubmatch(msg)
	if len(matches) < 2 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(matches[1]))
}
