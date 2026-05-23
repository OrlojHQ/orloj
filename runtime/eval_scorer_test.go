package agentruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

// ---------------------------------------------------------------------------
// exact_match strategy
// ---------------------------------------------------------------------------

func TestEvalScorer_ExactMatch(t *testing.T) {
	t.Parallel()
	scorer := &EvalScorer{}

	t.Run("output_contains match", func(t *testing.T) {
		result := scorer.Score(context.Background(), resources.EvalScoringConfig{Strategy: "exact_match"},
			resources.EvalExpected{OutputContains: "billing"},
			map[string]string{"prompt": "test"},
			"This is about billing issues")
		if result.Score == nil || *result.Score != 1.0 {
			t.Fatalf("expected score 1.0, got %v", result.Score)
		}
		if result.Pass == nil || !*result.Pass {
			t.Fatal("expected pass=true")
		}
	})

	t.Run("output_contains no match", func(t *testing.T) {
		result := scorer.Score(context.Background(), resources.EvalScoringConfig{Strategy: "exact_match"},
			resources.EvalExpected{OutputContains: "billing"},
			map[string]string{"prompt": "test"},
			"This is about shipping issues")
		if result.Score == nil || *result.Score != 0.0 {
			t.Fatalf("expected score 0.0, got %v", result.Score)
		}
		if result.Pass == nil || *result.Pass {
			t.Fatal("expected pass=false")
		}
	})

	t.Run("output_contains case insensitive", func(t *testing.T) {
		result := scorer.Score(context.Background(), resources.EvalScoringConfig{Strategy: "exact_match"},
			resources.EvalExpected{OutputContains: "BILLING"},
			map[string]string{"prompt": "test"},
			"This is about billing issues")
		if result.Score == nil || *result.Score != 1.0 {
			t.Fatalf("expected case-insensitive match, got score %v", result.Score)
		}
	})

	t.Run("output_not_contains pass", func(t *testing.T) {
		result := scorer.Score(context.Background(), resources.EvalScoringConfig{Strategy: "exact_match"},
			resources.EvalExpected{OutputNotContains: "error"},
			map[string]string{"prompt": "test"},
			"Everything went well")
		if result.Score == nil || *result.Score != 1.0 {
			t.Fatalf("expected score 1.0 when excluded term absent, got %v", result.Score)
		}
	})

	t.Run("output_not_contains fail", func(t *testing.T) {
		result := scorer.Score(context.Background(), resources.EvalScoringConfig{Strategy: "exact_match"},
			resources.EvalExpected{OutputNotContains: "error"},
			map[string]string{"prompt": "test"},
			"There was an error processing your request")
		if result.Score == nil || *result.Score != 0.0 {
			t.Fatalf("expected score 0.0 when excluded term present, got %v", result.Score)
		}
	})

	t.Run("output_matches regex match", func(t *testing.T) {
		result := scorer.Score(context.Background(), resources.EvalScoringConfig{Strategy: "exact_match"},
			resources.EvalExpected{OutputMatches: `order\s+#\d+`},
			map[string]string{"prompt": "test"},
			"Processing order #12345")
		if result.Score == nil || *result.Score != 1.0 {
			t.Fatalf("expected score 1.0, got %v", result.Score)
		}
	})

	t.Run("output_matches regex no match", func(t *testing.T) {
		result := scorer.Score(context.Background(), resources.EvalScoringConfig{Strategy: "exact_match"},
			resources.EvalExpected{OutputMatches: `order\s+#\d+`},
			map[string]string{"prompt": "test"},
			"No orders found")
		if result.Score == nil || *result.Score != 0.0 {
			t.Fatalf("expected score 0.0 for regex miss, got %v", result.Score)
		}
	})

	t.Run("combined conditions all must match", func(t *testing.T) {
		result := scorer.Score(context.Background(), resources.EvalScoringConfig{Strategy: "exact_match"},
			resources.EvalExpected{
				OutputContains:    "billing",
				OutputNotContains: "error",
			},
			map[string]string{"prompt": "test"},
			"This is about billing")
		if result.Score == nil || *result.Score != 1.0 {
			t.Fatalf("expected 1.0 when both conditions met, got %v", result.Score)
		}

		result2 := scorer.Score(context.Background(), resources.EvalScoringConfig{Strategy: "exact_match"},
			resources.EvalExpected{
				OutputContains:    "billing",
				OutputNotContains: "error",
			},
			map[string]string{"prompt": "test"},
			"billing error occurred")
		if result2.Score == nil || *result2.Score != 0.0 {
			t.Fatalf("expected 0.0 when not_contains fails, got %v", result2.Score)
		}
	})

	t.Run("json_path with equals", func(t *testing.T) {
		output := `{"category": "billing", "confidence": 0.95}`
		result := scorer.Score(context.Background(), resources.EvalScoringConfig{Strategy: "exact_match"},
			resources.EvalExpected{
				OutputJSONPath: "$.category",
				Equals:         "billing",
			},
			map[string]string{"prompt": "test"},
			output)
		if result.Score == nil || *result.Score != 1.0 {
			t.Fatalf("expected 1.0 for json_path equals match, got %v", result.Score)
		}
	})

	t.Run("json_path with not_equals", func(t *testing.T) {
		output := `{"category": "shipping"}`
		result := scorer.Score(context.Background(), resources.EvalScoringConfig{Strategy: "exact_match"},
			resources.EvalExpected{
				OutputJSONPath: "$.category",
				NotEquals:      "billing",
			},
			map[string]string{"prompt": "test"},
			output)
		if result.Score == nil || *result.Score != 1.0 {
			t.Fatalf("expected 1.0 for not_equals match, got %v", result.Score)
		}
	})

	t.Run("json_path equals fail", func(t *testing.T) {
		output := `{"category": "shipping"}`
		result := scorer.Score(context.Background(), resources.EvalScoringConfig{Strategy: "exact_match"},
			resources.EvalExpected{
				OutputJSONPath: "$.category",
				Equals:         "billing",
			},
			map[string]string{"prompt": "test"},
			output)
		if result.Score == nil || *result.Score != 0.0 {
			t.Fatalf("expected 0.0 for json_path equals mismatch, got %v", result.Score)
		}
	})

	t.Run("empty expected skips scoring", func(t *testing.T) {
		result := scorer.Score(context.Background(), resources.EvalScoringConfig{Strategy: "exact_match"},
			resources.EvalExpected{},
			map[string]string{"prompt": "test"},
			"any output")
		if result.Score != nil {
			t.Fatalf("expected nil score for empty expected, got %v", result.Score)
		}
	})

	t.Run("default strategy falls through to exact_match", func(t *testing.T) {
		result := scorer.Score(context.Background(), resources.EvalScoringConfig{},
			resources.EvalExpected{OutputContains: "hello"},
			map[string]string{"prompt": "test"},
			"hello world")
		if result.Score == nil || *result.Score != 1.0 {
			t.Fatalf("expected default strategy to behave as exact_match, got %v", result.Score)
		}
	})
}

// ---------------------------------------------------------------------------
// manual strategy
// ---------------------------------------------------------------------------

func TestEvalScorer_Manual(t *testing.T) {
	t.Parallel()
	scorer := &EvalScorer{}
	result := scorer.Score(context.Background(), resources.EvalScoringConfig{Strategy: "manual"},
		resources.EvalExpected{OutputContains: "billing"},
		map[string]string{"prompt": "test"},
		"output")
	if result.Score != nil || result.Pass != nil {
		t.Fatal("manual scoring should return nil score/pass")
	}
	if result.Error != "" {
		t.Fatalf("manual scoring should not error, got: %s", result.Error)
	}
}

// ---------------------------------------------------------------------------
// custom strategy
// ---------------------------------------------------------------------------

func TestEvalScorer_Custom(t *testing.T) {
	t.Parallel()
	scorer := &EvalScorer{}
	result := scorer.Score(context.Background(), resources.EvalScoringConfig{Strategy: "custom", ToolRef: "scorer-tool"},
		resources.EvalExpected{},
		map[string]string{"prompt": "test"},
		"output")
	if result.Error == "" {
		t.Fatal("custom scoring should return not-implemented error")
	}
}

// ---------------------------------------------------------------------------
// unknown strategy
// ---------------------------------------------------------------------------

func TestEvalScorer_UnknownStrategy(t *testing.T) {
	t.Parallel()
	scorer := &EvalScorer{}
	result := scorer.Score(context.Background(), resources.EvalScoringConfig{Strategy: "nonexistent"},
		resources.EvalExpected{},
		map[string]string{"prompt": "test"},
		"output")
	if result.Error == "" {
		t.Fatal("expected error for unknown strategy")
	}
}

// ---------------------------------------------------------------------------
// llm_judge strategy
// ---------------------------------------------------------------------------

func TestEvalScorer_LLMJudge_NoGateway(t *testing.T) {
	t.Parallel()
	scorer := &EvalScorer{Gateway: nil}
	result := scorer.Score(context.Background(), resources.EvalScoringConfig{Strategy: "llm_judge", ModelRef: "judge"},
		resources.EvalExpected{},
		map[string]string{"prompt": "test"},
		"output")
	if result.Error == "" {
		t.Fatal("expected error when no model gateway configured")
	}
}

type mockModelGateway struct {
	response ModelResponse
	err      error
	calls    int
}

func (m *mockModelGateway) Complete(ctx context.Context, req ModelRequest) (ModelResponse, error) {
	m.calls++
	return m.response, m.err
}

func TestEvalScorer_LLMJudge_Success(t *testing.T) {
	t.Parallel()

	verdict := map[string]any{
		"score":     0.85,
		"pass":      true,
		"reasoning": "Good response",
	}
	raw, _ := json.Marshal(verdict)

	gw := &mockModelGateway{
		response: ModelResponse{Content: string(raw)},
	}
	scorer := &EvalScorer{Gateway: gw}

	result := scorer.Score(context.Background(),
		resources.EvalScoringConfig{Strategy: "llm_judge", ModelRef: "judge-model", Rubric: "Be helpful"},
		resources.EvalExpected{},
		map[string]string{"prompt": "test input"},
		"test output")

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.Score == nil || *result.Score != 0.85 {
		t.Fatalf("expected score 0.85, got %v", result.Score)
	}
	if result.Pass == nil || !*result.Pass {
		t.Fatal("expected pass=true")
	}
	if result.Reasoning != "Good response" {
		t.Fatalf("expected reasoning 'Good response', got %q", result.Reasoning)
	}
	if gw.calls != 1 {
		t.Fatalf("expected 1 gateway call, got %d", gw.calls)
	}
}

func TestEvalScorer_LLMJudge_WithMarkdownWrapping(t *testing.T) {
	t.Parallel()

	content := "```json\n{\"score\": 0.7, \"pass\": true, \"reasoning\": \"decent\"}\n```"
	gw := &mockModelGateway{
		response: ModelResponse{Content: content},
	}
	scorer := &EvalScorer{Gateway: gw}

	result := scorer.Score(context.Background(),
		resources.EvalScoringConfig{Strategy: "llm_judge", ModelRef: "m"},
		resources.EvalExpected{},
		map[string]string{"prompt": "x"},
		"output")

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.Score == nil || *result.Score != 0.7 {
		t.Fatalf("expected score 0.7 with markdown wrapping, got %v", result.Score)
	}
}

func TestEvalScorer_LLMJudge_NonRetryableError(t *testing.T) {
	t.Parallel()

	gw := &mockModelGateway{
		err: fmt.Errorf("auth failed"),
	}
	scorer := &EvalScorer{Gateway: gw}

	result := scorer.Score(context.Background(),
		resources.EvalScoringConfig{Strategy: "llm_judge", ModelRef: "m"},
		resources.EvalExpected{},
		map[string]string{"prompt": "x"},
		"output")

	if result.Error == "" {
		t.Fatal("expected error for non-retryable gateway error")
	}
	if gw.calls != 1 {
		t.Fatalf("expected 1 call (no retries for non-retryable), got %d", gw.calls)
	}
}

func TestEvalScorer_LLMJudge_InvalidJSON(t *testing.T) {
	t.Parallel()

	gw := &mockModelGateway{
		response: ModelResponse{Content: "I think the score is about 0.8"},
	}
	scorer := &EvalScorer{Gateway: gw}

	result := scorer.Score(context.Background(),
		resources.EvalScoringConfig{Strategy: "llm_judge", ModelRef: "m"},
		resources.EvalExpected{},
		map[string]string{"prompt": "x"},
		"output")

	if result.Error == "" {
		t.Fatal("expected error for unparseable response")
	}
}

func TestEvalScorer_LLMJudge_DefaultRubric(t *testing.T) {
	t.Parallel()

	verdict := map[string]any{"score": 0.5, "pass": false, "reasoning": "ok"}
	raw, _ := json.Marshal(verdict)

	gw := &mockModelGateway{
		response: ModelResponse{Content: string(raw)},
	}
	scorer := &EvalScorer{Gateway: gw}

	result := scorer.Score(context.Background(),
		resources.EvalScoringConfig{Strategy: "llm_judge", ModelRef: "m"},
		resources.EvalExpected{},
		map[string]string{"prompt": "test"},
		"output")

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.Score == nil || *result.Score != 0.5 {
		t.Fatalf("expected score 0.5, got %v", result.Score)
	}
}
