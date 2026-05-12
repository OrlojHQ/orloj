package agentruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/OrlojHQ/orloj/resources"
)

// EvalScoreResult holds the outcome of scoring a single sample.
type EvalScoreResult struct {
	Score     *float64
	Pass      *bool
	Reasoning string
	Error     string
}

// EvalScorer runs the scoring pipeline for eval samples.
type EvalScorer struct {
	Gateway ModelGateway
}

// Score evaluates a single sample's output using the given scoring configuration.
// The output string is the agent system's response from task.status.output.
func (s *EvalScorer) Score(ctx context.Context, cfg resources.EvalScoringConfig, expected resources.EvalExpected, input map[string]string, output string) EvalScoreResult {
	strategy := strings.ToLower(strings.TrimSpace(cfg.Strategy))

	switch strategy {
	case resources.EvalScoringExactMatch, "":
		return s.scoreExactMatch(expected, output)
	case resources.EvalScoringLLMJudge:
		return s.scoreLLMJudge(ctx, cfg, expected, input, output)
	case resources.EvalScoringManual:
		return EvalScoreResult{}
	case resources.EvalScoringCustom:
		return EvalScoreResult{Error: "custom scoring not yet implemented"}
	default:
		return EvalScoreResult{Error: fmt.Sprintf("unknown scoring strategy %q", strategy)}
	}
}

// scoreExactMatch converts EvalExpected fields into EdgeCondition semantics and
// evaluates them against the output. Binary 0.0 or 1.0 score.
func (s *EvalScorer) scoreExactMatch(expected resources.EvalExpected, output string) EvalScoreResult {
	if expected.IsEmpty() {
		return EvalScoreResult{}
	}

	cond := &resources.EdgeCondition{
		OutputContains:    expected.OutputContains,
		OutputNotContains: expected.OutputNotContains,
		OutputMatches:     expected.OutputMatches,
		OutputJSONPath:    expected.OutputJSONPath,
		Equals:            expected.Equals,
		NotEquals:         expected.NotEquals,
		Contains:          expected.Contains,
		GreaterThan:       expected.GreaterThan,
		LessThan:          expected.LessThan,
	}

	matched := resources.EdgeConditionMatchesOutput(cond, output)
	score := 0.0
	pass := false
	if matched {
		score = 1.0
		pass = true
	}
	return EvalScoreResult{
		Score: &score,
		Pass:  &pass,
	}
}

const llmJudgeMaxRetries = 3

// scoreLLMJudge sends the input/output/rubric to a judge model for subjective scoring.
func (s *EvalScorer) scoreLLMJudge(ctx context.Context, cfg resources.EvalScoringConfig, expected resources.EvalExpected, input map[string]string, output string) EvalScoreResult {
	if s.Gateway == nil {
		return EvalScoreResult{Error: "model gateway not configured for llm_judge scoring"}
	}

	rubric := strings.TrimSpace(cfg.Rubric)
	if rubric == "" {
		rubric = "Evaluate the quality and correctness of the agent's response."
	}

	inputStr := ""
	if prompt, ok := input["prompt"]; ok {
		inputStr = prompt
	} else {
		raw, _ := json.Marshal(input)
		inputStr = string(raw)
	}

	judgePrompt := fmt.Sprintf(`You are an evaluation judge. Score the following agent response.

Input: %s

Agent Output: %s

Rubric: %s

Respond with ONLY a JSON object in this exact format:
{"score": <0.0-1.0>, "pass": <true/false>, "reasoning": "<brief explanation>"}`, inputStr, output, rubric)

	var lastErr error
	for attempt := 0; attempt < llmJudgeMaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return EvalScoreResult{Error: fmt.Sprintf("context cancelled during llm_judge retry: %v", ctx.Err())}
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}

		resp, err := s.Gateway.Complete(ctx, ModelRequest{
			ModelRef: cfg.ModelRef,
			Prompt:   judgePrompt,
			Messages: []ChatMessage{{Role: "user", Content: judgePrompt}},
		})
		if err != nil {
			_, retryable := IsModelGatewayError(err)
			if !retryable {
				return EvalScoreResult{Error: fmt.Sprintf("llm_judge model error: %v", err)}
			}
			lastErr = err
			continue
		}

		var verdict struct {
			Score     float64 `json:"score"`
			Pass      bool    `json:"pass"`
			Reasoning string  `json:"reasoning"`
		}

		content := strings.TrimSpace(resp.Content)
		start := strings.Index(content, "{")
		end := strings.LastIndex(content, "}")
		if start >= 0 && end > start {
			content = content[start : end+1]
		}

		if err := json.Unmarshal([]byte(content), &verdict); err != nil {
			return EvalScoreResult{Error: fmt.Sprintf("failed to parse llm_judge response: %v (raw: %s)", err, resp.Content)}
		}

		return EvalScoreResult{
			Score:     &verdict.Score,
			Pass:      &verdict.Pass,
			Reasoning: verdict.Reasoning,
		}
	}

	return EvalScoreResult{Error: fmt.Sprintf("llm_judge failed after %d retries: %v", llmJudgeMaxRetries, lastErr)}
}
