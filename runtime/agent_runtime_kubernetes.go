package agentruntime

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/OrlojHQ/orloj/resources"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AgentJobStore is the minimal interface for orchestrator↔pod communication
// via Postgres. Implemented by store.TaskStore.
type AgentJobStore interface {
	Get(ctx context.Context, name string) (resources.Task, bool, error)
	SetAgentJobInput(ctx context.Context, name string, input map[string]string, agent, messageID string) error
	SetAgentJobResult(ctx context.Context, name string, result *resources.AgentJobResult) error
	GetAgentJobResult(ctx context.Context, name string) (*resources.AgentJobResult, error)
	ClearAgentJobFields(ctx context.Context, name string) error
}

// KubernetesAgentConfig holds operator-level defaults for agent K8s execution.
type KubernetesAgentConfig struct {
	Namespace      string
	ServiceAccount string
	Image          string
	JobTTLSeconds  int32
	DefaultMemory  string
	DefaultCPU     string
	EnvConfigMap   string
	EnvSecretName  string
}

func (c KubernetesAgentConfig) normalized() KubernetesAgentConfig {
	out := c
	if strings.TrimSpace(out.Namespace) == "" {
		out.Namespace = "default"
	}
	if strings.TrimSpace(out.Image) == "" {
		if img := os.Getenv("ORLOJ_AGENT_K8S_IMAGE"); img != "" {
			out.Image = img
		}
	}
	if out.JobTTLSeconds <= 0 {
		out.JobTTLSeconds = 600
	}
	if strings.TrimSpace(out.DefaultMemory) == "" {
		out.DefaultMemory = "512Mi"
	}
	if strings.TrimSpace(out.DefaultCPU) == "" {
		out.DefaultCPU = "500m"
	}
	return out
}

// KubernetesAgentRuntime executes agents as ephemeral Kubernetes Jobs.
type KubernetesAgentRuntime struct {
	client KubernetesJobClient
	config KubernetesAgentConfig
	store  AgentJobStore
	logger *log.Logger
}

func NewKubernetesAgentRuntime(
	client KubernetesJobClient,
	config KubernetesAgentConfig,
	store AgentJobStore,
	logger *log.Logger,
) *KubernetesAgentRuntime {
	if logger == nil {
		logger = log.New(os.Stderr, "agent-k8s: ", log.LstdFlags)
	}
	return &KubernetesAgentRuntime{
		client: client,
		config: config.normalized(),
		store:  store,
		logger: logger,
	}
}

// CanRunAsJob checks whether an agent can run as a K8s Job. Agents with
// Docker-dependent tools (container isolation or stdio MCP with image) cannot
// run as Jobs and fall back to in-process execution.
func CanRunAsJob(agent resources.Agent, tools []resources.Tool, mcpServers map[string]resources.McpServer) bool {
	for _, toolName := range agent.Spec.Tools {
		toolName = strings.TrimSpace(toolName)
		for _, t := range tools {
			if !strings.EqualFold(scopedToolName(t), toolName) {
				continue
			}
			mode := strings.ToLower(strings.TrimSpace(t.Spec.Runtime.IsolationMode))
			if mode == "container" {
				return false
			}
			if strings.ToLower(strings.TrimSpace(t.Spec.Type)) == "mcp" {
				ref := strings.TrimSpace(t.Spec.McpServerRef)
				if ref != "" {
					if srv, ok := mcpServers[ref]; ok {
						if strings.ToLower(strings.TrimSpace(srv.Spec.Transport)) == "stdio" &&
							strings.TrimSpace(srv.Spec.Image) != "" {
							return false
						}
					}
				}
			}
		}
	}
	return true
}

func scopedToolName(t resources.Tool) string {
	ns := resources.NormalizeNamespace(t.Metadata.Namespace)
	name := strings.TrimSpace(t.Metadata.Name)
	if ns == "" || ns == "default" {
		return name
	}
	return ns + "/" + name
}

// ExecuteAgent runs an agent as a K8s Job and returns the result.
func (r *KubernetesAgentRuntime) ExecuteAgent(
	ctx context.Context,
	task resources.Task,
	agent resources.Agent,
	input map[string]string,
	attempt int,
	messageID string,
) (*resources.AgentJobResult, error) {
	taskKey := taskStoreKey(task)
	agentName := strings.TrimSpace(agent.Metadata.Name)
	jobName := agentJobName(taskKey, agentName, attempt)
	ns := strings.TrimSpace(r.config.Namespace)

	// Check for an existing Job (crash recovery).
	existing, err := r.client.GetJob(ctx, ns, jobName)
	if err == nil && existing != nil {
		return r.handleExistingJob(ctx, taskKey, existing, agentName, ns, jobName)
	}
	if err != nil && !k8serrors.IsNotFound(err) {
		r.logger.Printf("warning: failed to check for existing job %s: %v", jobName, err)
	}

	// Write input to store for the pod to read.
	if err := r.store.SetAgentJobInput(ctx, taskKey, input, agentName, messageID); err != nil {
		return nil, fmt.Errorf("set agent job input: %w", err)
	}

	// Build and create the Job.
	job := r.buildAgentJob(task, agent, attempt, messageID)
	created, err := r.client.CreateJob(ctx, ns, job)
	if err != nil {
		return nil, fmt.Errorf("create agent job: %w", err)
	}
	jobName = created.Name

	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = r.client.DeleteJob(cleanupCtx, ns, jobName)
	}()

	// Wait for Job completion.
	if err := r.waitForAgentJob(ctx, ns, jobName); err != nil {
		// Even on failure, try to read any result the pod may have written.
		result, readErr := r.store.GetAgentJobResult(ctx, taskKey)
		if readErr == nil && result != nil {
			_ = r.store.ClearAgentJobFields(ctx, taskKey)
			return result, nil
		}
		return nil, fmt.Errorf("agent job %s failed: %w", jobName, err)
	}

	// Read result from store.
	result, err := r.store.GetAgentJobResult(ctx, taskKey)
	if err != nil {
		return nil, fmt.Errorf("read agent job result: %w", err)
	}
	if result == nil {
		return nil, fmt.Errorf("agent job %s completed but no result found in store", jobName)
	}

	_ = r.store.ClearAgentJobFields(ctx, taskKey)
	return result, nil
}

func (r *KubernetesAgentRuntime) handleExistingJob(
	ctx context.Context,
	taskKey string,
	job *batchv1.Job,
	agentName, ns, jobName string,
) (*resources.AgentJobResult, error) {
	r.logger.Printf("found existing agent job %s (crash recovery)", jobName)

	cleanupJob := func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = r.client.DeleteJob(cleanupCtx, ns, jobName)
	}

	if isJobComplete(job) {
		result, err := r.store.GetAgentJobResult(ctx, taskKey)
		if err != nil {
			return nil, fmt.Errorf("read agent job result after recovery: %w", err)
		}
		if result == nil {
			return nil, fmt.Errorf("recovered agent job %s completed but no result in store", jobName)
		}
		_ = r.store.ClearAgentJobFields(ctx, taskKey)
		cleanupJob()
		return result, nil
	}

	if isJobFailed(job) {
		result, err := r.store.GetAgentJobResult(ctx, taskKey)
		if err == nil && result != nil {
			_ = r.store.ClearAgentJobFields(ctx, taskKey)
			cleanupJob()
			return result, nil
		}
		cleanupJob()
		return nil, fmt.Errorf("recovered agent job %s is in failed state", jobName)
	}

	// Job is still running -- resume watching.
	defer cleanupJob()
	if err := r.waitForAgentJob(ctx, ns, jobName); err != nil {
		result, readErr := r.store.GetAgentJobResult(ctx, taskKey)
		if readErr == nil && result != nil {
			_ = r.store.ClearAgentJobFields(ctx, taskKey)
			return result, nil
		}
		return nil, fmt.Errorf("recovered agent job %s failed: %w", jobName, err)
	}

	result, err := r.store.GetAgentJobResult(ctx, taskKey)
	if err != nil {
		return nil, fmt.Errorf("read agent job result after recovery watch: %w", err)
	}
	if result == nil {
		return nil, fmt.Errorf("recovered agent job %s completed but no result in store", jobName)
	}
	_ = r.store.ClearAgentJobFields(ctx, taskKey)
	return result, nil
}

func (r *KubernetesAgentRuntime) buildAgentJob(
	task resources.Task,
	agent resources.Agent,
	attempt int,
	messageID string,
) *batchv1.Job {
	cfg := r.config
	ns := strings.TrimSpace(cfg.Namespace)
	taskKey := taskStoreKey(task)
	agentName := strings.TrimSpace(agent.Metadata.Name)
	jobName := agentJobName(taskKey, agentName, attempt)

	args := []string{
		"--single-agent",
		"--task-id", taskKey,
		"--agent-name", agentName,
		"--attempt", fmt.Sprintf("%d", attempt),
	}
	if messageID != "" {
		args = append(args, "--message-id", messageID)
	}

	env := r.collectEnvVars()

	resourceReqs := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{},
		Limits:   corev1.ResourceList{},
	}
	if mem := strings.TrimSpace(cfg.DefaultMemory); mem != "" {
		if q, parseErr := resource.ParseQuantity(mem); parseErr == nil {
			resourceReqs.Requests[corev1.ResourceMemory] = q
			resourceReqs.Limits[corev1.ResourceMemory] = q
		} else {
			r.logger.Printf("warning: invalid memory resource %q: %v", mem, parseErr)
		}
	}
	if cpu := strings.TrimSpace(cfg.DefaultCPU); cpu != "" {
		if q, parseErr := resource.ParseQuantity(cpu); parseErr == nil {
			resourceReqs.Requests[corev1.ResourceCPU] = q
			resourceReqs.Limits[corev1.ResourceCPU] = q
		} else {
			r.logger.Printf("warning: invalid CPU resource %q: %v", cpu, parseErr)
		}
	}

	var activeDeadline *int64
	if timeout := strings.TrimSpace(agent.Spec.Limits.Timeout); timeout != "" {
		if d, err := time.ParseDuration(timeout); err == nil && d > 0 {
			secs := int64(d.Seconds())
			if secs < 1 {
				secs = 1
			}
			activeDeadline = &secs
		}
	}

	var backoffLimit int32
	ttl := cfg.JobTTLSeconds

	labels := map[string]string{
		"orloj.dev/component": "agent-job",
		"orloj.dev/agent":     sanitizeK8sLabelValue(agentName),
		"orloj.dev/task":      sanitizeK8sLabelValue(task.Metadata.Name),
		"orloj.dev/namespace": sanitizeK8sLabelValue(resources.NormalizeNamespace(task.Metadata.Namespace)),
	}

	container := corev1.Container{
		Name:      "agent",
		Image:     strings.TrimSpace(cfg.Image),
		Args:      args,
		Env:       env,
		Resources: resourceReqs,
	}

	podSpec := corev1.PodSpec{
		RestartPolicy: corev1.RestartPolicyNever,
		Containers:    []corev1.Container{container},
	}

	if cm := strings.TrimSpace(cfg.EnvConfigMap); cm != "" {
		podSpec.Containers[0].EnvFrom = append(podSpec.Containers[0].EnvFrom, corev1.EnvFromSource{
			ConfigMapRef: &corev1.ConfigMapEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: cm},
			},
		})
	}
	if sec := strings.TrimSpace(cfg.EnvSecretName); sec != "" {
		podSpec.Containers[0].EnvFrom = append(podSpec.Containers[0].EnvFrom, corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: sec},
			},
		})
	}
	if sa := strings.TrimSpace(cfg.ServiceAccount); sa != "" {
		podSpec.ServiceAccountName = sa
	}

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: ns,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			TTLSecondsAfterFinished: &ttl,
			ActiveDeadlineSeconds:   activeDeadline,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec:       podSpec,
			},
		},
	}
}

// collectEnvVars forwards relevant orchestrator env vars to the agent pod.
// This is the fallback for non-Helm deployments; Helm deployments should use
// envFrom ConfigMap/Secret references instead.
func (r *KubernetesAgentRuntime) collectEnvVars() []corev1.EnvVar {
	prefixes := []string{
		"ORLOJ_POSTGRES_",
		"ORLOJ_SECRET_",
		"ORLOJ_MODEL_SECRET_ENV_PREFIX",
		"ORLOJ_TOOL_SECRET_ENV_PREFIX",
		"ORLOJ_TOOL_K8S_",
		"ORLOJ_NATS_",
		"OTEL_EXPORTER_",
		"OTEL_SERVICE_NAME",
	}

	var env []corev1.EnvVar
	for _, kv := range os.Environ() {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key, val := parts[0], parts[1]
		for _, prefix := range prefixes {
			if strings.HasPrefix(key, prefix) || key == prefix {
				env = append(env, corev1.EnvVar{Name: key, Value: val})
				break
			}
		}
	}
	return env
}

func (r *KubernetesAgentRuntime) waitForAgentJob(ctx context.Context, namespace, jobName string) error {
	watcher, err := r.client.WatchJob(ctx, namespace, jobName)
	if err != nil {
		return fmt.Errorf("watch agent job %s: %w", jobName, err)
	}
	defer func() { watcher.Stop() }()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-watcher.ResultChan():
			if !ok {
				job, getErr := r.client.GetJob(ctx, namespace, jobName)
				if getErr != nil {
					return fmt.Errorf("watch closed and get failed for agent job %s: %w", jobName, getErr)
				}
				if isJobComplete(job) {
					return nil
				}
				if isJobFailed(job) {
					return fmt.Errorf("agent job %s failed", jobName)
				}
				watcher, err = r.client.WatchJob(ctx, namespace, jobName)
				if err != nil {
					return fmt.Errorf("re-watch agent job %s: %w", jobName, err)
				}
				continue
			}
			job, ok := event.Object.(*batchv1.Job)
			if !ok {
				continue
			}
			if isJobComplete(job) {
				return nil
			}
			if isJobFailed(job) {
				return fmt.Errorf("agent job %s failed", jobName)
			}
		}
	}
}

func isJobComplete(job *batchv1.Job) bool {
	for _, cond := range job.Status.Conditions {
		if cond.Type == batchv1.JobComplete && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func isJobFailed(job *batchv1.Job) bool {
	for _, cond := range job.Status.Conditions {
		if cond.Type == batchv1.JobFailed && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func taskStoreKey(task resources.Task) string {
	ns := resources.NormalizeNamespace(task.Metadata.Namespace)
	name := strings.TrimSpace(task.Metadata.Name)
	if ns == "" || ns == "default" {
		return name
	}
	return ns + "/" + name
}

func agentJobName(taskKey, agentName string, attempt int) string {
	return sanitizeK8sName(fmt.Sprintf("orloj-agent-%s-%s-a%d", taskKey, agentName, attempt))
}

// AgentJobResultToExecution converts an AgentJobResult from a K8s Job back
// into an AgentExecutionResult for use by the controller/consumer.
func AgentJobResultToExecution(r *resources.AgentJobResult, agentName string) AgentExecutionResult {
	result := AgentExecutionResult{
		Agent:           agentName,
		Steps:           r.Steps,
		ToolCalls:       r.ToolCalls,
		MemoryWrites:    r.MemoryWrites,
		EstimatedTokens: r.EstimatedTokens,
		TokensUsed:      r.TokensUsed,
		TokenSource:     r.TokenSource,
		Duration:        time.Duration(r.DurationMS) * time.Millisecond,
		Output:          r.Output,
		LastEvent:       r.LastEvent,
		Events:          r.Events,
	}
	result.StepEvents = make([]AgentStepEvent, len(r.StepEvents))
	for i, evt := range r.StepEvents {
		result.StepEvents[i] = AgentStepEvent{
			Timestamp:           evt.Timestamp,
			Type:                evt.Type,
			Step:                evt.Step,
			Tool:                evt.Tool,
			Message:             evt.Message,
			ErrorCode:           evt.ErrorCode,
			ErrorReason:         evt.ErrorReason,
			Retryable:           evt.Retryable,
			ToolContractVersion: evt.ToolContractVersion,
			ToolRequestID:       evt.ToolRequestID,
			ToolAttempt:         evt.ToolAttempt,
			LatencyMS:           evt.LatencyMS,
			Tokens:              evt.Tokens,
			InputTokens:         evt.InputTokens,
			OutputTokens:        evt.OutputTokens,
			UsageSource:         evt.UsageSource,
			ToolAuthProfile:     evt.ToolAuthProfile,
			ToolAuthSecretRef:   evt.ToolAuthSecretRef,
		}
	}
	return result
}

// ExecutionToAgentJobResult converts an AgentExecutionResult into an
// AgentJobResult for storage by the agent pod.
func ExecutionToAgentJobResult(r AgentExecutionResult, execErr error) *resources.AgentJobResult {
	result := &resources.AgentJobResult{
		Output:          r.Output,
		LastEvent:       r.LastEvent,
		Steps:           r.Steps,
		ToolCalls:       r.ToolCalls,
		MemoryWrites:    r.MemoryWrites,
		EstimatedTokens: r.EstimatedTokens,
		TokensUsed:      r.TokensUsed,
		TokenSource:     r.TokenSource,
		DurationMS:      r.Duration.Milliseconds(),
		Events:          r.Events,
	}
	if execErr != nil {
		result.Error = execErr.Error()
	}
	result.StepEvents = make([]resources.AgentJobStepEvt, len(r.StepEvents))
	for i, evt := range r.StepEvents {
		result.StepEvents[i] = resources.AgentJobStepEvt{
			Timestamp:           evt.Timestamp,
			Type:                evt.Type,
			Step:                evt.Step,
			Tool:                evt.Tool,
			Message:             evt.Message,
			ErrorCode:           evt.ErrorCode,
			ErrorReason:         evt.ErrorReason,
			Retryable:           evt.Retryable,
			ToolContractVersion: evt.ToolContractVersion,
			ToolRequestID:       evt.ToolRequestID,
			ToolAttempt:         evt.ToolAttempt,
			LatencyMS:           evt.LatencyMS,
			Tokens:              evt.Tokens,
			InputTokens:         evt.InputTokens,
			OutputTokens:        evt.OutputTokens,
			UsageSource:         evt.UsageSource,
			ToolAuthProfile:     evt.ToolAuthProfile,
			ToolAuthSecretRef:   evt.ToolAuthSecretRef,
		}
	}
	return result
}
