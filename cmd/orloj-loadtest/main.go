package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/OrlojHQ/orloj/resources"
)

type loadConfig struct {
	baseURL            string
	namespace          string
	tasks              int
	createConcurrency  int
	pollConcurrency    int
	pollInterval       time.Duration
	runTimeout         time.Duration
	taskSystem         string
	topicPrefix        string
	taskPriority       string
	taskRetryAttempts  int
	taskRetryBackoff   string
	messageRetryPolicy resources.TaskMessageRetryPolicy
	setup              bool
	minReadyWorkers    int
	workerReadyTimeout time.Duration

	injectInvalidSystemRate float64
	invalidSystemName       string
	injectTimeoutSystemRate float64
	timeoutSystemName       string
	timeoutAgentName        string
	timeoutAgentDuration    string
	injectExpiredLeaseRate  float64
	expiredLeaseOwner       string

	qualityProfilePath string
	qualityProfileName string
	minSuccessRate     float64
	maxDeadletterRate  float64
	maxFailedRate      float64
	maxTimedOut        int
	minRetryTotal      int
	minTakeoverEvents  int

	emitJSON bool
	verbose  bool
}

type qualityProfile struct {
	Name              string  `json:"name"`
	MinSuccessRate    float64 `json:"min_success_rate"`
	MaxDeadletterRate float64 `json:"max_deadletter_rate"`
	MaxFailedRate     float64 `json:"max_failed_rate"`
	MaxTimedOut       int     `json:"max_timed_out"`
	MinRetryTotal     int     `json:"min_retry_total"`
	MinTakeoverEvents int     `json:"min_takeover_events"`
}

type taskRun struct {
	name               string
	system             string
	created            time.Time
	injectInvalid      bool
	injectRetryStress  bool
	injectExpiredLease bool
}

type taskResult struct {
	name               string
	phase              string
	duration           time.Duration
	attempts           int
	taskRetries        int
	messageRetry       int
	deadletters        int
	takeovers          int
	assignmentCleared  int
	injectInvalid      bool
	injectRetryStress  bool
	injectExpiredLease bool
	lastError          string
}

type runSummary struct {
	requested    int
	created      int
	createErrors int
	timedOut     int

	succeeded  int
	failed     int
	deadletter int

	injectedInvalidRequested      int
	injectedRetryStressRequested  int
	injectedExpiredLeaseRequested int
	injectedExpiredLeaseApplied   int
	injectionErrors               int

	injectedTerminal int

	totalTaskRetry          int
	totalMessageRetry       int
	totalMessageDeadletters int
	totalTakeovers          int
	totalAssignmentCleared  int

	durations []time.Duration
	results   []taskResult
}

type runMetrics struct {
	Terminal       int     `json:"terminal"`
	SuccessRate    float64 `json:"success_rate"`
	DeadletterRate float64 `json:"deadletter_rate"`
	FailedRate     float64 `json:"failed_rate"`
	DurationP50Ms  int64   `json:"duration_p50_ms"`
	DurationP95Ms  int64   `json:"duration_p95_ms"`
	DurationMaxMs  int64   `json:"duration_max_ms"`
	RetryTotal     int     `json:"retry_total"`
}

type gateEvaluation struct {
	Passed     bool     `json:"passed"`
	Violations []string `json:"violations,omitempty"`
}

type reportConfig struct {
	Namespace  string `json:"namespace"`
	Tasks      int    `json:"tasks"`
	TaskSystem string `json:"task_system"`
	Profile    string `json:"quality_profile,omitempty"`
	Inject     struct {
		InvalidSystemRate float64 `json:"invalid_system_rate"`
		RetryStressRate   float64 `json:"retry_stress_rate"`
		ExpiredLeaseRate  float64 `json:"expired_lease_rate"`
	} `json:"inject"`
}

type reportSummary struct {
	Requested    int `json:"requested"`
	Created      int `json:"created"`
	CreateErrors int `json:"create_errors"`
	TimedOut     int `json:"timed_out"`

	Succeeded  int `json:"succeeded"`
	Failed     int `json:"failed"`
	Deadletter int `json:"deadletter"`

	InjectedInvalidRequested      int `json:"injected_invalid_requested"`
	InjectedRetryStressRequested  int `json:"injected_retry_stress_requested"`
	InjectedExpiredLeaseRequested int `json:"injected_expired_lease_requested"`
	InjectedExpiredLeaseApplied   int `json:"injected_expired_lease_applied"`
	InjectionErrors               int `json:"injection_errors"`
	InjectedTerminal              int `json:"injected_terminal"`

	TotalTaskRetry          int `json:"total_task_retry"`
	TotalMessageRetry       int `json:"total_message_retry"`
	TotalMessageDeadletters int `json:"total_message_deadletters"`
	TotalTakeovers          int `json:"total_takeovers"`
	TotalAssignmentCleared  int `json:"total_assignment_cleared"`
}

type reportTask struct {
	Name               string `json:"name"`
	Phase              string `json:"phase"`
	DurationMs         int64  `json:"duration_ms"`
	Attempts           int    `json:"attempts"`
	TaskRetries        int    `json:"task_retries"`
	MessageRetry       int    `json:"message_retry"`
	Deadletters        int    `json:"deadletters"`
	Takeovers          int    `json:"takeovers"`
	AssignmentCleared  int    `json:"assignment_cleared"`
	InjectInvalid      bool   `json:"inject_invalid"`
	InjectRetryStress  bool   `json:"inject_retry_stress"`
	InjectExpiredLease bool   `json:"inject_expired_lease"`
	LastError          string `json:"last_error,omitempty"`
}

type runReport struct {
	Timestamp  string         `json:"timestamp"`
	RunID      string         `json:"run_id"`
	Config     reportConfig   `json:"config"`
	Summary    reportSummary  `json:"summary"`
	Metrics    runMetrics     `json:"metrics"`
	Gates      gateEvaluation `json:"gates"`
	TopSlowest []reportTask   `json:"top_slowest,omitempty"`
}

type createStats struct {
	invalidRequested      int
	retryStressRequested  int
	expiredLeaseRequested int
}

type apiClient struct {
	baseURL string
	http    *http.Client
}

var baselineManifestFiles = []string{
	"examples/resources/memories/research_memory.yaml",
	"examples/resources/tools/web_search_tool.yaml",
	"examples/resources/tools/vector_db_tool.yaml",
	"examples/resources/secrets/search_api_key.yaml",
	"examples/resources/agents/planner_agent.yaml",
	"examples/resources/agents/research_agent.yaml",
	"examples/resources/agents/writer_agent.yaml",
	"examples/resources/agents/loadtest_timeout_agent.yaml",
	"examples/resources/agent-systems/report_system.yaml",
	"examples/resources/agent-systems/loadtest_timeout_system.yaml",
}

func main() {
	cfg := parseFlags()

	if strings.TrimSpace(cfg.qualityProfilePath) != "" {
		profile, err := loadQualityProfile(cfg.qualityProfilePath)
		if err != nil {
			fatalf("load quality profile failed: %v", err)
		}
		cfg = mergeQualityProfile(cfg, profile)
	}

	if err := validateConfig(cfg); err != nil {
		fatalf("invalid configuration: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.runTimeout)
	defer cancel()

	client := &apiClient{
		baseURL: strings.TrimRight(strings.TrimSpace(cfg.baseURL), "/"),
		http: &http.Client{
			Timeout: 15 * time.Second,
		},
	}

	if cfg.setup || cfg.injectTimeoutSystemRate > 0 {
		if err := ensureLoadtestModelEndpoint(ctx, client, cfg.namespace); err != nil {
			fatalf("failed to ensure model endpoint for loadtest resources: %v", err)
		}
	}

	if cfg.setup {
		logf("applying baseline manifests namespace=%s", cfg.namespace)
		if err := applyBaseline(ctx, client, cfg.namespace); err != nil {
			fatalf("failed to apply baseline manifests: %v", err)
		}
	}

	if cfg.injectTimeoutSystemRate > 0 {
		if err := ensureTimeoutScenarioResources(ctx, client, cfg); err != nil {
			fatalf("failed to ensure retry-stress resources: %v", err)
		}
	}

	if cfg.minReadyWorkers > 0 {
		logf("waiting for ready workers min=%d timeout=%s", cfg.minReadyWorkers, cfg.workerReadyTimeout)
		if err := waitForReadyWorkers(ctx, client, cfg.namespace, cfg.minReadyWorkers, cfg.workerReadyTimeout, cfg.pollInterval); err != nil {
			fatalf("worker readiness check failed: %v", err)
		}
	}

	runID := time.Now().UTC().Format("20060102-150405")
	logf(
		"starting load run id=%s namespace=%s tasks=%d create_concurrency=%d poll_concurrency=%d",
		runID,
		cfg.namespace,
		cfg.tasks,
		cfg.createConcurrency,
		cfg.pollConcurrency,
	)

	runs, createErrors, stats := createTasks(ctx, client, cfg, runID)
	summary := runSummary{
		requested:                     cfg.tasks,
		created:                       len(runs),
		createErrors:                  createErrors,
		injectedInvalidRequested:      stats.invalidRequested,
		injectedRetryStressRequested:  stats.retryStressRequested,
		injectedExpiredLeaseRequested: stats.expiredLeaseRequested,
		durations:                     make([]time.Duration, 0, len(runs)),
		results:                       make([]taskResult, 0, len(runs)),
	}
	if len(runs) == 0 {
		fatalf("no tasks were created successfully")
	}

	if cfg.injectExpiredLeaseRate > 0 {
		applied, injectErrs := applyExpiredLeaseSimulation(ctx, client, cfg, runs)
		summary.injectedExpiredLeaseApplied = applied
		summary.injectionErrors += injectErrs
	}

	results, timedOut := monitorTasks(ctx, client, cfg, runs)
	summary.timedOut = timedOut
	for _, res := range results {
		summary.results = append(summary.results, res)
		summary.durations = append(summary.durations, res.duration)
		summary.totalTaskRetry += res.taskRetries
		summary.totalMessageRetry += res.messageRetry
		summary.totalMessageDeadletters += res.deadletters
		summary.totalTakeovers += res.takeovers
		summary.totalAssignmentCleared += res.assignmentCleared
		if res.injectInvalid || res.injectRetryStress || res.injectExpiredLease {
			summary.injectedTerminal++
		}

		switch strings.ToLower(strings.TrimSpace(res.phase)) {
		case "succeeded":
			summary.succeeded++
		case "failed":
			summary.failed++
		case "deadletter":
			summary.deadletter++
		default:
			summary.failed++
		}
	}

	metrics := computeRunMetrics(summary)
	gates := evaluateRun(summary, metrics, cfg)
	report := buildRunReport(runID, cfg, summary, metrics, gates)

	if cfg.emitJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(report)
	} else {
		printSummary(summary, metrics, gates)
	}

	if !gates.Passed {
		os.Exit(2)
	}
	logf("load run completed successfully")
}

func parseFlags() loadConfig {
	cfg := loadConfig{}
	flag.StringVar(&cfg.baseURL, "base-url", "http://127.0.0.1:8080", "orloj API base URL")
	flag.StringVar(&cfg.namespace, "namespace", "default", "target namespace")
	flag.IntVar(&cfg.tasks, "tasks", 50, "number of tasks to create")
	flag.IntVar(&cfg.createConcurrency, "create-concurrency", 10, "concurrent task create workers")
	flag.IntVar(&cfg.pollConcurrency, "poll-concurrency", 20, "concurrent task status poll workers")
	flag.DurationVar(&cfg.pollInterval, "poll-interval", 500*time.Millisecond, "poll interval for task status")
	flag.DurationVar(&cfg.runTimeout, "run-timeout", 5*time.Minute, "global run timeout")

	flag.StringVar(&cfg.taskSystem, "task-system", "report-system", "AgentSystem name used by generated tasks")
	flag.StringVar(&cfg.topicPrefix, "topic-prefix", "loadtest-topic", "task input topic prefix")
	flag.StringVar(&cfg.taskPriority, "task-priority", "high", "task priority")
	flag.IntVar(&cfg.taskRetryAttempts, "task-retry-attempts", 3, "Task.spec.retry.max_attempts for generated tasks")
	flag.StringVar(&cfg.taskRetryBackoff, "task-retry-backoff", "2s", "Task.spec.retry.backoff for generated tasks")
	flag.IntVar(&cfg.messageRetryPolicy.MaxAttempts, "message-retry-attempts", 4, "Task.spec.message_retry.max_attempts for generated tasks")
	flag.StringVar(&cfg.messageRetryPolicy.Backoff, "message-retry-backoff", "200ms", "Task.spec.message_retry.backoff for generated tasks")
	flag.StringVar(&cfg.messageRetryPolicy.MaxBackoff, "message-retry-max-backoff", "2s", "Task.spec.message_retry.max_backoff for generated tasks")
	flag.StringVar(&cfg.messageRetryPolicy.Jitter, "message-retry-jitter", "full", "Task.spec.message_retry.jitter (none|full|equal)")

	flag.BoolVar(&cfg.setup, "setup", true, "apply baseline manifests before load run")
	flag.IntVar(&cfg.minReadyWorkers, "min-ready-workers", 2, "minimum ready workers required before run (0 disables check)")
	flag.DurationVar(&cfg.workerReadyTimeout, "worker-ready-timeout", 45*time.Second, "max wait for worker readiness check")

	flag.Float64Var(&cfg.injectInvalidSystemRate, "inject-invalid-system-rate", 0, "fraction of tasks routed to an invalid system")
	flag.StringVar(&cfg.invalidSystemName, "invalid-system-name", "missing-system-loadtest", "system name used for invalid-system injection")
	flag.Float64Var(&cfg.injectTimeoutSystemRate, "inject-timeout-system-rate", 0, "fraction of tasks routed to timeout system for retry-stress")
	flag.StringVar(&cfg.timeoutSystemName, "timeout-system-name", "loadtest-timeout-system", "AgentSystem name used for retry-stress injection")
	flag.StringVar(&cfg.timeoutAgentName, "timeout-agent-name", "loadtest-timeout-agent", "Agent name used by retry-stress timeout system")
	flag.StringVar(&cfg.timeoutAgentDuration, "timeout-agent-duration", "1ms", "timeout used by retry-stress agent limits.timeout")
	flag.Float64Var(&cfg.injectExpiredLeaseRate, "inject-expired-lease-rate", 0, "fraction of tasks patched to simulate worker crash/expired lease takeover")
	flag.StringVar(&cfg.expiredLeaseOwner, "expired-lease-owner", "worker-crashed-simulated", "synthetic worker id used for expired lease takeover injection")

	flag.StringVar(&cfg.qualityProfilePath, "quality-profile", "", "optional JSON profile that sets quality gates")
	flag.Float64Var(&cfg.minSuccessRate, "min-success-rate", 0, "minimum success rate gate (0 disables)")
	flag.Float64Var(&cfg.maxDeadletterRate, "max-deadletter-rate", -1, "maximum deadletter rate gate (-1 disables)")
	flag.Float64Var(&cfg.maxFailedRate, "max-failed-rate", -1, "maximum failed rate gate (-1 disables)")
	flag.IntVar(&cfg.maxTimedOut, "max-timed-out", 0, "maximum timed out tasks gate (-1 disables)")
	flag.IntVar(&cfg.minRetryTotal, "min-retry-total", -1, "minimum total retry count gate (-1 disables)")
	flag.IntVar(&cfg.minTakeoverEvents, "min-takeover-events", -1, "minimum takeover history event count gate (-1 disables)")

	flag.BoolVar(&cfg.emitJSON, "json", false, "emit machine-readable JSON report")
	flag.BoolVar(&cfg.verbose, "verbose", false, "print periodic progress")
	flag.Parse()
	return cfg
}

func loadQualityProfile(path string) (qualityProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return qualityProfile{}, err
	}
	var profile qualityProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return qualityProfile{}, fmt.Errorf("decode profile failed: %w", err)
	}
	if strings.TrimSpace(profile.Name) == "" {
		profile.Name = "default"
	}
	return profile, nil
}

func mergeQualityProfile(cfg loadConfig, profile qualityProfile) loadConfig {
	cfg.qualityProfileName = profile.Name
	if cfg.minSuccessRate <= 0 && profile.MinSuccessRate > 0 {
		cfg.minSuccessRate = profile.MinSuccessRate
	}
	if cfg.maxDeadletterRate < 0 && profile.MaxDeadletterRate >= 0 {
		cfg.maxDeadletterRate = profile.MaxDeadletterRate
	}
	if cfg.maxFailedRate < 0 && profile.MaxFailedRate >= 0 {
		cfg.maxFailedRate = profile.MaxFailedRate
	}
	if cfg.maxTimedOut < 0 && profile.MaxTimedOut >= 0 {
		cfg.maxTimedOut = profile.MaxTimedOut
	}
	if cfg.minRetryTotal < 0 && profile.MinRetryTotal >= 0 {
		cfg.minRetryTotal = profile.MinRetryTotal
	}
	if cfg.minTakeoverEvents < 0 && profile.MinTakeoverEvents >= 0 {
		cfg.minTakeoverEvents = profile.MinTakeoverEvents
	}
	return cfg
}

func validateConfig(cfg loadConfig) error {
	if strings.TrimSpace(cfg.baseURL) == "" {
		return fmt.Errorf("base-url is required")
	}
	if strings.TrimSpace(cfg.namespace) == "" {
		return fmt.Errorf("namespace is required")
	}
	if cfg.tasks <= 0 {
		return fmt.Errorf("tasks must be greater than zero")
	}
	if cfg.createConcurrency <= 0 {
		return fmt.Errorf("create-concurrency must be greater than zero")
	}
	if cfg.pollConcurrency <= 0 {
		return fmt.Errorf("poll-concurrency must be greater than zero")
	}
	if cfg.pollInterval <= 0 {
		return fmt.Errorf("poll-interval must be greater than zero")
	}
	if cfg.runTimeout <= 0 {
		return fmt.Errorf("run-timeout must be greater than zero")
	}
	if strings.TrimSpace(cfg.taskSystem) == "" {
		return fmt.Errorf("task-system is required")
	}
	if cfg.taskRetryAttempts <= 0 {
		return fmt.Errorf("task-retry-attempts must be greater than zero")
	}
	if _, err := time.ParseDuration(strings.TrimSpace(cfg.taskRetryBackoff)); err != nil {
		return fmt.Errorf("task-retry-backoff is invalid: %w", err)
	}
	if cfg.messageRetryPolicy.MaxAttempts <= 0 {
		return fmt.Errorf("message-retry-attempts must be greater than zero")
	}
	if _, err := time.ParseDuration(strings.TrimSpace(cfg.messageRetryPolicy.Backoff)); err != nil {
		return fmt.Errorf("message-retry-backoff is invalid: %w", err)
	}
	if _, err := time.ParseDuration(strings.TrimSpace(cfg.messageRetryPolicy.MaxBackoff)); err != nil {
		return fmt.Errorf("message-retry-max-backoff is invalid: %w", err)
	}
	jitter := strings.ToLower(strings.TrimSpace(cfg.messageRetryPolicy.Jitter))
	switch jitter {
	case "none", "full", "equal":
		cfg.messageRetryPolicy.Jitter = jitter
	default:
		return fmt.Errorf("message-retry-jitter must be one of none, full, equal")
	}

	if cfg.injectInvalidSystemRate < 0 || cfg.injectInvalidSystemRate > 1 {
		return fmt.Errorf("inject-invalid-system-rate must be between 0 and 1")
	}
	if cfg.injectTimeoutSystemRate < 0 || cfg.injectTimeoutSystemRate > 1 {
		return fmt.Errorf("inject-timeout-system-rate must be between 0 and 1")
	}
	if cfg.injectInvalidSystemRate+cfg.injectTimeoutSystemRate > 1 {
		return fmt.Errorf("inject-invalid-system-rate + inject-timeout-system-rate must be <= 1")
	}
	if cfg.injectExpiredLeaseRate < 0 || cfg.injectExpiredLeaseRate > 1 {
		return fmt.Errorf("inject-expired-lease-rate must be between 0 and 1")
	}
	if strings.TrimSpace(cfg.timeoutSystemName) == "" {
		return fmt.Errorf("timeout-system-name is required")
	}
	if strings.TrimSpace(cfg.timeoutAgentName) == "" {
		return fmt.Errorf("timeout-agent-name is required")
	}
	if _, err := time.ParseDuration(strings.TrimSpace(cfg.timeoutAgentDuration)); err != nil {
		return fmt.Errorf("timeout-agent-duration is invalid: %w", err)
	}
	if strings.TrimSpace(cfg.expiredLeaseOwner) == "" {
		return fmt.Errorf("expired-lease-owner is required")
	}

	if cfg.minSuccessRate < 0 || cfg.minSuccessRate > 1 {
		return fmt.Errorf("min-success-rate must be between 0 and 1")
	}
	if cfg.maxDeadletterRate > 1 {
		return fmt.Errorf("max-deadletter-rate must be <= 1")
	}
	if cfg.maxDeadletterRate < -1 {
		return fmt.Errorf("max-deadletter-rate must be -1 or between 0 and 1")
	}
	if cfg.maxFailedRate > 1 {
		return fmt.Errorf("max-failed-rate must be <= 1")
	}
	if cfg.maxFailedRate < -1 {
		return fmt.Errorf("max-failed-rate must be -1 or between 0 and 1")
	}
	if cfg.maxTimedOut < -1 {
		return fmt.Errorf("max-timed-out must be -1 or greater")
	}
	if cfg.minRetryTotal < -1 {
		return fmt.Errorf("min-retry-total must be -1 or greater")
	}
	if cfg.minTakeoverEvents < -1 {
		return fmt.Errorf("min-takeover-events must be -1 or greater")
	}
	return nil
}

func applyBaseline(ctx context.Context, client *apiClient, namespace string) error {
	for _, relPath := range baselineManifestFiles {
		data, err := os.ReadFile(relPath)
		if err != nil {
			return fmt.Errorf("read manifest %s failed: %w", relPath, err)
		}
		kind, err := resources.DetectKind(data)
		if err != nil {
			return fmt.Errorf("detect kind %s failed: %w", relPath, err)
		}
		endpoint, err := endpointForKind(kind)
		if err != nil {
			return fmt.Errorf("resolve endpoint for %s failed: %w", relPath, err)
		}
		if err := client.postRaw(ctx, endpoint, namespace, data); err != nil {
			return fmt.Errorf("apply %s failed: %w", relPath, err)
		}
		logf("applied %s", filepath.ToSlash(relPath))
	}
	return nil
}

func ensureTimeoutScenarioResources(ctx context.Context, client *apiClient, cfg loadConfig) error {
	agent := resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata: resources.ObjectMeta{
			Name:      cfg.timeoutAgentName,
			Namespace: cfg.namespace,
		},
		Spec: resources.AgentSpec{
			ModelRef: "openai-default",
			Prompt:   "Retry stress agent intentionally times out to exercise retry/deadletter behavior.",
			Limits:   resources.AgentLimits{MaxSteps: 2, Timeout: cfg.timeoutAgentDuration},
		},
	}
	system := resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata: resources.ObjectMeta{
			Name:      cfg.timeoutSystemName,
			Namespace: cfg.namespace,
		},
		Spec: resources.AgentSystemSpec{
			Agents: []string{cfg.timeoutAgentName},
		},
	}
	payloadAgent, err := json.Marshal(agent)
	if err != nil {
		return fmt.Errorf("marshal timeout agent failed: %w", err)
	}
	if err := client.postRaw(ctx, "agents", cfg.namespace, payloadAgent); err != nil {
		return fmt.Errorf("upsert timeout agent failed: %w", err)
	}
	payloadSystem, err := json.Marshal(system)
	if err != nil {
		return fmt.Errorf("marshal timeout system failed: %w", err)
	}
	if err := client.postRaw(ctx, "agent-systems", cfg.namespace, payloadSystem); err != nil {
		return fmt.Errorf("upsert timeout system failed: %w", err)
	}
	return nil
}

func ensureLoadtestModelEndpoint(ctx context.Context, client *apiClient, namespace string) error {
	const modelRefName = "openai-default"

	var existing resources.ModelEndpoint
	if err := client.getJSON(ctx, "model-endpoints/"+url.PathEscape(modelRefName), namespace, &existing); err == nil {
		return nil
	} else if !strings.Contains(err.Error(), "status=404") {
		return fmt.Errorf("lookup model endpoint %q failed: %w", modelRefName, err)
	}

	endpoint := resources.ModelEndpoint{
		APIVersion: "orloj.dev/v1",
		Kind:       "ModelEndpoint",
		Metadata: resources.ObjectMeta{
			Name:      modelRefName,
			Namespace: namespace,
		},
		Spec: resources.ModelEndpointSpec{
			Provider:     "mock",
			DefaultModel: "gpt-4o",
		},
	}
	payload, err := json.Marshal(endpoint)
	if err != nil {
		return fmt.Errorf("marshal model endpoint failed: %w", err)
	}
	if err := client.postRaw(ctx, "model-endpoints", namespace, payload); err != nil {
		return fmt.Errorf("create model endpoint %q failed: %w", modelRefName, err)
	}
	logf("created fallback model endpoint name=%s provider=mock default_model=gpt-4o", modelRefName)
	return nil
}

func endpointForKind(kind string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "agent":
		return "agents", nil
	case "agentsystem":
		return "agent-systems", nil
	case "modelendpoint":
		return "model-endpoints", nil
	case "tool":
		return "tools", nil
	case "secret":
		return "secrets", nil
	case "memory":
		return "memories", nil
	case "agentpolicy":
		return "agent-policies", nil
	case "agentrole":
		return "agent-roles", nil
	case "toolpermission":
		return "tool-permissions", nil
	case "task":
		return "tasks", nil
	case "taskschedule":
		return "task-schedules", nil
	case "worker":
		return "workers", nil
	default:
		return "", fmt.Errorf("unsupported kind %q", kind)
	}
}

func waitForReadyWorkers(
	ctx context.Context,
	client *apiClient,
	namespace string,
	minReady int,
	timeout time.Duration,
	pollInterval time.Duration,
) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		ready, total, err := client.countReadyWorkers(waitCtx, namespace)
		if err == nil && ready >= minReady {
			logf("worker readiness reached ready=%d total=%d", ready, total)
			return nil
		}
		if err == nil {
			logf("waiting for workers ready=%d total=%d min=%d", ready, total, minReady)
		}
		select {
		case <-waitCtx.Done():
			if err != nil {
				return fmt.Errorf("timed out while listing workers: %w", err)
			}
			return fmt.Errorf("ready workers %d below required minimum %d", ready, minReady)
		case <-ticker.C:
		}
	}
}

func createTasks(ctx context.Context, client *apiClient, cfg loadConfig, runID string) ([]taskRun, int, createStats) {
	jobs := make(chan int)
	results := make(chan taskRun, cfg.tasks)
	errCh := make(chan error, cfg.tasks)

	var wg sync.WaitGroup
	workerCount := cfg.createConcurrency
	if workerCount > cfg.tasks {
		workerCount = cfg.tasks
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	var randMu sync.Mutex

	for w := 0; w < workerCount; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				name := fmt.Sprintf("lt-%s-%04d", runID, idx+1)

				routeDraw := 0.0
				expiredLeaseDraw := 0.0
				randMu.Lock()
				routeDraw = rng.Float64()
				expiredLeaseDraw = rng.Float64()
				randMu.Unlock()

				system := cfg.taskSystem
				injectInvalid := false
				injectRetryStress := false
				injectExpiredLease := cfg.injectExpiredLeaseRate > 0 && expiredLeaseDraw < cfg.injectExpiredLeaseRate

				invalidCutoff := cfg.injectInvalidSystemRate
				retryStressCutoff := cfg.injectInvalidSystemRate + cfg.injectTimeoutSystemRate
				switch {
				case invalidCutoff > 0 && routeDraw < invalidCutoff:
					system = cfg.invalidSystemName
					injectInvalid = true
				case cfg.injectTimeoutSystemRate > 0 && routeDraw < retryStressCutoff:
					system = cfg.timeoutSystemName
					injectRetryStress = true
				}

				task := resources.Task{
					APIVersion: "orloj.dev/v1",
					Kind:       "Task",
					Metadata: resources.ObjectMeta{
						Name:      name,
						Namespace: cfg.namespace,
					},
					Spec: resources.TaskSpec{
						System:   system,
						Priority: cfg.taskPriority,
						Input: map[string]string{
							"topic": fmt.Sprintf("%s-%04d", cfg.topicPrefix, idx+1),
						},
						Retry: resources.TaskRetryPolicy{
							MaxAttempts: cfg.taskRetryAttempts,
							Backoff:     cfg.taskRetryBackoff,
						},
						MessageRetry: cfg.messageRetryPolicy,
						Requirements: resources.TaskRequirements{Region: "default", Model: "gpt-4o"},
					},
				}
				if err := client.createTask(ctx, cfg.namespace, task); err != nil {
					errCh <- fmt.Errorf("create task %s failed: %w", name, err)
					continue
				}

				results <- taskRun{
					name:               name,
					system:             system,
					created:            time.Now().UTC(),
					injectInvalid:      injectInvalid,
					injectRetryStress:  injectRetryStress,
					injectExpiredLease: injectExpiredLease,
				}
			}
		}()
	}

	go func() {
		for i := 0; i < cfg.tasks; i++ {
			jobs <- i
		}
		close(jobs)
		wg.Wait()
		close(results)
		close(errCh)
	}()

	runs := make([]taskRun, 0, cfg.tasks)
	createErrors := 0
	stats := createStats{}
	for run := range results {
		runs = append(runs, run)
		if run.injectInvalid {
			stats.invalidRequested++
		}
		if run.injectRetryStress {
			stats.retryStressRequested++
		}
		if run.injectExpiredLease {
			stats.expiredLeaseRequested++
		}
	}
	for err := range errCh {
		createErrors++
		logf("create error: %v", err)
	}
	logf(
		"task create completed created=%d errors=%d injected_invalid=%d injected_retry_stress=%d injected_expired_lease=%d",
		len(runs),
		createErrors,
		stats.invalidRequested,
		stats.retryStressRequested,
		stats.expiredLeaseRequested,
	)
	return runs, createErrors, stats
}

func applyExpiredLeaseSimulation(ctx context.Context, client *apiClient, cfg loadConfig, runs []taskRun) (int, int) {
	targets := make([]string, 0, len(runs))
	for _, run := range runs {
		if run.injectExpiredLease {
			targets = append(targets, run.name)
		}
	}
	if len(targets) == 0 {
		return 0, 0
	}

	jobs := make(chan string)
	results := make(chan error, len(targets))
	workerCount := cfg.pollConcurrency
	if workerCount > len(targets) {
		workerCount = len(targets)
	}
	if workerCount <= 0 {
		workerCount = 1
	}

	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for name := range jobs {
				results <- client.injectExpiredLease(ctx, cfg.namespace, name, cfg.expiredLeaseOwner)
			}
		}()
	}
	go func() {
		for _, name := range targets {
			jobs <- name
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	applied := 0
	errors := 0
	for err := range results {
		if err != nil {
			errors++
			logf("expired lease injection error: %v", err)
			continue
		}
		applied++
	}
	logf("expired lease injection completed requested=%d applied=%d errors=%d", len(targets), applied, errors)
	return applied, errors
}

func monitorTasks(ctx context.Context, client *apiClient, cfg loadConfig, runs []taskRun) ([]taskResult, int) {
	unfinished := make(map[string]taskRun, len(runs))
	for _, run := range runs {
		unfinished[run.name] = run
	}
	results := make([]taskResult, 0, len(runs))
	timedOut := 0
	ticker := time.NewTicker(cfg.pollInterval)
	defer ticker.Stop()

	for len(unfinished) > 0 {
		select {
		case <-ctx.Done():
			for _, run := range unfinished {
				timedOut++
				logf("task timed out name=%s", run.name)
			}
			return results, timedOut
		case <-ticker.C:
		}

		names := make([]string, 0, len(unfinished))
		for name := range unfinished {
			names = append(names, name)
		}
		if cfg.verbose {
			logf("polling task status remaining=%d", len(names))
		}

		type polled struct {
			name string
			task resources.Task
			err  error
		}
		polledCh := make(chan polled, len(names))
		sem := make(chan struct{}, cfg.pollConcurrency)
		var wg sync.WaitGroup
		for _, name := range names {
			name := name
			wg.Add(1)
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				task, err := client.getTask(ctx, cfg.namespace, name)
				<-sem
				polledCh <- polled{name: name, task: task, err: err}
			}()
		}
		wg.Wait()
		close(polledCh)

		for item := range polledCh {
			run, ok := unfinished[item.name]
			if !ok {
				continue
			}
			if item.err != nil {
				logf("poll warning task=%s err=%v", item.name, item.err)
				continue
			}
			phase := strings.ToLower(strings.TrimSpace(item.task.Status.Phase))
			if !isTaskTerminalPhase(phase) {
				continue
			}
			messageRetry, messageDeadletters := summarizeTaskMessages(item.task.Status.Messages)
			results = append(results, taskResult{
				name:               item.name,
				phase:              phase,
				duration:           time.Since(run.created),
				attempts:           item.task.Status.Attempts,
				taskRetries:        maxInt(item.task.Status.Attempts-1, 0),
				messageRetry:       messageRetry,
				deadletters:        messageDeadletters,
				takeovers:          countHistoryType(item.task.Status.History, "takeover"),
				assignmentCleared:  countHistoryType(item.task.Status.History, "assignment_cleared"),
				injectInvalid:      run.injectInvalid,
				injectRetryStress:  run.injectRetryStress,
				injectExpiredLease: run.injectExpiredLease,
				lastError:          strings.TrimSpace(item.task.Status.LastError),
			})
			delete(unfinished, item.name)
		}
	}
	return results, timedOut
}

func summarizeTaskMessages(messages []resources.TaskMessage) (int, int) {
	retries := 0
	deadletters := 0
	for _, msg := range messages {
		if msg.Attempts > 1 {
			retries += msg.Attempts - 1
		}
		if strings.EqualFold(strings.TrimSpace(msg.Phase), "deadletter") {
			deadletters++
		}
	}
	return retries, deadletters
}

func countHistoryType(history []resources.TaskHistoryEvent, eventType string) int {
	count := 0
	for _, item := range history {
		if strings.EqualFold(strings.TrimSpace(item.Type), strings.TrimSpace(eventType)) {
			count++
		}
	}
	return count
}

func isTaskTerminalPhase(phase string) bool {
	switch strings.ToLower(strings.TrimSpace(phase)) {
	case "succeeded", "failed", "deadletter":
		return true
	default:
		return false
	}
}

func computeRunMetrics(summary runSummary) runMetrics {
	metrics := runMetrics{}
	metrics.Terminal = summary.succeeded + summary.failed + summary.deadletter
	if metrics.Terminal > 0 {
		metrics.SuccessRate = float64(summary.succeeded) / float64(metrics.Terminal)
		metrics.DeadletterRate = float64(summary.deadletter) / float64(metrics.Terminal)
		metrics.FailedRate = float64(summary.failed) / float64(metrics.Terminal)
	}
	metrics.RetryTotal = summary.totalTaskRetry + summary.totalMessageRetry

	sortedDurations := make([]time.Duration, len(summary.durations))
	copy(sortedDurations, summary.durations)
	sort.Slice(sortedDurations, func(i, j int) bool { return sortedDurations[i] < sortedDurations[j] })
	metrics.DurationP50Ms = percentileDuration(sortedDurations, 50).Milliseconds()
	metrics.DurationP95Ms = percentileDuration(sortedDurations, 95).Milliseconds()
	if len(sortedDurations) > 0 {
		metrics.DurationMaxMs = sortedDurations[len(sortedDurations)-1].Milliseconds()
	}
	return metrics
}

func evaluateRun(summary runSummary, metrics runMetrics, cfg loadConfig) gateEvaluation {
	violations := make([]string, 0, 8)
	if metrics.Terminal == 0 {
		violations = append(violations, "no terminal task outcomes were observed")
	}
	if summary.createErrors > 0 {
		violations = append(violations, fmt.Sprintf("task creation errors=%d", summary.createErrors))
	}
	if summary.injectionErrors > 0 {
		violations = append(violations, fmt.Sprintf("failure injection errors=%d", summary.injectionErrors))
	}
	if cfg.maxTimedOut >= 0 && summary.timedOut > cfg.maxTimedOut {
		violations = append(violations, fmt.Sprintf("timed_out=%d above maximum %d", summary.timedOut, cfg.maxTimedOut))
	}
	if cfg.minSuccessRate > 0 && metrics.SuccessRate < cfg.minSuccessRate {
		violations = append(violations, fmt.Sprintf("success_rate %.3f below minimum %.3f", metrics.SuccessRate, cfg.minSuccessRate))
	}
	if cfg.maxDeadletterRate >= 0 && metrics.DeadletterRate > cfg.maxDeadletterRate {
		violations = append(violations, fmt.Sprintf("deadletter_rate %.3f above maximum %.3f", metrics.DeadletterRate, cfg.maxDeadletterRate))
	}
	if cfg.maxFailedRate >= 0 && metrics.FailedRate > cfg.maxFailedRate {
		violations = append(violations, fmt.Sprintf("failed_rate %.3f above maximum %.3f", metrics.FailedRate, cfg.maxFailedRate))
	}
	if cfg.minRetryTotal >= 0 && metrics.RetryTotal < cfg.minRetryTotal {
		violations = append(violations, fmt.Sprintf("retry_total=%d below minimum %d", metrics.RetryTotal, cfg.minRetryTotal))
	}
	if cfg.minTakeoverEvents >= 0 && summary.totalTakeovers < cfg.minTakeoverEvents {
		violations = append(violations, fmt.Sprintf("takeover_events=%d below minimum %d", summary.totalTakeovers, cfg.minTakeoverEvents))
	}
	return gateEvaluation{Passed: len(violations) == 0, Violations: violations}
}

func buildRunReport(runID string, cfg loadConfig, summary runSummary, metrics runMetrics, gates gateEvaluation) runReport {
	report := runReport{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		RunID:     runID,
		Metrics:   metrics,
		Gates:     gates,
	}
	report.Config.Namespace = cfg.namespace
	report.Config.Tasks = cfg.tasks
	report.Config.TaskSystem = cfg.taskSystem
	report.Config.Profile = cfg.qualityProfileName
	report.Config.Inject.InvalidSystemRate = cfg.injectInvalidSystemRate
	report.Config.Inject.RetryStressRate = cfg.injectTimeoutSystemRate
	report.Config.Inject.ExpiredLeaseRate = cfg.injectExpiredLeaseRate

	report.Summary = reportSummary{
		Requested:                     summary.requested,
		Created:                       summary.created,
		CreateErrors:                  summary.createErrors,
		TimedOut:                      summary.timedOut,
		Succeeded:                     summary.succeeded,
		Failed:                        summary.failed,
		Deadletter:                    summary.deadletter,
		InjectedInvalidRequested:      summary.injectedInvalidRequested,
		InjectedRetryStressRequested:  summary.injectedRetryStressRequested,
		InjectedExpiredLeaseRequested: summary.injectedExpiredLeaseRequested,
		InjectedExpiredLeaseApplied:   summary.injectedExpiredLeaseApplied,
		InjectionErrors:               summary.injectionErrors,
		InjectedTerminal:              summary.injectedTerminal,
		TotalTaskRetry:                summary.totalTaskRetry,
		TotalMessageRetry:             summary.totalMessageRetry,
		TotalMessageDeadletters:       summary.totalMessageDeadletters,
		TotalTakeovers:                summary.totalTakeovers,
		TotalAssignmentCleared:        summary.totalAssignmentCleared,
	}

	if len(summary.results) > 0 {
		sorted := make([]taskResult, len(summary.results))
		copy(sorted, summary.results)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].duration > sorted[j].duration })
		limit := 5
		if len(sorted) < limit {
			limit = len(sorted)
		}
		report.TopSlowest = make([]reportTask, 0, limit)
		for i := 0; i < limit; i++ {
			res := sorted[i]
			report.TopSlowest = append(report.TopSlowest, reportTask{
				Name:               res.name,
				Phase:              res.phase,
				DurationMs:         res.duration.Milliseconds(),
				Attempts:           res.attempts,
				TaskRetries:        res.taskRetries,
				MessageRetry:       res.messageRetry,
				Deadletters:        res.deadletters,
				Takeovers:          res.takeovers,
				AssignmentCleared:  res.assignmentCleared,
				InjectInvalid:      res.injectInvalid,
				InjectRetryStress:  res.injectRetryStress,
				InjectExpiredLease: res.injectExpiredLease,
				LastError:          res.lastError,
			})
		}
	}
	return report
}

func printSummary(summary runSummary, metrics runMetrics, gates gateEvaluation) {
	fmt.Println("\n=== Load Test Summary ===")
	fmt.Printf("requested=%d created=%d create_errors=%d terminal=%d timed_out=%d\n", summary.requested, summary.created, summary.createErrors, metrics.Terminal, summary.timedOut)
	fmt.Printf("succeeded=%d failed=%d deadletter=%d success_rate=%.3f failed_rate=%.3f deadletter_rate=%.3f\n", summary.succeeded, summary.failed, summary.deadletter, metrics.SuccessRate, metrics.FailedRate, metrics.DeadletterRate)
	fmt.Printf("task_retry_total=%d message_retry_total=%d retry_total=%d message_deadletters_total=%d\n", summary.totalTaskRetry, summary.totalMessageRetry, metrics.RetryTotal, summary.totalMessageDeadletters)
	fmt.Printf("takeovers_total=%d assignment_cleared_total=%d\n", summary.totalTakeovers, summary.totalAssignmentCleared)
	fmt.Printf("duration_p50=%s duration_p95=%s duration_max=%s\n", time.Duration(metrics.DurationP50Ms)*time.Millisecond, time.Duration(metrics.DurationP95Ms)*time.Millisecond, time.Duration(metrics.DurationMaxMs)*time.Millisecond)
	fmt.Printf(
		"injection_requested invalid=%d retry_stress=%d expired_lease=%d applied_expired_lease=%d injection_errors=%d terminal_with_injection=%d\n",
		summary.injectedInvalidRequested,
		summary.injectedRetryStressRequested,
		summary.injectedExpiredLeaseRequested,
		summary.injectedExpiredLeaseApplied,
		summary.injectionErrors,
		summary.injectedTerminal,
	)
	if gates.Passed {
		fmt.Println("quality_gates=passed")
	} else {
		fmt.Printf("quality_gates=failed violations=%d\n", len(gates.Violations))
		for _, violation := range gates.Violations {
			fmt.Printf("- %s\n", violation)
		}
	}

	if len(summary.results) == 0 {
		return
	}
	sorted := make([]taskResult, len(summary.results))
	copy(sorted, summary.results)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].duration > sorted[j].duration })
	limit := 5
	if len(sorted) < limit {
		limit = len(sorted)
	}
	fmt.Println("top_slowest_tasks:")
	for i := 0; i < limit; i++ {
		res := sorted[i]
		fmt.Printf(
			"  %d) name=%s phase=%s duration=%s attempts=%d task_retries=%d msg_retries=%d msg_deadletters=%d takeovers=%d injected_invalid=%t injected_retry_stress=%t injected_expired_lease=%t\n",
			i+1,
			res.name,
			res.phase,
			res.duration,
			res.attempts,
			res.taskRetries,
			res.messageRetry,
			res.deadletters,
			res.takeovers,
			res.injectInvalid,
			res.injectRetryStress,
			res.injectExpiredLease,
		)
	}
}

func percentileDuration(sortedDurations []time.Duration, percentile float64) time.Duration {
	if len(sortedDurations) == 0 {
		return 0
	}
	if percentile <= 0 {
		return sortedDurations[0]
	}
	if percentile >= 100 {
		return sortedDurations[len(sortedDurations)-1]
	}
	idx := int(math.Ceil((percentile/100.0)*float64(len(sortedDurations)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sortedDurations) {
		idx = len(sortedDurations) - 1
	}
	return sortedDurations[idx]
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (c *apiClient) createTask(ctx context.Context, namespace string, task resources.Task) error {
	payload, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("marshal task failed: %w", err)
	}
	return c.postRaw(ctx, "tasks", namespace, payload)
}

func (c *apiClient) getTask(ctx context.Context, namespace string, name string) (resources.Task, error) {
	path := fmt.Sprintf("tasks/%s", url.PathEscape(strings.TrimSpace(name)))
	var out resources.Task
	if err := c.getJSON(ctx, path, namespace, &out); err != nil {
		return resources.Task{}, err
	}
	return out, nil
}

func (c *apiClient) countReadyWorkers(ctx context.Context, namespace string) (int, int, error) {
	var out resources.WorkerList
	if err := c.getJSON(ctx, "workers", namespace, &out); err != nil {
		return 0, 0, err
	}
	ready := 0
	for _, item := range out.Items {
		if strings.EqualFold(strings.TrimSpace(item.Status.Phase), "ready") {
			ready++
		}
	}
	return ready, len(out.Items), nil
}

func (c *apiClient) injectExpiredLease(ctx context.Context, namespace, taskName, owner string) error {
	for attempt := 1; attempt <= 5; attempt++ {
		task, err := c.getTask(ctx, namespace, taskName)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		status := task.Status
		status.Phase = "Running"
		status.AssignedWorker = owner
		status.ClaimedBy = owner
		status.LeaseUntil = now.Add(-15 * time.Second).Format(time.RFC3339Nano)
		status.LastHeartbeat = now.Add(-30 * time.Second).Format(time.RFC3339Nano)
		if strings.TrimSpace(status.StartedAt) == "" {
			status.StartedAt = now.Add(-1 * time.Second).Format(time.RFC3339Nano)
		}
		if status.Attempts <= 0 {
			status.Attempts = 1
		}
		status.History = append(status.History, resources.TaskHistoryEvent{
			Timestamp: now.Format(time.RFC3339Nano),
			Type:      "failure_injection",
			Worker:    owner,
			Message:   "simulated worker crash with expired lease ownership",
		})
		if len(status.History) > 200 {
			status.History = status.History[len(status.History)-200:]
		}
		err = c.putTaskStatus(ctx, namespace, taskName, task.Metadata.ResourceVersion, status)
		if err == nil {
			return nil
		}
		if !isConflictErr(err) {
			return err
		}
		time.Sleep(time.Duration(attempt*15) * time.Millisecond)
	}
	return fmt.Errorf("task %s status update conflicted after retries", taskName)
}

func (c *apiClient) putTaskStatus(ctx context.Context, namespace, name, resourceVersion string, status resources.TaskStatus) error {
	path := fmt.Sprintf("tasks/%s/status", url.PathEscape(strings.TrimSpace(name)))
	body := map[string]any{
		"metadata": map[string]any{
			"resourceVersion": strings.TrimSpace(resourceVersion),
		},
		"status": status,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal task status patch failed: %w", err)
	}
	return c.putRaw(ctx, path, namespace, payload)
}

func isConflictErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "status=409")
}

func (c *apiClient) postRaw(ctx context.Context, endpoint string, namespace string, body []byte) error {
	endpoint = strings.Trim(strings.TrimSpace(endpoint), "/")
	if endpoint == "" {
		return fmt.Errorf("endpoint is required")
	}
	reqURL := fmt.Sprintf("%s/v1/%s", c.baseURL, endpoint)
	if strings.TrimSpace(namespace) != "" {
		reqURL += "?namespace=" + url.QueryEscape(strings.TrimSpace(namespace))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("POST %s status=%d: %s", reqURL, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return nil
}

func (c *apiClient) putRaw(ctx context.Context, path string, namespace string, body []byte) error {
	path = strings.Trim(strings.TrimSpace(path), "/")
	if path == "" {
		return fmt.Errorf("path is required")
	}
	reqURL := fmt.Sprintf("%s/v1/%s", c.baseURL, path)
	if strings.TrimSpace(namespace) != "" {
		sep := "?"
		if strings.Contains(reqURL, "?") {
			sep = "&"
		}
		reqURL += sep + "namespace=" + url.QueryEscape(strings.TrimSpace(namespace))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, reqURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("PUT %s status=%d: %s", reqURL, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return nil
}

func (c *apiClient) getJSON(ctx context.Context, path string, namespace string, out any) error {
	path = strings.Trim(strings.TrimSpace(path), "/")
	if path == "" {
		return fmt.Errorf("path is required")
	}
	reqURL := fmt.Sprintf("%s/v1/%s", c.baseURL, path)
	if strings.TrimSpace(namespace) != "" {
		sep := "?"
		if strings.Contains(reqURL, "?") {
			sep = "&"
		}
		reqURL += sep + "namespace=" + url.QueryEscape(strings.TrimSpace(namespace))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GET %s status=%d: %s", reqURL, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response failed: %w", err)
	}
	return nil
}

func logf(format string, args ...any) {
	fmt.Printf("[%s] %s\n", time.Now().UTC().Format(time.RFC3339), fmt.Sprintf(format, args...))
}

func fatalf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "[%s] ERROR: %s\n", time.Now().UTC().Format(time.RFC3339), msg)
	os.Exit(1)
}
