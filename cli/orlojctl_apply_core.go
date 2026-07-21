package cli

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/OrlojHQ/orloj/resources"
)

type applyOptions struct {
	Server              string
	Namespace           string
	ManifestPath        string
	IncludeRunnable     bool
	SkipRunnableAlways  bool
	SelectedRunnable    string
	DryRun              bool
	IgnoreTaskConflicts bool
	Out                 io.Writer
	Prefix              string
}

type appliedTask struct {
	ManifestName string
	ActualName   string
	Path         string
	Runnable     bool
}

type applyResult struct {
	Applied              int
	Created              int
	Updated              int
	Unchanged            int
	SkippedRunnableTasks int
	TaskConflicts        int
	AppliedTasks         []appliedTask
}

func applyManifests(opts applyOptions) (applyResult, error) {
	var result applyResult
	if strings.TrimSpace(opts.ManifestPath) == "" {
		return result, fmt.Errorf("-f is required")
	}
	if opts.Out == nil {
		opts.Out = io.Discard
	}

	info, err := os.Stat(opts.ManifestPath)
	if err != nil {
		return result, fmt.Errorf("cannot access %s: %w", opts.ManifestPath, err)
	}
	isDir := info.IsDir()

	files, err := manifestPaths(opts.ManifestPath)
	if err != nil {
		return result, err
	}
	if len(files) == 0 {
		return result, fmt.Errorf("no manifest files found in %s", opts.ManifestPath)
	}

	filtered := make([]string, 0, len(files))
	for _, file := range files {
		raw, readErr := os.ReadFile(file)
		if readErr != nil {
			filtered = append(filtered, file)
			continue
		}
		kind, detectErr := resources.DetectKind(raw)
		if detectErr != nil || !strings.EqualFold(strings.TrimSpace(kind), "task") {
			filtered = append(filtered, file)
			continue
		}
		task, taskErr := resources.ParseTaskManifest(raw)
		if taskErr != nil {
			filtered = append(filtered, file)
			continue
		}
		runnable := !strings.EqualFold(strings.TrimSpace(task.Spec.Mode), "template")
		if !runnable {
			filtered = append(filtered, file)
			continue
		}
		if (isDir || opts.SkipRunnableAlways) && !opts.IncludeRunnable {
			result.SkippedRunnableTasks++
			applyPrint(opts, "skipped task/%s (mode: %s) from %s; use --run to include\n", task.Metadata.Name, task.Spec.Mode, file)
			continue
		}
		if opts.IncludeRunnable && strings.TrimSpace(opts.SelectedRunnable) != "" &&
			!strings.EqualFold(strings.TrimSpace(task.Metadata.Name), strings.TrimSpace(opts.SelectedRunnable)) {
			result.SkippedRunnableTasks++
			applyPrint(opts, "skipped task/%s from %s; --task selects %s\n", task.Metadata.Name, file, opts.SelectedRunnable)
			continue
		}
		filtered = append(filtered, file)
	}
	files = filtered

	var applyErrs []string
	for _, file := range files {
		raw, readErr := os.ReadFile(file)
		if readErr != nil {
			applyErrs = append(applyErrs, fmt.Sprintf("%s: %v", file, readErr))
			continue
		}

		kind, detectErr := resources.DetectKind(raw)
		if detectErr != nil {
			if isDir {
				continue
			}
			applyErrs = append(applyErrs, fmt.Sprintf("%s: %v", file, detectErr))
			continue
		}

		endpoint, payload, name, buildErr := buildApplyRequest(kind, raw)
		if buildErr != nil {
			applyErrs = append(applyErrs, fmt.Sprintf("%s: %v", file, buildErr))
			continue
		}
		if strings.TrimSpace(opts.Namespace) != "" {
			payload, buildErr = overridePayloadNamespace(payload, opts.Namespace)
			if buildErr != nil {
				applyErrs = append(applyErrs, fmt.Sprintf("%s: %v", file, buildErr))
				continue
			}
		}

		if opts.DryRun {
			action, previewErr := previewApplyChange(opts.Server, endpoint, name, payload)
			if previewErr != nil {
				applyErrs = append(applyErrs, fmt.Sprintf("%s: %v", file, previewErr))
				continue
			}
			switch action {
			case "create":
				result.Created++
			case "update":
				result.Updated++
			default:
				result.Unchanged++
			}
			applyPrint(opts, "dry-run %s %s/%s\n", action, strings.ToLower(kind), name)
			result.Applied++
			continue
		}

		postURL := strings.TrimRight(opts.Server, "/") + endpoint
		query := url.Values{}
		if strings.TrimSpace(opts.Namespace) != "" {
			query.Set("namespace", strings.TrimSpace(opts.Namespace))
		}
		if opts.IncludeRunnable && endpoint == "/v1/tasks" {
			query.Set("rerun", "true")
		}
		if opts.IncludeRunnable && endpoint == "/v1/eval-runs" {
			query.Set("run", "true")
		}
		if encoded := query.Encode(); encoded != "" {
			postURL += "?" + encoded
		}

		resp, postErr := http.Post(postURL, "application/json", bytes.NewReader(payload))
		if postErr != nil {
			applyErrs = append(applyErrs, fmt.Sprintf("%s: apply request failed: %v", file, postErr))
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode >= 300 {
			if opts.IgnoreTaskConflicts && endpoint == "/v1/tasks" && resp.StatusCode == http.StatusConflict {
				result.TaskConflicts++
				applyPrint(opts, "skipped task/%s rerun: task is still active\n", name)
				continue
			}
			applyErrs = append(applyErrs, fmt.Sprintf("%s: %s", file, bytes.TrimSpace(body)))
			continue
		}

		actualName := nameFromResponseBody(body, name)
		isSuspendedEvalRun := !opts.IncludeRunnable && endpoint == "/v1/eval-runs"
		if actualName != name {
			applyPrint(opts, "applied %s/%s (rerun as %s)\n", strings.ToLower(kind), name, actualName)
		} else if isSuspendedEvalRun {
			applyPrint(opts, "applied %s/%s (suspended; use --run or 'orlojctl eval start %s' to execute)\n", strings.ToLower(kind), name, name)
		} else {
			applyPrint(opts, "applied %s/%s\n", strings.ToLower(kind), name)
		}
		result.Applied++

		if endpoint == "/v1/tasks" {
			task, taskErr := resources.ParseTaskManifest(raw)
			if taskErr == nil {
				result.AppliedTasks = append(result.AppliedTasks, appliedTask{
					ManifestName: name,
					ActualName:   actualName,
					Path:         file,
					Runnable:     !strings.EqualFold(strings.TrimSpace(task.Spec.Mode), "template"),
				})
			}
		}
	}

	if len(applyErrs) > 0 {
		if result.SkippedRunnableTasks > 0 {
			applyPrint(opts, "\n%d applied, %d skipped runnable task(s), %d failed:\n", result.Applied, result.SkippedRunnableTasks, len(applyErrs))
		} else {
			applyPrint(opts, "\n%d applied, %d failed:\n", result.Applied, len(applyErrs))
		}
		for _, applyErr := range applyErrs {
			applyPrint(opts, "  error  %s\n", applyErr)
		}
		return result, fmt.Errorf("apply failed for %d file(s)", len(applyErrs))
	}

	if opts.DryRun {
		applyPrint(opts, "\ndry-run summary: %d checked, %d create, %d update, %d unchanged\n", result.Applied, result.Created, result.Updated, result.Unchanged)
		return result, nil
	}

	if isDir || result.Applied > 1 || result.SkippedRunnableTasks > 0 {
		if result.SkippedRunnableTasks > 0 {
			applyPrint(opts, "\n%d file(s) applied, %d runnable task(s) skipped\n", result.Applied, result.SkippedRunnableTasks)
		} else {
			applyPrint(opts, "\n%d file(s) applied\n", result.Applied)
		}
	}
	return result, nil
}

func applyPrint(opts applyOptions, format string, args ...any) {
	fmt.Fprint(opts.Out, opts.Prefix)
	fmt.Fprintf(opts.Out, format, args...)
}
