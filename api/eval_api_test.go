package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

func seedEvalDataset(t *testing.T, base string) {
	t.Helper()
	postJSON(t, base+"/v1/eval-datasets", resources.EvalDataset{
		APIVersion: "orloj.dev/v1",
		Kind:       "EvalDataset",
		Metadata:   resources.ObjectMeta{Name: "golden"},
		Spec: resources.EvalDatasetSpec{
			Description: "test dataset",
			Samples: []resources.EvalSample{
				{Name: "s1", Input: map[string]string{"prompt": "hello"}, Expected: resources.EvalExpected{OutputContains: "hi"}},
				{Name: "s2", Input: map[string]string{"prompt": "world"}},
			},
		},
	})
}

func seedEvalRun(t *testing.T, base string) {
	t.Helper()
	seedEvalDataset(t, base)
	postJSON(t, base+"/v1/eval-runs", resources.EvalRun{
		APIVersion: "orloj.dev/v1",
		Kind:       "EvalRun",
		Metadata:   resources.ObjectMeta{Name: "run-1"},
		Spec: resources.EvalRunSpec{
			DatasetRef: "golden",
			System:     "test-system",
			Scoring:    resources.EvalScoringConfig{Strategy: "exact_match"},
		},
	})
}

// ---------------------------------------------------------------------------
// EvalDataset CRUD
// ---------------------------------------------------------------------------

func TestEvalDatasetCRUD(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	seedEvalDataset(t, server.URL)

	resp, err := http.Get(server.URL + "/v1/eval-datasets/golden")
	if err != nil {
		t.Fatalf("get eval dataset: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	var ds resources.EvalDataset
	if err := json.NewDecoder(resp.Body).Decode(&ds); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if ds.Metadata.Name != "golden" {
		t.Fatalf("expected name golden, got %q", ds.Metadata.Name)
	}
	if len(ds.Spec.Samples) != 2 {
		t.Fatalf("expected 2 samples, got %d", len(ds.Spec.Samples))
	}

	listResp, err := http.Get(server.URL + "/v1/eval-datasets")
	if err != nil {
		t.Fatalf("list eval datasets: %v", err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(listResp.Body)
		t.Fatalf("expected 200, got %d: %s", listResp.StatusCode, b)
	}
	var list resources.EvalDatasetList
	if err := json.NewDecoder(listResp.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(list.Items))
	}

	req, _ := http.NewRequest(http.MethodDelete, server.URL+"/v1/eval-datasets/golden", nil)
	delResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	defer delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(delResp.Body)
		t.Fatalf("expected 204, got %d: %s", delResp.StatusCode, b)
	}

	getResp2, err := http.Get(server.URL + "/v1/eval-datasets/golden")
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	defer getResp2.Body.Close()
	if getResp2.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", getResp2.StatusCode)
	}
}

func TestEvalDatasetPostInvalid(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	body, _ := json.Marshal(resources.EvalDataset{
		APIVersion: "orloj.dev/v1",
		Kind:       "EvalDataset",
		Metadata:   resources.ObjectMeta{Name: "bad"},
		Spec:       resources.EvalDatasetSpec{},
	})
	resp, err := http.Post(server.URL+"/v1/eval-datasets", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 400 for empty samples, got %d: %s", resp.StatusCode, b)
	}
}

func TestEvalDatasetGetNotFound(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	resp, err := http.Get(server.URL + "/v1/eval-datasets/nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// EvalRun CRUD
// ---------------------------------------------------------------------------

func TestEvalRunCRUD(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	seedEvalRun(t, server.URL)

	resp, err := http.Get(server.URL + "/v1/eval-runs/run-1")
	if err != nil {
		t.Fatalf("get eval run: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	var run resources.EvalRun
	if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if run.Metadata.Name != "run-1" {
		t.Fatalf("expected name run-1, got %q", run.Metadata.Name)
	}
	if run.Spec.DatasetRef != "golden" {
		t.Fatalf("expected dataset golden, got %q", run.Spec.DatasetRef)
	}
	if run.Status.Phase != resources.EvalRunPhasePending {
		t.Fatalf("expected Pending phase, got %q", run.Status.Phase)
	}

	listResp, err := http.Get(server.URL + "/v1/eval-runs")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	defer listResp.Body.Close()
	var list resources.EvalRunList
	json.NewDecoder(listResp.Body).Decode(&list)
	if len(list.Items) != 1 {
		t.Fatalf("expected 1 run, got %d", len(list.Items))
	}

	req, _ := http.NewRequest(http.MethodDelete, server.URL+"/v1/eval-runs/run-1", nil)
	delResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete request failed: %v", err)
	}
	defer delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(delResp.Body)
		t.Fatalf("expected 204 delete, got %d: %s", delResp.StatusCode, b)
	}
}

func TestEvalRunPostInvalid(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	body, _ := json.Marshal(resources.EvalRun{
		APIVersion: "orloj.dev/v1",
		Kind:       "EvalRun",
		Metadata:   resources.ObjectMeta{Name: "bad"},
		Spec:       resources.EvalRunSpec{},
	})
	resp, err := http.Post(server.URL+"/v1/eval-runs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 400 for missing dataset, got %d: %s", resp.StatusCode, b)
	}
}

// ---------------------------------------------------------------------------
// Export
// ---------------------------------------------------------------------------

func TestEvalRunExportJSON(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	seedEvalRun(t, server.URL)

	resp, err := http.Get(server.URL + "/v1/eval-runs/run-1/export")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	var results []resources.EvalSampleResult
	json.NewDecoder(resp.Body).Decode(&results)
}

func TestEvalRunExportCSV(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	seedEvalRun(t, server.URL)

	resp, err := http.Get(server.URL + "/v1/eval-runs/run-1/export?format=csv")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/csv") {
		t.Fatalf("expected text/csv content type, got %q", ct)
	}
}

func TestEvalRunExportInvalidFormat(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	seedEvalRun(t, server.URL)

	resp, err := http.Get(server.URL + "/v1/eval-runs/run-1/export?format=xml")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for xml format, got %d", resp.StatusCode)
	}
}

func TestEvalRunExportNotFound(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	resp, err := http.Get(server.URL + "/v1/eval-runs/nonexistent/export")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Annotate
// ---------------------------------------------------------------------------

func TestEvalRunAnnotateSample(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	seedEvalRun(t, server.URL)

	// Push the run into a state with results so we can annotate.
	statusBody, _ := json.Marshal(map[string]any{
		"status": map[string]any{
			"phase": "PendingReview",
			"results": []map[string]any{
				{"sample_name": "s1", "output": "hi there"},
				{"sample_name": "s2", "output": "world"},
			},
		},
	})
	req, _ := http.NewRequest(http.MethodPut, server.URL+"/v1/eval-runs/run-1/status", bytes.NewReader(statusBody))
	req.Header.Set("Content-Type", "application/json")
	statusResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer statusResp.Body.Close()
	if statusResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(statusResp.Body)
		t.Fatalf("status update expected 200, got %d: %s", statusResp.StatusCode, b)
	}

	score := 0.9
	pass := true
	annotation, _ := json.Marshal(map[string]any{
		"score":   score,
		"pass":    pass,
		"comment": "looks good",
	})
	annReq, _ := http.NewRequest(http.MethodPut, server.URL+"/v1/eval-runs/run-1/results/s1", bytes.NewReader(annotation))
	annReq.Header.Set("Content-Type", "application/json")
	annResp, err := http.DefaultClient.Do(annReq)
	if err != nil {
		t.Fatal(err)
	}
	defer annResp.Body.Close()
	if annResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(annResp.Body)
		t.Fatalf("annotate expected 200, got %d: %s", annResp.StatusCode, b)
	}

	var updated resources.EvalRun
	json.NewDecoder(annResp.Body).Decode(&updated)
	for _, r := range updated.Status.Results {
		if r.SampleName == "s1" {
			if r.Score == nil || *r.Score != 0.9 {
				t.Fatalf("expected score 0.9, got %v", r.Score)
			}
			if r.Pass == nil || !*r.Pass {
				t.Fatal("expected pass=true")
			}
			if r.Comment != "looks good" {
				t.Fatalf("expected comment 'looks good', got %q", r.Comment)
			}
			return
		}
	}
	t.Fatal("sample s1 not found in results")
}

func TestEvalRunAnnotateSampleNotFound(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	seedEvalRun(t, server.URL)

	ann, _ := json.Marshal(map[string]any{"score": 0.5})
	req, _ := http.NewRequest(http.MethodPut, server.URL+"/v1/eval-runs/run-1/results/nonexistent", bytes.NewReader(ann))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("annotate request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for missing sample, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Bulk import
// ---------------------------------------------------------------------------

func TestEvalRunBulkImport(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	seedEvalRun(t, server.URL)

	statusBody, _ := json.Marshal(map[string]any{
		"status": map[string]any{
			"phase": "PendingReview",
			"results": []map[string]any{
				{"sample_name": "s1", "output": "hi"},
				{"sample_name": "s2", "output": "world"},
			},
		},
	})
	req, _ := http.NewRequest(http.MethodPut, server.URL+"/v1/eval-runs/run-1/status", bytes.NewReader(statusBody))
	req.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(req)

	score1 := 0.8
	pass1 := true
	score2 := 0.3
	pass2 := false
	annotations, _ := json.Marshal([]map[string]any{
		{"sample_name": "s1", "score": score1, "pass": pass1, "comment": "good"},
		{"sample_name": "s2", "score": score2, "pass": pass2, "comment": "poor"},
	})
	impReq, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/eval-runs/run-1/results", bytes.NewReader(annotations))
	impReq.Header.Set("Content-Type", "application/json")
	impResp, err := http.DefaultClient.Do(impReq)
	if err != nil {
		t.Fatal(err)
	}
	defer impResp.Body.Close()
	if impResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(impResp.Body)
		t.Fatalf("expected 200, got %d: %s", impResp.StatusCode, b)
	}

	var updated resources.EvalRun
	json.NewDecoder(impResp.Body).Decode(&updated)
	if len(updated.Status.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(updated.Status.Results))
	}
	for _, r := range updated.Status.Results {
		if r.SampleName == "s1" && (r.Score == nil || *r.Score != 0.8) {
			t.Fatalf("s1: expected score 0.8, got %v", r.Score)
		}
		if r.SampleName == "s2" && (r.Score == nil || *r.Score != 0.3) {
			t.Fatalf("s2: expected score 0.3, got %v", r.Score)
		}
	}
}

// ---------------------------------------------------------------------------
// Finalize
// ---------------------------------------------------------------------------

func TestEvalRunFinalize(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	seedEvalRun(t, server.URL)

	score := 1.0
	pass := true
	statusBody, _ := json.Marshal(map[string]any{
		"status": map[string]any{
			"phase": "PendingReview",
			"results": []map[string]any{
				{"sample_name": "s1", "output": "hi", "score": score, "pass": pass},
			},
		},
	})
	req, _ := http.NewRequest(http.MethodPut, server.URL+"/v1/eval-runs/run-1/status", bytes.NewReader(statusBody))
	req.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(req)

	finReq, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/eval-runs/run-1/finalize", nil)
	finResp, err := http.DefaultClient.Do(finReq)
	if err != nil {
		t.Fatal(err)
	}
	defer finResp.Body.Close()
	if finResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(finResp.Body)
		t.Fatalf("expected 200, got %d: %s", finResp.StatusCode, b)
	}

	var run resources.EvalRun
	json.NewDecoder(finResp.Body).Decode(&run)
	if run.Status.Phase != resources.EvalRunPhaseSucceeded {
		t.Fatalf("expected Succeeded after finalize, got %q", run.Status.Phase)
	}
	if run.Status.Summary.PassRate != 1.0 {
		t.Fatalf("expected pass rate 1.0, got %f", run.Status.Summary.PassRate)
	}
}

func TestEvalRunFinalizeWrongPhase(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	seedEvalRun(t, server.URL)

	finReq, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/eval-runs/run-1/finalize", nil)
	finResp, err := http.DefaultClient.Do(finReq)
	if err != nil {
		t.Fatalf("finalize request failed: %v", err)
	}
	defer finResp.Body.Close()
	if finResp.StatusCode != http.StatusConflict {
		b, _ := io.ReadAll(finResp.Body)
		t.Fatalf("expected 409 for finalize on Pending, got %d: %s", finResp.StatusCode, b)
	}
}

// ---------------------------------------------------------------------------
// Cancel
// ---------------------------------------------------------------------------

func TestEvalRunCancel(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	seedEvalRun(t, server.URL)

	cancelReq, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/eval-runs/run-1/cancel", nil)
	cancelResp, err := http.DefaultClient.Do(cancelReq)
	if err != nil {
		t.Fatal(err)
	}
	defer cancelResp.Body.Close()
	if cancelResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(cancelResp.Body)
		t.Fatalf("expected 200, got %d: %s", cancelResp.StatusCode, b)
	}
	var run resources.EvalRun
	json.NewDecoder(cancelResp.Body).Decode(&run)
	if run.Status.Phase != resources.EvalRunPhaseCancelled {
		t.Fatalf("expected Cancelled, got %q", run.Status.Phase)
	}
}

func TestEvalRunCancelTerminalPhase(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	seedEvalRun(t, server.URL)

	statusBody, _ := json.Marshal(map[string]any{
		"status": map[string]any{"phase": "Succeeded"},
	})
	req, _ := http.NewRequest(http.MethodPut, server.URL+"/v1/eval-runs/run-1/status", bytes.NewReader(statusBody))
	req.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(req)

	cancelReq, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/eval-runs/run-1/cancel", nil)
	cancelResp, err := http.DefaultClient.Do(cancelReq)
	if err != nil {
		t.Fatalf("cancel request failed: %v", err)
	}
	defer cancelResp.Body.Close()
	if cancelResp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 for cancel on Succeeded, got %d", cancelResp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Start / Suspended
// ---------------------------------------------------------------------------

func TestEvalRunReapplyResetsNonTerminalPhase(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	seedEvalRun(t, server.URL)

	// Push the run into Running phase (simulating a stuck eval run).
	statusBody, _ := json.Marshal(map[string]any{
		"status": map[string]any{
			"phase":        "Running",
			"totalSamples": 2,
			"results": []map[string]any{
				{"sample_name": "s1", "task_name": "eval-run-1-s1"},
				{"sample_name": "s2", "task_name": "eval-run-1-s2"},
			},
		},
	})
	req, _ := http.NewRequest(http.MethodPut, server.URL+"/v1/eval-runs/run-1/status", bytes.NewReader(statusBody))
	req.Header.Set("Content-Type", "application/json")
	statusResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer statusResp.Body.Close()
	if statusResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(statusResp.Body)
		t.Fatalf("status update expected 200, got %d: %s", statusResp.StatusCode, b)
	}

	// Re-apply the same eval run.
	body, _ := json.Marshal(resources.EvalRun{
		APIVersion: "orloj.dev/v1",
		Kind:       "EvalRun",
		Metadata:   resources.ObjectMeta{Name: "run-1"},
		Spec: resources.EvalRunSpec{
			DatasetRef: "golden",
			System:     "test-system",
			Scoring:    resources.EvalScoringConfig{Strategy: "exact_match"},
		},
	})
	resp, err := http.Post(server.URL+"/v1/eval-runs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, b)
	}

	var run resources.EvalRun
	json.NewDecoder(resp.Body).Decode(&run)
	if run.Status.Phase != resources.EvalRunPhasePending {
		t.Fatalf("expected re-apply of Running eval run to reset to Pending, got %q", run.Status.Phase)
	}
	if len(run.Status.Results) != 0 {
		t.Fatalf("expected results to be cleared on reset, got %d", len(run.Status.Results))
	}
}

func TestEvalRunReapplyPreservesSucceeded(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	seedEvalRun(t, server.URL)

	// Push the run into Succeeded phase.
	score := 1.0
	pass := true
	statusBody, _ := json.Marshal(map[string]any{
		"status": map[string]any{
			"phase": "Succeeded",
			"results": []map[string]any{
				{"sample_name": "s1", "score": score, "pass": pass, "output": "hi"},
			},
		},
	})
	req, _ := http.NewRequest(http.MethodPut, server.URL+"/v1/eval-runs/run-1/status", bytes.NewReader(statusBody))
	req.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(req)

	// Re-apply the same eval run.
	body, _ := json.Marshal(resources.EvalRun{
		APIVersion: "orloj.dev/v1",
		Kind:       "EvalRun",
		Metadata:   resources.ObjectMeta{Name: "run-1"},
		Spec: resources.EvalRunSpec{
			DatasetRef: "golden",
			System:     "test-system",
			Scoring:    resources.EvalScoringConfig{Strategy: "exact_match"},
		},
	})
	resp, err := http.Post(server.URL+"/v1/eval-runs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var run resources.EvalRun
	json.NewDecoder(resp.Body).Decode(&run)
	if run.Status.Phase != resources.EvalRunPhaseSucceeded {
		t.Fatalf("expected re-apply of Succeeded eval run to preserve phase, got %q", run.Status.Phase)
	}
	if len(run.Status.Results) != 1 {
		t.Fatalf("expected results to be preserved, got %d", len(run.Status.Results))
	}
}

func TestEvalRunPostDefaultsSuspended(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	seedEvalDataset(t, server.URL)

	body, _ := json.Marshal(resources.EvalRun{
		APIVersion: "orloj.dev/v1",
		Kind:       "EvalRun",
		Metadata:   resources.ObjectMeta{Name: "auto-suspended"},
		Spec: resources.EvalRunSpec{
			DatasetRef: "golden",
			System:     "test-system",
		},
	})
	resp, err := http.Post(server.URL+"/v1/eval-runs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, b)
	}
	var run resources.EvalRun
	json.NewDecoder(resp.Body).Decode(&run)
	if !run.Spec.Suspended {
		t.Fatal("expected new eval run to be suspended by default")
	}
}

func TestEvalRunPostWithRunQueryParam(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	seedEvalDataset(t, server.URL)

	body, _ := json.Marshal(resources.EvalRun{
		APIVersion: "orloj.dev/v1",
		Kind:       "EvalRun",
		Metadata:   resources.ObjectMeta{Name: "immediate-run"},
		Spec: resources.EvalRunSpec{
			DatasetRef: "golden",
			System:     "test-system",
		},
	})
	resp, err := http.Post(server.URL+"/v1/eval-runs?run=true", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, b)
	}
	var run resources.EvalRun
	json.NewDecoder(resp.Body).Decode(&run)
	if run.Spec.Suspended {
		t.Fatal("expected eval run with ?run=true to not be suspended")
	}
}

func TestEvalRunStart(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	seedEvalDataset(t, server.URL)

	body, _ := json.Marshal(resources.EvalRun{
		APIVersion: "orloj.dev/v1",
		Kind:       "EvalRun",
		Metadata:   resources.ObjectMeta{Name: "start-me"},
		Spec: resources.EvalRunSpec{
			DatasetRef: "golden",
			System:     "test-system",
		},
	})
	http.Post(server.URL+"/v1/eval-runs", "application/json", bytes.NewReader(body))

	startReq, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/eval-runs/start-me/start", nil)
	startResp, err := http.DefaultClient.Do(startReq)
	if err != nil {
		t.Fatal(err)
	}
	defer startResp.Body.Close()
	if startResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(startResp.Body)
		t.Fatalf("expected 200, got %d: %s", startResp.StatusCode, b)
	}
	var run resources.EvalRun
	json.NewDecoder(startResp.Body).Decode(&run)
	if run.Spec.Suspended {
		t.Fatal("expected suspended to be false after start")
	}
	if run.Status.Phase != resources.EvalRunPhasePending {
		t.Fatalf("expected Pending phase, got %q", run.Status.Phase)
	}
}

func TestEvalRunStartNotSuspended(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	seedEvalDataset(t, server.URL)

	body, _ := json.Marshal(resources.EvalRun{
		APIVersion: "orloj.dev/v1",
		Kind:       "EvalRun",
		Metadata:   resources.ObjectMeta{Name: "already-active"},
		Spec: resources.EvalRunSpec{
			DatasetRef: "golden",
			System:     "test-system",
		},
	})
	http.Post(server.URL+"/v1/eval-runs?run=true", "application/json", bytes.NewReader(body))

	startReq, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/eval-runs/already-active/start", nil)
	startResp, err := http.DefaultClient.Do(startReq)
	if err != nil {
		t.Fatal(err)
	}
	defer startResp.Body.Close()
	if startResp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 for start on non-suspended run, got %d", startResp.StatusCode)
	}
}

func TestEvalRunStartWrongPhase(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	seedEvalDataset(t, server.URL)

	body, _ := json.Marshal(resources.EvalRun{
		APIVersion: "orloj.dev/v1",
		Kind:       "EvalRun",
		Metadata:   resources.ObjectMeta{Name: "wrong-phase"},
		Spec: resources.EvalRunSpec{
			DatasetRef: "golden",
			System:     "test-system",
		},
	})
	http.Post(server.URL+"/v1/eval-runs", "application/json", bytes.NewReader(body))

	statusBody, _ := json.Marshal(map[string]any{
		"status": map[string]any{"phase": "Succeeded"},
	})
	req, _ := http.NewRequest(http.MethodPut, server.URL+"/v1/eval-runs/wrong-phase/status", bytes.NewReader(statusBody))
	req.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(req)

	startReq, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/eval-runs/wrong-phase/start", nil)
	startResp, err := http.DefaultClient.Do(startReq)
	if err != nil {
		t.Fatal(err)
	}
	defer startResp.Body.Close()
	if startResp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 for start on wrong phase, got %d", startResp.StatusCode)
	}
}

func TestEvalRunStartNotFound(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	startReq, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/eval-runs/nonexistent/start", nil)
	startResp, err := http.DefaultClient.Do(startReq)
	if err != nil {
		t.Fatal(err)
	}
	defer startResp.Body.Close()
	if startResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", startResp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Compare
// ---------------------------------------------------------------------------

func TestEvalRunCompare(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	seedEvalDataset(t, server.URL)

	for _, name := range []string{"run-a", "run-b"} {
		postJSON(t, server.URL+"/v1/eval-runs", resources.EvalRun{
			APIVersion: "orloj.dev/v1",
			Kind:       "EvalRun",
			Metadata:   resources.ObjectMeta{Name: name},
			Spec: resources.EvalRunSpec{
				DatasetRef: "golden",
				System:     "test-system",
			},
		})
		score := 0.9
		pass := true
		statusBody, _ := json.Marshal(map[string]any{
			"status": map[string]any{
				"phase": "Succeeded",
				"summary": map[string]any{
					"pass_rate":  0.8,
					"mean_score": 0.85,
				},
				"results": []map[string]any{
					{"sample_name": "s1", "score": score, "pass": pass},
				},
			},
		})
		req, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/v1/eval-runs/%s/status", server.URL, name), bytes.NewReader(statusBody))
		req.Header.Set("Content-Type", "application/json")
		http.DefaultClient.Do(req)
	}

	resp, err := http.Get(server.URL + "/v1/eval-runs/compare?runs=run-a,run-b")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	runs := result["runs"].([]any)
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs in comparison, got %d", len(runs))
	}
	summary := result["summary"].(map[string]any)
	if _, ok := summary["run-a"]; !ok {
		t.Fatal("expected run-a in summary")
	}
	if _, ok := summary["run-b"]; !ok {
		t.Fatal("expected run-b in summary")
	}
}

func TestEvalRunCompareTooFewRuns(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	resp, err := http.Get(server.URL + "/v1/eval-runs/compare?runs=only-one")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for single run compare, got %d", resp.StatusCode)
	}
}

func TestEvalRunCompareMissingParam(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	resp, err := http.Get(server.URL + "/v1/eval-runs/compare")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing runs param, got %d", resp.StatusCode)
	}
}

func TestEvalRunCompareNotFound(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	resp, err := http.Get(server.URL + "/v1/eval-runs/compare?runs=a,b")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for missing runs, got %d", resp.StatusCode)
	}
}
