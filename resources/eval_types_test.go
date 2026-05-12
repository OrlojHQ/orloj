package resources

import (
	"testing"
)

func TestEvalDatasetNormalize(t *testing.T) {
	t.Parallel()

	t.Run("valid minimal dataset", func(t *testing.T) {
		ds := EvalDataset{
			Metadata: ObjectMeta{Name: "test-dataset"},
			Spec: EvalDatasetSpec{
				Samples: []EvalSample{
					{Name: "s1", Input: map[string]string{"prompt": "hello"}},
				},
			},
		}
		if err := ds.Normalize(); err != nil {
			t.Fatal(err)
		}
		if ds.APIVersion != "orloj.dev/v1" {
			t.Fatalf("expected orloj.dev/v1, got %q", ds.APIVersion)
		}
		if ds.Kind != "EvalDataset" {
			t.Fatalf("expected EvalDataset, got %q", ds.Kind)
		}
		if ds.Metadata.Namespace != DefaultNamespace {
			t.Fatalf("expected default namespace, got %q", ds.Metadata.Namespace)
		}
		if ds.Status.Phase != "Ready" {
			t.Fatalf("expected Ready phase, got %q", ds.Status.Phase)
		}
	})

	t.Run("empty name rejected", func(t *testing.T) {
		ds := EvalDataset{
			Metadata: ObjectMeta{Name: ""},
			Spec:     EvalDatasetSpec{Samples: []EvalSample{{Name: "s1", Input: map[string]string{"prompt": "a"}}}},
		}
		if err := ds.Normalize(); err == nil {
			t.Fatal("expected error for empty name")
		}
	})

	t.Run("empty samples rejected", func(t *testing.T) {
		ds := EvalDataset{
			Metadata: ObjectMeta{Name: "ds"},
			Spec:     EvalDatasetSpec{},
		}
		if err := ds.Normalize(); err == nil {
			t.Fatal("expected error for empty samples")
		}
	})

	t.Run("duplicate sample names rejected", func(t *testing.T) {
		ds := EvalDataset{
			Metadata: ObjectMeta{Name: "ds"},
			Spec: EvalDatasetSpec{
				Samples: []EvalSample{
					{Name: "s1", Input: map[string]string{"prompt": "a"}},
					{Name: "S1", Input: map[string]string{"prompt": "b"}},
				},
			},
		}
		if err := ds.Normalize(); err == nil {
			t.Fatal("expected error for duplicate sample names")
		}
	})

	t.Run("empty sample input rejected", func(t *testing.T) {
		ds := EvalDataset{
			Metadata: ObjectMeta{Name: "ds"},
			Spec: EvalDatasetSpec{
				Samples: []EvalSample{
					{Name: "s1", Input: nil},
				},
			},
		}
		if err := ds.Normalize(); err == nil {
			t.Fatal("expected error for empty input")
		}
	})

	t.Run("invalid regex rejected", func(t *testing.T) {
		ds := EvalDataset{
			Metadata: ObjectMeta{Name: "ds"},
			Spec: EvalDatasetSpec{
				Samples: []EvalSample{
					{Name: "s1", Input: map[string]string{"prompt": "a"}, Expected: EvalExpected{OutputMatches: "[invalid"}},
				},
			},
		}
		if err := ds.Normalize(); err == nil {
			t.Fatal("expected error for invalid regex")
		}
	})

	t.Run("invalid scoring strategy rejected", func(t *testing.T) {
		ds := EvalDataset{
			Metadata: ObjectMeta{Name: "ds"},
			Spec: EvalDatasetSpec{
				Samples: []EvalSample{
					{Name: "s1", Input: map[string]string{"prompt": "a"}, Scoring: &EvalScoringConfig{Strategy: "bogus"}},
				},
			},
		}
		if err := ds.Normalize(); err == nil {
			t.Fatal("expected error for invalid scoring strategy")
		}
	})

	t.Run("llm_judge requires model_ref", func(t *testing.T) {
		ds := EvalDataset{
			Metadata: ObjectMeta{Name: "ds"},
			Spec: EvalDatasetSpec{
				Samples: []EvalSample{
					{Name: "s1", Input: map[string]string{"prompt": "a"}, Scoring: &EvalScoringConfig{Strategy: "llm_judge"}},
				},
			},
		}
		if err := ds.Normalize(); err == nil {
			t.Fatal("expected error for llm_judge without model_ref")
		}
	})

	t.Run("custom requires tool_ref", func(t *testing.T) {
		ds := EvalDataset{
			Metadata: ObjectMeta{Name: "ds"},
			Spec: EvalDatasetSpec{
				Samples: []EvalSample{
					{Name: "s1", Input: map[string]string{"prompt": "a"}, Scoring: &EvalScoringConfig{Strategy: "custom"}},
				},
			},
		}
		if err := ds.Normalize(); err == nil {
			t.Fatal("expected error for custom without tool_ref")
		}
	})
}

func TestEvalRunNormalize(t *testing.T) {
	t.Parallel()

	t.Run("valid minimal run", func(t *testing.T) {
		run := EvalRun{
			Metadata: ObjectMeta{Name: "test-run"},
			Spec: EvalRunSpec{
				DatasetRef: "my-dataset",
				System:     "my-system",
			},
		}
		if err := run.Normalize(); err != nil {
			t.Fatal(err)
		}
		if run.Spec.Concurrency != 5 {
			t.Fatalf("expected concurrency 5, got %d", run.Spec.Concurrency)
		}
		if run.Status.Phase != EvalRunPhasePending {
			t.Fatalf("expected Pending phase, got %q", run.Status.Phase)
		}
	})

	t.Run("empty dataset_ref rejected", func(t *testing.T) {
		run := EvalRun{
			Metadata: ObjectMeta{Name: "run"},
			Spec:     EvalRunSpec{System: "my-system"},
		}
		if err := run.Normalize(); err == nil {
			t.Fatal("expected error for empty dataset_ref")
		}
	})

	t.Run("empty system rejected", func(t *testing.T) {
		run := EvalRun{
			Metadata: ObjectMeta{Name: "run"},
			Spec:     EvalRunSpec{DatasetRef: "ds"},
		}
		if err := run.Normalize(); err == nil {
			t.Fatal("expected error for empty system")
		}
	})

	t.Run("invalid timeout rejected", func(t *testing.T) {
		run := EvalRun{
			Metadata: ObjectMeta{Name: "run"},
			Spec: EvalRunSpec{
				DatasetRef: "ds",
				System:     "sys",
				Timeout:    "invalid",
			},
		}
		if err := run.Normalize(); err == nil {
			t.Fatal("expected error for invalid timeout")
		}
	})

	t.Run("valid timeout accepted", func(t *testing.T) {
		run := EvalRun{
			Metadata: ObjectMeta{Name: "run"},
			Spec: EvalRunSpec{
				DatasetRef: "ds",
				System:     "sys",
				Timeout:    "10m",
			},
		}
		if err := run.Normalize(); err != nil {
			t.Fatalf("expected valid timeout, got error: %v", err)
		}
	})

	t.Run("suspended field preserved", func(t *testing.T) {
		run := EvalRun{
			Metadata: ObjectMeta{Name: "run"},
			Spec: EvalRunSpec{
				DatasetRef: "ds",
				System:     "sys",
				Suspended:  true,
			},
		}
		if err := run.Normalize(); err != nil {
			t.Fatal(err)
		}
		if !run.Spec.Suspended {
			t.Fatal("expected Suspended to be preserved as true")
		}
	})

	t.Run("suspended defaults to false", func(t *testing.T) {
		run := EvalRun{
			Metadata: ObjectMeta{Name: "run"},
			Spec: EvalRunSpec{
				DatasetRef: "ds",
				System:     "sys",
			},
		}
		if err := run.Normalize(); err != nil {
			t.Fatal(err)
		}
		if run.Spec.Suspended {
			t.Fatal("expected Suspended to default to false")
		}
	})
}

func TestComputeEvalSummary(t *testing.T) {
	t.Parallel()

	t.Run("empty results", func(t *testing.T) {
		s := ComputeEvalSummary(nil)
		if s.PassRate != 0 || s.MeanScore != 0 {
			t.Fatalf("expected zeros for empty results")
		}
	})

	t.Run("mixed results", func(t *testing.T) {
		score1 := 1.0
		score2 := 0.5
		pass1 := true
		pass2 := false
		results := []EvalSampleResult{
			{Score: &score1, Pass: &pass1, Tokens: 100},
			{Score: &score2, Pass: &pass2, Tokens: 200},
		}
		s := ComputeEvalSummary(results)
		if s.PassRate != 0.5 {
			t.Fatalf("expected pass rate 0.5, got %f", s.PassRate)
		}
		expectedMean := (1.0 + 0.5) / 2.0
		if s.MeanScore != expectedMean {
			t.Fatalf("expected mean score %f, got %f", expectedMean, s.MeanScore)
		}
		if s.TotalTokens != 300 {
			t.Fatalf("expected 300 total tokens, got %d", s.TotalTokens)
		}
	})
}

func TestEvalExpectedIsEmpty(t *testing.T) {
	t.Parallel()

	if !((EvalExpected{}).IsEmpty()) {
		t.Fatal("zero value expected should be empty")
	}
	if (EvalExpected{OutputContains: "x"}).IsEmpty() {
		t.Fatal("non-zero expected should not be empty")
	}
}

func TestParseEvalDatasetManifest(t *testing.T) {
	t.Parallel()

	jsonData := []byte(`{
		"apiVersion": "orloj.dev/v1",
		"kind": "EvalDataset",
		"metadata": {"name": "test-ds"},
		"spec": {
			"samples": [
				{"name": "s1", "input": {"prompt": "hello"}, "expected": {"output_contains": "hi"}}
			]
		}
	}`)
	ds, err := ParseEvalDatasetManifest(jsonData)
	if err != nil {
		t.Fatal(err)
	}
	if ds.Metadata.Name != "test-ds" {
		t.Fatalf("expected test-ds, got %q", ds.Metadata.Name)
	}
	if len(ds.Spec.Samples) != 1 {
		t.Fatalf("expected 1 sample, got %d", len(ds.Spec.Samples))
	}
}

func TestParseEvalRunManifest(t *testing.T) {
	t.Parallel()

	jsonData := []byte(`{
		"apiVersion": "orloj.dev/v1",
		"kind": "EvalRun",
		"metadata": {"name": "test-run"},
		"spec": {
			"dataset_ref": "my-ds",
			"system": "my-sys",
			"scoring": {"strategy": "exact_match"},
			"concurrency": 3
		}
	}`)
	run, err := ParseEvalRunManifest(jsonData)
	if err != nil {
		t.Fatal(err)
	}
	if run.Spec.DatasetRef != "my-ds" {
		t.Fatalf("expected my-ds, got %q", run.Spec.DatasetRef)
	}
	if run.Spec.Concurrency != 3 {
		t.Fatalf("expected 3, got %d", run.Spec.Concurrency)
	}
}
