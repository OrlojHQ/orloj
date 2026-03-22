package api

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/OrlojHQ/orloj/resources"
)

type taskMessageFilter struct {
	phases        map[string]struct{}
	fromAgent     string
	toAgent       string
	branchID      string
	traceID       string
	maxResults    int
	phaseFiltered bool
}

type taskMessageListResponse struct {
	Name            string             `json:"name"`
	Namespace       string             `json:"namespace"`
	Total           int                `json:"total"`
	FilteredFrom    int                `json:"filtered_from"`
	LifecycleCounts map[string]int     `json:"lifecycle_counts"`
	Messages        []resources.TaskMessage `json:"messages"`
}

type taskMessageTotals struct {
	Messages      int   `json:"messages"`
	Queued        int   `json:"queued"`
	Running       int   `json:"running"`
	RetryPending  int   `json:"retrypending"`
	Succeeded     int   `json:"succeeded"`
	DeadLetter    int   `json:"deadletter"`
	InFlight      int   `json:"in_flight"`
	RetryCount    int   `json:"retry_count"`
	DeadLetters   int   `json:"deadletters"`
	LatencyMSAvg  int64 `json:"latency_ms_avg"`
	LatencyMSP95  int64 `json:"latency_ms_p95"`
	LatencySample int   `json:"latency_sample_size"`
}

type taskMessageAgentMetric struct {
	Agent         string `json:"agent"`
	Inbound       int    `json:"inbound"`
	Outbound      int    `json:"outbound"`
	Queued        int    `json:"queued"`
	Running       int    `json:"running"`
	RetryPending  int    `json:"retrypending"`
	Succeeded     int    `json:"succeeded"`
	DeadLetter    int    `json:"deadletter"`
	InFlight      int    `json:"in_flight"`
	RetryCount    int    `json:"retry_count"`
	DeadLetters   int    `json:"deadletters"`
	LatencyMSAvg  int64  `json:"latency_ms_avg"`
	LatencyMSP95  int64  `json:"latency_ms_p95"`
	LatencySample int    `json:"latency_sample_size"`
}

type taskMessageEdgeMetric struct {
	FromAgent     string `json:"from_agent"`
	ToAgent       string `json:"to_agent"`
	Messages      int    `json:"messages"`
	Queued        int    `json:"queued"`
	Running       int    `json:"running"`
	RetryPending  int    `json:"retrypending"`
	Succeeded     int    `json:"succeeded"`
	DeadLetter    int    `json:"deadletter"`
	InFlight      int    `json:"in_flight"`
	RetryCount    int    `json:"retry_count"`
	DeadLetters   int    `json:"deadletters"`
	LatencyMSAvg  int64  `json:"latency_ms_avg"`
	LatencyMSP95  int64  `json:"latency_ms_p95"`
	LatencySample int    `json:"latency_sample_size"`
}

type taskMessageMetricsResponse struct {
	Name       string                   `json:"name"`
	Namespace  string                   `json:"namespace"`
	Generated  string                   `json:"generated_at"`
	Totals     taskMessageTotals        `json:"totals"`
	PerAgent   []taskMessageAgentMetric `json:"per_agent"`
	PerEdge    []taskMessageEdgeMetric  `json:"per_edge"`
	FilterInfo map[string]any           `json:"filters,omitempty"`
}

func (s *Server) getTaskMessages(w http.ResponseWriter, r *http.Request, name string) {
	filter, err := parseTaskMessageFilter(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	task, ok, err := s.stores.Tasks.Get(scopedNameForRequest(r, name))
	if writeStoreFetchError(w, err) { return }
	if !ok {
		http.Error(w, fmt.Sprintf("task %q not found", name), http.StatusNotFound)
		return
	}
	base := filterTaskMessages(task.Status.Messages, filter, false)
	selected := filterTaskMessages(base, filter, true)
	if filter.maxResults > 0 && len(selected) > filter.maxResults {
		selected = selected[len(selected)-filter.maxResults:]
	}
	resp := taskMessageListResponse{
		Name:            task.Metadata.Name,
		Namespace:       resources.NormalizeNamespace(task.Metadata.Namespace),
		Total:           len(selected),
		FilteredFrom:    len(base),
		LifecycleCounts: buildLifecycleCounts(base),
		Messages:        selected,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) getTaskMessageMetrics(w http.ResponseWriter, r *http.Request, name string) {
	filter, err := parseTaskMessageFilter(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	task, ok, err := s.stores.Tasks.Get(scopedNameForRequest(r, name))
	if writeStoreFetchError(w, err) { return }
	if !ok {
		http.Error(w, fmt.Sprintf("task %q not found", name), http.StatusNotFound)
		return
	}
	filtered := filterTaskMessages(task.Status.Messages, filter, false)
	filtered = filterTaskMessages(filtered, filter, true)
	if filter.maxResults > 0 && len(filtered) > filter.maxResults {
		filtered = filtered[len(filtered)-filter.maxResults:]
	}

	totals := taskMessageTotals{}
	latencies := make([]int64, 0, len(filtered))
	agentBuckets := make(map[string]*metricAccumulator, 8)
	edgeBuckets := make(map[string]*metricAccumulator, 8)
	for _, msg := range filtered {
		phase := normalizeTaskMessagePhase(msg.Phase)
		recordPhase(&totals.Queued, &totals.Running, &totals.RetryPending, &totals.Succeeded, &totals.DeadLetter, phase)
		totals.Messages++
		retries := maxInt(msg.Attempts-1, 0)
		totals.RetryCount += retries
		if phase == "deadletter" {
			totals.DeadLetters++
		}
		if lat, ok := taskMessageLatencyMS(msg); ok {
			latencies = append(latencies, lat)
		}

		to := strings.TrimSpace(msg.ToAgent)
		if to != "" {
			acc := getMetricAccumulator(agentBuckets, to)
			acc.inbound++
			recordPhase(&acc.queued, &acc.running, &acc.retryPending, &acc.succeeded, &acc.deadletter, phase)
			acc.retryCount += retries
			if phase == "deadletter" {
				acc.deadletters++
			}
			if lat, ok := taskMessageLatencyMS(msg); ok {
				acc.latencies = append(acc.latencies, lat)
			}
		}
		from := strings.TrimSpace(msg.FromAgent)
		if from != "" {
			acc := getMetricAccumulator(agentBuckets, from)
			acc.outbound++
		}

		edgeKey := strings.ToLower(from) + "->" + strings.ToLower(to)
		if edgeKey != "->" {
			acc := getMetricAccumulator(edgeBuckets, edgeKey)
			acc.from = from
			acc.to = to
			acc.messages++
			recordPhase(&acc.queued, &acc.running, &acc.retryPending, &acc.succeeded, &acc.deadletter, phase)
			acc.retryCount += retries
			if phase == "deadletter" {
				acc.deadletters++
			}
			if lat, ok := taskMessageLatencyMS(msg); ok {
				acc.latencies = append(acc.latencies, lat)
			}
		}
	}
	totals.InFlight = totals.Queued + totals.Running + totals.RetryPending
	totals.LatencyMSAvg, totals.LatencyMSP95, totals.LatencySample = summarizeLatencies(latencies)

	perAgent := make([]taskMessageAgentMetric, 0, len(agentBuckets))
	for name, acc := range agentBuckets {
		avg, p95, sample := summarizeLatencies(acc.latencies)
		perAgent = append(perAgent, taskMessageAgentMetric{
			Agent:         name,
			Inbound:       acc.inbound,
			Outbound:      acc.outbound,
			Queued:        acc.queued,
			Running:       acc.running,
			RetryPending:  acc.retryPending,
			Succeeded:     acc.succeeded,
			DeadLetter:    acc.deadletter,
			InFlight:      acc.queued + acc.running + acc.retryPending,
			RetryCount:    acc.retryCount,
			DeadLetters:   acc.deadletters,
			LatencyMSAvg:  avg,
			LatencyMSP95:  p95,
			LatencySample: sample,
		})
	}
	sort.Slice(perAgent, func(i, j int) bool {
		return strings.ToLower(perAgent[i].Agent) < strings.ToLower(perAgent[j].Agent)
	})

	perEdge := make([]taskMessageEdgeMetric, 0, len(edgeBuckets))
	for _, acc := range edgeBuckets {
		avg, p95, sample := summarizeLatencies(acc.latencies)
		perEdge = append(perEdge, taskMessageEdgeMetric{
			FromAgent:     acc.from,
			ToAgent:       acc.to,
			Messages:      acc.messages,
			Queued:        acc.queued,
			Running:       acc.running,
			RetryPending:  acc.retryPending,
			Succeeded:     acc.succeeded,
			DeadLetter:    acc.deadletter,
			InFlight:      acc.queued + acc.running + acc.retryPending,
			RetryCount:    acc.retryCount,
			DeadLetters:   acc.deadletters,
			LatencyMSAvg:  avg,
			LatencyMSP95:  p95,
			LatencySample: sample,
		})
	}
	sort.Slice(perEdge, func(i, j int) bool {
		left := strings.ToLower(perEdge[i].FromAgent) + "->" + strings.ToLower(perEdge[i].ToAgent)
		right := strings.ToLower(perEdge[j].FromAgent) + "->" + strings.ToLower(perEdge[j].ToAgent)
		return left < right
	})

	resp := taskMessageMetricsResponse{
		Name:      task.Metadata.Name,
		Namespace: resources.NormalizeNamespace(task.Metadata.Namespace),
		Generated: time.Now().UTC().Format(time.RFC3339Nano),
		Totals:    totals,
		PerAgent:  perAgent,
		PerEdge:   perEdge,
		FilterInfo: map[string]any{
			"phase_filtered": filter.phaseFiltered,
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

type metricAccumulator struct {
	from         string
	to           string
	inbound      int
	outbound     int
	messages     int
	queued       int
	running      int
	retryPending int
	succeeded    int
	deadletter   int
	retryCount   int
	deadletters  int
	latencies    []int64
}

func getMetricAccumulator(buckets map[string]*metricAccumulator, key string) *metricAccumulator {
	if acc, ok := buckets[key]; ok {
		return acc
	}
	acc := &metricAccumulator{}
	buckets[key] = acc
	return acc
}

func parseTaskMessageFilter(r *http.Request) (taskMessageFilter, error) {
	filter := taskMessageFilter{
		phases:     make(map[string]struct{}),
		maxResults: 0,
	}
	if r == nil {
		return filter, nil
	}
	query := r.URL.Query()
	rawPhase := strings.TrimSpace(query.Get("phase"))
	if rawPhase == "" {
		rawPhase = strings.TrimSpace(query.Get("lifecycle"))
	}
	if rawPhase != "" {
		for _, item := range strings.Split(rawPhase, ",") {
			value := normalizeTaskMessagePhase(item)
			if value == "" {
				continue
			}
			if !isKnownTaskMessagePhase(value) {
				return taskMessageFilter{}, fmt.Errorf("invalid phase %q: expected queued,running,retrypending,succeeded,deadletter", strings.TrimSpace(item))
			}
			filter.phases[value] = struct{}{}
		}
		filter.phaseFiltered = len(filter.phases) > 0
	}
	filter.fromAgent = strings.TrimSpace(query.Get("from_agent"))
	filter.toAgent = strings.TrimSpace(query.Get("to_agent"))
	filter.branchID = strings.TrimSpace(query.Get("branch_id"))
	filter.traceID = strings.TrimSpace(query.Get("trace_id"))
	if rawLimit := strings.TrimSpace(query.Get("limit")); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil || limit < 0 {
			return taskMessageFilter{}, fmt.Errorf("invalid limit %q: expected non-negative integer", rawLimit)
		}
		filter.maxResults = limit
	}
	return filter, nil
}

func filterTaskMessages(messages []resources.TaskMessage, filter taskMessageFilter, phaseOnly bool) []resources.TaskMessage {
	if len(messages) == 0 {
		return []resources.TaskMessage{}
	}
	out := make([]resources.TaskMessage, 0, len(messages))
	for _, msg := range messages {
		if !phaseOnly {
			if filter.fromAgent != "" && !strings.EqualFold(strings.TrimSpace(msg.FromAgent), filter.fromAgent) {
				continue
			}
			if filter.toAgent != "" && !strings.EqualFold(strings.TrimSpace(msg.ToAgent), filter.toAgent) {
				continue
			}
			if filter.branchID != "" && !strings.EqualFold(strings.TrimSpace(msg.BranchID), filter.branchID) {
				continue
			}
			if filter.traceID != "" && !strings.EqualFold(strings.TrimSpace(msg.TraceID), filter.traceID) {
				continue
			}
			out = append(out, msg)
			continue
		}
		if len(filter.phases) == 0 {
			out = append(out, msg)
			continue
		}
		phase := normalizeTaskMessagePhase(msg.Phase)
		if _, ok := filter.phases[phase]; ok {
			out = append(out, msg)
		}
	}
	return out
}

func buildLifecycleCounts(messages []resources.TaskMessage) map[string]int {
	counts := map[string]int{
		"queued":       0,
		"running":      0,
		"retrypending": 0,
		"succeeded":    0,
		"deadletter":   0,
	}
	for _, msg := range messages {
		phase := normalizeTaskMessagePhase(msg.Phase)
		if _, ok := counts[phase]; ok {
			counts[phase]++
		}
	}
	return counts
}

func isKnownTaskMessagePhase(phase string) bool {
	switch phase {
	case "queued", "running", "retrypending", "succeeded", "deadletter":
		return true
	default:
		return false
	}
}

func normalizeTaskMessagePhase(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	value = strings.ReplaceAll(value, "-", "")
	value = strings.ReplaceAll(value, "_", "")
	switch value {
	case "queued":
		return "queued"
	case "running":
		return "running"
	case "retrypending":
		return "retrypending"
	case "succeeded", "success":
		return "succeeded"
	case "deadletter":
		return "deadletter"
	default:
		return value
	}
}

func recordPhase(queued, running, retryPending, succeeded, deadletter *int, phase string) {
	switch phase {
	case "queued":
		*queued = *queued + 1
	case "running":
		*running = *running + 1
	case "retrypending":
		*retryPending = *retryPending + 1
	case "succeeded":
		*succeeded = *succeeded + 1
	case "deadletter":
		*deadletter = *deadletter + 1
	}
}

func taskMessageLatencyMS(msg resources.TaskMessage) (int64, bool) {
	start, ok := parseTaskMessageTime(msg.Timestamp)
	if !ok {
		return 0, false
	}
	end, ok := parseTaskMessageTime(msg.ProcessedAt)
	if !ok {
		return 0, false
	}
	if end.Before(start) {
		return 0, false
	}
	return end.Sub(start).Milliseconds(), true
}

func parseTaskMessageTime(raw string) (time.Time, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, false
	}
	ts, err := time.Parse(time.RFC3339Nano, value)
	if err == nil {
		return ts.UTC(), true
	}
	ts, err = time.Parse(time.RFC3339, value)
	if err == nil {
		return ts.UTC(), true
	}
	return time.Time{}, false
}

func summarizeLatencies(samples []int64) (avg int64, p95 int64, count int) {
	if len(samples) == 0 {
		return 0, 0, 0
	}
	count = len(samples)
	sorted := make([]int64, len(samples))
	copy(sorted, samples)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	var total int64
	for _, value := range sorted {
		total += value
	}
	avg = total / int64(len(sorted))
	idx := int(math.Ceil(float64(len(sorted))*0.95)) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	p95 = sorted[idx]
	return avg, p95, count
}

func maxInt(a, b int) int {
	if a >= b {
		return a
	}
	return b
}
