package resources

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	yaml "go.yaml.in/yaml/v2"
)

// Eval scoring strategy constants.
const (
	EvalScoringExactMatch = "exact_match"
	EvalScoringLLMJudge   = "llm_judge"
	EvalScoringManual     = "manual"
	EvalScoringCustom     = "custom"
)

// Eval run phase constants.
const (
	EvalRunPhasePending       = "Pending"
	EvalRunPhaseRunning       = "Running"
	EvalRunPhaseScoring       = "Scoring"
	EvalRunPhasePendingReview = "PendingReview"
	EvalRunPhaseSucceeded     = "Succeeded"
	EvalRunPhaseFailed        = "Failed"
	EvalRunPhaseCancelled     = "Cancelled"
)

// ---------------------------------------------------------------------------
// EvalDataset
// ---------------------------------------------------------------------------

// EvalDataset is a declarative list of (input, expected output) pairs.
type EvalDataset struct {
	APIVersion string            `json:"apiVersion" yaml:"apiVersion"`
	Kind       string            `json:"kind" yaml:"kind"`
	Metadata   ObjectMeta        `json:"metadata" yaml:"metadata"`
	Spec       EvalDatasetSpec   `json:"spec" yaml:"spec"`
	Status     EvalDatasetStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

type EvalDatasetSpec struct {
	Description string       `json:"description,omitempty" yaml:"description,omitempty"`
	Samples     []EvalSample `json:"samples" yaml:"samples"`
}

type EvalDatasetStatus struct {
	Phase string `json:"phase,omitempty" yaml:"phase,omitempty"`
}

type EvalDatasetList struct {
	ListMeta `json:",inline" yaml:",inline"`
	Items    []EvalDataset `json:"items" yaml:"items"`
}

// EvalSample represents a single evaluation sample within a dataset.
type EvalSample struct {
	Name     string            `json:"name" yaml:"name"`
	Input    map[string]string `json:"input" yaml:"input"`
	Expected EvalExpected      `json:"expected,omitempty" yaml:"expected,omitempty"`
	Scoring  *EvalScoringConfig `json:"scoring,omitempty" yaml:"scoring,omitempty"`
}

// EvalExpected mirrors EdgeCondition semantics for deterministic output matching.
type EvalExpected struct {
	OutputContains    string `json:"output_contains,omitempty" yaml:"output_contains,omitempty"`
	OutputNotContains string `json:"output_not_contains,omitempty" yaml:"output_not_contains,omitempty"`
	OutputMatches     string `json:"output_matches,omitempty" yaml:"output_matches,omitempty"`
	OutputJSONPath    string `json:"output_json_path,omitempty" yaml:"output_json_path,omitempty"`
	Equals            string `json:"equals,omitempty" yaml:"equals,omitempty"`
	NotEquals         string `json:"not_equals,omitempty" yaml:"not_equals,omitempty"`
	Contains          string `json:"contains,omitempty" yaml:"contains,omitempty"`
	GreaterThan       string `json:"greater_than,omitempty" yaml:"greater_than,omitempty"`
	LessThan          string `json:"less_than,omitempty" yaml:"less_than,omitempty"`
}

func (e EvalExpected) IsEmpty() bool {
	return e.OutputContains == "" && e.OutputNotContains == "" &&
		e.OutputMatches == "" && e.OutputJSONPath == "" &&
		e.Equals == "" && e.NotEquals == "" && e.Contains == "" &&
		e.GreaterThan == "" && e.LessThan == ""
}

// EvalScoringConfig configures how a sample or run is scored.
type EvalScoringConfig struct {
	Strategy string `json:"strategy,omitempty" yaml:"strategy,omitempty"`
	ModelRef string `json:"model_ref,omitempty" yaml:"model_ref,omitempty"`
	Rubric   string `json:"rubric,omitempty" yaml:"rubric,omitempty"`
	ToolRef  string `json:"tool_ref,omitempty" yaml:"tool_ref,omitempty"`
}

func (d *EvalDataset) Normalize() error {
	if d.APIVersion == "" {
		d.APIVersion = "orloj.dev/v1"
	}
	if d.Kind == "" {
		d.Kind = "EvalDataset"
	}
	if !strings.EqualFold(d.Kind, "EvalDataset") {
		return fmt.Errorf("unsupported kind %q for EvalDataset", d.Kind)
	}
	NormalizeObjectMetaNamespace(&d.Metadata)
	if err := ValidateMetadataName(d.Metadata.Name); err != nil {
		return err
	}
	if len(d.Spec.Samples) == 0 {
		return fmt.Errorf("spec.samples must contain at least one sample")
	}
	seen := make(map[string]struct{}, len(d.Spec.Samples))
	for i := range d.Spec.Samples {
		s := &d.Spec.Samples[i]
		s.Name = strings.TrimSpace(s.Name)
		if s.Name == "" {
			return fmt.Errorf("spec.samples[%d].name is required", i)
		}
		lower := strings.ToLower(s.Name)
		if _, exists := seen[lower]; exists {
			return fmt.Errorf("duplicate sample name %q in spec.samples", s.Name)
		}
		seen[lower] = struct{}{}
		if len(s.Input) == 0 {
			return fmt.Errorf("spec.samples[%d].input must not be empty", i)
		}
		if s.Expected.OutputMatches != "" {
			if _, err := regexp.Compile(s.Expected.OutputMatches); err != nil {
				return fmt.Errorf("spec.samples[%d].expected.output_matches: invalid regex: %w", i, err)
			}
		}
		if s.Scoring != nil {
			if err := validateScoringConfig(*s.Scoring); err != nil {
				return fmt.Errorf("spec.samples[%d].scoring: %w", i, err)
			}
		}
	}
	if d.Status.Phase == "" {
		d.Status.Phase = "Ready"
	}
	return nil
}

func ParseEvalDatasetManifest(data []byte) (EvalDataset, error) {
	if err := rejectMultiDocumentYAML(data); err != nil {
		return EvalDataset{}, err
	}
	var out EvalDataset
	if json.Valid(data) {
		if err := json.Unmarshal(data, &out); err != nil {
			return EvalDataset{}, fmt.Errorf("failed to decode JSON manifest: %w", err)
		}
	} else {
		if err := yaml.Unmarshal(data, &out); err != nil {
			return EvalDataset{}, fmt.Errorf("failed to decode YAML manifest: %w", err)
		}
	}
	if err := out.Normalize(); err != nil {
		return EvalDataset{}, err
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// EvalRun
// ---------------------------------------------------------------------------

// EvalRun represents a single evaluation execution.
type EvalRun struct {
	APIVersion string        `json:"apiVersion" yaml:"apiVersion"`
	Kind       string        `json:"kind" yaml:"kind"`
	Metadata   ObjectMeta    `json:"metadata" yaml:"metadata"`
	Spec       EvalRunSpec   `json:"spec" yaml:"spec"`
	Status     EvalRunStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

type EvalRunList struct {
	ListMeta `json:",inline" yaml:",inline"`
	Items    []EvalRun `json:"items" yaml:"items"`
}

type EvalRunSpec struct {
	DatasetRef            string                    `json:"dataset_ref" yaml:"dataset_ref"`
	System                string                    `json:"system" yaml:"system"`
	ModelEndpointOverride string                    `json:"model_endpoint_override,omitempty" yaml:"model_endpoint_override,omitempty"`
	AgentOverrides        map[string]AgentOverride  `json:"agent_overrides,omitempty" yaml:"agent_overrides,omitempty"`
	Scoring               EvalScoringConfig         `json:"scoring,omitempty" yaml:"scoring,omitempty"`
	Concurrency           int                       `json:"concurrency,omitempty" yaml:"concurrency,omitempty"`
	Timeout               string                    `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	Labels                map[string]string         `json:"labels,omitempty" yaml:"labels,omitempty"`
	Suspended             bool                      `json:"suspended,omitempty" yaml:"suspended,omitempty"`
}

// AgentOverride holds per-agent prompt/model overrides for A/B comparisons.
type AgentOverride struct {
	Prompt   string `json:"prompt,omitempty" yaml:"prompt,omitempty"`
	ModelRef string `json:"model_ref,omitempty" yaml:"model_ref,omitempty"`
}

type EvalRunStatus struct {
	Phase            string             `json:"phase,omitempty" yaml:"phase,omitempty"`
	TotalSamples     int                `json:"totalSamples,omitempty" yaml:"totalSamples,omitempty"`
	CompletedSamples int                `json:"completedSamples,omitempty" yaml:"completedSamples,omitempty"`
	PassedSamples    int                `json:"passedSamples,omitempty" yaml:"passedSamples,omitempty"`
	FailedSamples    int                `json:"failedSamples,omitempty" yaml:"failedSamples,omitempty"`
	ErroredSamples   int                `json:"erroredSamples,omitempty" yaml:"erroredSamples,omitempty"`
	Results          []EvalSampleResult `json:"results,omitempty" yaml:"results,omitempty"`
	Summary          EvalSummary        `json:"summary,omitempty" yaml:"summary,omitempty"`
	DatasetGeneration int64             `json:"datasetGeneration,omitempty" yaml:"datasetGeneration,omitempty"`
	StartedAt        string             `json:"startedAt,omitempty" yaml:"startedAt,omitempty"`
	CompletedAt      string             `json:"completedAt,omitempty" yaml:"completedAt,omitempty"`
	CancelledAt      string             `json:"cancelledAt,omitempty" yaml:"cancelledAt,omitempty"`
}

// EvalSampleResult holds the outcome for a single evaluated sample.
type EvalSampleResult struct {
	SampleName string   `json:"sample_name" yaml:"sample_name"`
	TaskName   string   `json:"task_name,omitempty" yaml:"task_name,omitempty"`
	Score      *float64 `json:"score" yaml:"score"`
	Pass       *bool    `json:"pass" yaml:"pass"`
	Error      string   `json:"error,omitempty" yaml:"error,omitempty"`
	Latency    string   `json:"latency,omitempty" yaml:"latency,omitempty"`
	Tokens     int      `json:"tokens,omitempty" yaml:"tokens,omitempty"`
	Output     string   `json:"output,omitempty" yaml:"output,omitempty"`
	Reasoning  string   `json:"reasoning,omitempty" yaml:"reasoning,omitempty"`
	Comment    string   `json:"comment,omitempty" yaml:"comment,omitempty"`
}

// EvalSummary holds aggregate metrics for a completed eval run.
type EvalSummary struct {
	PassRate    float64 `json:"pass_rate,omitempty" yaml:"pass_rate,omitempty"`
	MeanScore   float64 `json:"mean_score,omitempty" yaml:"mean_score,omitempty"`
	P50Latency  string  `json:"p50_latency,omitempty" yaml:"p50_latency,omitempty"`
	P95Latency  string  `json:"p95_latency,omitempty" yaml:"p95_latency,omitempty"`
	TotalTokens int     `json:"total_tokens,omitempty" yaml:"total_tokens,omitempty"`
	TotalCost   float64 `json:"total_cost,omitempty" yaml:"total_cost,omitempty"`
}

func (r *EvalRun) Normalize() error {
	if r.APIVersion == "" {
		r.APIVersion = "orloj.dev/v1"
	}
	if r.Kind == "" {
		r.Kind = "EvalRun"
	}
	if !strings.EqualFold(r.Kind, "EvalRun") {
		return fmt.Errorf("unsupported kind %q for EvalRun", r.Kind)
	}
	NormalizeObjectMetaNamespace(&r.Metadata)
	if err := ValidateMetadataName(r.Metadata.Name); err != nil {
		return err
	}
	r.Spec.DatasetRef = strings.TrimSpace(r.Spec.DatasetRef)
	if r.Spec.DatasetRef == "" {
		return fmt.Errorf("spec.dataset_ref is required")
	}
	r.Spec.System = strings.TrimSpace(r.Spec.System)
	if r.Spec.System == "" {
		return fmt.Errorf("spec.system is required")
	}
	r.Spec.ModelEndpointOverride = strings.TrimSpace(r.Spec.ModelEndpointOverride)
	if r.Spec.Concurrency <= 0 {
		r.Spec.Concurrency = 5
	}
	r.Spec.Timeout = strings.TrimSpace(r.Spec.Timeout)
	if r.Spec.Timeout != "" {
		if _, err := time.ParseDuration(r.Spec.Timeout); err != nil {
			return fmt.Errorf("invalid spec.timeout %q: %w", r.Spec.Timeout, err)
		}
	}
	if r.Spec.Scoring.Strategy != "" {
		if err := validateScoringConfig(r.Spec.Scoring); err != nil {
			return fmt.Errorf("spec.scoring: %w", err)
		}
	}
	if r.Status.Phase == "" {
		r.Status.Phase = EvalRunPhasePending
	}
	return nil
}

func ParseEvalRunManifest(data []byte) (EvalRun, error) {
	if err := rejectMultiDocumentYAML(data); err != nil {
		return EvalRun{}, err
	}
	var out EvalRun
	if json.Valid(data) {
		if err := json.Unmarshal(data, &out); err != nil {
			return EvalRun{}, fmt.Errorf("failed to decode JSON manifest: %w", err)
		}
	} else {
		if err := yaml.Unmarshal(data, &out); err != nil {
			return EvalRun{}, fmt.Errorf("failed to decode YAML manifest: %w", err)
		}
	}
	if err := out.Normalize(); err != nil {
		return EvalRun{}, err
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

var validScoringStrategies = map[string]bool{
	EvalScoringExactMatch: true,
	EvalScoringLLMJudge:   true,
	EvalScoringManual:     true,
	EvalScoringCustom:     true,
}

func validateScoringConfig(cfg EvalScoringConfig) error {
	strategy := strings.ToLower(strings.TrimSpace(cfg.Strategy))
	if strategy == "" {
		return fmt.Errorf("strategy is required")
	}
	if !validScoringStrategies[strategy] {
		return fmt.Errorf("invalid strategy %q: expected one of exact_match, llm_judge, manual, custom", cfg.Strategy)
	}
	if strategy == EvalScoringLLMJudge && strings.TrimSpace(cfg.ModelRef) == "" {
		return fmt.Errorf("model_ref is required for llm_judge strategy")
	}
	if strategy == EvalScoringCustom && strings.TrimSpace(cfg.ToolRef) == "" {
		return fmt.Errorf("tool_ref is required for custom strategy")
	}
	return nil
}

// ComputeEvalSummary calculates aggregate metrics from a set of sample results.
func ComputeEvalSummary(results []EvalSampleResult) EvalSummary {
	var summary EvalSummary
	if len(results) == 0 {
		return summary
	}
	var scored int
	var totalScore float64
	var passed int
	for _, r := range results {
		if r.Score != nil {
			scored++
			totalScore += *r.Score
		}
		if r.Pass != nil && *r.Pass {
			passed++
		}
		summary.TotalTokens += r.Tokens
	}
	if scored > 0 {
		summary.MeanScore = totalScore / float64(scored)
	}
	total := len(results)
	if total > 0 {
		summary.PassRate = float64(passed) / float64(total)
	}
	return summary
}
