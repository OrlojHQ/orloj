package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
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

	"github.com/AnonJon/orloj/crds"
	"github.com/AnonJon/orloj/eventbus"
)

const defaultServer = "http://127.0.0.1:8080"

func Run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}

	switch args[0] {
	case "apply":
		return runApply(args[1:])
	case "get":
		return runGet(args[1:])
	case "delete":
		return runDelete(args[1:])
	case "logs":
		return runLogs(args[1:])
	case "trace":
		return runTrace(args[1:])
	case "graph":
		return runGraph(args[1:])
	case "events":
		return runEvents(args[1:])
	case "rollback":
		return fmt.Errorf("%q is not implemented in MVP", args[0])
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runApply(args []string) error {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	manifestPath := fs.String("f", "", "path to resource manifest")
	server := fs.String("server", defaultServer, "Agent API server URL")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *manifestPath == "" {
		return errors.New("-f is required")
	}

	raw, err := os.ReadFile(*manifestPath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", *manifestPath, err)
	}

	kind, err := crds.DetectKind(raw)
	if err != nil {
		return err
	}

	endpoint, payload, name, err := buildApplyRequest(kind, raw)
	if err != nil {
		return err
	}

	resp, err := http.Post(*server+endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("apply request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("apply failed: %s", bytes.TrimSpace(body))
	}

	fmt.Printf("applied %s/%s\n", strings.ToLower(kind), name)
	return nil
}

func buildApplyRequest(kind string, raw []byte) (string, []byte, string, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "agent":
		obj, err := crds.ParseAgentManifest(raw)
		if err != nil {
			return "", nil, "", err
		}
		payload, err := json.Marshal(obj)
		return "/v1/agents", payload, obj.Metadata.Name, err
	case "agentsystem":
		obj, err := crds.ParseAgentSystemManifest(raw)
		if err != nil {
			return "", nil, "", err
		}
		payload, err := json.Marshal(obj)
		return "/v1/agent-systems", payload, obj.Metadata.Name, err
	case "modelendpoint":
		obj, err := crds.ParseModelEndpointManifest(raw)
		if err != nil {
			return "", nil, "", err
		}
		payload, err := json.Marshal(obj)
		return "/v1/model-endpoints", payload, obj.Metadata.Name, err
	case "tool":
		obj, err := crds.ParseToolManifest(raw)
		if err != nil {
			return "", nil, "", err
		}
		payload, err := json.Marshal(obj)
		return "/v1/tools", payload, obj.Metadata.Name, err
	case "secret":
		obj, err := crds.ParseSecretManifest(raw)
		if err != nil {
			return "", nil, "", err
		}
		payload, err := json.Marshal(obj)
		return "/v1/secrets", payload, obj.Metadata.Name, err
	case "memory":
		obj, err := crds.ParseMemoryManifest(raw)
		if err != nil {
			return "", nil, "", err
		}
		payload, err := json.Marshal(obj)
		return "/v1/memories", payload, obj.Metadata.Name, err
	case "agentpolicy":
		obj, err := crds.ParseAgentPolicyManifest(raw)
		if err != nil {
			return "", nil, "", err
		}
		payload, err := json.Marshal(obj)
		return "/v1/agent-policies", payload, obj.Metadata.Name, err
	case "agentrole":
		obj, err := crds.ParseAgentRoleManifest(raw)
		if err != nil {
			return "", nil, "", err
		}
		payload, err := json.Marshal(obj)
		return "/v1/agent-roles", payload, obj.Metadata.Name, err
	case "toolpermission":
		obj, err := crds.ParseToolPermissionManifest(raw)
		if err != nil {
			return "", nil, "", err
		}
		payload, err := json.Marshal(obj)
		return "/v1/tool-permissions", payload, obj.Metadata.Name, err
	case "task":
		obj, err := crds.ParseTaskManifest(raw)
		if err != nil {
			return "", nil, "", err
		}
		payload, err := json.Marshal(obj)
		return "/v1/tasks", payload, obj.Metadata.Name, err
	case "taskschedule":
		obj, err := crds.ParseTaskScheduleManifest(raw)
		if err != nil {
			return "", nil, "", err
		}
		payload, err := json.Marshal(obj)
		return "/v1/task-schedules", payload, obj.Metadata.Name, err
	case "taskwebhook":
		obj, err := crds.ParseTaskWebhookManifest(raw)
		if err != nil {
			return "", nil, "", err
		}
		payload, err := json.Marshal(obj)
		return "/v1/task-webhooks", payload, obj.Metadata.Name, err
	case "worker":
		obj, err := crds.ParseWorkerManifest(raw)
		if err != nil {
			return "", nil, "", err
		}
		payload, err := json.Marshal(obj)
		return "/v1/workers", payload, obj.Metadata.Name, err
	default:
		return "", nil, "", fmt.Errorf("unsupported kind %q", kind)
	}
}

func runGet(args []string) error {
	if len(args) == 0 {
		return errors.New("resource is required (example: get agents)")
	}

	resource := normalizeResource(args[0])
	if resource == "" {
		return fmt.Errorf("unsupported resource %q", args[0])
	}

	fs := flag.NewFlagSet("get", flag.ContinueOnError)
	server := fs.String("server", defaultServer, "Agent API server URL")
	watch := fs.Bool("w", false, "watch for incremental updates (tasks only)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *watch {
		if resource != "tasks" {
			return errors.New("-w is currently supported for tasks only")
		}
		return watchTasks(*server)
	}

	endpoint, err := listEndpointForResource(resource)
	if err != nil {
		return err
	}

	resp, err := http.Get(*server + endpoint)
	if err != nil {
		return fmt.Errorf("get request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("get failed: %s", bytes.TrimSpace(body))
	}

	switch resource {
	case "agents":
		var list crds.AgentList
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tMODEL\tSTATUS\tTOOLS")
		for _, item := range list.Items {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%d\n", item.Metadata.Name, item.Spec.Model, item.Status.Phase, len(item.Spec.Tools))
		}
		_ = tw.Flush()
	case "agent-systems":
		var list crds.AgentSystemList
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
		var list crds.ModelEndpointList
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tPROVIDER\tBASE_URL\tDEFAULT_MODEL\tSTATUS")
		for _, item := range list.Items {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
				item.Metadata.Name,
				item.Spec.Provider,
				item.Spec.BaseURL,
				item.Spec.DefaultModel,
				item.Status.Phase,
			)
		}
		_ = tw.Flush()
	case "tools":
		var list crds.ToolList
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
		var list crds.SecretList
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tKEYS\tSTATUS")
		for _, item := range list.Items {
			fmt.Fprintf(tw, "%s\t%d\t%s\n", item.Metadata.Name, len(item.Spec.Data), item.Status.Phase)
		}
		_ = tw.Flush()
	case "memories":
		var list crds.MemoryList
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
		var list crds.AgentPolicyList
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tMODE\tSYSTEM_TARGETS\tTASK_TARGETS\tTOKENS\tALLOWED_MODELS\tBLOCKED_TOOLS\tSTATUS")
		for _, item := range list.Items {
			fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%d\t%d\t%d\t%s\n",
				item.Metadata.Name,
				item.Spec.ApplyMode,
				len(item.Spec.TargetSystems),
				len(item.Spec.TargetTasks),
				item.Spec.MaxTokensPerRun,
				len(item.Spec.AllowedModels),
				len(item.Spec.BlockedTools),
				item.Status.Phase,
			)
		}
		_ = tw.Flush()
	case "agent-roles":
		var list crds.AgentRoleList
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tPERMISSIONS\tSTATUS")
		for _, item := range list.Items {
			fmt.Fprintf(tw, "%s\t%d\t%s\n",
				item.Metadata.Name,
				len(item.Spec.Permissions),
				item.Status.Phase,
			)
		}
		_ = tw.Flush()
	case "tool-permissions":
		var list crds.ToolPermissionList
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tTOOL\tACTION\tMODE\tREQUIRED_PERMISSIONS\tSTATUS")
		for _, item := range list.Items {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\n",
				item.Metadata.Name,
				item.Spec.ToolRef,
				item.Spec.Action,
				item.Spec.MatchMode,
				len(item.Spec.RequiredPermissions),
				item.Status.Phase,
			)
		}
		_ = tw.Flush()
	case "tasks":
		var list crds.TaskList
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tSYSTEM\tPRIORITY\tSTATUS\tATTEMPTS\tASSIGNED_WORKER\tCLAIMED_BY\tLEASE_UNTIL\tNEXT_ATTEMPT\tLAST_ERROR")
		for _, item := range list.Items {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\t%s\t%s\t%s\t%s\n",
				item.Metadata.Name,
				item.Spec.System,
				item.Spec.Priority,
				item.Status.Phase,
				item.Status.Attempts,
				item.Status.AssignedWorker,
				item.Status.ClaimedBy,
				item.Status.LeaseUntil,
				item.Status.NextAttemptAt,
				compactError(item.Status.LastError),
			)
		}
		_ = tw.Flush()
	case "task-schedules":
		var list crds.TaskScheduleList
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tTASK_REF\tSCHEDULE\tTIME_ZONE\tSUSPEND\tSTATUS\tLAST_SCHEDULE\tNEXT_SCHEDULE\tACTIVE_RUNS\tLAST_ERROR")
		for _, item := range list.Items {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%t\t%s\t%s\t%s\t%d\t%s\n",
				item.Metadata.Name,
				item.Spec.TaskRef,
				item.Spec.Schedule,
				item.Spec.TimeZone,
				item.Spec.Suspend,
				item.Status.Phase,
				item.Status.LastScheduleTime,
				item.Status.NextScheduleTime,
				len(item.Status.ActiveRuns),
				compactError(item.Status.LastError),
			)
		}
		_ = tw.Flush()
	case "task-webhooks":
		var list crds.TaskWebhookList
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tTASK_REF\tENDPOINT_ID\tENDPOINT_PATH\tSUSPEND\tSTATUS\tLAST_DELIVERY\tLAST_EVENT\tLAST_TASK\tLAST_ERROR")
		for _, item := range list.Items {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%t\t%s\t%s\t%s\t%s\t%s\n",
				item.Metadata.Name,
				item.Spec.TaskRef,
				item.Status.EndpointID,
				item.Status.EndpointPath,
				item.Spec.Suspend,
				item.Status.Phase,
				item.Status.LastDeliveryTime,
				item.Status.LastEventID,
				item.Status.LastTriggeredTask,
				compactError(item.Status.LastError),
			)
		}
		_ = tw.Flush()
	case "workers":
		var list crds.WorkerList
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tREGION\tGPU\tMAX_CONCURRENCY\tSTATUS\tLAST_HEARTBEAT")
		for _, item := range list.Items {
			fmt.Fprintf(tw, "%s\t%s\t%t\t%d\t%s\t%s\n",
				item.Metadata.Name,
				item.Spec.Region,
				item.Spec.Capabilities.GPU,
				item.Spec.MaxConcurrentTasks,
				item.Status.Phase,
				item.Status.LastHeartbeat,
			)
		}
		_ = tw.Flush()
	}

	return nil
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
	case "memories", "memory":
		return "memories"
	case "agent-policies", "agentpolicies", "agentpolicy", "policies", "policy":
		return "agent-policies"
	case "agent-roles", "agentroles", "agentrole", "roles", "role":
		return "agent-roles"
	case "tool-permissions", "toolpermissions", "toolpermission":
		return "tool-permissions"
	case "tasks", "task":
		return "tasks"
	case "task-schedules", "taskschedules", "taskschedule":
		return "task-schedules"
	case "task-webhooks", "taskwebhooks", "taskwebhook":
		return "task-webhooks"
	case "workers", "worker":
		return "workers"
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
	case "memories":
		return "/v1/memories", nil
	case "agent-policies":
		return "/v1/agent-policies", nil
	case "agent-roles":
		return "/v1/agent-roles", nil
	case "tool-permissions":
		return "/v1/tool-permissions", nil
	case "tasks":
		return "/v1/tasks", nil
	case "task-schedules":
		return "/v1/task-schedules", nil
	case "task-webhooks":
		return "/v1/task-webhooks", nil
	case "workers":
		return "/v1/workers", nil
	default:
		return "", fmt.Errorf("unsupported resource %q", resource)
	}
}

func runLogs(args []string) error {
	fs := flag.NewFlagSet("logs", flag.ContinueOnError)
	server := fs.String("server", defaultServer, "Agent API server URL")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		return errors.New("usage: orlojctl logs <agent-name>|task/<task-name>")
	}
	target := fs.Arg(0)
	endpoint := ""
	name := target
	if strings.HasPrefix(strings.ToLower(target), "task/") {
		name = strings.TrimSpace(target[len("task/"):])
		endpoint = *server + "/v1/tasks/" + name + "/logs"
	} else {
		endpoint = *server + "/v1/agents/" + name + "/logs"
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

func runDelete(args []string) error {
	fs := flag.NewFlagSet("delete", flag.ContinueOnError)
	server := fs.String("server", defaultServer, "Agent API server URL")
	namespace := fs.String("namespace", "", "resource namespace override")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		return errors.New("usage: orlojctl delete <resource> <name>")
	}

	resource := normalizeResource(fs.Arg(0))
	if resource == "" {
		return fmt.Errorf("unsupported resource %q", fs.Arg(0))
	}
	name := strings.TrimSpace(fs.Arg(1))
	if name == "" {
		return errors.New("resource name is required")
	}
	endpoint, err := listEndpointForResource(resource)
	if err != nil {
		return err
	}

	url := strings.TrimRight(*server, "/") + endpoint + "/" + name
	if strings.TrimSpace(*namespace) != "" {
		url += "?namespace=" + strings.TrimSpace(*namespace)
	}
	req, err := http.NewRequest(http.MethodDelete, url, nil)
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

func runTrace(args []string) error {
	fs := flag.NewFlagSet("trace", flag.ContinueOnError)
	server := fs.String("server", defaultServer, "Agent API server URL")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		return errors.New("usage: orlojctl trace task <task-name>")
	}
	resource := strings.ToLower(strings.TrimSpace(fs.Arg(0)))
	name := strings.TrimSpace(fs.Arg(1))
	if resource != "task" && resource != "tasks" {
		return fmt.Errorf("unsupported trace resource %q (only task is supported)", resource)
	}
	if name == "" {
		return errors.New("task name is required")
	}

	taskResp, err := http.Get(*server + "/v1/tasks/" + name)
	if err != nil {
		return fmt.Errorf("trace task request failed: %w", err)
	}
	defer taskResp.Body.Close()
	if taskResp.StatusCode >= 300 {
		body, _ := io.ReadAll(taskResp.Body)
		return fmt.Errorf("trace task failed: %s", bytes.TrimSpace(body))
	}
	var task crds.Task
	if err := json.NewDecoder(taskResp.Body).Decode(&task); err != nil {
		return fmt.Errorf("failed to decode task response: %w", err)
	}

	logsResp, err := http.Get(*server + "/v1/tasks/" + name + "/logs")
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
				event.Timestamp,
				event.Type,
				event.Agent,
				event.LatencyMS,
				event.Tokens,
				event.ToolCalls,
				event.MemoryWrites,
				event.Message,
			)
		}
	}
	fmt.Println("Timeline:")
	for _, line := range logsPayload.Logs {
		fmt.Printf("  %s\n", line)
	}
	return nil
}

func runGraph(args []string) error {
	fs := flag.NewFlagSet("graph", flag.ContinueOnError)
	server := fs.String("server", defaultServer, "Agent API server URL")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		return errors.New("usage: orlojctl graph system|task <name>")
	}

	resource := strings.ToLower(strings.TrimSpace(fs.Arg(0)))
	name := strings.TrimSpace(fs.Arg(1))
	if name == "" {
		return errors.New("graph target name is required")
	}

	switch resource {
	case "system", "agent-system", "agentsystem":
		return renderSystemGraph(*server, name)
	case "task", "tasks":
		return renderTaskGraph(*server, name)
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

	var system crds.AgentSystem
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

	var task crds.Task
	if err := json.NewDecoder(taskResp.Body).Decode(&task); err != nil {
		return fmt.Errorf("failed to decode task response: %w", err)
	}

	var system *crds.AgentSystem
	if strings.TrimSpace(task.Spec.System) != "" {
		systemResp, err := http.Get(server + "/v1/agent-systems/" + task.Spec.System)
		if err == nil {
			defer systemResp.Body.Close()
			if systemResp.StatusCode < 300 {
				var loaded crds.AgentSystem
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

func systemEntryPoints(system crds.AgentSystem) []string {
	if len(system.Spec.Agents) == 0 {
		return nil
	}
	indegree := make(map[string]int, len(system.Spec.Agents))
	for _, name := range system.Spec.Agents {
		indegree[name] = 0
	}
	for _, node := range system.Spec.Graph {
		for _, to := range crds.GraphOutgoingAgents(node) {
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

func systemGraphLines(system crds.AgentSystem) []string {
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
			targets = crds.GraphOutgoingAgents(edge)
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

func taskExecutionOrder(task crds.Task, system *crds.AgentSystem) []string {
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

func taskOrderFromSystem(system crds.AgentSystem) []string {
	if len(system.Spec.Agents) == 0 {
		return nil
	}
	if len(system.Spec.Graph) == 0 {
		out := make([]string, len(system.Spec.Agents))
		copy(out, system.Spec.Agents)
		return out
	}

	// Topological traversal from entrypoints; append disconnected nodes in declaration order.
	indegree := make(map[string]int, len(system.Spec.Agents))
	for _, agent := range system.Spec.Agents {
		indegree[agent] = 0
	}
	for _, node := range system.Spec.Graph {
		for _, to := range crds.GraphOutgoingAgents(node) {
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
		for _, to := range crds.GraphOutgoingAgents(node) {
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

func taskGraphMetrics(task crds.Task, order []string) map[string]taskAgentMetrics {
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

func printUsage() {
	fmt.Print(`orlojctl - Agents-as-Code CLI (MVP)

Usage:
  orlojctl apply -f <resource.yaml>
  orlojctl get [-w] agents|agent-systems|model-endpoints|tools|secrets|memories|agent-policies|agent-roles|tool-permissions|tasks|task-schedules|task-webhooks|workers
  orlojctl delete <resource> <name>
  orlojctl logs <agent-name>|task/<task-name>
  orlojctl trace task <task-name>
  orlojctl graph system|task <name>
  orlojctl events [--source=<s>] [--type=<t>] [--kind=<k>] [--name=<n>] [--namespace=<ns>] [--since=<id>] [--once] [--timeout=<duration>] [--raw]
`)
}

func compactError(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 80 {
		return s
	}
	return s[:77] + "..."
}

func watchTasks(server string) error {
	resp, err := http.Get(server + "/v1/tasks/watch")
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
			Type     string    `json:"type"`
			Resource crds.Task `json:"resource"`
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

func runEvents(args []string) error {
	fs := flag.NewFlagSet("events", flag.ContinueOnError)
	server := fs.String("server", defaultServer, "Agent API server URL")
	since := fs.Uint64("since", 0, "event id to resume from")
	source := fs.String("source", "", "filter by event source (example: apiserver)")
	eventType := fs.String("type", "", "filter by event type (example: resource.created)")
	kind := fs.String("kind", "", "filter by resource kind (example: Task)")
	name := fs.String("name", "", "filter by resource name")
	namespace := fs.String("namespace", "", "filter by resource namespace")
	once := fs.Bool("once", false, "exit after first matching event")
	timeout := fs.Duration("timeout", 0, "max time to wait for matching events (example: 30s)")
	raw := fs.Bool("raw", false, "print raw event JSON payload")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *timeout < 0 {
		return errors.New("--timeout must be >= 0")
	}

	streamURL, err := eventsWatchURL(*server, eventFilters{
		Since:     *since,
		Source:    *source,
		Type:      *eventType,
		Kind:      *kind,
		Name:      *name,
		Namespace: *namespace,
	})
	if err != nil {
		return err
	}

	reqCtx := context.Background()
	cancel := func() {}
	if *timeout > 0 {
		reqCtx, cancel = context.WithTimeout(reqCtx, *timeout)
	}
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, streamURL, nil)
	if err != nil {
		return fmt.Errorf("events watch request build failed: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if errors.Is(reqCtx.Err(), context.DeadlineExceeded) {
			return eventsTimeoutError(*timeout, *once, 0)
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
		if *raw {
			fmt.Println(payload)
			if *once {
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
		if *once {
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(reqCtx.Err(), context.DeadlineExceeded) {
			return eventsTimeoutError(*timeout, *once, received)
		}
		return fmt.Errorf("events watch stream error: %w", err)
	}
	if errors.Is(reqCtx.Err(), context.DeadlineExceeded) {
		return eventsTimeoutError(*timeout, *once, received)
	}
	if *once {
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
