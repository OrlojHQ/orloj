package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/OrlojHQ/orloj/resources"
	yaml "go.yaml.in/yaml/v2"
)

type exitCodeError struct {
	code int
	err  error
}

func (e *exitCodeError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *exitCodeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

// ExitCode returns process exit code for a CLI error.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var coded *exitCodeError
	if errors.As(err, &coded) && coded != nil && coded.code > 0 {
		return coded.code
	}
	return 1
}

func newExitCodeError(code int, format string, args ...any) error {
	return &exitCodeError{code: code, err: fmt.Errorf(format, args...)}
}

func normalizeOutputFormat(raw string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(raw))
	if v == "" {
		return "table", nil
	}
	switch v {
	case "table", "json", "yaml", "yml":
		if v == "yml" {
			return "yaml", nil
		}
		return v, nil
	default:
		return "", fmt.Errorf("unsupported output format %q (expected table, json, or yaml)", raw)
	}
}

func printStructuredOutput(body []byte, format string) error {
	if len(bytes.TrimSpace(body)) == 0 {
		fmt.Println("{}")
		return nil
	}
	if format == "table" {
		format = "yaml"
	}
	switch format {
	case "json":
		var out bytes.Buffer
		if err := json.Indent(&out, body, "", "  "); err != nil {
			return fmt.Errorf("failed to format json output: %w", err)
		}
		fmt.Println(out.String())
		return nil
	case "yaml":
		var obj any
		if err := json.Unmarshal(body, &obj); err != nil {
			return fmt.Errorf("failed to decode response as json: %w", err)
		}
		y, err := yaml.Marshal(obj)
		if err != nil {
			return fmt.Errorf("failed to format yaml output: %w", err)
		}
		fmt.Print(string(y))
		return nil
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
}

func isMemoryEntriesResource(resource string) bool {
	switch strings.ToLower(strings.TrimSpace(resource)) {
	case "memory-entries", "memoryentries", "memoryentry":
		return true
	default:
		return false
	}
}

func resourceURL(server, resource, name, namespace string) (string, error) {
	endpoint, err := listEndpointForResource(resource)
	if err != nil {
		return "", err
	}
	full := strings.TrimRight(server, "/") + endpoint
	if strings.TrimSpace(name) != "" {
		full += "/" + url.PathEscape(strings.TrimSpace(name))
	}
	if strings.TrimSpace(namespace) != "" {
		sep := "?"
		if strings.Contains(full, "?") {
			sep = "&"
		}
		full += sep + "namespace=" + url.QueryEscape(strings.TrimSpace(namespace))
	}
	return full, nil
}

func fetchResourceRaw(server, resource, name, namespace string) ([]byte, error) {
	requestURL, err := resourceURL(server, resource, name, namespace)
	if err != nil {
		return nil, err
	}
	resp, err := http.Get(requestURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s", bytes.TrimSpace(body))
	}
	return body, nil
}

func fetchURLRaw(requestURL string) ([]byte, error) {
	resp, err := http.Get(requestURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s", bytes.TrimSpace(body))
	}
	return body, nil
}

// --- memory-entries ---

func newMemoryEntriesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "memory-entries <memory-name>",
		Short: "List or query memory entries",
		Args:  cobra.ExactArgs(1),
		RunE:  runMemoryEntries,
	}
	cmd.Flags().String("query", "", "semantic query")
	cmd.Flags().String("prefix", "", "prefix filter for key listing")
	cmd.Flags().Int("limit", 100, "max entries returned")
	cmd.Flags().StringP("output", "o", "table", "output format: table|json|yaml")
	return cmd
}

func runMemoryEntries(cmd *cobra.Command, args []string) error {
	server := resolveServer(cmd)
	ns := resolveNamespace(cmd)
	query, _ := cmd.Flags().GetString("query")
	prefix, _ := cmd.Flags().GetString("prefix")
	limit, _ := cmd.Flags().GetInt("limit")
	output, _ := cmd.Flags().GetString("output")
	format, err := normalizeOutputFormat(output)
	if err != nil {
		return err
	}

	name := strings.TrimSpace(args[0])
	if name == "" {
		return errors.New("memory name is required")
	}
	if limit <= 0 {
		return errors.New("--limit must be > 0")
	}

	requestURL := strings.TrimRight(server, "/") + "/v1/memories/" + url.PathEscape(name) + "/entries"
	values := url.Values{}
	if strings.TrimSpace(ns) != "" {
		values.Set("namespace", strings.TrimSpace(ns))
	}
	if strings.TrimSpace(query) != "" {
		values.Set("q", strings.TrimSpace(query))
	}
	if strings.TrimSpace(prefix) != "" {
		values.Set("prefix", strings.TrimSpace(prefix))
	}
	values.Set("limit", strconv.Itoa(limit))
	requestURL += "?" + values.Encode()

	resp, err := http.Get(requestURL)
	if err != nil {
		return fmt.Errorf("memory entries request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("memory entries failed: %s", bytes.TrimSpace(body))
	}
	if format != "table" {
		return printStructuredOutput(body, format)
	}

	var payload struct {
		Entries []any `json:"entries"`
		Count   int   `json:"count"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("failed to decode memory entries response: %w", err)
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "INDEX\tENTRY")
	for i, entry := range payload.Entries {
		raw, _ := json.Marshal(entry)
		fmt.Fprintf(tw, "%d\t%s\n", i+1, string(raw))
	}
	_ = tw.Flush()
	fmt.Printf("\ncount=%d\n", payload.Count)
	return nil
}

// --- describe ---

func newDescribeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "describe <resource> <name>",
		Short: "Show details of a resource",
		Args:  cobra.ExactArgs(2),
		RunE:  runDescribe,
	}
	cmd.Flags().StringP("output", "o", "table", "output format: table|json|yaml")
	return cmd
}

func runDescribe(cmd *cobra.Command, args []string) error {
	server := resolveServer(cmd)
	ns := resolveNamespace(cmd)
	output, _ := cmd.Flags().GetString("output")
	format, err := normalizeOutputFormat(output)
	if err != nil {
		return err
	}

	resource := normalizeResource(args[0])
	if resource == "" {
		return fmt.Errorf("unsupported resource %q", args[0])
	}
	name := strings.TrimSpace(args[1])
	raw, err := fetchResourceRaw(server, resource, name, ns)
	if err != nil {
		return fmt.Errorf("describe failed: %w", err)
	}
	if format != "table" {
		return printStructuredOutput(raw, format)
	}

	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return fmt.Errorf("failed to decode resource: %w", err)
	}
	meta, _ := obj["metadata"].(map[string]any)
	namespace := "default"
	if value, ok := meta["namespace"].(string); ok && strings.TrimSpace(value) != "" {
		namespace = value
	}
	kind, _ := obj["kind"].(string)
	status, _ := obj["status"].(map[string]any)
	phase, _ := status["phase"].(string)
	fmt.Printf("Kind: %s\n", kind)
	fmt.Printf("Name: %s\n", name)
	fmt.Printf("Namespace: %s\n", namespace)
	if strings.TrimSpace(phase) != "" {
		fmt.Printf("Phase: %s\n", phase)
	}
	fmt.Println("Resource:")
	return printStructuredOutput(raw, "yaml")
}

// --- edit ---

func newEditCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "edit <resource> <name>",
		Short: "Edit a resource in your editor",
		Args:  cobra.ExactArgs(2),
		RunE:  runEdit,
	}
}

func runEdit(cmd *cobra.Command, args []string) error {
	server := resolveServer(cmd)
	ns := resolveNamespace(cmd)

	resource := normalizeResource(args[0])
	if resource == "" {
		return fmt.Errorf("unsupported resource %q", args[0])
	}
	name := strings.TrimSpace(args[1])
	if name == "" {
		return errors.New("resource name is required")
	}

	raw, err := fetchResourceRaw(server, resource, name, ns)
	if err != nil {
		return fmt.Errorf("edit fetch failed: %w", err)
	}
	var obj any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return fmt.Errorf("failed to decode resource: %w", err)
	}
	yamlRaw, err := yaml.Marshal(obj)
	if err != nil {
		return fmt.Errorf("failed to render yaml: %w", err)
	}

	tmpDir := os.TempDir()
	tmpFile := filepath.Join(tmpDir, fmt.Sprintf("orlojctl-edit-%s-%s.yaml", resource, name))
	if writeErr := os.WriteFile(tmpFile, yamlRaw, 0o600); writeErr != nil {
		return fmt.Errorf("failed to write temp edit file: %w", writeErr)
	}
	defer os.Remove(tmpFile)

	editor := strings.TrimSpace(os.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if editor == "" {
		editor = "vi"
	}
	editorCmd := exec.Command(editor, tmpFile)
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr
	if err := editorCmd.Run(); err != nil {
		return fmt.Errorf("editor command failed: %w", err)
	}

	editedRaw, err := os.ReadFile(tmpFile)
	if err != nil {
		return fmt.Errorf("failed to read edited file: %w", err)
	}
	if bytes.Equal(bytes.TrimSpace(editedRaw), bytes.TrimSpace(yamlRaw)) {
		fmt.Println("no changes detected")
		return nil
	}

	kind, err := resources.DetectKind(editedRaw)
	if err != nil {
		return fmt.Errorf("edited manifest is invalid: %w", err)
	}
	endpoint, payload, editedName, err := buildApplyRequest(kind, editedRaw)
	if err != nil {
		return fmt.Errorf("edited manifest parse failed: %w", err)
	}
	if strings.TrimSpace(ns) != "" {
		payload, err = overridePayloadNamespace(payload, ns)
		if err != nil {
			return err
		}
	}
	resp, err := http.Post(strings.TrimRight(server, "/")+endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("edit apply request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("edit apply failed: %s", bytes.TrimSpace(body))
	}
	fmt.Printf("applied %s/%s\n", strings.ToLower(kind), editedName)
	return nil
}

// --- diff ---

func newDiffCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Show differences between local manifests and live resources",
		RunE:  runDiff,
	}
	cmd.Flags().StringP("file", "f", "", "path to resource manifest file or directory")
	cmd.Flags().Bool("run", false, "include runnable Task manifests when diffing a directory")
	return cmd
}

func runDiff(cmd *cobra.Command, args []string) error {
	manifestPath, _ := cmd.Flags().GetString("file")
	includeRunnable, _ := cmd.Flags().GetBool("run")
	server := resolveServer(cmd)
	ns := resolveNamespace(cmd)

	if manifestPath == "" {
		return errors.New("-f is required")
	}

	info, err := os.Stat(manifestPath)
	if err != nil {
		return fmt.Errorf("cannot access %s: %w", manifestPath, err)
	}
	isDir := info.IsDir()

	files, err := manifestPaths(manifestPath)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no manifest files found in %s", manifestPath)
	}

	skippedRunnableTasks := 0
	if isDir && !includeRunnable {
		filtered := make([]string, 0, len(files))
		for _, f := range files {
			raw, readErr := os.ReadFile(f)
			if readErr != nil {
				filtered = append(filtered, f)
				continue
			}
			kind, detectErr := resources.DetectKind(raw)
			if detectErr != nil || !strings.EqualFold(strings.TrimSpace(kind), "task") {
				filtered = append(filtered, f)
				continue
			}
			task, taskErr := resources.ParseTaskManifest(raw)
			if taskErr != nil {
				filtered = append(filtered, f)
				continue
			}
			if task.Spec.Mode == "template" {
				filtered = append(filtered, f)
				continue
			}
			skippedRunnableTasks++
			fmt.Printf("skipped task/%s (mode: %s) from %s; use --run to include\n", task.Metadata.Name, task.Spec.Mode, f)
		}
		files = filtered
	}

	checked := 0
	created := 0
	updated := 0
	unchanged := 0
	var diffErrs []string

	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			diffErrs = append(diffErrs, fmt.Sprintf("%s: %v", f, err))
			continue
		}

		kind, err := resources.DetectKind(raw)
		if err != nil {
			diffErrs = append(diffErrs, fmt.Sprintf("%s: %v", f, err))
			continue
		}

		endpoint, payload, name, err := buildApplyRequest(kind, raw)
		if err != nil {
			diffErrs = append(diffErrs, fmt.Sprintf("%s: %v", f, err))
			continue
		}
		if strings.TrimSpace(ns) != "" {
			payload, err = overridePayloadNamespace(payload, ns)
			if err != nil {
				diffErrs = append(diffErrs, fmt.Sprintf("%s: %v", f, err))
				continue
			}
		}

		desiredDoc, err := canonicalComparableDocument(payload)
		if err != nil {
			diffErrs = append(diffErrs, fmt.Sprintf("%s: %v", f, err))
			continue
		}

		resourceNS := metadataNamespaceFromPayload(payload)
		currentRaw, exists, err := fetchLiveResourceForDiff(server, endpoint, name, resourceNS)
		if err != nil {
			diffErrs = append(diffErrs, fmt.Sprintf("%s: %v", f, err))
			continue
		}

		oldLabel := fmt.Sprintf("live/%s/%s", strings.ToLower(kind), name)
		newLabel := fmt.Sprintf("desired/%s/%s", strings.ToLower(kind), name)
		currentDoc := ""
		action := "create"
		if exists {
			currentDoc, err = canonicalComparableDocument(currentRaw)
			if err != nil {
				diffErrs = append(diffErrs, fmt.Sprintf("%s: %v", f, err))
				continue
			}
			if currentDoc == desiredDoc {
				action = "no-op"
			} else {
				action = "update"
			}
		} else {
			oldLabel = "/dev/null"
		}

		switch action {
		case "create":
			created++
			fmt.Printf("diff create %s/%s (%s)\n", strings.ToLower(kind), name, f)
			fmt.Print(renderUnifiedDiff("", desiredDoc, oldLabel, newLabel))
		case "update":
			updated++
			fmt.Printf("diff update %s/%s (%s)\n", strings.ToLower(kind), name, f)
			fmt.Print(renderUnifiedDiff(currentDoc, desiredDoc, oldLabel, newLabel))
		default:
			unchanged++
			fmt.Printf("diff no-op %s/%s (%s)\n", strings.ToLower(kind), name, f)
		}
		checked++
	}

	if len(diffErrs) > 0 {
		if skippedRunnableTasks > 0 {
			fmt.Printf("\n%d checked, %d skipped runnable task(s), %d failed:\n", checked, skippedRunnableTasks, len(diffErrs))
		} else {
			fmt.Printf("\n%d checked, %d failed:\n", checked, len(diffErrs))
		}
		for _, item := range diffErrs {
			fmt.Printf("  error  %s\n", item)
		}
		return fmt.Errorf("diff failed for %d file(s)", len(diffErrs))
	}

	fmt.Printf("\ndiff summary: %d checked, %d create, %d update, %d unchanged\n", checked, created, updated, unchanged)
	if skippedRunnableTasks > 0 {
		fmt.Printf("%d runnable task(s) skipped\n", skippedRunnableTasks)
	}
	return nil
}

func metadataNamespaceFromPayload(payload []byte) string {
	var obj struct {
		Metadata struct {
			Namespace string `json:"namespace"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(payload, &obj); err != nil {
		return ""
	}
	return strings.TrimSpace(obj.Metadata.Namespace)
}

func previewApplyChange(server, endpoint, name string, payload []byte) (string, error) {
	ns := metadataNamespaceFromPayload(payload)
	requestURL := strings.TrimRight(server, "/") + endpoint + "/" + url.PathEscape(name)
	if ns != "" {
		requestURL += "?namespace=" + url.QueryEscape(ns)
	}
	resp, err := http.Get(requestURL)
	if err != nil {
		return "", fmt.Errorf("dry-run get failed: %w", err)
	}
	defer resp.Body.Close()
	currentRaw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return "create", nil
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("dry-run get failed: %s", bytes.TrimSpace(currentRaw))
	}

	var desired map[string]any
	if err := json.Unmarshal(payload, &desired); err != nil {
		return "", fmt.Errorf("dry-run decode desired failed: %w", err)
	}
	var current map[string]any
	if err := json.Unmarshal(currentRaw, &current); err != nil {
		return "", fmt.Errorf("dry-run decode live failed: %w", err)
	}
	normalizeComparableResource(desired)
	normalizeComparableResource(current)
	if reflect.DeepEqual(desired, current) {
		return "no-op", nil
	}
	return "update", nil
}

func normalizeComparableResource(obj map[string]any) {
	if obj == nil {
		return
	}
	delete(obj, "status")
	meta, _ := obj["metadata"].(map[string]any)
	if meta == nil {
		return
	}
	delete(meta, "resourceVersion")
	delete(meta, "generation")
	delete(meta, "createdAt")
}

func fetchLiveResourceForDiff(server, endpoint, name, namespace string) ([]byte, bool, error) {
	requestURL := strings.TrimRight(server, "/") + endpoint + "/" + url.PathEscape(name)
	if strings.TrimSpace(namespace) != "" {
		requestURL += "?namespace=" + url.QueryEscape(strings.TrimSpace(namespace))
	}
	resp, err := http.Get(requestURL)
	if err != nil {
		return nil, false, fmt.Errorf("diff get failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return nil, false, nil
	}
	if resp.StatusCode >= 300 {
		return nil, false, fmt.Errorf("diff get failed: %s", bytes.TrimSpace(body))
	}
	return body, true, nil
}

func canonicalComparableDocument(rawJSON []byte) (string, error) {
	var obj map[string]any
	if err := json.Unmarshal(rawJSON, &obj); err != nil {
		return "", fmt.Errorf("failed to decode json payload for diff: %w", err)
	}
	normalizeComparableResource(obj)
	rendered, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to format diff payload: %w", err)
	}
	return string(rendered) + "\n", nil
}

func overridePayloadNamespace(payload []byte, namespace string) ([]byte, error) {
	trimmed := strings.TrimSpace(namespace)
	if trimmed == "" {
		return payload, nil
	}
	var obj map[string]any
	if err := json.Unmarshal(payload, &obj); err != nil {
		return nil, fmt.Errorf("failed to decode payload for namespace override: %w", err)
	}
	meta, _ := obj["metadata"].(map[string]any)
	if meta == nil {
		meta = map[string]any{}
	}
	meta["namespace"] = trimmed
	obj["metadata"] = meta
	updated, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to encode payload for namespace override: %w", err)
	}
	return updated, nil
}

type diffLineOp struct {
	kind byte
	line string
}

func splitDiffLines(text string) []string {
	trimmed := strings.TrimSuffix(text, "\n")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

func lineDiffOps(a, b []string) []diffLineOp {
	dp := make([][]int, len(a)+1)
	for i := range dp {
		dp[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
				continue
			}
			if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	i, j := 0, 0
	ops := make([]diffLineOp, 0, len(a)+len(b))
	for i < len(a) && j < len(b) {
		if a[i] == b[j] {
			ops = append(ops, diffLineOp{kind: ' ', line: a[i]})
			i++
			j++
			continue
		}
		if dp[i+1][j] >= dp[i][j+1] {
			ops = append(ops, diffLineOp{kind: '-', line: a[i]})
			i++
			continue
		}
		ops = append(ops, diffLineOp{kind: '+', line: b[j]})
		j++
	}
	for i < len(a) {
		ops = append(ops, diffLineOp{kind: '-', line: a[i]})
		i++
	}
	for j < len(b) {
		ops = append(ops, diffLineOp{kind: '+', line: b[j]})
		j++
	}
	return ops
}

func renderUnifiedDiff(oldText, newText, oldLabel, newLabel string) string {
	oldLines := splitDiffLines(oldText)
	newLines := splitDiffLines(newText)
	ops := lineDiffOps(oldLines, newLines)

	var out strings.Builder
	fmt.Fprintf(&out, "--- %s\n", oldLabel)
	fmt.Fprintf(&out, "+++ %s\n", newLabel)
	fmt.Fprintf(&out, "@@ -1,%d +1,%d @@\n", len(oldLines), len(newLines))
	for _, op := range ops {
		out.WriteByte(op.kind)
		out.WriteString(op.line)
		out.WriteByte('\n')
	}
	return out.String()
}

// --- wait ---

func newWaitCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wait <resource>/<name>",
		Short: "Wait for a resource to reach a condition",
		Args:  cobra.ExactArgs(1),
		RunE:  runWait,
	}
	cmd.Flags().String("for", "condition=Complete", "wait condition expression")
	cmd.Flags().Duration("timeout", 5*time.Minute, "maximum wait time")
	cmd.Flags().Duration("interval", 2*time.Second, "poll interval")
	return cmd
}

func runWait(cmd *cobra.Command, args []string) error {
	server := resolveServer(cmd)
	ns := resolveNamespace(cmd)
	forValue, _ := cmd.Flags().GetString("for")
	timeout, _ := cmd.Flags().GetDuration("timeout")
	interval, _ := cmd.Flags().GetDuration("interval")

	target := strings.TrimSpace(args[0])
	parts := strings.SplitN(target, "/", 2)
	if len(parts) != 2 {
		return newExitCodeError(2, "target must be <resource>/<name>")
	}
	resource := normalizeResource(parts[0])
	if resource == "" {
		return newExitCodeError(2, "unsupported wait resource %q", parts[0])
	}
	name := strings.TrimSpace(parts[1])
	if name == "" {
		return newExitCodeError(2, "resource name is required")
	}
	condition := strings.TrimSpace(forValue)
	if strings.HasPrefix(strings.ToLower(condition), "condition=") {
		condition = strings.TrimSpace(condition[len("condition="):])
	}
	if condition == "" {
		return newExitCodeError(2, "--for condition cannot be empty")
	}
	if interval <= 0 {
		return newExitCodeError(2, "--interval must be > 0")
	}
	if timeout <= 0 {
		return newExitCodeError(2, "--timeout must be > 0")
	}

	deadline := time.Now().Add(timeout)
	for {
		raw, err := fetchResourceRaw(server, resource, name, ns)
		if err != nil {
			return newExitCodeError(2, "wait request failed: %v", err)
		}
		phase := extractStatusPhase(raw)
		if waitConditionMet(condition, phase) {
			fmt.Printf("%s/%s condition met: %s (phase=%s)\n", resource, name, condition, phase)
			return nil
		}
		if time.Now().After(deadline) {
			return newExitCodeError(1, "timed out after %s waiting for %s/%s condition=%s (last phase=%s)", timeout, resource, name, condition, phase)
		}
		time.Sleep(interval)
	}
}

func extractStatusPhase(raw []byte) string {
	var obj struct {
		Status struct {
			Phase string `json:"phase"`
		} `json:"status"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return ""
	}
	return strings.TrimSpace(obj.Status.Phase)
}

func waitConditionMet(condition, phase string) bool {
	c := strings.ToLower(strings.TrimSpace(condition))
	p := strings.ToLower(strings.TrimSpace(phase))
	switch c {
	case "complete", "completed", "succeeded":
		return p == "succeeded" || p == "ready"
	case "failed", "deadletter", "error":
		return p == "failed" || p == "deadletter" || p == "error"
	default:
		return strings.EqualFold(c, p)
	}
}

// --- cancel ---

func newCancelCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cancel task <name>",
		Short: "Cancel a running task",
		Args:  cobra.ExactArgs(2),
		RunE:  runCancel,
	}
	cmd.Flags().String("reason", "", "cancellation reason")
	return cmd
}

func runCancel(cmd *cobra.Command, args []string) error {
	if !strings.EqualFold(args[0], "task") {
		return errors.New("usage: orlojctl cancel task <name> [--reason text]")
	}
	server := resolveServer(cmd)
	ns := resolveNamespace(cmd)
	reason, _ := cmd.Flags().GetString("reason")

	name := strings.TrimSpace(args[1])
	taskRaw, err := fetchResourceRaw(server, "tasks", name, ns)
	if err != nil {
		return fmt.Errorf("cancel get task failed: %w", err)
	}
	var task resources.Task
	if err := json.Unmarshal(taskRaw, &task); err != nil {
		return fmt.Errorf("cancel decode task failed: %w", err)
	}
	phase := strings.ToLower(strings.TrimSpace(task.Status.Phase))
	if phase == "succeeded" || phase == "failed" || phase == "deadletter" {
		fmt.Printf("task/%s is already terminal (%s)\n", name, task.Status.Phase)
		return nil
	}
	cancelReason := strings.TrimSpace(reason)
	if cancelReason == "" {
		cancelReason = "task canceled via orlojctl"
	}
	task.Status.Phase = "Failed"
	task.Status.LastError = cancelReason
	task.Status.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	task.Status.NextAttemptAt = ""
	task.Status.AssignedWorker = ""
	task.Status.ClaimedBy = ""
	task.Status.LeaseUntil = ""
	task.Status.LastHeartbeat = ""
	task.Status.ObservedGeneration = task.Metadata.Generation

	patch := map[string]any{
		"metadata": map[string]any{
			"resourceVersion": task.Metadata.ResourceVersion,
		},
		"status": task.Status,
	}
	payload, _ := json.Marshal(patch)
	requestURL := strings.TrimRight(server, "/") + "/v1/tasks/" + url.PathEscape(name) + "/status"
	if strings.TrimSpace(ns) != "" {
		requestURL += "?namespace=" + url.QueryEscape(strings.TrimSpace(ns))
	}
	req, err := http.NewRequest(http.MethodPut, requestURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("cancel request build failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("cancel request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("cancel failed: %s", bytes.TrimSpace(body))
	}
	fmt.Printf("canceled task/%s\n", name)
	return nil
}

// --- retry ---

func newRetryCommand() *cobra.Command {
	var overrides stringSliceFlag
	cmd := &cobra.Command{
		Use:   "retry task <name>",
		Short: "Retry a failed or completed task",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !strings.EqualFold(args[0], "task") {
				return errors.New("usage: orlojctl retry task <name> [--with-overrides key=value ...]")
			}
			server := resolveServer(cmd)
			ns := resolveNamespace(cmd)

			name := strings.TrimSpace(args[1])
			taskRaw, err := fetchResourceRaw(server, "tasks", name, ns)
			if err != nil {
				return fmt.Errorf("retry get task failed: %w", err)
			}
			var task resources.Task
			if err := json.Unmarshal(taskRaw, &task); err != nil {
				return fmt.Errorf("retry decode task failed: %w", err)
			}
			phase := strings.ToLower(strings.TrimSpace(task.Status.Phase))
			if phase == "running" || phase == "pending" || phase == "waitingapproval" {
				return fmt.Errorf("task/%s is not retryable while phase=%s", name, task.Status.Phase)
			}

			if task.Spec.Input == nil {
				task.Spec.Input = map[string]string{}
			}
			for _, override := range overrides {
				parts := strings.SplitN(override, "=", 2)
				if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
					return fmt.Errorf("invalid --with-overrides %q: expected key=value", override)
				}
				task.Spec.Input[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}

			retryName := fmt.Sprintf("%s-retry-%d", name, time.Now().UnixMilli())
			labels := make(map[string]string)
			for k, v := range task.Metadata.Labels {
				labels[k] = v
			}
			labels["orloj.dev/source-task"] = task.Metadata.Name
			labels["orloj.dev/source-task-namespace"] = resources.NormalizeNamespace(task.Metadata.Namespace)
			retryTask := resources.Task{
				APIVersion: "orloj.dev/v1",
				Kind:       "Task",
				Metadata: resources.ObjectMeta{
					Name:      retryName,
					Namespace: resources.NormalizeNamespace(task.Metadata.Namespace),
					Labels:    labels,
				},
				Spec: task.Spec,
			}
			payload, err := json.Marshal(retryTask)
			if err != nil {
				return fmt.Errorf("retry marshal task failed: %w", err)
			}
			requestURL := strings.TrimRight(server, "/") + "/v1/tasks"
			if strings.TrimSpace(ns) != "" {
				requestURL += "?namespace=" + url.QueryEscape(strings.TrimSpace(ns))
			}
			resp, err := http.Post(requestURL, "application/json", bytes.NewReader(payload))
			if err != nil {
				return fmt.Errorf("retry create task failed: %w", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode >= 300 {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("retry failed: %s", bytes.TrimSpace(body))
			}
			fmt.Printf("retried task/%s as task/%s\n", name, retryName)
			return nil
		},
	}
	cmd.Flags().Var(&overrides, "with-overrides", "task input override key=value (repeatable)")
	return cmd
}

// --- top ---

func newTopCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "top <workers|tasks>",
		Short: "Show summary statistics",
		Args:  cobra.ExactArgs(1),
		RunE:  runTop,
	}
	cmd.Flags().StringP("output", "o", "table", "output format: table|json|yaml")
	return cmd
}

func runTop(cmd *cobra.Command, args []string) error {
	server := resolveServer(cmd)
	ns := resolveNamespace(cmd)
	output, _ := cmd.Flags().GetString("output")
	format, err := normalizeOutputFormat(output)
	if err != nil {
		return err
	}

	mode := strings.ToLower(strings.TrimSpace(args[0]))
	switch mode {
	case "workers":
		raw, err := fetchResourceRaw(server, "workers", "", ns)
		if err != nil {
			return fmt.Errorf("top workers failed: %w", err)
		}
		if format != "table" {
			return printStructuredOutput(raw, format)
		}
		var list resources.WorkerList
		if err := json.Unmarshal(raw, &list); err != nil {
			return err
		}
		totalTasks := 0
		ready := 0
		for _, worker := range list.Items {
			totalTasks += worker.Status.CurrentTasks
			if strings.EqualFold(worker.Status.Phase, "Ready") {
				ready++
			}
		}
		fmt.Printf("workers=%d ready=%d current_tasks=%d\n", len(list.Items), ready, totalTasks)
		return nil
	case "tasks":
		raw, err := fetchResourceRaw(server, "tasks", "", ns)
		if err != nil {
			return fmt.Errorf("top tasks failed: %w", err)
		}
		if format != "table" {
			return printStructuredOutput(raw, format)
		}
		var list resources.TaskList
		if err := json.Unmarshal(raw, &list); err != nil {
			return err
		}
		counts := map[string]int{}
		estimatedTokens := 0
		for _, task := range list.Items {
			phase := strings.ToLower(strings.TrimSpace(task.Status.Phase))
			counts[phase]++
			if task.Status.Output != nil {
				estimatedTokens += parseIntOrZero(task.Status.Output["tokens_estimated_total"])
			}
		}
		fmt.Printf("tasks=%d pending=%d running=%d waiting_approval=%d succeeded=%d failed=%d deadletter=%d est_tokens_total=%d\n",
			len(list.Items), counts["pending"], counts["running"], counts["waitingapproval"], counts["succeeded"], counts["failed"], counts["deadletter"], estimatedTokens)
		return nil
	default:
		return fmt.Errorf("unsupported top target %q (expected workers or tasks)", mode)
	}
}

// --- messages ---

func newMessagesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "messages <task/<task-name>|task <task-name>>",
		Short: "List task messages",
		Args:  cobra.RangeArgs(1, 2),
		RunE:  runMessages,
	}
	cmd.Flags().String("agent", "", "filter messages where from/to agent matches")
	cmd.Flags().String("phase", "", "phase filter")
	cmd.Flags().StringP("output", "o", "table", "output format: table|json|yaml")
	cmd.Flags().Int("limit", 0, "max messages returned (0 means no limit)")
	return cmd
}

func runMessages(cmd *cobra.Command, args []string) error {
	server := resolveServer(cmd)
	ns := resolveNamespace(cmd)
	agent, _ := cmd.Flags().GetString("agent")
	phase, _ := cmd.Flags().GetString("phase")
	output, _ := cmd.Flags().GetString("output")
	limit, _ := cmd.Flags().GetInt("limit")
	format, err := normalizeOutputFormat(output)
	if err != nil {
		return err
	}
	target, err := parseTaskTarget(args)
	if err != nil {
		return err
	}

	values := url.Values{}
	if strings.TrimSpace(phase) != "" {
		values.Set("phase", strings.TrimSpace(phase))
	}
	if limit > 0 {
		values.Set("limit", strconv.Itoa(limit))
	}
	if strings.TrimSpace(ns) != "" {
		values.Set("namespace", strings.TrimSpace(ns))
	}
	requestURL := strings.TrimRight(server, "/") + "/v1/tasks/" + url.PathEscape(target) + "/messages"
	if encoded := values.Encode(); encoded != "" {
		requestURL += "?" + encoded
	}
	body, err := fetchURLRaw(requestURL)
	if err != nil {
		return fmt.Errorf("messages failed: %w", err)
	}

	var payload struct {
		Name            string                  `json:"name"`
		Namespace       string                  `json:"namespace"`
		Total           int                     `json:"total"`
		FilteredFrom    int                     `json:"filtered_from"`
		LifecycleCounts map[string]int          `json:"lifecycle_counts"`
		Messages        []resources.TaskMessage `json:"messages"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("failed to decode messages response: %w", err)
	}
	if strings.TrimSpace(agent) != "" {
		filtered := make([]resources.TaskMessage, 0, len(payload.Messages))
		for _, msg := range payload.Messages {
			if strings.EqualFold(strings.TrimSpace(msg.FromAgent), strings.TrimSpace(agent)) ||
				strings.EqualFold(strings.TrimSpace(msg.ToAgent), strings.TrimSpace(agent)) {
				filtered = append(filtered, msg)
			}
		}
		payload.Messages = filtered
		payload.Total = len(filtered)
		payload.FilteredFrom = len(filtered)
	}
	if format != "table" {
		raw, _ := json.Marshal(payload)
		return printStructuredOutput(raw, format)
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "TIME\tFROM\tTO\tTYPE\tPHASE\tATTEMPTS\tCONTENT")
	for _, msg := range payload.Messages {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%d\t%s\n",
			msg.Timestamp, msg.FromAgent, msg.ToAgent,
			msg.Type, msg.Phase, msg.Attempts, compactError(msg.Content))
	}
	_ = tw.Flush()
	return nil
}

func parseTaskTarget(args []string) (string, error) {
	if len(args) == 0 {
		return "", errors.New("usage: target must be task/<task-name> or task <task-name>")
	}
	if len(args) == 1 {
		target := strings.TrimSpace(args[0])
		if strings.HasPrefix(strings.ToLower(target), "task/") {
			name := strings.TrimSpace(target[len("task/"):])
			if name == "" {
				return "", errors.New("task name is required")
			}
			return name, nil
		}
		return "", errors.New("usage: target must be task/<task-name> or task <task-name>")
	}
	if len(args) == 2 && strings.EqualFold(strings.TrimSpace(args[0]), "task") {
		name := strings.TrimSpace(args[1])
		if name == "" {
			return "", errors.New("task name is required")
		}
		return name, nil
	}
	return "", errors.New("usage: target must be task/<task-name> or task <task-name>")
}

// --- metrics ---

func newMetricsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "metrics <task/<task-name>|task <task-name>>",
		Short: "Show task metrics",
		Args:  cobra.RangeArgs(1, 2),
		RunE:  runMetrics,
	}
	cmd.Flags().String("phase", "", "phase filter")
	cmd.Flags().StringP("output", "o", "table", "output format: table|json|yaml")
	cmd.Flags().Int("limit", 0, "max message samples used for metrics (0 means no limit)")
	return cmd
}

func runMetrics(cmd *cobra.Command, args []string) error {
	server := resolveServer(cmd)
	ns := resolveNamespace(cmd)
	phase, _ := cmd.Flags().GetString("phase")
	output, _ := cmd.Flags().GetString("output")
	limit, _ := cmd.Flags().GetInt("limit")
	format, err := normalizeOutputFormat(output)
	if err != nil {
		return err
	}
	target, err := parseTaskTarget(args)
	if err != nil {
		return err
	}

	values := url.Values{}
	if strings.TrimSpace(phase) != "" {
		values.Set("phase", strings.TrimSpace(phase))
	}
	if limit > 0 {
		values.Set("limit", strconv.Itoa(limit))
	}
	if strings.TrimSpace(ns) != "" {
		values.Set("namespace", strings.TrimSpace(ns))
	}
	requestURL := strings.TrimRight(server, "/") + "/v1/tasks/" + url.PathEscape(target) + "/metrics"
	if encoded := values.Encode(); encoded != "" {
		requestURL += "?" + encoded
	}
	body, err := fetchURLRaw(requestURL)
	if err != nil {
		return fmt.Errorf("metrics failed: %w", err)
	}
	if format != "table" {
		return printStructuredOutput(body, format)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("failed to decode metrics response: %w", err)
	}
	name, _ := payload["name"].(string)
	nsName, _ := payload["namespace"].(string)
	fmt.Printf("task=%s namespace=%s\n", name, nsName)
	if totals, ok := payload["totals"].(map[string]any); ok {
		fmt.Printf("messages=%v in_flight=%v retry_count=%v deadletters=%v latency_ms_avg=%v latency_ms_p95=%v\n",
			totals["messages"], totals["in_flight"], totals["retry_count"], totals["deadletters"], totals["latency_ms_avg"], totals["latency_ms_p95"])
	}
	perAgent, _ := payload["per_agent"].([]any)
	if len(perAgent) > 0 {
		fmt.Println("per-agent:")
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "AGENT\tINBOUND\tOUTBOUND\tIN_FLIGHT\tSUCCEEDED\tDEADLETTER\tLAT_MS_AVG\tLAT_MS_P95")
		for _, row := range perAgent {
			m, _ := row.(map[string]any)
			fmt.Fprintf(tw, "%v\t%v\t%v\t%v\t%v\t%v\t%v\t%v\n",
				m["agent"], m["inbound"], m["outbound"], m["in_flight"], m["succeeded"], m["deadletter"], m["latency_ms_avg"], m["latency_ms_p95"])
		}
		_ = tw.Flush()
	}
	return nil
}

// --- health ---

func newHealthCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "health",
		Short: "Check server health",
		Args:  cobra.NoArgs,
		RunE:  runHealth,
	}
	cmd.Flags().StringP("output", "o", "table", "output format: table|json|yaml")
	return cmd
}

func runHealth(cmd *cobra.Command, args []string) error {
	server := resolveServer(cmd)
	output, _ := cmd.Flags().GetString("output")
	format, err := normalizeOutputFormat(output)
	if err != nil {
		return err
	}
	resp, err := http.Get(strings.TrimRight(server, "/") + "/healthz")
	if err != nil {
		return fmt.Errorf("health request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("health failed: %s", bytes.TrimSpace(body))
	}
	if format != "table" {
		return printStructuredOutput(body, format)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("failed to decode health response: %w", err)
	}
	fmt.Printf("server=%s status=%v\n", strings.TrimRight(server, "/"), payload["status"])
	return nil
}

// --- status ---

func newStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show server status overview",
		Args:  cobra.NoArgs,
		RunE:  runStatus,
	}
	cmd.Flags().StringP("output", "o", "table", "output format: table|json|yaml")
	return cmd
}

func runStatus(cmd *cobra.Command, args []string) error {
	server := resolveServer(cmd)
	output, _ := cmd.Flags().GetString("output")
	format, err := normalizeOutputFormat(output)
	if err != nil {
		return err
	}

	base := strings.TrimRight(server, "/")
	healthRaw, err := fetchURLRaw(base + "/healthz")
	if err != nil {
		return fmt.Errorf("status health failed: %w", err)
	}
	capRaw, err := fetchURLRaw(base + "/v1/capabilities")
	if err != nil {
		return fmt.Errorf("status capabilities failed: %w", err)
	}
	workersRaw, err := fetchURLRaw(base + "/v1/workers")
	if err != nil {
		return fmt.Errorf("status workers failed: %w", err)
	}
	namespacesRaw, err := fetchURLRaw(base + "/v1/namespaces")
	if err != nil {
		return fmt.Errorf("status namespaces failed: %w", err)
	}
	authRaw, authErr := fetchURLRaw(base + "/v1/auth/config")

	var health struct {
		Status string `json:"status"`
	}
	var caps struct {
		GeneratedAt  string `json:"generated_at"`
		Capabilities []any  `json:"capabilities"`
	}
	var workers resources.WorkerList
	var namespaces struct {
		Namespaces []string `json:"namespaces"`
	}
	var auth struct {
		Mode               string `json:"mode"`
		SetupRequired      bool   `json:"setup_required"`
		SetupTokenRequired bool   `json:"setup_token_required"`
	}
	_ = json.Unmarshal(healthRaw, &health)
	_ = json.Unmarshal(capRaw, &caps)
	_ = json.Unmarshal(workersRaw, &workers)
	_ = json.Unmarshal(namespacesRaw, &namespaces)
	if authErr == nil {
		_ = json.Unmarshal(authRaw, &auth)
	}
	if strings.TrimSpace(auth.Mode) == "" {
		auth.Mode = "unknown"
	}

	snapshot := map[string]any{
		"server":                    base,
		"health":                    health.Status,
		"auth_mode":                 auth.Mode,
		"auth_setup_required":       auth.SetupRequired,
		"auth_setup_token_required": auth.SetupTokenRequired,
		"capabilities_count":        len(caps.Capabilities),
		"capabilities_at":           caps.GeneratedAt,
		"workers_count":             len(workers.Items),
		"namespaces_count":          len(namespaces.Namespaces),
	}
	if format != "table" {
		raw, _ := json.Marshal(snapshot)
		return printStructuredOutput(raw, format)
	}
	fmt.Printf("server=%s\n", base)
	fmt.Printf("health=%s\n", health.Status)
	fmt.Printf("auth_mode=%s setup_required=%t setup_token_required=%t\n", auth.Mode, auth.SetupRequired, auth.SetupTokenRequired)
	fmt.Printf("capabilities=%d generated_at=%s\n", len(caps.Capabilities), caps.GeneratedAt)
	fmt.Printf("workers=%d\n", len(workers.Items))
	fmt.Printf("namespaces=%d\n", len(namespaces.Namespaces))
	return nil
}

// --- completion ---

func newCompletionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "completion [bash|zsh|fish]",
		Short: "Generate shell completion script",
		Args:  cobra.ExactArgs(1),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			switch strings.ToLower(strings.TrimSpace(args[0])) {
			case "bash":
				return cmd.Root().GenBashCompletion(os.Stdout)
			case "zsh":
				return cmd.Root().GenZshCompletion(os.Stdout)
			case "fish":
				return cmd.Root().GenFishCompletion(os.Stdout, true)
			default:
				return fmt.Errorf("unsupported shell %q (expected bash, zsh, or fish)", args[0])
			}
		},
	}
}
