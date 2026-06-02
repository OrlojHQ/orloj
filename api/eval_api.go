package api

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/store"
)

// ---------------------------------------------------------------------------
// /v1/eval-datasets
// ---------------------------------------------------------------------------

func (s *Server) handleEvalDatasets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, cont, err := fetchListPage(r.Context(), r, s.stores.EvalDatasets.ListCursor, func(item resources.EvalDataset) resources.ObjectMeta { return item.Metadata })
		if writeListPageError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, resources.EvalDatasetList{ListMeta: resources.ListMeta{Continue: cont}, Items: items})
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := resources.ParseEvalDatasetManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		existing, ok, err := s.stores.EvalDatasets.Get(r.Context(), store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name))
		if writeStoreFetchError(w, err) {
			return
		}
		if ok {
			obj.Status = existing.Status
		}
		obj, err = s.stores.EvalDatasets.Upsert(r.Context(), obj)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		s.logApply("EvalDataset", obj.Metadata.Name)
		s.publishResourceEvent("EvalDataset", obj.Metadata.Name, "created", obj)
		writeJSON(w, http.StatusCreated, obj)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleEvalDatasetByName(w http.ResponseWriter, r *http.Request) {
	name := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/eval-datasets/"), "/")
	if name == "" {
		http.Error(w, "eval dataset name is required", http.StatusBadRequest)
		return
	}
	if strings.HasSuffix(name, "/status") {
		s.handleEvalDatasetStatus(w, r, strings.TrimSuffix(name, "/status"))
		return
	}
	key := scopedNameForRequest(r, name)
	switch r.Method {
	case http.MethodGet:
		obj, ok, err := s.stores.EvalDatasets.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("eval dataset %q not found", name), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, obj)
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := resources.ParseEvalDatasetManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		current, ok, err := s.stores.EvalDatasets.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("eval dataset %q not found", name), http.StatusNotFound)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &obj.Metadata, current.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		obj.Status = current.Status
		obj, err = s.stores.EvalDatasets.Upsert(r.Context(), obj)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		s.publishResourceEvent("EvalDataset", obj.Metadata.Name, "updated", obj)
		writeJSON(w, http.StatusOK, obj)
	case http.MethodDelete:
		if err := s.stores.EvalDatasets.Delete(r.Context(), key); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		s.publishResourceEvent("EvalDataset", name, "deleted", map[string]any{"metadata": map[string]string{"name": name, "namespace": requestNamespace(r)}})
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleEvalDatasetStatus(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	key := scopedNameForRequest(r, name)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	var statusUpdate struct {
		Status resources.EvalDatasetStatus `json:"status"`
	}
	if err := json.Unmarshal(body, &statusUpdate); err != nil {
		http.Error(w, fmt.Sprintf("invalid status update: %v", err), http.StatusBadRequest)
		return
	}
	obj, ok, err := s.stores.EvalDatasets.Get(r.Context(), key)
	if writeStoreFetchError(w, err) {
		return
	}
	if !ok {
		http.Error(w, fmt.Sprintf("eval dataset %q not found", name), http.StatusNotFound)
		return
	}
	obj.Status = statusUpdate.Status
	obj, err = s.stores.EvalDatasets.Upsert(r.Context(), obj)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	s.publishResourceEvent("EvalDataset", obj.Metadata.Name, "updated", obj)
	writeJSON(w, http.StatusOK, obj)
}

// ---------------------------------------------------------------------------
// /v1/eval-runs
// ---------------------------------------------------------------------------

func (s *Server) handleEvalRuns(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, cont, err := fetchListPage(r.Context(), r, s.stores.EvalRuns.ListCursor, func(item resources.EvalRun) resources.ObjectMeta { return item.Metadata })
		if writeListPageError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, resources.EvalRunList{ListMeta: resources.ListMeta{Continue: cont}, Items: items})
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := resources.ParseEvalRunManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		existing, ok, err := s.stores.EvalRuns.Get(r.Context(), store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name))
		if writeStoreFetchError(w, err) {
			return
		}
		if ok {
			switch existing.Status.Phase {
			case resources.EvalRunPhaseSucceeded, resources.EvalRunPhasePendingReview:
				obj.Status = existing.Status
			default:
				obj.Status = resources.EvalRunStatus{Phase: resources.EvalRunPhasePending}
			}
		} else if r.URL.Query().Get("run") != "true" {
			obj.Spec.Suspended = true
		}
		obj, err = s.stores.EvalRuns.Upsert(r.Context(), obj)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		s.logApply("EvalRun", obj.Metadata.Name)
		s.publishResourceEvent("EvalRun", obj.Metadata.Name, "created", obj)
		writeJSON(w, http.StatusCreated, obj)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleEvalRunByName(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/eval-runs/"), "/")
	if path == "" {
		http.Error(w, "eval run name is required", http.StatusBadRequest)
		return
	}

	if strings.HasSuffix(path, "/status") {
		name := strings.TrimSuffix(path, "/status")
		s.handleEvalRunStatus(w, r, name)
		return
	}
	if strings.HasSuffix(path, "/export") {
		name := strings.TrimSuffix(path, "/export")
		s.handleEvalRunExport(w, r, name)
		return
	}
	if strings.HasSuffix(path, "/finalize") {
		name := strings.TrimSuffix(path, "/finalize")
		s.handleEvalRunFinalize(w, r, name)
		return
	}
	if strings.HasSuffix(path, "/start") {
		name := strings.TrimSuffix(path, "/start")
		s.handleEvalRunStart(w, r, name)
		return
	}
	if strings.HasSuffix(path, "/cancel") {
		name := strings.TrimSuffix(path, "/cancel")
		s.handleEvalRunCancel(w, r, name)
		return
	}
	if strings.Contains(path, "/results") {
		parts := strings.SplitN(path, "/results", 2)
		name := parts[0]
		rest := ""
		if len(parts) > 1 {
			rest = strings.Trim(parts[1], "/")
		}
		if rest != "" {
			s.handleEvalRunAnnotateSample(w, r, name, rest)
		} else {
			s.handleEvalRunBulkImport(w, r, name)
		}
		return
	}

	name := path
	key := scopedNameForRequest(r, name)
	switch r.Method {
	case http.MethodGet:
		obj, ok, err := s.stores.EvalRuns.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("eval run %q not found", name), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, obj)
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := resources.ParseEvalRunManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		current, ok, err := s.stores.EvalRuns.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("eval run %q not found", name), http.StatusNotFound)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &obj.Metadata, current.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		obj.Status = current.Status
		obj, err = s.stores.EvalRuns.Upsert(r.Context(), obj)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		s.publishResourceEvent("EvalRun", obj.Metadata.Name, "updated", obj)
		writeJSON(w, http.StatusOK, obj)
	case http.MethodDelete:
		if err := s.stores.EvalRuns.Delete(r.Context(), key); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		s.publishResourceEvent("EvalRun", name, "deleted", map[string]any{"metadata": map[string]string{"name": name, "namespace": requestNamespace(r)}})
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleEvalRunStatus(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	key := scopedNameForRequest(r, name)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	var statusUpdate struct {
		Status resources.EvalRunStatus `json:"status"`
	}
	if err := json.Unmarshal(body, &statusUpdate); err != nil {
		http.Error(w, fmt.Sprintf("invalid status update: %v", err), http.StatusBadRequest)
		return
	}
	obj, err := s.stores.EvalRuns.UpdateStatus(r.Context(), key, statusUpdate.Status)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	s.publishResourceEvent("EvalRun", obj.Metadata.Name, "updated", obj)
	writeJSON(w, http.StatusOK, obj)
}

// ---------------------------------------------------------------------------
// Export / Annotate / Import / Finalize / Cancel
// ---------------------------------------------------------------------------

func (s *Server) handleEvalRunExport(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	key := scopedNameForRequest(r, name)
	obj, ok, err := s.stores.EvalRuns.Get(r.Context(), key)
	if writeStoreFetchError(w, err) {
		return
	}
	if !ok {
		http.Error(w, fmt.Sprintf("eval run %q not found", name), http.StatusNotFound)
		return
	}

	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "json"
	}

	switch format {
	case "json":
		writeJSON(w, http.StatusOK, obj.Status.Results)
	case "csv":
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s-results.csv", name))
		cw := csv.NewWriter(w)
		_ = cw.Write([]string{"sample_name", "output", "score", "pass", "error", "comment"})
		for _, result := range obj.Status.Results {
			scoreStr := ""
			if result.Score != nil {
				scoreStr = strconv.FormatFloat(*result.Score, 'f', -1, 64)
			}
			passStr := ""
			if result.Pass != nil {
				passStr = strconv.FormatBool(*result.Pass)
			}
			_ = cw.Write([]string{
				result.SampleName,
				result.Output,
				scoreStr,
				passStr,
				result.Error,
				result.Comment,
			})
		}
		cw.Flush()
	default:
		http.Error(w, fmt.Sprintf("unsupported export format %q (expected json or csv)", format), http.StatusBadRequest)
	}
}

func (s *Server) handleEvalRunAnnotateSample(w http.ResponseWriter, r *http.Request, runName, sampleName string) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	key := scopedNameForRequest(r, runName)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	var annotation struct {
		Score   *float64 `json:"score"`
		Pass    *bool    `json:"pass"`
		Comment string   `json:"comment"`
	}
	if err := json.Unmarshal(body, &annotation); err != nil {
		http.Error(w, fmt.Sprintf("invalid annotation: %v", err), http.StatusBadRequest)
		return
	}

	obj, ok, err := s.stores.EvalRuns.Get(r.Context(), key)
	if writeStoreFetchError(w, err) {
		return
	}
	if !ok {
		http.Error(w, fmt.Sprintf("eval run %q not found", runName), http.StatusNotFound)
		return
	}

	found := false
	for i := range obj.Status.Results {
		if strings.EqualFold(obj.Status.Results[i].SampleName, sampleName) {
			obj.Status.Results[i].Score = annotation.Score
			obj.Status.Results[i].Pass = annotation.Pass
			obj.Status.Results[i].Comment = annotation.Comment
			found = true
			break
		}
	}
	if !found {
		http.Error(w, fmt.Sprintf("sample %q not found in eval run %q", sampleName, runName), http.StatusNotFound)
		return
	}

	obj, err = s.stores.EvalRuns.UpdateStatus(r.Context(), key, obj.Status)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	s.publishResourceEvent("EvalRun", obj.Metadata.Name, "updated", obj)
	writeJSON(w, http.StatusOK, obj)
}

func (s *Server) handleEvalRunBulkImport(w http.ResponseWriter, r *http.Request, runName string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	key := scopedNameForRequest(r, runName)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	var annotations []struct {
		SampleName string   `json:"sample_name"`
		Score      *float64 `json:"score"`
		Pass       *bool    `json:"pass"`
		Comment    string   `json:"comment"`
	}
	if err := json.Unmarshal(body, &annotations); err != nil {
		http.Error(w, fmt.Sprintf("invalid bulk annotations: %v", err), http.StatusBadRequest)
		return
	}

	obj, ok, err := s.stores.EvalRuns.Get(r.Context(), key)
	if writeStoreFetchError(w, err) {
		return
	}
	if !ok {
		http.Error(w, fmt.Sprintf("eval run %q not found", runName), http.StatusNotFound)
		return
	}

	resultIndex := make(map[string]int, len(obj.Status.Results))
	for i, r := range obj.Status.Results {
		resultIndex[strings.ToLower(r.SampleName)] = i
	}
	for _, ann := range annotations {
		idx, ok := resultIndex[strings.ToLower(ann.SampleName)]
		if !ok {
			continue
		}
		obj.Status.Results[idx].Score = ann.Score
		obj.Status.Results[idx].Pass = ann.Pass
		obj.Status.Results[idx].Comment = ann.Comment
	}

	obj, err = s.stores.EvalRuns.UpdateStatus(r.Context(), key, obj.Status)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	s.publishResourceEvent("EvalRun", obj.Metadata.Name, "updated", obj)
	writeJSON(w, http.StatusOK, obj)
}

func (s *Server) handleEvalRunFinalize(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	key := scopedNameForRequest(r, name)
	obj, ok, err := s.stores.EvalRuns.Get(r.Context(), key)
	if writeStoreFetchError(w, err) {
		return
	}
	if !ok {
		http.Error(w, fmt.Sprintf("eval run %q not found", name), http.StatusNotFound)
		return
	}
	if obj.Status.Phase != resources.EvalRunPhasePendingReview {
		http.Error(w, fmt.Sprintf("eval run %q is in phase %q, not PendingReview", name, obj.Status.Phase), http.StatusConflict)
		return
	}

	recomputeRunCounts(&obj.Status)
	obj.Status.Summary = resources.ComputeEvalSummary(obj.Status.Results)
	obj.Status.Phase = resources.EvalRunPhaseSucceeded

	obj, err = s.stores.EvalRuns.UpdateStatus(r.Context(), key, obj.Status)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	s.publishResourceEvent("EvalRun", obj.Metadata.Name, "updated", obj)
	writeJSON(w, http.StatusOK, obj)
}

func (s *Server) handleEvalRunStart(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	key := scopedNameForRequest(r, name)
	obj, ok, err := s.stores.EvalRuns.Get(r.Context(), key)
	if writeStoreFetchError(w, err) {
		return
	}
	if !ok {
		http.Error(w, fmt.Sprintf("eval run %q not found", name), http.StatusNotFound)
		return
	}
	if !obj.Spec.Suspended {
		http.Error(w, fmt.Sprintf("eval run %q is not suspended (phase: %s)", name, obj.Status.Phase), http.StatusConflict)
		return
	}
	if obj.Status.Phase != resources.EvalRunPhasePending {
		http.Error(w, fmt.Sprintf("eval run %q is in phase %q, not Pending", name, obj.Status.Phase), http.StatusConflict)
		return
	}

	obj.Spec.Suspended = false
	obj, err = s.stores.EvalRuns.Upsert(r.Context(), obj)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	s.publishResourceEvent("EvalRun", obj.Metadata.Name, "updated", obj)
	writeJSON(w, http.StatusOK, obj)
}

func (s *Server) handleEvalRunCancel(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	key := scopedNameForRequest(r, name)
	obj, ok, err := s.stores.EvalRuns.Get(r.Context(), key)
	if writeStoreFetchError(w, err) {
		return
	}
	if !ok {
		http.Error(w, fmt.Sprintf("eval run %q not found", name), http.StatusNotFound)
		return
	}
	if obj.Status.Phase == resources.EvalRunPhaseSucceeded ||
		obj.Status.Phase == resources.EvalRunPhaseFailed ||
		obj.Status.Phase == resources.EvalRunPhaseCancelled {
		http.Error(w, fmt.Sprintf("eval run %q is already in terminal phase %q", name, obj.Status.Phase), http.StatusConflict)
		return
	}

	obj.Status.Phase = resources.EvalRunPhaseCancelled
	obj, err = s.stores.EvalRuns.UpdateStatus(r.Context(), key, obj.Status)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	s.publishResourceEvent("EvalRun", obj.Metadata.Name, "updated", obj)
	writeJSON(w, http.StatusOK, obj)
}

// ---------------------------------------------------------------------------
// Compare
// ---------------------------------------------------------------------------

func (s *Server) handleEvalRunsCompare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	runsParam := strings.TrimSpace(r.URL.Query().Get("runs"))
	if runsParam == "" {
		http.Error(w, "runs query parameter is required", http.StatusBadRequest)
		return
	}
	runNames := strings.Split(runsParam, ",")
	if len(runNames) < 2 {
		http.Error(w, "at least 2 runs are required for comparison", http.StatusBadRequest)
		return
	}

	type runSummary struct {
		PassRate    float64 `json:"pass_rate"`
		MeanScore   float64 `json:"mean_score"`
		TotalTokens int     `json:"total_tokens"`
	}

	summaryMap := make(map[string]runSummary, len(runNames))
	allSamples := make(map[string]map[string]resources.EvalSampleResult)

	for _, rn := range runNames {
		rn = strings.TrimSpace(rn)
		key := scopedNameForRequest(r, rn)
		obj, ok, err := s.stores.EvalRuns.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("eval run %q not found", rn), http.StatusNotFound)
			return
		}
		summaryMap[rn] = runSummary{
			PassRate:    obj.Status.Summary.PassRate,
			MeanScore:   obj.Status.Summary.MeanScore,
			TotalTokens: obj.Status.Summary.TotalTokens,
		}
		for _, res := range obj.Status.Results {
			if allSamples[res.SampleName] == nil {
				allSamples[res.SampleName] = make(map[string]resources.EvalSampleResult)
			}
			allSamples[res.SampleName][rn] = res
		}
	}

	type compareResponse struct {
		Runs    []string                `json:"runs"`
		Summary map[string]runSummary   `json:"summary"`
		Samples []json.RawMessage       `json:"samples"`
	}

	samples := make([]json.RawMessage, 0, len(allSamples))
	for sampleName, runResults := range allSamples {
		entry := map[string]any{
			"name":    sampleName,
			"results": make(map[string]map[string]any),
		}
		results := entry["results"].(map[string]map[string]any)
		for rn, res := range runResults {
			results[rn] = map[string]any{
				"score": res.Score,
				"pass":  res.Pass,
			}
		}
		raw, _ := json.Marshal(entry)
		samples = append(samples, raw)
	}

	cleanNames := make([]string, 0, len(runNames))
	for _, rn := range runNames {
		cleanNames = append(cleanNames, strings.TrimSpace(rn))
	}

	resp := compareResponse{
		Runs:    cleanNames,
		Summary: summaryMap,
		Samples: samples,
	}
	writeJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func recomputeRunCounts(status *resources.EvalRunStatus) {
	var passed, failed, errored, completed int
	for _, r := range status.Results {
		if r.Error != "" {
			errored++
		} else if r.Pass != nil {
			if *r.Pass {
				passed++
			} else {
				failed++
			}
		}
		if r.Score != nil || r.Pass != nil || r.Error != "" {
			completed++
		}
	}
	status.CompletedSamples = completed
	status.PassedSamples = passed
	status.FailedSamples = failed
	status.ErroredSamples = errored
}
