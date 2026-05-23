package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	TaskDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "orloj",
		Name:      "task_duration_seconds",
		Help:      "Duration of task execution in seconds.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"namespace", "system", "status"})

	AgentStepDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "orloj",
		Name:      "agent_step_duration_seconds",
		Help:      "Duration of a single agent step in seconds.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"agent", "step_type"})

	TokensUsed = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "orloj",
		Name:      "tokens_used_total",
		Help:      "Total tokens consumed by agent executions.",
	}, []string{"agent", "model", "type"})

	MessagesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "orloj",
		Name:      "messages_total",
		Help:      "Total agent messages by phase.",
	}, []string{"phase", "agent"})

	DeadLettersTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "orloj",
		Name:      "deadletters_total",
		Help:      "Total messages moved to dead-letter.",
	}, []string{"agent"})

	RetriesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "orloj",
		Name:      "retries_total",
		Help:      "Total message retries.",
	}, []string{"agent"})

	InFlightMessages = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "orloj",
		Name:      "inflight_messages",
		Help:      "Current in-flight agent messages.",
	}, []string{"agent"})

	// Tool execution metrics (all tool types).
	ToolExecutionDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "orloj",
		Name:      "tool_execution_duration_seconds",
		Help:      "Duration of tool execution in seconds.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"tool", "type", "status"})

	// WASM-specific metrics.
	WASMFuelConsumed = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "orloj",
		Name:      "wasm_fuel_consumed",
		Help:      "Total fuel consumed by WASM tool executions.",
	}, []string{"tool"})

	WASMCompilationCacheHits = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "orloj",
		Name:      "wasm_compilation_cache_hits_total",
		Help:      "Total WASM module compilation cache hits.",
	}, []string{"tool"})

	WASMCompilationCacheMisses = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "orloj",
		Name:      "wasm_compilation_cache_misses_total",
		Help:      "Total WASM module compilation cache misses.",
	}, []string{"tool"})

	WASMModuleFetchDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "orloj",
		Name:      "wasm_module_fetch_duration_seconds",
		Help:      "Duration of remote WASM module fetches in seconds.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"source"})

	// A2A protocol metrics.
	A2AInboundRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "orloj",
		Name:      "a2a_inbound_requests_total",
		Help:      "Total inbound A2A JSON-RPC calls.",
	}, []string{"method", "status", "agent"})

	A2AInboundRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "orloj",
		Name:      "a2a_inbound_request_duration_seconds",
		Help:      "Latency of inbound A2A request handling.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"method", "agent"})

	A2AOutboundRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "orloj",
		Name:      "a2a_outbound_requests_total",
		Help:      "Total outbound A2A tool calls.",
	}, []string{"remote_agent", "status"})

	A2AOutboundRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "orloj",
		Name:      "a2a_outbound_request_duration_seconds",
		Help:      "Latency of outbound A2A calls.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"remote_agent"})

	A2ACardCacheHitsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "orloj",
		Name:      "a2a_card_cache_hits_total",
		Help:      "Agent Card cache hits.",
	})

	A2ACardCacheMissesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "orloj",
		Name:      "a2a_card_cache_misses_total",
		Help:      "Agent Card cache misses / refreshes.",
	})

	A2AActiveSubscriptions = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "orloj",
		Name:      "a2a_active_subscriptions",
		Help:      "Currently open A2A streaming subscribe connections.",
	}, []string{"agent"})
)

// RecordToolExecution records a tool invocation's duration and outcome.
func RecordToolExecution(tool, toolType, status string, durationSec float64) {
	ToolExecutionDuration.WithLabelValues(tool, toolType, status).Observe(durationSec)
}

// RecordWASMExecution records WASM-specific execution metrics.
func RecordWASMExecution(tool string, fuelConsumed uint64) {
	if fuelConsumed > 0 {
		WASMFuelConsumed.WithLabelValues(tool).Add(float64(fuelConsumed))
	}
}

// RecordWASMCacheHit increments the compilation cache hit counter.
func RecordWASMCacheHit(tool string) {
	WASMCompilationCacheHits.WithLabelValues(tool).Inc()
}

// RecordWASMCacheMiss increments the compilation cache miss counter.
func RecordWASMCacheMiss(tool string) {
	WASMCompilationCacheMisses.WithLabelValues(tool).Inc()
}

// RecordWASMModuleFetch records the duration of a remote module fetch.
func RecordWASMModuleFetch(source string, durationSec float64) {
	WASMModuleFetchDuration.WithLabelValues(source).Observe(durationSec)
}

// RecordA2AInbound records an inbound A2A JSON-RPC request.
func RecordA2AInbound(method, status, agent string, durationSec float64) {
	A2AInboundRequestsTotal.WithLabelValues(method, status, agent).Inc()
	A2AInboundRequestDuration.WithLabelValues(method, agent).Observe(durationSec)
}

// RecordA2AOutbound records an outbound A2A tool call.
func RecordA2AOutbound(remoteAgent, status string, durationSec float64) {
	A2AOutboundRequestsTotal.WithLabelValues(remoteAgent, status).Inc()
	A2AOutboundRequestDuration.WithLabelValues(remoteAgent).Observe(durationSec)
}

// RecordA2ACardCacheHit increments the card cache hit counter.
func RecordA2ACardCacheHit() { A2ACardCacheHitsTotal.Inc() }

// RecordA2ACardCacheMiss increments the card cache miss counter.
func RecordA2ACardCacheMiss() { A2ACardCacheMissesTotal.Inc() }

// RecordAgentExecution updates Prometheus counters after an agent finishes.
func RecordAgentExecution(agent, model string, durationSec float64, tokensUsed, tokensEstimated int) {
	AgentStepDuration.WithLabelValues(agent, "execute").Observe(durationSec)
	if tokensUsed > 0 {
		TokensUsed.WithLabelValues(agent, model, "used").Add(float64(tokensUsed))
	}
	if tokensEstimated > 0 {
		TokensUsed.WithLabelValues(agent, model, "estimated").Add(float64(tokensEstimated))
	}
}

// RecordTaskCompletion records task-level duration and status.
func RecordTaskCompletion(namespace, system, status string, durationSec float64) {
	TaskDuration.WithLabelValues(namespace, system, status).Observe(durationSec)
}

// RecordMessagePhase increments message counters.
func RecordMessagePhase(phase, agent string) {
	MessagesTotal.WithLabelValues(phase, agent).Inc()
}

// RecordDeadLetter increments the dead-letter counter.
func RecordDeadLetter(agent string) {
	DeadLettersTotal.WithLabelValues(agent).Inc()
}

// RecordRetry increments the retry counter.
func RecordRetry(agent string) {
	RetriesTotal.WithLabelValues(agent).Inc()
}
