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
)

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
