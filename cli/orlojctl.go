package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/OrlojHQ/orloj/eventbus"
	"github.com/OrlojHQ/orloj/internal/version"
	"github.com/OrlojHQ/orloj/resources"
)

func Run(args []string) error {
	cmd := newRootCommand()
	cmd.SetArgs(args)
	return cmd.Execute()
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "orlojctl",
		Short:         "Orloj CLI",
		Version:       version.String(),
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadOrlojctlConfig()
			if err != nil {
				return fmt.Errorf("orlojctl config: %w", err)
			}
			resolvedCliConfig = cfg

			token, _ := cmd.Root().PersistentFlags().GetString("api-token")
			if token == "" {
				token = strings.TrimSpace(os.Getenv("ORLOJCTL_API_TOKEN"))
			}
			if token == "" {
				token = strings.TrimSpace(os.Getenv("ORLOJ_API_TOKEN"))
			}
			if token == "" {
				token = tokenFromProfile(cfg)
			}
			configureDefaultHTTPClient(token)
			return nil
		},
	}
	root.PersistentFlags().String("api-token", "", "Bearer token applied to all HTTP requests")
	root.PersistentFlags().StringP("namespace", "n", "", "Default namespace for namespace-aware commands")
	root.PersistentFlags().String("server", "", "Agent API server URL")

	root.AddCommand(
		newApplyCommand(),
		newCreateCommand(),
		newApproveCommand(),
		newDenyCommand(),
		newRequestChangesCommand(),
		newGetCommand(),
		newDeleteCommand(),
		newDescribeCommand(),
		newEditCommand(),
		newDiffCommand(),
		newWaitCommand(),
		newCancelCommand(),
		newRetryCommand(),
		newTopCommand(),
		newRunCommand(),
		newInitCommand(),
		newLogsCommand(),
		newTraceCommand(),
		newGraphCommand(),
		newEventsCommand(),
		newMessagesCommand(),
		newMetricsCommand(),
		newMemoryEntriesCommand(),
		newHealthCommand(),
		newStatusCommand(),
		newCompletionCommand(),
		newAdminCommand(),
		newAuthCommand(),
		newConfigCommand(),
		newSealCommand(),
		newValidateCommand(),
		newToolCommand(),
		newEvalCommand(),
		newA2ACommand(),
	)

	return root
}

func resolveServer(cmd *cobra.Command) string {
	s, _ := cmd.Flags().GetString("server")
	if s != "" {
		return s
	}
	return defaultServerResolved(resolvedCliConfig)
}

func resolveNamespace(cmd *cobra.Command) string {
	ns, _ := cmd.Flags().GetString("namespace")
	return strings.TrimSpace(ns)
}

type authRoundTripper struct {
	base  http.RoundTripper
	token string
}

func (t authRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.TrimSpace(t.token) == "" {
		return t.base.RoundTrip(req)
	}
	if strings.TrimSpace(req.Header.Get("Authorization")) != "" {
		return t.base.RoundTrip(req)
	}
	next := req.Clone(req.Context())
	next.Header = req.Header.Clone()
	next.Header.Set("Authorization", "Bearer "+strings.TrimSpace(t.token))
	return t.base.RoundTrip(next)
}

func configureDefaultHTTPClient(token string) {
	base := http.DefaultTransport
	if base == nil {
		base = http.DefaultTransport
	}
	http.DefaultClient = &http.Client{
		Transport: authRoundTripper{base: base, token: strings.TrimSpace(token)},
	}
}

// --- apply ---

func newApplyCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply a resource manifest",
		RunE:  runApply,
	}
	cmd.Flags().StringP("file", "f", "", "path to resource manifest file or directory")
	cmd.Flags().Bool("run", false, "include runnable Task manifests and start EvalRun manifests immediately")
	cmd.Flags().Bool("dry-run", false, "preview changes without persisting")
	return cmd
}

func runApply(cmd *cobra.Command, args []string) error {
	manifestPath, _ := cmd.Flags().GetString("file")
	includeRunnable, _ := cmd.Flags().GetBool("run")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
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
	skippedNonManifest := 0
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

	var applyErrs []string
	applied := 0
	created := 0
	updated := 0
	unchanged := 0
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			applyErrs = append(applyErrs, fmt.Sprintf("%s: %v", f, err))
			continue
		}

		kind, err := resources.DetectKind(raw)
		if err != nil {
			if isDir {
				skippedNonManifest++
				continue
			}
			applyErrs = append(applyErrs, fmt.Sprintf("%s: %v", f, err))
			continue
		}

		endpoint, payload, name, err := buildApplyRequest(kind, raw)
		if err != nil {
			applyErrs = append(applyErrs, fmt.Sprintf("%s: %v", f, err))
			continue
		}
		if strings.TrimSpace(ns) != "" {
			payload, err = overridePayloadNamespace(payload, ns)
			if err != nil {
				applyErrs = append(applyErrs, fmt.Sprintf("%s: %v", f, err))
				continue
			}
		}
		if dryRun {
			action, previewErr := previewApplyChange(server, endpoint, name, payload)
			if previewErr != nil {
				applyErrs = append(applyErrs, fmt.Sprintf("%s: %v", f, previewErr))
				continue
			}
			switch action {
			case "create":
				created++
			case "update":
				updated++
			default:
				unchanged++
			}
			fmt.Printf("dry-run %s %s/%s\n", action, strings.ToLower(kind), name)
			applied++
			continue
		}

		postURL := server + endpoint
		if includeRunnable && endpoint == "/v1/tasks" {
			postURL += "?rerun=true"
		}
		if includeRunnable && endpoint == "/v1/eval-runs" {
			postURL += "?run=true"
		}
		resp, err := http.Post(postURL, "application/json", bytes.NewReader(payload))
		if err != nil {
			applyErrs = append(applyErrs, fmt.Sprintf("%s: apply request failed: %v", f, err))
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode >= 300 {
			applyErrs = append(applyErrs, fmt.Sprintf("%s: %s", f, bytes.TrimSpace(body)))
			continue
		}

		actualName := nameFromResponseBody(body, name)
		isSuspendedEvalRun := !includeRunnable && endpoint == "/v1/eval-runs"
		if actualName != name {
			fmt.Printf("applied %s/%s (rerun as %s)\n", strings.ToLower(kind), name, actualName)
		} else if isSuspendedEvalRun {
			fmt.Printf("applied %s/%s (suspended; use --run or 'orlojctl eval start %s' to execute)\n", strings.ToLower(kind), name, name)
		} else {
			fmt.Printf("applied %s/%s\n", strings.ToLower(kind), name)
		}
		applied++
	}

	if len(applyErrs) > 0 {
		if skippedRunnableTasks > 0 {
			fmt.Printf("\n%d applied, %d skipped runnable task(s), %d failed:\n", applied, skippedRunnableTasks, len(applyErrs))
		} else {
			fmt.Printf("\n%d applied, %d failed:\n", applied, len(applyErrs))
		}
		for _, e := range applyErrs {
			fmt.Printf("  error  %s\n", e)
		}
		return fmt.Errorf("apply failed for %d file(s)", len(applyErrs))
	}

	if dryRun {
		fmt.Printf("\ndry-run summary: %d checked, %d create, %d update, %d unchanged\n", applied, created, updated, unchanged)
		return nil
	}

	if isDir || applied > 1 || skippedRunnableTasks > 0 {
		if skippedRunnableTasks > 0 {
			fmt.Printf("\n%d file(s) applied, %d runnable task(s) skipped\n", applied, skippedRunnableTasks)
		} else {
			fmt.Printf("\n%d file(s) applied\n", applied)
		}
	}
	return nil
}

type stringSliceFlag []string

func (f *stringSliceFlag) String() string { return strings.Join(*f, ", ") }
func (f *stringSliceFlag) Set(v string) error {
	*f = append(*f, v)
	return nil
}
func (f *stringSliceFlag) Type() string { return "stringSlice" }

// --- create ---

func newCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a resource",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newCreateSecretCommand(), newCreateTokenCommand())
	return cmd
}

func newCreateSecretCommand() *cobra.Command {
	var literals stringSliceFlag
	cmd := &cobra.Command{
		Use:   "secret <name>",
		Short: "Create a secret from literal key=value pairs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			server := resolveServer(cmd)
			ns := resolveNamespace(cmd)
			if ns == "" {
				ns = "default"
			}
			name := strings.TrimSpace(args[0])
			if name == "" {
				return errors.New("secret name cannot be empty")
			}
			if len(literals) == 0 {
				return errors.New("at least one --from-literal key=value is required")
			}

			stringData := make(map[string]string, len(literals))
			for _, lit := range literals {
				parts := strings.SplitN(lit, "=", 2)
				if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
					return fmt.Errorf("invalid --from-literal %q: expected key=value", lit)
				}
				stringData[strings.TrimSpace(parts[0])] = parts[1]
			}

			secret := resources.Secret{
				APIVersion: "orloj.dev/v1",
				Kind:       "Secret",
				Metadata:   resources.ObjectMeta{Name: name, Namespace: ns},
				Spec:       resources.SecretSpec{StringData: stringData},
			}
			if err := secret.Normalize(); err != nil {
				return fmt.Errorf("invalid secret: %w", err)
			}

			payload, err := json.Marshal(secret)
			if err != nil {
				return fmt.Errorf("marshal secret: %w", err)
			}

			resp, err := http.Post(server+"/v1/secrets", "application/json", bytes.NewReader(payload))
			if err != nil {
				return fmt.Errorf("create secret request failed: %w", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode >= 300 {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("create secret failed: %s", bytes.TrimSpace(body))
			}

			fmt.Printf("created secret/%s\n", name)
			return nil
		},
	}
	cmd.Flags().Var(&literals, "from-literal", "key=value pair (repeatable)")
	return cmd
}

func newCreateTokenCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token <name>",
		Short: "Create an API token",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			server := resolveServer(cmd)
			role, _ := cmd.Flags().GetString("role")
			name := strings.TrimSpace(args[0])
			if name == "" {
				return errors.New("token name is required")
			}
			if strings.TrimSpace(role) == "" {
				return errors.New("--role is required")
			}
			payload, _ := json.Marshal(map[string]string{
				"name": name,
				"role": strings.TrimSpace(role),
			})
			resp, err := http.Post(strings.TrimRight(server, "/")+"/v1/tokens", "application/json", bytes.NewReader(payload))
			if err != nil {
				return fmt.Errorf("create token request failed: %w", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode >= 300 {
				return fmt.Errorf("create token failed: %s", bytes.TrimSpace(body))
			}
			var created struct {
				Name      string `json:"name"`
				Role      string `json:"role"`
				Token     string `json:"token"`
				CreatedAt string `json:"created_at"`
			}
			if err := json.Unmarshal(body, &created); err != nil {
				return fmt.Errorf("decode create-token response failed: %w", err)
			}
			fmt.Printf("created token/%s role=%s\n", created.Name, created.Role)
			fmt.Printf("token: %s\n", strings.TrimSpace(created.Token))
			return nil
		},
	}
	cmd.Flags().String("role", "", "token role: admin|writer|reader|controller")
	return cmd
}

// --- approve / deny ---

func newApproveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "approve <resource> <name>",
		Short: "Approve a pending approval",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runApprovalDecision(cmd, "approve", args)
		},
	}
	cmd.Flags().String("decided-by", "", "identity of the decision maker")
	cmd.Flags().String("comment", "", "comment for the decision")
	cmd.Flags().String("reason", "", "reason for the decision")
	return cmd
}

func newDenyCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deny <resource> <name>",
		Short: "Deny a pending approval",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runApprovalDecision(cmd, "deny", args)
		},
	}
	cmd.Flags().String("decided-by", "", "identity of the decision maker")
	cmd.Flags().String("comment", "", "comment for the decision")
	cmd.Flags().String("reason", "", "reason for the decision")
	return cmd
}

func newRequestChangesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "request-changes <resource> <name>",
		Short: "Request changes on a pending task approval",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runApprovalDecision(cmd, "request-changes", args)
		},
	}
	cmd.Flags().String("decided-by", "", "identity of the decision maker")
	cmd.Flags().String("comment", "", "comment for the requested changes")
	cmd.Flags().String("reason", "", "reason for the requested changes")
	return cmd
}

func runApprovalDecision(cmd *cobra.Command, action string, args []string) error {
	server := resolveServer(cmd)
	ns := resolveNamespace(cmd)
	decidedBy, _ := cmd.Flags().GetString("decided-by")
	comment, _ := cmd.Flags().GetString("comment")
	reason, _ := cmd.Flags().GetString("reason")
	if strings.TrimSpace(comment) == "" {
		comment = reason
	}

	resourceArg := args[0]
	resource := normalizeResource(resourceArg)
	if resource != "tool-approvals" && resource != "task-approvals" {
		return fmt.Errorf("unsupported %s resource %q (supported: tool-approval, task-approval)", action, resourceArg)
	}
	if action == "request-changes" && resource != "task-approvals" {
		return fmt.Errorf("request-changes is only supported for task-approval resources")
	}
	name := strings.TrimSpace(args[1])
	if name == "" {
		return errors.New("approval name is required")
	}

	path := "/v1/" + resource + "/" + url.PathEscape(name) + "/" + action
	requestURL := strings.TrimRight(server, "/") + path
	if strings.TrimSpace(ns) != "" {
		requestURL += "?namespace=" + url.QueryEscape(strings.TrimSpace(ns))
	}

	bodyPayload := map[string]string{}
	if trimmed := strings.TrimSpace(decidedBy); trimmed != "" {
		bodyPayload["decided_by"] = trimmed
	}
	if trimmed := strings.TrimSpace(comment); trimmed != "" {
		bodyPayload["comment"] = trimmed
		bodyPayload["reason"] = trimmed
	}

	var (
		body   io.Reader
		reqErr error
	)
	if len(bodyPayload) > 0 {
		raw, marshalErr := json.Marshal(bodyPayload)
		if marshalErr != nil {
			return fmt.Errorf("failed to encode decision payload: %w", marshalErr)
		}
		body = bytes.NewReader(raw)
	}

	req, reqErr := http.NewRequest(http.MethodPost, requestURL, body)
	if reqErr != nil {
		return fmt.Errorf("%s request build failed: %w", action, reqErr)
	}
	if len(bodyPayload) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s request failed: %w", action, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s failed: %s", action, bytes.TrimSpace(payload))
	}

	switch action {
	case "approve":
		fmt.Printf("approved %s/%s\n", strings.TrimSuffix(resource, "s"), name)
	case "deny":
		fmt.Printf("denied %s/%s\n", strings.TrimSuffix(resource, "s"), name)
	default:
		fmt.Printf("requested changes for %s/%s\n", strings.TrimSuffix(resource, "s"), name)
	}
	return nil
}

// --- get ---

func newGetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <resource> [name]",
		Short: "Display one or many resources",
		Args:  cobra.RangeArgs(1, 2),
		RunE:  runGet,
	}
	cmd.Flags().BoolP("watch", "w", false, "watch for incremental updates (tasks only)")
	cmd.Flags().StringP("output", "o", "table", "output format: table|json|yaml")
	return cmd
}

func runGet(cmd *cobra.Command, args []string) error {
	if isMemoryEntriesResource(args[0]) {
		return errors.New("use 'orlojctl memory-entries <name>' for memory entries")
	}

	resource := normalizeResource(args[0])
	if resource == "" {
		return fmt.Errorf("unsupported resource %q", args[0])
	}

	server := resolveServer(cmd)
	ns := resolveNamespace(cmd)
	watch, _ := cmd.Flags().GetBool("watch")
	output, _ := cmd.Flags().GetString("output")
	format, err := normalizeOutputFormat(output)
	if err != nil {
		return err
	}

	name := ""
	if len(args) == 2 {
		name = strings.TrimSpace(args[1])
		if name == "" {
			return errors.New("resource name cannot be empty")
		}
	}
	if resource == "tokens" && name != "" {
		return errors.New("usage: orlojctl get tokens")
	}
	if watch {
		if resource != "tasks" {
			return errors.New("-w is currently supported for tasks only")
		}
		if name != "" {
			return errors.New("-w does not support selecting a single resource name")
		}
		return watchTasks(server, ns)
	}

	endpoint, err := listEndpointForResource(resource)
	if err != nil {
		return err
	}
	requestURL := strings.TrimRight(server, "/") + endpoint
	if name != "" {
		requestURL += "/" + url.PathEscape(name)
	}
	if strings.TrimSpace(ns) != "" {
		sep := "?"
		if strings.Contains(requestURL, "?") {
			sep = "&"
		}
		requestURL += sep + "namespace=" + url.QueryEscape(strings.TrimSpace(ns))
	}

	resp, err := http.Get(requestURL)
	if err != nil {
		return fmt.Errorf("get request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("get failed: %s", bytes.TrimSpace(body))
	}
	if format != "table" || name != "" {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response: %w", err)
		}
		return printStructuredOutput(body, format)
	}

	switch resource {
	case "agents":
		var list resources.AgentList
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tMODEL_REF\tSTATUS\tTOOLS")
		for _, item := range list.Items {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%d\n", item.Metadata.Name, item.Spec.ModelRef, item.Status.Phase, len(item.Spec.Tools))
		}
		_ = tw.Flush()
	case "agent-systems":
		var list resources.AgentSystemList
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tSTATUS\tAGENTS")
		for _, item := range list.Items {
			fmt.Fprintf(tw, "%s\t%s\t%d\n", item.Metadata.Name, item.Status.Phase, len(item.Spec.Agents))
		}
		_ = tw.Flush()
	case "model-endpoints":
		var list resources.ModelEndpointList
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tPROVIDER\tBASE_URL\tDEFAULT_MODEL\tSTATUS")
		for _, item := range list.Items {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
				item.Metadata.Name, item.Spec.Provider, item.Spec.BaseURL, item.Spec.DefaultModel, item.Status.Phase)
		}
		_ = tw.Flush()
	case "tools":
		var list resources.ToolList
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tTYPE\tENDPOINT\tSTATUS")
		for _, item := range list.Items {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", item.Metadata.Name, item.Spec.Type, item.Spec.Endpoint, item.Status.Phase)
		}
		_ = tw.Flush()
	case "secrets":
		var list resources.SecretList
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tKEYS\tSTATUS")
		for _, item := range list.Items {
			fmt.Fprintf(tw, "%s\t%d\t%s\n", item.Metadata.Name, len(item.Spec.Data), item.Status.Phase)
		}
		_ = tw.Flush()
	case "sealed-secrets":
		var list resources.SealedSecretList
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tKEYS\tSTATUS")
		for _, item := range list.Items {
			fmt.Fprintf(tw, "%s\t%d\t%s\n", item.Metadata.Name, len(item.Spec.EncryptedData), item.Status.Phase)
		}
		_ = tw.Flush()
	case "memories":
		var list resources.MemoryList
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tTYPE\tPROVIDER\tSTATUS")
		for _, item := range list.Items {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", item.Metadata.Name, item.Spec.Type, item.Spec.Provider, item.Status.Phase)
		}
		_ = tw.Flush()
	case "agent-policies":
		var list resources.AgentPolicyList
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tMODE\tSYSTEM_TARGETS\tTASK_TARGETS\tTOKENS\tALLOWED_MODELS\tBLOCKED_TOOLS\tSTATUS")
		for _, item := range list.Items {
			fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%d\t%d\t%d\t%s\n",
				item.Metadata.Name, item.Spec.ApplyMode,
				len(item.Spec.TargetSystems), len(item.Spec.TargetTasks),
				item.Spec.MaxTokensPerRun, len(item.Spec.AllowedModels),
				len(item.Spec.BlockedTools), item.Status.Phase)
		}
		_ = tw.Flush()
	case "agent-roles":
		var list resources.AgentRoleList
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tPERMISSIONS\tSTATUS")
		for _, item := range list.Items {
			fmt.Fprintf(tw, "%s\t%d\t%s\n", item.Metadata.Name, len(item.Spec.Permissions), item.Status.Phase)
		}
		_ = tw.Flush()
	case "tool-permissions":
		var list resources.ToolPermissionList
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tTOOL\tACTION\tMODE\tREQUIRED_PERMISSIONS\tSTATUS")
		for _, item := range list.Items {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\n",
				item.Metadata.Name, item.Spec.ToolRef, item.Spec.Action,
				item.Spec.MatchMode, len(item.Spec.RequiredPermissions), item.Status.Phase)
		}
		_ = tw.Flush()
	case "tool-approvals":
		var list resources.ToolApprovalList
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tTASK\tTOOL\tINPUT\tOPERATION\tAGENT\tPHASE\tEXPIRES_AT\tDECIDED_BY")
		for _, item := range list.Items {
			inputPreview := item.Spec.Input
			if len(inputPreview) > 40 {
				inputPreview = inputPreview[:37] + "..."
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				item.Metadata.Name, item.Spec.TaskRef, item.Spec.Tool,
				inputPreview, item.Spec.OperationClass, item.Spec.Agent,
				item.Status.Phase, item.Status.ExpiresAt, item.Status.DecidedBy)
		}
		_ = tw.Flush()
	case "task-approvals":
		var list resources.TaskApprovalList
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tTASK\tCHECKPOINT\tTYPE\tAGENT\tPHASE\tREVIEW_CYCLE\tDECIDED_BY")
		for _, item := range list.Items {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\n",
				item.Metadata.Name, item.Spec.TaskRef, item.Spec.CheckpointID,
				item.Spec.CheckpointType, item.Spec.Agent, item.Status.Phase,
				item.Spec.ReviewCycle, item.Status.DecidedBy)
		}
		_ = tw.Flush()
	case "tasks":
		var list resources.TaskList
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tSYSTEM\tPRIORITY\tSTATUS\tATTEMPTS\tASSIGNED_WORKER\tCLAIMED_BY\tLEASE_UNTIL\tNEXT_ATTEMPT\tLAST_ERROR")
		for _, item := range list.Items {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\t%s\t%s\t%s\t%s\n",
				item.Metadata.Name, item.Spec.System, item.Spec.Priority,
				item.Status.Phase, item.Status.Attempts, item.Status.AssignedWorker,
				item.Status.ClaimedBy, item.Status.LeaseUntil, item.Status.NextAttemptAt,
				compactError(item.Status.LastError))
		}
		_ = tw.Flush()
	case "task-schedules":
		var list resources.TaskScheduleList
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tTASK_REF\tSCHEDULE\tTIME_ZONE\tSUSPEND\tSTATUS\tLAST_SCHEDULE\tNEXT_SCHEDULE\tACTIVE_RUNS\tLAST_ERROR")
		for _, item := range list.Items {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%t\t%s\t%s\t%s\t%d\t%s\n",
				item.Metadata.Name, item.Spec.TaskRef, item.Spec.Schedule,
				item.Spec.TimeZone, item.Spec.Suspend, item.Status.Phase,
				item.Status.LastScheduleTime, item.Status.NextScheduleTime,
				len(item.Status.ActiveRuns), compactError(item.Status.LastError))
		}
		_ = tw.Flush()
	case "task-webhooks":
		var list resources.TaskWebhookList
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tTASK_REF\tENDPOINT_ID\tENDPOINT_PATH\tSUSPEND\tSTATUS\tLAST_DELIVERY\tLAST_EVENT\tLAST_TASK\tLAST_ERROR")
		for _, item := range list.Items {
			taskRefCol := item.Spec.TaskRef
			if taskRefCol == "" && item.Spec.TaskTemplate != nil {
				taskRefCol = "<inline>"
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%t\t%s\t%s\t%s\t%s\t%s\n",
				item.Metadata.Name, taskRefCol, item.Status.EndpointID,
				item.Status.EndpointPath, item.Spec.Suspend, item.Status.Phase,
				item.Status.LastDeliveryTime, item.Status.LastEventID,
				item.Status.LastTriggeredTask, compactError(item.Status.LastError))
		}
		_ = tw.Flush()
	case "workers":
		var list resources.WorkerList
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tREGION\tGPU\tMAX_CONCURRENCY\tSTATUS\tLAST_HEARTBEAT")
		for _, item := range list.Items {
			fmt.Fprintf(tw, "%s\t%s\t%t\t%d\t%s\t%s\n",
				item.Metadata.Name, item.Spec.Region, item.Spec.Capabilities.GPU,
				item.Spec.MaxConcurrentTasks, item.Status.Phase, item.Status.LastHeartbeat)
		}
		_ = tw.Flush()
	case "mcp-servers":
		var list resources.McpServerList
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tTRANSPORT\tSTATUS\tTOOLS\tLAST_SYNCED")
		for _, item := range list.Items {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n",
				item.Metadata.Name, item.Spec.Transport, item.Status.Phase,
				len(item.Status.GeneratedTools), item.Status.LastSyncedAt)
		}
		_ = tw.Flush()
	case "tokens":
		var list struct {
			Items []struct {
				Name      string `json:"name"`
				Role      string `json:"role"`
				CreatedAt string `json:"created_at"`
			} `json:"items"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tROLE\tCREATED_AT")
		for _, item := range list.Items {
			fmt.Fprintf(tw, "%s\t%s\t%s\n", item.Name, item.Role, item.CreatedAt)
		}
		_ = tw.Flush()
	case "eval-datasets":
		var list resources.EvalDatasetList
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tSAMPLES\tSTATUS")
		for _, item := range list.Items {
			fmt.Fprintf(tw, "%s\t%d\t%s\n", item.Metadata.Name, len(item.Spec.Samples), item.Status.Phase)
		}
		_ = tw.Flush()
	case "eval-runs":
		var list resources.EvalRunList
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tDATASET\tSYSTEM\tPHASE\tPASS_RATE\tSAMPLES")
		for _, item := range list.Items {
			passRate := ""
			if item.Status.Phase == resources.EvalRunPhaseSucceeded {
				passRate = fmt.Sprintf("%.0f%%", item.Status.Summary.PassRate*100)
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%d/%d\n",
				item.Metadata.Name, item.Spec.DatasetRef, item.Spec.System,
				item.Status.Phase, passRate,
				item.Status.CompletedSamples, item.Status.TotalSamples)
		}
		_ = tw.Flush()
	}

	return nil
}

// --- delete ---

func newDeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <resource> <name>",
		Short: "Delete a resource",
		Args:  cobra.ExactArgs(2),
		RunE:  runDelete,
	}
	return cmd
}

func runDelete(cmd *cobra.Command, args []string) error {
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
	endpoint, err := listEndpointForResource(resource)
	if err != nil {
		return err
	}

	reqURL := strings.TrimRight(server, "/") + endpoint + "/" + name
	if strings.TrimSpace(ns) != "" {
		reqURL += "?namespace=" + strings.TrimSpace(ns)
	}
	req, err := http.NewRequest(http.MethodDelete, reqURL, nil)
	if err != nil {
		return fmt.Errorf("delete request build failed: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("delete request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete failed: %s", bytes.TrimSpace(body))
	}
	fmt.Printf("deleted %s/%s\n", resource, name)
	return nil
}

// --- logs ---

func newLogsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "logs <agent-name>|task/<task-name>",
		Short: "View agent or task logs",
		Args:  cobra.ExactArgs(1),
		RunE:  runLogs,
	}
}

func runLogs(cmd *cobra.Command, args []string) error {
	server := resolveServer(cmd)
	target := args[0]
	endpoint := ""
	name := target
	if strings.HasPrefix(strings.ToLower(target), "task/") {
		name = strings.TrimSpace(target[len("task/"):])
		endpoint = server + "/v1/tasks/" + name + "/logs"
	} else {
		endpoint = server + "/v1/agents/" + name + "/logs"
	}
	if name == "" {
		return errors.New("logs target name is required")
	}

	resp, err := http.Get(endpoint)
	if err != nil {
		return fmt.Errorf("logs request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("logs failed: %s", bytes.TrimSpace(body))
	}

	var payload struct {
		Name string   `json:"name"`
		Logs []string `json:"logs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return fmt.Errorf("failed to decode logs response: %w", err)
	}

	for _, line := range payload.Logs {
		fmt.Println(line)
	}
	return nil
}

// --- trace ---

func newTraceCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "trace <resource> <name>",
		Short: "Show execution trace for a task",
		Args:  cobra.ExactArgs(2),
		RunE:  runTrace,
	}
}

func runTrace(cmd *cobra.Command, args []string) error {
	server := resolveServer(cmd)
	resource := strings.ToLower(strings.TrimSpace(args[0]))
	name := strings.TrimSpace(args[1])
	if resource != "task" && resource != "tasks" {
		return fmt.Errorf("unsupported trace resource %q (only task is supported)", resource)
	}
	if name == "" {
		return errors.New("task name is required")
	}

	taskResp, err := http.Get(server + "/v1/tasks/" + name)
	if err != nil {
		return fmt.Errorf("trace task request failed: %w", err)
	}
	defer taskResp.Body.Close()
	if taskResp.StatusCode >= 300 {
		body, _ := io.ReadAll(taskResp.Body)
		return fmt.Errorf("trace task failed: %s", bytes.TrimSpace(body))
	}
	var task resources.Task
	if err := json.NewDecoder(taskResp.Body).Decode(&task); err != nil {
		return fmt.Errorf("failed to decode task response: %w", err)
	}

	logsResp, err := http.Get(server + "/v1/tasks/" + name + "/logs")
	if err != nil {
		return fmt.Errorf("trace task logs request failed: %w", err)
	}
	defer logsResp.Body.Close()
	if logsResp.StatusCode >= 300 {
		body, _ := io.ReadAll(logsResp.Body)
		return fmt.Errorf("trace task logs failed: %s", bytes.TrimSpace(body))
	}
	var logsPayload struct {
		Name string   `json:"name"`
		Logs []string `json:"logs"`
	}
	if err := json.NewDecoder(logsResp.Body).Decode(&logsPayload); err != nil {
		return fmt.Errorf("failed to decode task logs: %w", err)
	}

	fmt.Printf("Task: %s\n", task.Metadata.Name)
	fmt.Printf("Phase: %s\n", task.Status.Phase)
	fmt.Printf("Attempts: %d\n", task.Status.Attempts)
	fmt.Printf("ClaimedBy: %s\n", task.Status.ClaimedBy)
	if task.Status.LeaseUntil != "" {
		fmt.Printf("LeaseUntil: %s\n", task.Status.LeaseUntil)
	}
	if task.Status.LastError != "" {
		fmt.Printf("LastError: %s\n", task.Status.LastError)
	}
	if len(task.Status.Output) > 0 {
		if order := strings.TrimSpace(task.Status.Output["execution_order"]); order != "" {
			fmt.Printf("ExecutionOrder: %s\n", order)
		}
		if total := strings.TrimSpace(task.Status.Output["tokens_estimated_total"]); total != "" {
			fmt.Printf("EstimatedTokens: %s\n", total)
		}
	}
	if len(task.Status.History) > 0 {
		fmt.Println("History:")
		for _, event := range task.Status.History {
			fmt.Printf("  %s [%s] worker=%s %s\n", event.Timestamp, event.Type, event.Worker, event.Message)
		}
	}
	if len(task.Status.Messages) > 0 {
		fmt.Println("Messages:")
		for _, message := range task.Status.Messages {
			fmt.Printf("  %s %s -> %s %s\n", message.Timestamp, message.FromAgent, message.ToAgent, message.Content)
		}
	}
	if len(task.Status.Trace) > 0 {
		fmt.Println("Trace:")
		for _, event := range task.Status.Trace {
			fmt.Printf("  %s [%s] agent=%s latency_ms=%d tokens=%d tools=%d memory=%d %s\n",
				event.Timestamp, event.Type, event.Agent, event.LatencyMS,
				event.Tokens, event.ToolCalls, event.MemoryWrites, event.Message)
		}
	}
	fmt.Println("Timeline:")
	for _, line := range logsPayload.Logs {
		fmt.Printf("  %s\n", line)
	}
	return nil
}

// --- graph ---

func newGraphCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "graph <system|task> <name>",
		Short: "Display agent system or task execution graph",
		Args:  cobra.ExactArgs(2),
		RunE:  runGraph,
	}
}

func runGraph(cmd *cobra.Command, args []string) error {
	server := resolveServer(cmd)
	resource := strings.ToLower(strings.TrimSpace(args[0]))
	name := strings.TrimSpace(args[1])
	if name == "" {
		return errors.New("graph target name is required")
	}

	switch resource {
	case "system", "agent-system", "agentsystem":
		return renderSystemGraph(server, name)
	case "task", "tasks":
		return renderTaskGraph(server, name)
	default:
		return fmt.Errorf("unsupported graph resource %q (expected system or task)", resource)
	}
}

func renderSystemGraph(server, name string) error {
	resp, err := http.Get(server + "/v1/agent-systems/" + name)
	if err != nil {
		return fmt.Errorf("graph system request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("graph system failed: %s", bytes.TrimSpace(body))
	}

	var system resources.AgentSystem
	if err := json.NewDecoder(resp.Body).Decode(&system); err != nil {
		return fmt.Errorf("failed to decode agentsystem response: %w", err)
	}

	fmt.Printf("System: %s\n", system.Metadata.Name)
	fmt.Printf("Agents: %d\n", len(system.Spec.Agents))
	roots := systemEntryPoints(system)
	if len(roots) > 0 {
		fmt.Printf("Entrypoints: %s\n", strings.Join(roots, ", "))
	}
	fmt.Println("Graph:")
	for _, line := range systemGraphLines(system) {
		fmt.Printf("  %s\n", line)
	}
	return nil
}

func renderTaskGraph(server, name string) error {
	taskResp, err := http.Get(server + "/v1/tasks/" + name)
	if err != nil {
		return fmt.Errorf("graph task request failed: %w", err)
	}
	defer taskResp.Body.Close()
	if taskResp.StatusCode >= 300 {
		body, _ := io.ReadAll(taskResp.Body)
		return fmt.Errorf("graph task failed: %s", bytes.TrimSpace(body))
	}

	var task resources.Task
	if err := json.NewDecoder(taskResp.Body).Decode(&task); err != nil {
		return fmt.Errorf("failed to decode task response: %w", err)
	}

	var system *resources.AgentSystem
	if strings.TrimSpace(task.Spec.System) != "" {
		systemResp, err := http.Get(server + "/v1/agent-systems/" + task.Spec.System)
		if err == nil {
			defer systemResp.Body.Close()
			if systemResp.StatusCode < 300 {
				var loaded resources.AgentSystem
				if decodeErr := json.NewDecoder(systemResp.Body).Decode(&loaded); decodeErr == nil {
					system = &loaded
				}
			}
		}
	}

	order := taskExecutionOrder(task, system)
	metrics := taskGraphMetrics(task, order)

	fmt.Printf("Task: %s\n", task.Metadata.Name)
	fmt.Printf("System: %s\n", task.Spec.System)
	fmt.Printf("Phase: %s\n", task.Status.Phase)
	fmt.Printf("Attempts: %d\n", task.Status.Attempts)
	if total := strings.TrimSpace(task.Status.Output["tokens_estimated_total"]); total != "" {
		fmt.Printf("EstimatedTokens: %s\n", total)
	}
	if task.Status.LastError != "" {
		fmt.Printf("LastError: %s\n", task.Status.LastError)
	}

	fmt.Println("Execution Graph:")
	if len(order) == 0 {
		fmt.Println("  (no execution data)")
		return nil
	}
	for i, agent := range order {
		node := metrics[agent]
		parts := make([]string, 0, 6)
		if node.Status != "" {
			parts = append(parts, "status="+node.Status)
		}
		if node.LatencyMS > 0 {
			parts = append(parts, "latency_ms="+strconv.FormatInt(node.LatencyMS, 10))
		}
		if node.Tokens > 0 {
			parts = append(parts, "tokens="+strconv.Itoa(node.Tokens))
		}
		if node.ToolCalls > 0 {
			parts = append(parts, "tools="+strconv.Itoa(node.ToolCalls))
		}
		if node.MemoryWrites > 0 {
			parts = append(parts, "memory="+strconv.Itoa(node.MemoryWrites))
		}
		if node.Message != "" {
			parts = append(parts, "message="+node.Message)
		}
		line := agent
		if len(parts) > 0 {
			line += " (" + strings.Join(parts, ", ") + ")"
		}
		fmt.Printf("  %s\n", line)
		if i < len(order)-1 {
			fmt.Printf("    -> %s\n", order[i+1])
		}
	}
	return nil
}

func systemEntryPoints(system resources.AgentSystem) []string {
	if len(system.Spec.Agents) == 0 {
		return nil
	}
	indegree := make(map[string]int, len(system.Spec.Agents))
	for _, name := range system.Spec.Agents {
		indegree[name] = 0
	}
	for _, node := range system.Spec.Graph {
		for _, to := range resources.GraphOutgoingAgents(node) {
			if _, ok := indegree[to]; ok {
				indegree[to]++
			}
		}
	}
	roots := make([]string, 0, len(indegree))
	for _, name := range system.Spec.Agents {
		if indegree[name] == 0 {
			roots = append(roots, name)
		}
	}
	return roots
}

func systemGraphLines(system resources.AgentSystem) []string {
	if len(system.Spec.Agents) == 0 {
		return nil
	}
	lines := make([]string, 0, len(system.Spec.Agents))
	useDeclaredOrder := len(system.Spec.Graph) == 0
	for idx, name := range system.Spec.Agents {
		targets := make([]string, 0, 2)
		if useDeclaredOrder {
			if idx+1 < len(system.Spec.Agents) {
				targets = append(targets, system.Spec.Agents[idx+1])
			}
		} else if edge, ok := system.Spec.Graph[name]; ok {
			targets = resources.GraphOutgoingAgents(edge)
		}
		if len(targets) == 0 {
			lines = append(lines, fmt.Sprintf("%s -> (end)", name))
			continue
		}
		for _, to := range targets {
			lines = append(lines, fmt.Sprintf("%s -> %s", name, to))
		}
	}
	return lines
}

func taskExecutionOrder(task resources.Task, system *resources.AgentSystem) []string {
	if order := parseExecutionOrder(task.Status.Output); len(order) > 0 {
		return order
	}
	if system != nil {
		return taskOrderFromSystem(*system)
	}

	seen := map[string]struct{}{}
	order := make([]string, 0, len(task.Status.Trace))
	for _, event := range task.Status.Trace {
		agent := strings.TrimSpace(event.Agent)
		if agent == "" {
			continue
		}
		if _, ok := seen[agent]; ok {
			continue
		}
		seen[agent] = struct{}{}
		order = append(order, agent)
	}
	return order
}

func parseExecutionOrder(output map[string]string) []string {
	order := make([]string, 0)
	joined := strings.TrimSpace(output["execution_order"])
	if joined != "" {
		parts := strings.Split(joined, "->")
		for _, part := range parts {
			name := strings.TrimSpace(part)
			if name != "" {
				order = append(order, name)
			}
		}
		return order
	}

	indexToName := map[int]string{}
	for key, value := range output {
		if !strings.HasPrefix(key, "agent.") || !strings.HasSuffix(key, ".name") {
			continue
		}
		trimmed := strings.TrimPrefix(key, "agent.")
		parts := strings.Split(trimmed, ".")
		if len(parts) < 2 {
			continue
		}
		idx, err := strconv.Atoi(parts[0])
		if err != nil || idx <= 0 {
			continue
		}
		name := strings.TrimSpace(value)
		if name == "" {
			continue
		}
		indexToName[idx] = name
	}
	if len(indexToName) == 0 {
		return order
	}

	indexes := make([]int, 0, len(indexToName))
	for idx := range indexToName {
		indexes = append(indexes, idx)
	}
	sort.Ints(indexes)
	for _, idx := range indexes {
		order = append(order, indexToName[idx])
	}
	return order
}

func taskOrderFromSystem(system resources.AgentSystem) []string {
	if len(system.Spec.Agents) == 0 {
		return nil
	}
	if len(system.Spec.Graph) == 0 {
		out := make([]string, len(system.Spec.Agents))
		copy(out, system.Spec.Agents)
		return out
	}

	indegree := make(map[string]int, len(system.Spec.Agents))
	for _, agent := range system.Spec.Agents {
		indegree[agent] = 0
	}
	for _, node := range system.Spec.Graph {
		for _, to := range resources.GraphOutgoingAgents(node) {
			if _, ok := indegree[to]; ok {
				indegree[to]++
			}
		}
	}
	queue := make([]string, 0, len(system.Spec.Agents))
	queued := make(map[string]struct{}, len(system.Spec.Agents))
	for _, agent := range system.Spec.Agents {
		if indegree[agent] != 0 {
			continue
		}
		queue = append(queue, agent)
		queued[agent] = struct{}{}
	}
	seen := map[string]struct{}{}
	order := make([]string, 0, len(system.Spec.Agents))
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if _, ok := seen[current]; ok {
			continue
		}
		seen[current] = struct{}{}
		order = append(order, current)
		node, ok := system.Spec.Graph[current]
		if !ok {
			continue
		}
		for _, to := range resources.GraphOutgoingAgents(node) {
			if _, tracked := indegree[to]; !tracked {
				continue
			}
			indegree[to]--
			if indegree[to] == 0 {
				if _, alreadyQueued := queued[to]; alreadyQueued {
					continue
				}
				queue = append(queue, to)
				queued[to] = struct{}{}
			}
		}
	}
	for _, name := range system.Spec.Agents {
		if _, ok := seen[name]; ok {
			continue
		}
		order = append(order, name)
	}
	return order
}

type taskAgentMetrics struct {
	Status       string
	Message      string
	LatencyMS    int64
	Tokens       int
	ToolCalls    int
	MemoryWrites int
}

func taskGraphMetrics(task resources.Task, order []string) map[string]taskAgentMetrics {
	metrics := make(map[string]taskAgentMetrics, len(order))
	for _, name := range order {
		metrics[name] = taskAgentMetrics{Status: "pending"}
	}

	for _, event := range task.Status.Trace {
		agent := strings.TrimSpace(event.Agent)
		if agent == "" {
			continue
		}
		current := metrics[agent]
		switch strings.ToLower(strings.TrimSpace(event.Type)) {
		case "agent_start":
			current.Status = "running"
			current.Message = ""
		case "agent_end":
			current.Status = "succeeded"
			current.LatencyMS = event.LatencyMS
			current.Tokens = event.Tokens
			current.ToolCalls = event.ToolCalls
			current.MemoryWrites = event.MemoryWrites
			current.Message = strings.TrimSpace(event.Message)
		case "agent_error", "policy_violation", "agent_missing", "token_budget_exceeded":
			current.Status = "failed"
			current.Message = strings.TrimSpace(event.Message)
		}
		metrics[agent] = current
	}

	for idx, agent := range order {
		prefix := fmt.Sprintf("agent.%d.", idx+1)
		current := metrics[agent]
		if current.Status == "" {
			current.Status = "pending"
		}
		if current.LatencyMS == 0 {
			current.LatencyMS = parseInt64OrZero(task.Status.Output[prefix+"duration_ms"])
		}
		if current.Tokens == 0 {
			current.Tokens = parseIntOrZero(task.Status.Output[prefix+"estimated_tokens"])
		}
		if current.ToolCalls == 0 {
			current.ToolCalls = parseIntOrZero(task.Status.Output[prefix+"tool_calls"])
		}
		if current.MemoryWrites == 0 {
			current.MemoryWrites = parseIntOrZero(task.Status.Output[prefix+"memory_writes"])
		}
		if current.Message == "" {
			current.Message = strings.TrimSpace(task.Status.Output[prefix+"last_event"])
		}
		metrics[agent] = current
	}
	return metrics
}

func parseIntOrZero(value string) int {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return n
}

func parseInt64OrZero(value string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// --- run ---

func newRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run [key=value ...]",
		Short: "Create and watch a task execution",
		RunE:  runRun,
	}
	cmd.Flags().String("system", "", "AgentSystem name to execute (required)")
	cmd.Flags().Duration("poll", 2*time.Second, "status poll interval")
	cmd.Flags().Duration("timeout", 5*time.Minute, "maximum wait time")
	return cmd
}

func runRun(cmd *cobra.Command, args []string) error {
	server := resolveServer(cmd)
	ns := resolveNamespace(cmd)
	if ns == "" {
		ns = "default"
	}
	system, _ := cmd.Flags().GetString("system")
	pollInterval, _ := cmd.Flags().GetDuration("poll")
	timeout, _ := cmd.Flags().GetDuration("timeout")

	if system == "" {
		return errors.New("--system is required")
	}

	input := make(map[string]string)
	for _, arg := range args {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid input %q: expected key=value", arg)
		}
		input[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}

	taskName := fmt.Sprintf("run-%s-%d", system, time.Now().UnixMilli())
	task := resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: taskName, Namespace: ns},
		Spec: resources.TaskSpec{
			System: system,
			Input:  input,
		},
	}

	payload, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("marshal task: %w", err)
	}

	createURL := fmt.Sprintf("%s/v1/tasks?namespace=%s", server, url.QueryEscape(ns))
	resp, err := http.Post(createURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create task: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create task failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	fmt.Printf("task %s created, watching...\n", taskName)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for task %s", taskName)
		case <-ticker.C:
			taskURL := fmt.Sprintf("%s/v1/tasks/%s?namespace=%s", server, url.PathEscape(taskName), url.QueryEscape(ns))
			getResp, err := http.Get(taskURL)
			if err != nil {
				fmt.Fprintf(os.Stderr, "poll error: %v\n", err)
				continue
			}
			body, _ := io.ReadAll(getResp.Body)
			getResp.Body.Close()
			if getResp.StatusCode >= 300 {
				fmt.Fprintf(os.Stderr, "poll error (%d): %s\n", getResp.StatusCode, strings.TrimSpace(string(body)))
				continue
			}

			var result resources.Task
			if err := json.Unmarshal(body, &result); err != nil {
				fmt.Fprintf(os.Stderr, "parse error: %v\n", err)
				continue
			}

			phase := strings.ToLower(strings.TrimSpace(result.Status.Phase))
			switch phase {
			case "succeeded":
				fmt.Printf("task %s succeeded\n", taskName)
				if result.Status.Output != nil {
					out, _ := json.MarshalIndent(result.Status.Output, "", "  ")
					fmt.Println(string(out))
				}
				return nil
			case "failed", "deadletter":
				fmt.Printf("task %s %s\n", taskName, phase)
				if result.Status.LastError != "" {
					fmt.Printf("error: %s\n", result.Status.LastError)
				}
				return fmt.Errorf("task %s", phase)
			default:
				fmt.Printf("task %s: %s\n", taskName, result.Status.Phase)
			}
		}
	}
}

// --- events ---

func newEventsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "events",
		Short: "Watch event stream",
		RunE:  runEvents,
	}
	cmd.Flags().Uint64("since", 0, "event id to resume from")
	cmd.Flags().String("source", "", "filter by event source")
	cmd.Flags().String("type", "", "filter by event type")
	cmd.Flags().String("kind", "", "filter by resource kind")
	cmd.Flags().String("name", "", "filter by resource name")
	cmd.Flags().Bool("once", false, "exit after first matching event")
	cmd.Flags().Duration("timeout", 0, "max time to wait for matching events")
	cmd.Flags().Bool("raw", false, "print raw event JSON payload")
	return cmd
}

func runEvents(cmd *cobra.Command, args []string) error {
	server := resolveServer(cmd)
	ns := resolveNamespace(cmd)
	since, _ := cmd.Flags().GetUint64("since")
	source, _ := cmd.Flags().GetString("source")
	eventType, _ := cmd.Flags().GetString("type")
	kind, _ := cmd.Flags().GetString("kind")
	nameFilter, _ := cmd.Flags().GetString("name")
	once, _ := cmd.Flags().GetBool("once")
	timeout, _ := cmd.Flags().GetDuration("timeout")
	raw, _ := cmd.Flags().GetBool("raw")

	if timeout < 0 {
		return errors.New("--timeout must be >= 0")
	}

	streamURL, err := eventsWatchURL(server, eventFilters{
		Since:     since,
		Source:    source,
		Type:      eventType,
		Kind:      kind,
		Name:      nameFilter,
		Namespace: ns,
	})
	if err != nil {
		return err
	}

	reqCtx := context.Background()
	cancel := func() {}
	if timeout > 0 {
		reqCtx, cancel = context.WithTimeout(reqCtx, timeout)
	}
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, streamURL, nil)
	if err != nil {
		return fmt.Errorf("events watch request build failed: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if errors.Is(reqCtx.Err(), context.DeadlineExceeded) {
			return eventsTimeoutError(timeout, once, 0)
		}
		return fmt.Errorf("events watch request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("events watch failed: %s", bytes.TrimSpace(body))
	}

	scanner := bufio.NewScanner(resp.Body)
	buffer := make([]byte, 0, 64*1024)
	scanner.Buffer(buffer, 1024*1024)
	received := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		received++
		if raw {
			fmt.Println(payload)
			if once {
				return nil
			}
			continue
		}
		var evt eventbus.Event
		if err := json.Unmarshal([]byte(payload), &evt); err != nil {
			fmt.Printf("%s event=decode_error payload=%s\n", time.Now().UTC().Format(time.RFC3339), payload)
			continue
		}
		fmt.Println(formatEventLine(evt))
		if once {
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(reqCtx.Err(), context.DeadlineExceeded) {
			return eventsTimeoutError(timeout, once, received)
		}
		return fmt.Errorf("events watch stream error: %w", err)
	}
	if errors.Is(reqCtx.Err(), context.DeadlineExceeded) {
		return eventsTimeoutError(timeout, once, received)
	}
	if once {
		return errors.New("event stream closed before receiving a matching event")
	}
	return nil
}

type eventFilters struct {
	Since     uint64
	Source    string
	Type      string
	Kind      string
	Name      string
	Namespace string
}

func eventsWatchURL(server string, filters eventFilters) (string, error) {
	base, err := url.Parse(strings.TrimSpace(server))
	if err != nil {
		return "", fmt.Errorf("invalid --server URL %q: %w", server, err)
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/v1/events/watch"
	q := base.Query()
	if filters.Since > 0 {
		q.Set("since", strconv.FormatUint(filters.Since, 10))
	}
	if strings.TrimSpace(filters.Source) != "" {
		q.Set("source", strings.TrimSpace(filters.Source))
	}
	if strings.TrimSpace(filters.Type) != "" {
		q.Set("type", strings.TrimSpace(filters.Type))
	}
	if strings.TrimSpace(filters.Kind) != "" {
		q.Set("kind", strings.TrimSpace(filters.Kind))
	}
	if strings.TrimSpace(filters.Name) != "" {
		q.Set("name", strings.TrimSpace(filters.Name))
	}
	if strings.TrimSpace(filters.Namespace) != "" {
		q.Set("namespace", strings.TrimSpace(filters.Namespace))
	}
	base.RawQuery = q.Encode()
	return base.String(), nil
}

func formatEventLine(evt eventbus.Event) string {
	ts := strings.TrimSpace(evt.Timestamp)
	if ts == "" {
		ts = time.Now().UTC().Format(time.RFC3339)
	}

	parts := []string{
		ts,
		"id=" + strconv.FormatUint(evt.ID, 10),
	}
	if strings.TrimSpace(evt.Source) != "" {
		parts = append(parts, "source="+evt.Source)
	}
	parts = append(parts, "type="+evt.Type)
	if strings.TrimSpace(evt.Kind) != "" {
		parts = append(parts, "kind="+evt.Kind)
	}
	if strings.TrimSpace(evt.Name) != "" {
		parts = append(parts, "name="+evt.Name)
	}
	if strings.TrimSpace(evt.Namespace) != "" {
		parts = append(parts, "namespace="+evt.Namespace)
	}
	if strings.TrimSpace(evt.Action) != "" {
		parts = append(parts, "action="+evt.Action)
	}
	if strings.TrimSpace(evt.Message) != "" {
		parts = append(parts, "message="+evt.Message)
	}
	return strings.Join(parts, " ")
}

func eventsTimeoutError(timeout time.Duration, once bool, received int) error {
	if timeout <= 0 {
		return nil
	}
	if once || received == 0 {
		return fmt.Errorf("events watch timed out after %s without receiving a matching event", timeout)
	}
	return nil
}

// --- admin ---

func newAdminCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Administrative commands",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		newAdminCreateUserCommand(),
		newAdminListUsersCommand(),
		newAdminDeleteUserCommand(),
		newAdminResetPasswordCommand(),
	)
	return cmd
}

func newAdminCreateUserCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create-user <username>",
		Short: "Create a new user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			server := resolveServer(cmd)
			role, _ := cmd.Flags().GetString("role")
			username := strings.TrimSpace(args[0])
			if username == "" {
				return errors.New("username is required")
			}
			payload, _ := json.Marshal(map[string]string{
				"username": username,
				"role":     strings.TrimSpace(role),
			})
			resp, err := http.Post(strings.TrimRight(server, "/")+"/v1/auth/users", "application/json", bytes.NewReader(payload))
			if err != nil {
				return fmt.Errorf("create user request failed: %w", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode >= 300 {
				return fmt.Errorf("create user failed: %s", bytes.TrimSpace(body))
			}
			var created struct {
				Username string `json:"username"`
				Role     string `json:"role"`
				Password string `json:"password"`
			}
			if err := json.Unmarshal(body, &created); err != nil {
				return fmt.Errorf("decode create-user response failed: %w", err)
			}
			fmt.Printf("created user/%s role=%s\n", created.Username, created.Role)
			fmt.Printf("password: %s\n", strings.TrimSpace(created.Password))
			return nil
		},
	}
	cmd.Flags().String("role", "reader", "user role: admin|writer|reader|controller")
	return cmd
}

func newAdminListUsersCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list-users",
		Short: "List all users",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			server := resolveServer(cmd)
			resp, err := http.Get(strings.TrimRight(server, "/") + "/v1/auth/users")
			if err != nil {
				return fmt.Errorf("list users request failed: %w", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode >= 300 {
				return fmt.Errorf("list users failed: %s", bytes.TrimSpace(body))
			}
			var list struct {
				Items []struct {
					Username  string `json:"username"`
					Role      string `json:"role"`
					CreatedAt string `json:"created_at"`
					UpdatedAt string `json:"updated_at"`
				} `json:"items"`
			}
			if err := json.Unmarshal(body, &list); err != nil {
				return fmt.Errorf("decode list-users response failed: %w", err)
			}
			tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "USERNAME\tROLE\tCREATED_AT\tUPDATED_AT")
			for _, item := range list.Items {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", item.Username, item.Role, item.CreatedAt, item.UpdatedAt)
			}
			_ = tw.Flush()
			return nil
		},
	}
}

func newAdminDeleteUserCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "delete-user <username>",
		Short: "Delete a user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			server := resolveServer(cmd)
			username := strings.TrimSpace(args[0])
			if username == "" {
				return errors.New("username is required")
			}
			req, err := http.NewRequest(http.MethodDelete, strings.TrimRight(server, "/")+"/v1/auth/users/"+url.PathEscape(username), nil)
			if err != nil {
				return fmt.Errorf("delete user request build failed: %w", err)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("delete user request failed: %w", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode >= 300 {
				return fmt.Errorf("delete user failed: %s", bytes.TrimSpace(body))
			}
			fmt.Printf("deleted user/%s\n", username)
			return nil
		},
	}
}

func newAdminResetPasswordCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reset-password",
		Short: "Reset a user password",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			server := resolveServer(cmd)
			username, _ := cmd.Flags().GetString("username")
			newPassword, _ := cmd.Flags().GetString("new-password")
			if strings.TrimSpace(username) == "" {
				return errors.New("--username is required")
			}
			if strings.TrimSpace(newPassword) == "" {
				return errors.New("--new-password is required")
			}
			payload, _ := json.Marshal(map[string]string{
				"username":     strings.TrimSpace(username),
				"new_password": strings.TrimSpace(newPassword),
			})
			resp, err := http.Post(strings.TrimRight(server, "/")+"/v1/auth/admin/reset-password", "application/json", bytes.NewReader(payload))
			if err != nil {
				return fmt.Errorf("password reset request failed: %w", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode >= 300 {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("password reset failed: %s", bytes.TrimSpace(body))
			}
			fmt.Printf("password reset for %s\n", strings.TrimSpace(username))
			return nil
		},
	}
	cmd.Flags().String("username", "", "target username")
	cmd.Flags().String("new-password", "", "new admin password")
	return cmd
}

// --- auth ---

func newAuthCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authentication commands",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newAuthWhoamiCommand(), newAuthLoginCommand())
	return cmd
}

func newAuthLoginCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate and store a CLI token in the active profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			server := resolveServer(cmd)
			username, _ := cmd.Flags().GetString("username")
			password, _ := cmd.Flags().GetString("password")

			username = strings.TrimSpace(username)
			if username == "" {
				fmt.Print("Username: ")
				scanner := bufio.NewScanner(os.Stdin)
				if scanner.Scan() {
					username = strings.TrimSpace(scanner.Text())
				}
				if username == "" {
					return errors.New("username is required")
				}
			}

			if password == "" {
				fmt.Print("Password: ")
				raw, err := readPassword()
				if err != nil {
					return fmt.Errorf("read password: %w", err)
				}
				fmt.Println()
				password = string(raw)
			}
			if strings.TrimSpace(password) == "" {
				return errors.New("password is required")
			}

			payload, _ := json.Marshal(map[string]string{
				"username": username,
				"password": password,
			})
			loginURL := strings.TrimRight(server, "/") + "/v1/auth/cli-token"
			resp, err := http.Post(loginURL, "application/json", bytes.NewReader(payload))
			if err != nil {
				return fmt.Errorf("login request failed: %w", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode == http.StatusUnauthorized {
				return errors.New("invalid credentials")
			}
			if resp.StatusCode >= 300 {
				return fmt.Errorf("login failed: %s", bytes.TrimSpace(body))
			}

			var result struct {
				Name     string `json:"name"`
				Role     string `json:"role"`
				Token    string `json:"token"`
				Username string `json:"username"`
			}
			if err := json.Unmarshal(body, &result); err != nil {
				return fmt.Errorf("decode login response: %w", err)
			}

			cfg, err := loadOrlojctlConfig()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			profileName := strings.TrimSpace(cfg.CurrentProfile)
			if profileName == "" {
				profileName = "default"
			}
			pe := cfg.Profiles[profileName]
			pe.Token = strings.TrimSpace(result.Token)
			if strings.TrimSpace(pe.Server) == "" {
				pe.Server = strings.TrimRight(strings.TrimSpace(server), "/")
			}
			cfg.Profiles[profileName] = pe
			cfg.CurrentProfile = profileName
			if err := saveOrlojctlConfig(cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			resolvedCliConfig = cfg

			fmt.Printf("logged in as %s (role=%s) on %s\n", result.Username, result.Role, server)
			fmt.Printf("token saved to profile %q\n", profileName)
			return nil
		},
	}
	cmd.Flags().StringP("username", "u", "", "username (prompted if omitted)")
	cmd.Flags().StringP("password", "p", "", "password (prompted if omitted)")
	return cmd
}

func newAuthWhoamiCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Display current identity",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			server := resolveServer(cmd)
			resp, err := http.Get(strings.TrimRight(server, "/") + "/v1/auth/me")
			if err != nil {
				return fmt.Errorf("whoami request failed: %w", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode >= 300 {
				return fmt.Errorf("whoami failed: %s", bytes.TrimSpace(body))
			}
			var me struct {
				Authenticated bool   `json:"authenticated"`
				Username      string `json:"username"`
				Name          string `json:"name"`
				Role          string `json:"role"`
				Method        string `json:"method"`
			}
			if err := json.Unmarshal(body, &me); err != nil {
				return fmt.Errorf("decode whoami response failed: %w", err)
			}
			if !me.Authenticated {
				return errors.New("not authenticated")
			}
			name := strings.TrimSpace(me.Name)
			if name == "" {
				name = strings.TrimSpace(me.Username)
			}
			if name == "" {
				name = "anonymous"
			}
			role := strings.TrimSpace(me.Role)
			if role == "" {
				role = "none"
			}
			method := strings.TrimSpace(me.Method)
			if method == "" {
				method = "unknown"
			}
			fmt.Printf("method=%s name=%s role=%s\n", method, name, role)
			return nil
		},
	}
}

// --- shared helpers ---

var applyEndpoints = map[string]string{
	"agent":          "/v1/agents",
	"agentsystem":    "/v1/agent-systems",
	"modelendpoint":  "/v1/model-endpoints",
	"tool":           "/v1/tools",
	"secret":         "/v1/secrets",
	"sealedsecret":   "/v1/sealed-secrets",
	"memory":         "/v1/memories",
	"contextadapter": "/v1/context-adapters",
	"agentpolicy":    "/v1/agent-policies",
	"agentrole":      "/v1/agent-roles",
	"toolpermission": "/v1/tool-permissions",
	"toolapproval":   "/v1/tool-approvals",
	"taskapproval":   "/v1/task-approvals",
	"task":           "/v1/tasks",
	"taskschedule":   "/v1/task-schedules",
	"taskwebhook":    "/v1/task-webhooks",
	"worker":         "/v1/workers",
	"mcpserver":      "/v1/mcp-servers",
	"evaldataset":    "/v1/eval-datasets",
	"evalrun":        "/v1/eval-runs",
}

func buildApplyRequest(kind string, raw []byte) (string, []byte, string, error) {
	normKind, name, obj, err := resources.ParseManifest(kind, raw)
	if err != nil {
		return "", nil, "", err
	}
	endpoint, ok := applyEndpoints[normKind]
	if !ok {
		return "", nil, "", fmt.Errorf("unsupported kind %q", normKind)
	}
	payload, err := json.Marshal(obj)
	if err != nil {
		return "", nil, "", err
	}
	return endpoint, payload, name, nil
}

func nameFromResponseBody(body []byte, fallback string) string {
	var obj struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(body, &obj); err != nil {
		return fallback
	}
	if strings.TrimSpace(obj.Metadata.Name) == "" {
		return fallback
	}
	return obj.Metadata.Name
}

func normalizeResource(resource string) string {
	switch strings.ToLower(strings.TrimSpace(resource)) {
	case "agents", "agent":
		return "agents"
	case "agent-systems", "agentsystems", "agentsystem":
		return "agent-systems"
	case "model-endpoints", "modelendpoints", "modelendpoint":
		return "model-endpoints"
	case "tools", "tool":
		return "tools"
	case "secrets", "secret":
		return "secrets"
	case "sealed-secrets", "sealedsecrets", "sealedsecret":
		return "sealed-secrets"
	case "memories", "memory":
		return "memories"
	case "agent-policies", "agentpolicies", "agentpolicy", "policies", "policy":
		return "agent-policies"
	case "agent-roles", "agentroles", "agentrole", "roles", "role":
		return "agent-roles"
	case "tool-permissions", "toolpermissions", "toolpermission":
		return "tool-permissions"
	case "tool-approvals", "tool-approval", "toolapprovals", "toolapproval":
		return "tool-approvals"
	case "task-approvals", "task-approval", "taskapprovals", "taskapproval":
		return "task-approvals"
	case "tasks", "task":
		return "tasks"
	case "task-schedules", "taskschedules", "taskschedule":
		return "task-schedules"
	case "task-webhooks", "taskwebhooks", "taskwebhook":
		return "task-webhooks"
	case "workers", "worker":
		return "workers"
	case "mcp-servers", "mcpservers", "mcpserver":
		return "mcp-servers"
	case "tokens", "token":
		return "tokens"
	case "eval-datasets", "evaldatasets", "evaldataset":
		return "eval-datasets"
	case "eval-runs", "evalruns", "evalrun":
		return "eval-runs"
	default:
		return ""
	}
}

func listEndpointForResource(resource string) (string, error) {
	switch resource {
	case "agents":
		return "/v1/agents", nil
	case "agent-systems":
		return "/v1/agent-systems", nil
	case "model-endpoints":
		return "/v1/model-endpoints", nil
	case "tools":
		return "/v1/tools", nil
	case "secrets":
		return "/v1/secrets", nil
	case "sealed-secrets":
		return "/v1/sealed-secrets", nil
	case "memories":
		return "/v1/memories", nil
	case "agent-policies":
		return "/v1/agent-policies", nil
	case "agent-roles":
		return "/v1/agent-roles", nil
	case "tool-permissions":
		return "/v1/tool-permissions", nil
	case "tool-approvals":
		return "/v1/tool-approvals", nil
	case "task-approvals":
		return "/v1/task-approvals", nil
	case "tasks":
		return "/v1/tasks", nil
	case "task-schedules":
		return "/v1/task-schedules", nil
	case "task-webhooks":
		return "/v1/task-webhooks", nil
	case "workers":
		return "/v1/workers", nil
	case "mcp-servers":
		return "/v1/mcp-servers", nil
	case "tokens":
		return "/v1/tokens", nil
	case "eval-datasets":
		return "/v1/eval-datasets", nil
	case "eval-runs":
		return "/v1/eval-runs", nil
	default:
		return "", fmt.Errorf("unsupported resource %q", resource)
	}
}

func compactError(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 80 {
		return s
	}
	return s[:77] + "..."
}

func watchTasks(server, namespace string) error {
	endpoint := strings.TrimRight(server, "/") + "/v1/tasks/watch"
	if strings.TrimSpace(namespace) != "" {
		endpoint += "?namespace=" + url.QueryEscape(strings.TrimSpace(namespace))
	}
	resp, err := http.Get(endpoint)
	if err != nil {
		return fmt.Errorf("task watch request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("task watch failed: %s", bytes.TrimSpace(body))
	}

	scanner := bufio.NewScanner(resp.Body)
	buffer := make([]byte, 0, 64*1024)
	scanner.Buffer(buffer, 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		var event struct {
			Type     string         `json:"type"`
			Resource resources.Task `json:"resource"`
		}
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			fmt.Printf("%s event=decode_error payload=%s\n", time.Now().UTC().Format(time.RFC3339), payload)
			continue
		}
		fmt.Printf("%s event=%s task=%s phase=%s attempts=%d claimed_by=%s\n",
			time.Now().UTC().Format(time.RFC3339),
			event.Type,
			event.Resource.Metadata.Name,
			event.Resource.Status.Phase,
			event.Resource.Status.Attempts,
			event.Resource.Status.ClaimedBy,
		)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("task watch stream error: %w", err)
	}
	return nil
}
