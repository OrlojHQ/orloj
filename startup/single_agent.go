package startup

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/OrlojHQ/orloj/resources"
	agentruntime "github.com/OrlojHQ/orloj/runtime"
)

// SingleAgentConfig holds the flags/config for --single-agent mode.
type SingleAgentConfig struct {
	TaskID    string
	AgentName string
	Attempt   int
	MessageID string

	ModelSecretEnvPrefix string
	ToolSecretEnvPrefix  string

	IsolatedToolRuntime agentruntime.ToolRuntime
	WasmToolRuntime     agentruntime.ToolRuntime
	McpSessionManager   *agentruntime.McpSessionManager
	CliToolConfig       agentruntime.CLIToolRuntimeConfig
	SecretResolver      agentruntime.SecretResolver
	Extensions          agentruntime.Extensions
	KubernetesTools     agentruntime.ToolRuntime
	A2ATools            agentruntime.ToolRuntime
}

// RunSingleAgent loads a task and agent from the stores, executes the agent,
// and writes the result back to the task store. This is the core of the
// --single-agent pod mode used by KubernetesAgentRuntime.
func RunSingleAgent(ctx context.Context, stores *StoreSet, cfg SingleAgentConfig, logger *log.Logger) error {
	taskKey := strings.TrimSpace(cfg.TaskID)
	agentName := strings.TrimSpace(cfg.AgentName)
	if taskKey == "" {
		return fmt.Errorf("--task-id is required in --single-agent mode")
	}
	if agentName == "" {
		return fmt.Errorf("--agent-name is required in --single-agent mode")
	}

	task, ok, err := stores.Tasks.Get(ctx, taskKey)
	if err != nil {
		return fmt.Errorf("load task %s: %w", taskKey, err)
	}
	if !ok {
		return fmt.Errorf("task %s not found", taskKey)
	}

	ns := resources.NormalizeNamespace(task.Metadata.Namespace)
	agentLookup := agentName
	if ns != "" && ns != "default" && !strings.Contains(agentName, "/") {
		agentLookup = ns + "/" + agentName
	}

	agent, ok, err := stores.Agents.Get(ctx, agentLookup)
	if err != nil {
		return fmt.Errorf("load agent %s: %w", agentLookup, err)
	}
	if !ok {
		return fmt.Errorf("agent %s not found", agentLookup)
	}

	systemName := strings.TrimSpace(task.Spec.System)
	if systemName == "" {
		return fmt.Errorf("task %s has no system reference", taskKey)
	}
	systemLookup := systemName
	if ns != "" && ns != "default" && !strings.Contains(systemName, "/") {
		systemLookup = ns + "/" + systemName
	}

	system, ok, err := stores.AgentSystems.Get(ctx, systemLookup)
	if err != nil {
		return fmt.Errorf("load agent system %s: %w", systemLookup, err)
	}
	if !ok {
		return fmt.Errorf("agent system %s not found", systemLookup)
	}

	input := task.Status.AgentJobInput
	if input == nil {
		input = make(map[string]string)
	}

	// Apply context adapter if this is the first agent in the system.
	if caRef := strings.TrimSpace(system.Spec.ContextAdapter); caRef != "" {
		if isFirstAgent(agent.Metadata.Name, system) {
			logger.Printf("context adapter %s referenced for first agent; pod-mode adapter execution deferred to orchestrator", caRef)
		}
	}

	// Build model gateway.
	modelGateway := agentruntime.NewModelRouter(agentruntime.ModelRouterConfig{
		Endpoints:       stores.ModelEPs,
		Secrets:         stores.Secrets,
		SecretEnvPrefix: cfg.ModelSecretEnvPrefix,
	})

	// Build tool runtime stack (mirrors controllers/task_controller.go pattern).
	toolRuntime := agentruntime.BuildGovernedToolRuntimeForAgent(
		ctx,
		nil,
		cfg.IsolatedToolRuntime,
		stores.Tools,
		ns,
		agent.Spec.Tools,
	)
	if cfg.McpSessionManager != nil {
		agentruntime.ConfigureMcpRuntime(toolRuntime, cfg.McpSessionManager, stores.McpServers, ns)
	}
	agentruntime.ConfigureHttpRuntime(toolRuntime, cfg.SecretResolver, ns)
	agentruntime.ConfigureCliRuntime(toolRuntime, cfg.SecretResolver, nil, cfg.CliToolConfig, ns)
	agentruntime.ConfigureExternalRuntime(toolRuntime, cfg.SecretResolver, ns)
	agentruntime.ConfigureGRPCRuntime(toolRuntime, cfg.SecretResolver, ns)
	agentruntime.ConfigureWebhookCallbackRuntime(toolRuntime, cfg.SecretResolver, ns)
	agentruntime.ConfigureWasmRuntime(toolRuntime, cfg.WasmToolRuntime, ns)
	if cfg.KubernetesTools != nil {
		agentruntime.ConfigureKubernetesRuntime(toolRuntime, cfg.KubernetesTools, ns)
	}
	if cfg.A2ATools != nil {
		agentruntime.ConfigureA2ARuntime(toolRuntime, cfg.A2ATools, ns)
	}

	var finalRT agentruntime.ToolRuntime = toolRuntime
	if agentruntime.AgentHasOrlojTools(agent) {
		finalRT = agentruntime.NewOrlojToolRuntime(toolRuntime, stores.Tasks, agentruntime.OrlojToolConfig{
			ParentNamespace: ns,
		})
	}

	executor := agentruntime.NewTaskExecutorWithRuntime(logger, nil, modelGateway, nil)

	var stepEvents []agentruntime.AgentStepEvent
	executor.OnStepEvent = func(evt agentruntime.AgentStepEvent) {
		stepEvents = append(stepEvents, evt)
	}

	// Resolve model ref.
	_, effectiveModel, modelErr := resources.ResolveAgentModelRef(ctx, ns, agent.Spec.ModelRef, stores.ModelEPs)
	if modelErr != nil {
		logger.Printf("model ref resolution failed for agent %s: %v", agentName, modelErr)
	} else {
		agent.Spec.Model = effectiveModel
	}

	start := time.Now()
	result, execErr := executor.ExecuteAgentWithRuntime(ctx, agent, input, finalRT)
	result.StepEvents = stepEvents
	result.Duration = time.Since(start)

	jobResult := agentruntime.ExecutionToAgentJobResult(result, execErr)
	if ctx.Err() != nil && jobResult.ExitReason == "" {
		jobResult.ExitReason = "canceled"
	}

	writeCtx, writeCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer writeCancel()
	if writeErr := stores.Tasks.SetAgentJobResult(writeCtx, taskKey, jobResult); writeErr != nil {
		return fmt.Errorf("write agent job result: %w", writeErr)
	}
	logger.Printf("single-agent complete agent=%s steps=%d tool_calls=%d duration=%s error=%v",
		agentName, result.Steps, result.ToolCalls, result.Duration, execErr)
	return nil
}

func isFirstAgent(agentName string, system resources.AgentSystem) bool {
	if len(system.Spec.Agents) == 0 {
		return false
	}
	return strings.EqualFold(
		strings.TrimSpace(system.Spec.Agents[0]),
		strings.TrimSpace(agentName),
	)
}
