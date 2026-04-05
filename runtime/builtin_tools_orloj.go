package agentruntime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/store"
)

const (
	ToolOrlojTaskCreate = "orloj.task.create"
	ToolOrlojTaskList   = "orloj.task.list"
)

var builtinOrlojTools = map[string]struct{}{
	ToolOrlojTaskCreate: {},
	ToolOrlojTaskList:   {},
}

// IsBuiltinOrlojTool returns true if the tool name is a built-in orloj tool.
func IsBuiltinOrlojTool(name string) bool {
	_, ok := builtinOrlojTools[strings.TrimSpace(name)]
	return ok
}

// BuiltinOrlojToolNames returns the sorted list of built-in orloj tool names.
func BuiltinOrlojToolNames() []string {
	names := make([]string, 0, len(builtinOrlojTools))
	for name := range builtinOrlojTools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// AgentHasOrlojTools returns true if any of the agent's allowed_tools are orloj tools.
func AgentHasOrlojTools(agent resources.Agent) bool {
	for _, t := range agent.Spec.AllowedTools {
		if IsBuiltinOrlojTool(t) {
			return true
		}
	}
	return false
}

// OrlojTaskStore is the subset of store.TaskStore used by OrlojToolRuntime.
type OrlojTaskStore interface {
	Get(ctx context.Context, name string) (resources.Task, bool, error)
	Upsert(ctx context.Context, item resources.Task) (resources.Task, error)
	ListPaged(ctx context.Context, limit, offset int, namespace string) ([]resources.Task, error)
}

// OrlojToolConfig holds policy-derived limits for the orloj tool runtime.
type OrlojToolConfig struct {
	ParentNamespace string
	ParentTaskName  string
	CurrentDepth    int
	MaxDepth        int
	MaxChildren     int
}

// OrlojToolRuntime wraps a ToolRuntime and intercepts built-in orloj tool calls.
type OrlojToolRuntime struct {
	delegate   ToolRuntime
	taskStore  OrlojTaskStore
	config     OrlojToolConfig
	childCount int
}

func NewOrlojToolRuntime(delegate ToolRuntime, taskStore OrlojTaskStore, config OrlojToolConfig) *OrlojToolRuntime {
	if config.MaxDepth <= 0 {
		config.MaxDepth = 5
	}
	if config.MaxChildren <= 0 {
		config.MaxChildren = 20
	}
	return &OrlojToolRuntime{
		delegate:  delegate,
		taskStore: taskStore,
		config:    config,
	}
}

func (r *OrlojToolRuntime) Call(ctx context.Context, tool string, input string) (string, error) {
	tool = strings.TrimSpace(tool)
	if !IsBuiltinOrlojTool(tool) {
		if r.delegate != nil {
			return r.delegate.Call(ctx, tool, input)
		}
		return "", fmt.Errorf("unsupported tool: %s", tool)
	}
	switch tool {
	case ToolOrlojTaskCreate:
		return r.handleTaskCreate(ctx, input)
	case ToolOrlojTaskList:
		return r.handleTaskList(ctx, input)
	default:
		return "", fmt.Errorf("unknown orloj tool: %s", tool)
	}
}

func (r *OrlojToolRuntime) handleTaskCreate(ctx context.Context, input string) (string, error) {
	var req struct {
		Template string            `json:"template"`
		Input    map[string]string `json:"input"`
		Labels   map[string]string `json:"labels"`
	}
	if err := json.Unmarshal([]byte(input), &req); err != nil {
		return "", fmt.Errorf("orloj.task.create: invalid input: %w", err)
	}
	templateName := strings.TrimSpace(req.Template)
	if templateName == "" {
		return "", fmt.Errorf("orloj.task.create: template is required")
	}

	if r.childCount >= r.config.MaxChildren {
		return "", fmt.Errorf("orloj.task.create: max child tasks reached (%d)", r.config.MaxChildren)
	}
	if r.config.CurrentDepth >= r.config.MaxDepth {
		return "", fmt.Errorf("orloj.task.create: max child depth reached (%d)", r.config.MaxDepth)
	}

	templateKey := store.ScopedName(r.config.ParentNamespace, templateName)
	tmpl, ok, err := r.taskStore.Get(ctx, templateKey)
	if err != nil {
		return "", fmt.Errorf("orloj.task.create: error looking up template: %w", err)
	}
	if !ok {
		return "", fmt.Errorf("orloj.task.create: template %q not found", templateName)
	}
	if !strings.EqualFold(tmpl.Spec.Mode, "template") {
		return "", fmt.Errorf("orloj.task.create: task %q is not a template (mode=%s)", templateName, tmpl.Spec.Mode)
	}

	suffix, err := shortRandomSuffix()
	if err != nil {
		return "", fmt.Errorf("orloj.task.create: failed to generate name: %w", err)
	}
	parentShort := truncateName(r.config.ParentTaskName, 20)
	childName := fmt.Sprintf("%s-%s-%s", templateName, parentShort, suffix)

	child := resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata: resources.ObjectMeta{
			Name:      childName,
			Namespace: r.config.ParentNamespace,
			Labels:    make(map[string]string),
		},
		Spec: resources.TaskSpec{
			System: tmpl.Spec.System,
			Mode:   "run",
			Input:  make(map[string]string),
		},
	}

	for k, v := range tmpl.Spec.Input {
		child.Spec.Input[k] = v
	}
	for k, v := range req.Input {
		child.Spec.Input[k] = v
	}

	child.Metadata.Labels["orloj.dev/parent-task"] = r.config.ParentTaskName
	child.Metadata.Labels["orloj.dev/depth"] = fmt.Sprintf("%d", r.config.CurrentDepth+1)
	child.Metadata.Labels["orloj.dev/created-by"] = "orloj.task.create"
	for k, v := range req.Labels {
		child.Metadata.Labels[k] = v
	}

	created, err := r.taskStore.Upsert(ctx, child)
	if err != nil {
		return "", fmt.Errorf("orloj.task.create: failed to create task: %w", err)
	}
	r.childCount++

	resp, _ := json.Marshal(map[string]string{
		"status":   "created",
		"name":     created.Metadata.Name,
		"phase":    created.Status.Phase,
		"template": templateName,
		"system":   created.Spec.System,
	})
	return string(resp), nil
}

func (r *OrlojToolRuntime) handleTaskList(ctx context.Context, input string) (string, error) {
	var req struct {
		Labels map[string]string `json:"labels"`
		Limit  int               `json:"limit"`
	}
	if input != "" {
		_ = json.Unmarshal([]byte(input), &req)
	}
	if req.Limit <= 0 {
		req.Limit = 20
	}

	// Fetch a generous page; we filter by labels in-memory.
	tasks, err := r.taskStore.ListPaged(ctx, 0, 0, r.config.ParentNamespace)
	if err != nil {
		return "", fmt.Errorf("orloj.task.list: %w", err)
	}

	type taskSummary struct {
		Name      string            `json:"name"`
		Phase     string            `json:"phase"`
		System    string            `json:"system"`
		Labels    map[string]string `json:"labels,omitempty"`
		CreatedAt string            `json:"created_at,omitempty"`
	}
	var results []taskSummary
	for _, t := range tasks {
		if !matchesLabels(t.Metadata.Labels, req.Labels) {
			continue
		}
		results = append(results, taskSummary{
			Name:      t.Metadata.Name,
			Phase:     t.Status.Phase,
			System:    t.Spec.System,
			Labels:    t.Metadata.Labels,
			CreatedAt: t.Metadata.CreatedAt,
		})
		if len(results) >= req.Limit {
			break
		}
	}

	resp, _ := json.Marshal(map[string]any{
		"tasks": results,
		"count": len(results),
	})
	return string(resp), nil
}

func matchesLabels(taskLabels, filter map[string]string) bool {
	for k, v := range filter {
		if taskLabels[k] != v {
			return false
		}
	}
	return true
}

func shortRandomSuffix() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func truncateName(name string, maxLen int) string {
	if len(name) <= maxLen {
		return name
	}
	return name[:maxLen]
}

func parseDepthLabel(labels map[string]string) int {
	v, ok := labels["orloj.dev/depth"]
	if !ok {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 0 {
		return 0
	}
	return n
}
