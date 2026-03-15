package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/OrlojHQ/orloj/crds"
)

type alertConfig struct {
	baseURL         string
	namespace       string
	apiToken        string
	profilePath     string
	taskNamePrefix  string
	taskSystem      string
	pollConcurrency int
	timeout         time.Duration
	emitJSON        bool
	verbose         bool
}

type thresholdProfile struct {
	Name                    string  `json:"name"`
	MinTasks                int     `json:"min_tasks"`
	RetryStormTotal         int     `json:"retry_storm_total"`
	RetryStormRate          float64 `json:"retry_storm_rate"`
	DeadletterGrowthTotal   int     `json:"deadletter_growth_total"`
	DeadletterGrowthRate    float64 `json:"deadletter_growth_rate"`
	InFlightMax             int     `json:"in_flight_max"`
	LatencyP95MsMax         int64   `json:"latency_p95_ms_max"`
	RequireAnyTaskSucceeded bool    `json:"require_any_task_succeeded"`
}

type taskMetricsResponse struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Totals    struct {
		Messages     int   `json:"messages"`
		Queued       int   `json:"queued"`
		Running      int   `json:"running"`
		RetryPending int   `json:"retrypending"`
		Succeeded    int   `json:"succeeded"`
		DeadLetter   int   `json:"deadletter"`
		InFlight     int   `json:"in_flight"`
		RetryCount   int   `json:"retry_count"`
		DeadLetters  int   `json:"deadletters"`
		LatencyP95Ms int64 `json:"latency_ms_p95"`
	} `json:"totals"`
}

type taskAggregate struct {
	Tasks              int     `json:"tasks"`
	SucceededTasks     int     `json:"succeeded_tasks"`
	FailedTasks        int     `json:"failed_tasks"`
	DeadletterTasks    int     `json:"deadletter_tasks"`
	RetryCountTotal    int     `json:"retry_count_total"`
	DeadlettersTotal   int     `json:"deadletters_total"`
	InFlightTotal      int     `json:"in_flight_total"`
	LatencyP95MsMax    int64   `json:"latency_p95_ms_max"`
	RetryRatePerTask   float64 `json:"retry_rate_per_task"`
	DeadletterTaskRate float64 `json:"deadletter_task_rate"`
}

type alertViolation struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type alertReport struct {
	Timestamp  string           `json:"timestamp"`
	BaseURL    string           `json:"base_url"`
	Namespace  string           `json:"namespace"`
	Profile    string           `json:"profile"`
	Aggregate  taskAggregate    `json:"aggregate"`
	Violations []alertViolation `json:"violations"`
	Sample     []string         `json:"sample_tasks,omitempty"`
}

type apiClient struct {
	baseURL  string
	apiToken string
	http     *http.Client
}

func main() {
	cfg := parseFlags()
	if err := validateConfig(cfg); err != nil {
		fatalf("invalid config: %v", err)
	}
	profile, err := loadProfile(cfg.profilePath)
	if err != nil {
		fatalf("load profile failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()

	client := &apiClient{
		baseURL:  strings.TrimRight(strings.TrimSpace(cfg.baseURL), "/"),
		apiToken: strings.TrimSpace(cfg.apiToken),
		http:     &http.Client{Timeout: 20 * time.Second},
	}

	tasks, err := client.listTasks(ctx, cfg.namespace)
	if err != nil {
		fatalf("list tasks failed: %v", err)
	}
	filtered := filterTasks(tasks.Items, cfg)
	if cfg.verbose {
		logf("tasks listed=%d filtered=%d", len(tasks.Items), len(filtered))
	}

	aggregate, sample := collectAggregate(ctx, client, cfg, filtered)
	report := buildReport(cfg, profile, aggregate, sample)

	if cfg.emitJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(report)
	} else {
		printReport(report)
	}

	if len(report.Violations) > 0 {
		os.Exit(2)
	}
}

func parseFlags() alertConfig {
	cfg := alertConfig{}
	flag.StringVar(&cfg.baseURL, "base-url", "http://127.0.0.1:8080", "orloj API base URL")
	flag.StringVar(&cfg.namespace, "namespace", "default", "target namespace")
	flag.StringVar(&cfg.apiToken, "api-token", os.Getenv("ORLOJ_API_TOKEN"), "optional bearer token for API auth")
	flag.StringVar(&cfg.profilePath, "profile", "monitoring/alerts/retry-deadletter-default.json", "alert threshold profile JSON file")
	flag.StringVar(&cfg.taskNamePrefix, "task-name-prefix", "", "optional task metadata.name prefix filter")
	flag.StringVar(&cfg.taskSystem, "task-system", "", "optional Task.spec.system filter")
	flag.IntVar(&cfg.pollConcurrency, "poll-concurrency", 20, "concurrent task metrics fetch workers")
	flag.DurationVar(&cfg.timeout, "timeout", 2*time.Minute, "global command timeout")
	flag.BoolVar(&cfg.emitJSON, "json", true, "emit JSON output")
	flag.BoolVar(&cfg.verbose, "verbose", false, "verbose progress logs")
	flag.Parse()
	return cfg
}

func validateConfig(cfg alertConfig) error {
	if strings.TrimSpace(cfg.baseURL) == "" {
		return fmt.Errorf("base-url is required")
	}
	if strings.TrimSpace(cfg.namespace) == "" {
		return fmt.Errorf("namespace is required")
	}
	if cfg.pollConcurrency <= 0 {
		return fmt.Errorf("poll-concurrency must be > 0")
	}
	if cfg.timeout <= 0 {
		return fmt.Errorf("timeout must be > 0")
	}
	if strings.TrimSpace(cfg.profilePath) == "" {
		return fmt.Errorf("profile is required")
	}
	return nil
}

func loadProfile(path string) (thresholdProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return thresholdProfile{}, err
	}
	var profile thresholdProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return thresholdProfile{}, fmt.Errorf("decode profile failed: %w", err)
	}
	if strings.TrimSpace(profile.Name) == "" {
		profile.Name = "default"
	}
	return profile, nil
}

func filterTasks(tasks []crds.Task, cfg alertConfig) []crds.Task {
	out := make([]crds.Task, 0, len(tasks))
	prefix := strings.ToLower(strings.TrimSpace(cfg.taskNamePrefix))
	system := strings.ToLower(strings.TrimSpace(cfg.taskSystem))
	for _, task := range tasks {
		name := strings.ToLower(strings.TrimSpace(task.Metadata.Name))
		if prefix != "" && !strings.HasPrefix(name, prefix) {
			continue
		}
		if system != "" && strings.ToLower(strings.TrimSpace(task.Spec.System)) != system {
			continue
		}
		out = append(out, task)
	}
	return out
}

func collectAggregate(ctx context.Context, client *apiClient, cfg alertConfig, tasks []crds.Task) (taskAggregate, []string) {
	agg := taskAggregate{Tasks: len(tasks)}
	if len(tasks) == 0 {
		return agg, nil
	}

	type item struct {
		name    string
		phase   string
		metrics taskMetricsResponse
		err     error
	}
	jobs := make(chan crds.Task)
	results := make(chan item, len(tasks))

	workerCount := cfg.pollConcurrency
	if workerCount > len(tasks) {
		workerCount = len(tasks)
	}
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range jobs {
				m, err := client.getTaskMetrics(ctx, cfg.namespace, task.Metadata.Name)
				results <- item{
					name:    task.Metadata.Name,
					phase:   strings.ToLower(strings.TrimSpace(task.Status.Phase)),
					metrics: m,
					err:     err,
				}
			}
		}()
	}

	go func() {
		for _, task := range tasks {
			jobs <- task
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	sample := make([]string, 0, len(tasks))
	for res := range results {
		sample = append(sample, res.name)
		switch res.phase {
		case "succeeded":
			agg.SucceededTasks++
		case "deadletter":
			agg.DeadletterTasks++
		case "failed":
			agg.FailedTasks++
		}
		if res.err != nil {
			agg.FailedTasks++
			continue
		}
		agg.RetryCountTotal += res.metrics.Totals.RetryCount
		agg.DeadlettersTotal += res.metrics.Totals.DeadLetters
		agg.InFlightTotal += res.metrics.Totals.InFlight
		if res.metrics.Totals.LatencyP95Ms > agg.LatencyP95MsMax {
			agg.LatencyP95MsMax = res.metrics.Totals.LatencyP95Ms
		}
	}

	if agg.Tasks > 0 {
		agg.RetryRatePerTask = float64(agg.RetryCountTotal) / float64(agg.Tasks)
		agg.DeadletterTaskRate = float64(agg.DeadletterTasks) / float64(agg.Tasks)
	}
	sort.Strings(sample)
	if len(sample) > 10 {
		sample = sample[:10]
	}
	return agg, sample
}

func buildReport(cfg alertConfig, profile thresholdProfile, agg taskAggregate, sample []string) alertReport {
	violations := evaluateViolations(profile, agg)
	return alertReport{
		Timestamp:  time.Now().UTC().Format(time.RFC3339Nano),
		BaseURL:    cfg.baseURL,
		Namespace:  cfg.namespace,
		Profile:    profile.Name,
		Aggregate:  agg,
		Violations: violations,
		Sample:     sample,
	}
}

func evaluateViolations(profile thresholdProfile, agg taskAggregate) []alertViolation {
	violations := make([]alertViolation, 0, 6)
	if profile.MinTasks > 0 && agg.Tasks < profile.MinTasks {
		violations = append(violations, alertViolation{
			Code:     "insufficient_sample",
			Severity: "warning",
			Message:  fmt.Sprintf("task sample %d below minimum %d", agg.Tasks, profile.MinTasks),
		})
		return violations
	}
	if profile.RequireAnyTaskSucceeded && agg.SucceededTasks == 0 {
		violations = append(violations, alertViolation{
			Code:     "no_success",
			Severity: "critical",
			Message:  "no tasks reached Succeeded state",
		})
	}
	if profile.RetryStormTotal > 0 && agg.RetryCountTotal >= profile.RetryStormTotal {
		violations = append(violations, alertViolation{
			Code:     "retry_storm_total",
			Severity: "critical",
			Message:  fmt.Sprintf("retry_count_total=%d threshold=%d", agg.RetryCountTotal, profile.RetryStormTotal),
		})
	}
	if profile.RetryStormRate > 0 && agg.RetryRatePerTask >= profile.RetryStormRate {
		violations = append(violations, alertViolation{
			Code:     "retry_storm_rate",
			Severity: "critical",
			Message:  fmt.Sprintf("retry_rate_per_task=%.3f threshold=%.3f", agg.RetryRatePerTask, profile.RetryStormRate),
		})
	}
	if profile.DeadletterGrowthTotal > 0 && agg.DeadlettersTotal >= profile.DeadletterGrowthTotal {
		violations = append(violations, alertViolation{
			Code:     "deadletter_growth_total",
			Severity: "critical",
			Message:  fmt.Sprintf("deadletters_total=%d threshold=%d", agg.DeadlettersTotal, profile.DeadletterGrowthTotal),
		})
	}
	if profile.DeadletterGrowthRate > 0 && agg.DeadletterTaskRate >= profile.DeadletterGrowthRate {
		violations = append(violations, alertViolation{
			Code:     "deadletter_growth_rate",
			Severity: "critical",
			Message:  fmt.Sprintf("deadletter_task_rate=%.3f threshold=%.3f", agg.DeadletterTaskRate, profile.DeadletterGrowthRate),
		})
	}
	if profile.InFlightMax > 0 && agg.InFlightTotal >= profile.InFlightMax {
		violations = append(violations, alertViolation{
			Code:     "in_flight_high",
			Severity: "warning",
			Message:  fmt.Sprintf("in_flight_total=%d threshold=%d", agg.InFlightTotal, profile.InFlightMax),
		})
	}
	if profile.LatencyP95MsMax > 0 && agg.LatencyP95MsMax >= profile.LatencyP95MsMax {
		violations = append(violations, alertViolation{
			Code:     "latency_p95_high",
			Severity: "warning",
			Message:  fmt.Sprintf("latency_p95_ms_max=%d threshold=%d", agg.LatencyP95MsMax, profile.LatencyP95MsMax),
		})
	}
	return violations
}

func printReport(report alertReport) {
	fmt.Println("=== Alert Check Report ===")
	fmt.Printf("timestamp=%s namespace=%s profile=%s\n", report.Timestamp, report.Namespace, report.Profile)
	fmt.Printf("tasks=%d succeeded=%d failed=%d deadletter=%d\n", report.Aggregate.Tasks, report.Aggregate.SucceededTasks, report.Aggregate.FailedTasks, report.Aggregate.DeadletterTasks)
	fmt.Printf("retry_total=%d retry_rate=%.3f deadletters_total=%d deadletter_rate=%.3f\n", report.Aggregate.RetryCountTotal, report.Aggregate.RetryRatePerTask, report.Aggregate.DeadlettersTotal, report.Aggregate.DeadletterTaskRate)
	fmt.Printf("in_flight_total=%d latency_p95_ms_max=%d\n", report.Aggregate.InFlightTotal, report.Aggregate.LatencyP95MsMax)
	if len(report.Violations) == 0 {
		fmt.Println("status=ok")
		return
	}
	fmt.Printf("status=alert violations=%d\n", len(report.Violations))
	for _, v := range report.Violations {
		fmt.Printf("- [%s] %s: %s\n", strings.ToUpper(v.Severity), v.Code, v.Message)
	}
}

func (c *apiClient) listTasks(ctx context.Context, namespace string) (crds.TaskList, error) {
	var out crds.TaskList
	if err := c.getJSON(ctx, "tasks", namespace, &out); err != nil {
		return crds.TaskList{}, err
	}
	return out, nil
}

func (c *apiClient) getTaskMetrics(ctx context.Context, namespace, name string) (taskMetricsResponse, error) {
	path := fmt.Sprintf("tasks/%s/metrics", url.PathEscape(strings.TrimSpace(name)))
	var out taskMetricsResponse
	if err := c.getJSON(ctx, path, namespace, &out); err != nil {
		return taskMetricsResponse{}, err
	}
	return out, nil
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
	if strings.TrimSpace(c.apiToken) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.apiToken))
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
	fmt.Fprintf(os.Stderr, "[%s] ERROR: %s\n", time.Now().UTC().Format(time.RFC3339), fmt.Sprintf(format, args...))
	os.Exit(1)
}
