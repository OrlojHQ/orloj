package agentruntime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

// mockOrlojTaskStore is a minimal in-memory task store for testing.
type mockOrlojTaskStore struct {
	tasks map[string]resources.Task
}

func newMockOrlojTaskStore() *mockOrlojTaskStore {
	return &mockOrlojTaskStore{tasks: make(map[string]resources.Task)}
}

func (s *mockOrlojTaskStore) Get(_ context.Context, name string) (resources.Task, bool, error) {
	t, ok := s.tasks[name]
	return t, ok, nil
}

func (s *mockOrlojTaskStore) Upsert(_ context.Context, item resources.Task) (resources.Task, error) {
	if err := item.Normalize(); err != nil {
		return resources.Task{}, err
	}
	key := resources.NormalizeNamespace(item.Metadata.Namespace) + "/" + item.Metadata.Name
	s.tasks[key] = item
	return item, nil
}

func (s *mockOrlojTaskStore) ListPaged(_ context.Context, limit, _ int, namespace string) ([]resources.Task, error) {
	var out []resources.Task
	for _, t := range s.tasks {
		if namespace != "" && !strings.EqualFold(resources.NormalizeNamespace(t.Metadata.Namespace), namespace) {
			continue
		}
		out = append(out, t)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *mockOrlojTaskStore) put(ns, name string, task resources.Task) {
	key := resources.NormalizeNamespace(ns) + "/" + name
	s.tasks[key] = task
}

func defaultOrlojConfig() OrlojToolConfig {
	return OrlojToolConfig{
		ParentNamespace: "default",
		ParentTaskName:  "parent-task",
		CurrentDepth:    0,
		MaxDepth:        5,
		MaxChildren:     20,
	}
}

func TestIsBuiltinOrlojTool(t *testing.T) {
	if !IsBuiltinOrlojTool("orloj.task.create") {
		t.Fatal("expected orloj.task.create to be a builtin tool")
	}
	if !IsBuiltinOrlojTool("orloj.task.list") {
		t.Fatal("expected orloj.task.list to be a builtin tool")
	}
	if IsBuiltinOrlojTool("orloj.task.get") {
		t.Fatal("expected orloj.task.get to not be a builtin tool")
	}
	if IsBuiltinOrlojTool("web-search") {
		t.Fatal("expected web-search to not be a builtin tool")
	}
}

func TestBuiltinOrlojToolNames(t *testing.T) {
	names := BuiltinOrlojToolNames()
	if len(names) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(names))
	}
	if names[0] != "orloj.task.create" || names[1] != "orloj.task.list" {
		t.Fatalf("unexpected tool names: %v", names)
	}
}

func TestAgentHasOrlojTools(t *testing.T) {
	agent := resources.Agent{
		Spec: resources.AgentSpec{AllowedTools: []string{"web-search", "orloj.task.create"}},
	}
	if !AgentHasOrlojTools(agent) {
		t.Fatal("expected agent to have orloj tools")
	}

	agent.Spec.AllowedTools = []string{"web-search"}
	if AgentHasOrlojTools(agent) {
		t.Fatal("expected agent without orloj tools")
	}
}

func TestOrlojToolRuntime_DelegatesToNonOrlojTools(t *testing.T) {
	delegate := &MockToolClient{}
	rt := NewOrlojToolRuntime(delegate, newMockOrlojTaskStore(), defaultOrlojConfig())

	result, err := rt.Call(context.Background(), "web-search", `{"query":"test"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "tool=web-search") {
		t.Fatalf("expected delegated call, got %s", result)
	}
}

func TestOrlojTaskCreate_Success(t *testing.T) {
	store := newMockOrlojTaskStore()
	store.put("default", "write-article", resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "write-article", Namespace: "default"},
		Spec: resources.TaskSpec{
			System: "writing-dept",
			Mode:   "template",
			Input:  map[string]string{"topic": "default-topic"},
		},
		Status: resources.TaskStatus{Phase: "Pending"},
	})

	rt := NewOrlojToolRuntime(nil, store, defaultOrlojConfig())
	result, err := rt.Call(context.Background(), "orloj.task.create", `{"template":"write-article","input":{"topic":"AI safety"}}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp map[string]string
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if resp["status"] != "created" {
		t.Fatalf("expected status=created, got %s", resp["status"])
	}
	if resp["phase"] != "Pending" {
		t.Fatalf("expected phase=Pending, got %s", resp["phase"])
	}
	if resp["system"] != "writing-dept" {
		t.Fatalf("expected system=writing-dept, got %s", resp["system"])
	}
	if resp["template"] != "write-article" {
		t.Fatalf("expected template=write-article, got %s", resp["template"])
	}
	if !strings.HasPrefix(resp["name"], "write-article-parent-task-") {
		t.Fatalf("expected name prefix write-article-parent-task-, got %s", resp["name"])
	}

	// Verify child task was created in store with correct properties
	childName := resp["name"]
	child, ok, _ := store.Get(context.Background(), "default/"+childName)
	if !ok {
		t.Fatal("child task not found in store")
	}
	if child.Spec.Mode != "run" {
		t.Fatalf("expected child mode=run, got %s", child.Spec.Mode)
	}
	if child.Spec.Input["topic"] != "AI safety" {
		t.Fatalf("expected topic override, got %s", child.Spec.Input["topic"])
	}
	if child.Metadata.Labels["orloj.dev/parent-task"] != "parent-task" {
		t.Fatalf("expected parent-task label, got %s", child.Metadata.Labels["orloj.dev/parent-task"])
	}
	if child.Metadata.Labels["orloj.dev/depth"] != "1" {
		t.Fatalf("expected depth=1, got %s", child.Metadata.Labels["orloj.dev/depth"])
	}
	if child.Metadata.Labels["orloj.dev/created-by"] != "orloj.task.create" {
		t.Fatalf("expected created-by label, got %s", child.Metadata.Labels["orloj.dev/created-by"])
	}
}

func TestOrlojTaskCreate_TemplateDefaultsMerged(t *testing.T) {
	store := newMockOrlojTaskStore()
	store.put("default", "research-task", resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "research-task", Namespace: "default"},
		Spec: resources.TaskSpec{
			System: "research-dept",
			Mode:   "template",
			Input:  map[string]string{"topic": "default", "depth": "shallow"},
		},
		Status: resources.TaskStatus{Phase: "Pending"},
	})

	rt := NewOrlojToolRuntime(nil, store, defaultOrlojConfig())
	result, err := rt.Call(context.Background(), "orloj.task.create", `{"template":"research-task","input":{"topic":"AI safety"}}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp map[string]string
	json.Unmarshal([]byte(result), &resp)

	child, ok, _ := store.Get(context.Background(), "default/"+resp["name"])
	if !ok {
		t.Fatal("child not found")
	}
	if child.Spec.Input["topic"] != "AI safety" {
		t.Fatalf("expected override topic=AI safety, got %s", child.Spec.Input["topic"])
	}
	if child.Spec.Input["depth"] != "shallow" {
		t.Fatalf("expected default depth=shallow, got %s", child.Spec.Input["depth"])
	}
}

func TestOrlojTaskCreate_TemplateNotFound(t *testing.T) {
	store := newMockOrlojTaskStore()
	rt := NewOrlojToolRuntime(nil, store, defaultOrlojConfig())

	_, err := rt.Call(context.Background(), "orloj.task.create", `{"template":"nonexistent"}`)
	if err == nil {
		t.Fatal("expected error for missing template")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found error, got: %v", err)
	}
}

func TestOrlojTaskCreate_RejectsNonTemplate(t *testing.T) {
	store := newMockOrlojTaskStore()
	store.put("default", "running-task", resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "running-task", Namespace: "default"},
		Spec: resources.TaskSpec{
			System: "some-system",
			Mode:   "run",
		},
		Status: resources.TaskStatus{Phase: "Running"},
	})

	rt := NewOrlojToolRuntime(nil, store, defaultOrlojConfig())
	_, err := rt.Call(context.Background(), "orloj.task.create", `{"template":"running-task"}`)
	if err == nil {
		t.Fatal("expected error for non-template task")
	}
	if !strings.Contains(err.Error(), "not a template") {
		t.Fatalf("expected not-a-template error, got: %v", err)
	}
}

func TestOrlojTaskCreate_RequiresTemplate(t *testing.T) {
	rt := NewOrlojToolRuntime(nil, newMockOrlojTaskStore(), defaultOrlojConfig())
	_, err := rt.Call(context.Background(), "orloj.task.create", `{}`)
	if err == nil {
		t.Fatal("expected error for missing template")
	}
	if !strings.Contains(err.Error(), "template is required") {
		t.Fatalf("expected template-required error, got: %v", err)
	}
}

func TestOrlojTaskCreate_MaxChildrenLimit(t *testing.T) {
	store := newMockOrlojTaskStore()
	store.put("default", "tmpl", resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "tmpl", Namespace: "default"},
		Spec:       resources.TaskSpec{System: "sys", Mode: "template"},
		Status:     resources.TaskStatus{Phase: "Pending"},
	})

	cfg := defaultOrlojConfig()
	cfg.MaxChildren = 2
	rt := NewOrlojToolRuntime(nil, store, cfg)

	for i := 0; i < 2; i++ {
		_, err := rt.Call(context.Background(), "orloj.task.create", `{"template":"tmpl"}`)
		if err != nil {
			t.Fatalf("child %d creation should succeed: %v", i, err)
		}
	}

	_, err := rt.Call(context.Background(), "orloj.task.create", `{"template":"tmpl"}`)
	if err == nil {
		t.Fatal("expected error when max children reached")
	}
	if !strings.Contains(err.Error(), "max child tasks") {
		t.Fatalf("expected max-children error, got: %v", err)
	}
}

func TestOrlojTaskCreate_MaxDepthLimit(t *testing.T) {
	store := newMockOrlojTaskStore()
	store.put("default", "tmpl", resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "tmpl", Namespace: "default"},
		Spec:       resources.TaskSpec{System: "sys", Mode: "template"},
		Status:     resources.TaskStatus{Phase: "Pending"},
	})

	cfg := defaultOrlojConfig()
	cfg.CurrentDepth = 5
	cfg.MaxDepth = 5
	rt := NewOrlojToolRuntime(nil, store, cfg)

	_, err := rt.Call(context.Background(), "orloj.task.create", `{"template":"tmpl"}`)
	if err == nil {
		t.Fatal("expected error when max depth reached")
	}
	if !strings.Contains(err.Error(), "max child depth") {
		t.Fatalf("expected max-depth error, got: %v", err)
	}
}

func TestOrlojTaskCreate_CustomLabels(t *testing.T) {
	store := newMockOrlojTaskStore()
	store.put("default", "tmpl", resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "tmpl", Namespace: "default"},
		Spec:       resources.TaskSpec{System: "sys", Mode: "template"},
		Status:     resources.TaskStatus{Phase: "Pending"},
	})

	rt := NewOrlojToolRuntime(nil, store, defaultOrlojConfig())
	result, err := rt.Call(context.Background(), "orloj.task.create", `{"template":"tmpl","labels":{"department":"engineering"}}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp map[string]string
	json.Unmarshal([]byte(result), &resp)

	child, ok, _ := store.Get(context.Background(), "default/"+resp["name"])
	if !ok {
		t.Fatal("child not found")
	}
	if child.Metadata.Labels["department"] != "engineering" {
		t.Fatalf("expected department label, got %v", child.Metadata.Labels)
	}
}

func TestOrlojTaskList_Empty(t *testing.T) {
	store := newMockOrlojTaskStore()
	rt := NewOrlojToolRuntime(nil, store, defaultOrlojConfig())

	result, err := rt.Call(context.Background(), "orloj.task.list", `{}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp struct {
		Tasks []any `json:"tasks"`
		Count int   `json:"count"`
	}
	json.Unmarshal([]byte(result), &resp)
	if resp.Count != 0 {
		t.Fatalf("expected 0 tasks, got %d", resp.Count)
	}
}

func TestOrlojTaskList_FilterByLabels(t *testing.T) {
	store := newMockOrlojTaskStore()
	store.put("default", "task-a", resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata: resources.ObjectMeta{
			Name: "task-a", Namespace: "default",
			Labels: map[string]string{"orloj.dev/parent-task": "parent-task"},
		},
		Spec:   resources.TaskSpec{System: "sys", Mode: "run"},
		Status: resources.TaskStatus{Phase: "Succeeded"},
	})
	store.put("default", "task-b", resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata: resources.ObjectMeta{
			Name: "task-b", Namespace: "default",
			Labels: map[string]string{"orloj.dev/parent-task": "other-parent"},
		},
		Spec:   resources.TaskSpec{System: "sys", Mode: "run"},
		Status: resources.TaskStatus{Phase: "Running"},
	})

	rt := NewOrlojToolRuntime(nil, store, defaultOrlojConfig())
	result, err := rt.Call(context.Background(), "orloj.task.list", `{"labels":{"orloj.dev/parent-task":"parent-task"}}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp struct {
		Tasks []map[string]any `json:"tasks"`
		Count int              `json:"count"`
	}
	json.Unmarshal([]byte(result), &resp)
	if resp.Count != 1 {
		t.Fatalf("expected 1 filtered task, got %d", resp.Count)
	}
	if resp.Tasks[0]["name"] != "task-a" {
		t.Fatalf("expected task-a, got %v", resp.Tasks[0]["name"])
	}
}

func TestOrlojTaskList_RespectsLimit(t *testing.T) {
	store := newMockOrlojTaskStore()
	for i := 0; i < 5; i++ {
		name := "task-" + string(rune('a'+i))
		store.put("default", name, resources.Task{
			APIVersion: "orloj.dev/v1",
			Kind:       "Task",
			Metadata:   resources.ObjectMeta{Name: name, Namespace: "default"},
			Spec:       resources.TaskSpec{System: "sys", Mode: "run"},
			Status:     resources.TaskStatus{Phase: "Succeeded"},
		})
	}

	rt := NewOrlojToolRuntime(nil, store, defaultOrlojConfig())
	result, err := rt.Call(context.Background(), "orloj.task.list", `{"limit":2}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp struct {
		Count int `json:"count"`
	}
	json.Unmarshal([]byte(result), &resp)
	if resp.Count != 2 {
		t.Fatalf("expected 2 tasks (limit), got %d", resp.Count)
	}
}

func TestOrlojTaskList_NamespaceScoping(t *testing.T) {
	store := newMockOrlojTaskStore()
	store.put("default", "task-default", resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "task-default", Namespace: "default"},
		Spec:       resources.TaskSpec{System: "sys", Mode: "run"},
		Status:     resources.TaskStatus{Phase: "Succeeded"},
	})
	store.put("other-ns", "task-other", resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "task-other", Namespace: "other-ns"},
		Spec:       resources.TaskSpec{System: "sys", Mode: "run"},
		Status:     resources.TaskStatus{Phase: "Succeeded"},
	})

	rt := NewOrlojToolRuntime(nil, store, defaultOrlojConfig())
	result, err := rt.Call(context.Background(), "orloj.task.list", `{}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp struct {
		Count int `json:"count"`
	}
	json.Unmarshal([]byte(result), &resp)
	if resp.Count != 1 {
		t.Fatalf("expected 1 task (namespace-scoped), got %d", resp.Count)
	}
}

func TestOrlojToolSchemas(t *testing.T) {
	schema, ok := builtinToolSchemaForName(ToolOrlojTaskCreate)
	if !ok {
		t.Fatal("expected builtin schema for orloj.task.create")
	}
	if schema.Description == "" {
		t.Fatal("expected non-empty description")
	}
	required, ok := schema.Parameters["required"].([]string)
	if !ok {
		t.Fatalf("expected required []string, got %#v", schema.Parameters["required"])
	}
	if len(required) != 1 || required[0] != "template" {
		t.Fatalf("unexpected required fields: %v", required)
	}

	schema, ok = builtinToolSchemaForName(ToolOrlojTaskList)
	if !ok {
		t.Fatal("expected builtin schema for orloj.task.list")
	}
	if schema.Description == "" {
		t.Fatal("expected non-empty description for task.list")
	}
}

func TestMinimumChildDepth(t *testing.T) {
	policies := []resources.AgentPolicy{
		{Spec: resources.AgentPolicySpec{MaxChildDepth: 10}},
		{Spec: resources.AgentPolicySpec{MaxChildDepth: 3}},
		{Spec: resources.AgentPolicySpec{MaxChildDepth: 0}},
	}
	if got := MinimumChildDepth(policies); got != 3 {
		t.Fatalf("expected 3, got %d", got)
	}
	if got := MinimumChildDepth(nil); got != 0 {
		t.Fatalf("expected 0 for nil policies, got %d", got)
	}
}

func TestMinimumChildTasks(t *testing.T) {
	policies := []resources.AgentPolicy{
		{Spec: resources.AgentPolicySpec{MaxChildTasks: 50}},
		{Spec: resources.AgentPolicySpec{MaxChildTasks: 10}},
	}
	if got := MinimumChildTasks(policies); got != 10 {
		t.Fatalf("expected 10, got %d", got)
	}
}

func TestParseDepthLabel(t *testing.T) {
	if got := parseDepthLabel(nil); got != 0 {
		t.Fatalf("expected 0 for nil labels, got %d", got)
	}
	if got := parseDepthLabel(map[string]string{}); got != 0 {
		t.Fatalf("expected 0 for empty labels, got %d", got)
	}
	if got := parseDepthLabel(map[string]string{"orloj.dev/depth": "3"}); got != 3 {
		t.Fatalf("expected 3, got %d", got)
	}
	if got := parseDepthLabel(map[string]string{"orloj.dev/depth": "invalid"}); got != 0 {
		t.Fatalf("expected 0 for invalid, got %d", got)
	}
	if got := parseDepthLabel(map[string]string{"orloj.dev/depth": "-1"}); got != 0 {
		t.Fatalf("expected 0 for negative, got %d", got)
	}
}
