package agentruntime

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/resources"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

type fakeKubernetesJobClient struct {
	createdJob *batchv1.Job
	createErr  error

	watchCh  chan watch.Event
	watchErr error

	getJob    *batchv1.Job
	getJobErr error

	pods    []corev1.Pod
	listErr error

	logs    string
	logsErr error

	deleteErr error
}

func (f *fakeKubernetesJobClient) CreateJob(_ context.Context, namespace string, job *batchv1.Job) (*batchv1.Job, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.createdJob = job.DeepCopy()
	f.createdJob.Namespace = namespace
	return f.createdJob, nil
}

func (f *fakeKubernetesJobClient) WatchJob(_ context.Context, _, _ string) (watch.Interface, error) {
	if f.watchErr != nil {
		return nil, f.watchErr
	}
	return watch.NewProxyWatcher(f.watchCh), nil
}

func (f *fakeKubernetesJobClient) GetJob(_ context.Context, _, _ string) (*batchv1.Job, error) {
	return f.getJob, f.getJobErr
}

func (f *fakeKubernetesJobClient) GetPodLogs(_ context.Context, _, _ string) (string, error) {
	return f.logs, f.logsErr
}

func (f *fakeKubernetesJobClient) ListPods(_ context.Context, _, _ string) ([]corev1.Pod, error) {
	return f.pods, f.listErr
}

func (f *fakeKubernetesJobClient) DeleteJob(_ context.Context, _, _ string) error {
	return f.deleteErr
}

func newCompletedJobEvent(name string) watch.Event {
	return watch.Event{
		Type: watch.Modified,
		Object: &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Status: batchv1.JobStatus{
				Conditions: []batchv1.JobCondition{
					{Type: batchv1.JobComplete, Status: corev1.ConditionTrue},
				},
			},
		},
	}
}

func newFailedJobEvent(name, reason string) watch.Event {
	return watch.Event{
		Type: watch.Modified,
		Object: &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Status: batchv1.JobStatus{
				Conditions: []batchv1.JobCondition{
					{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Reason: reason},
				},
			},
		},
	}
}

func TestKubernetesToolRuntimeCLISuccess(t *testing.T) {
	watchCh := make(chan watch.Event, 1)
	watchCh <- newCompletedJobEvent("test-job")

	client := &fakeKubernetesJobClient{
		watchCh: watchCh,
		pods:    []corev1.Pod{{ObjectMeta: metav1.ObjectMeta{Name: "test-pod"}}},
		logs:    "tool output here",
	}

	registry := NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
		"my-tool": {
			Type: "cli",
			Cli: resources.ToolCliSpec{
				Image:   "alpine:latest",
				Command: "echo",
				Args:    []string{"hello"},
			},
		},
	})

	rt := NewKubernetesToolRuntimeWithClient(client, KubernetesToolConfig{
		Namespace:    "test-ns",
		JobTTLSeconds: 60,
	}, nil)
	rt = rt.WithRegistry(registry).(*KubernetesToolRuntime)

	result, err := rt.Call(context.Background(), "my-tool", `{}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "tool output here" {
		t.Fatalf("expected 'tool output here', got %q", result)
	}
	if client.createdJob == nil {
		t.Fatal("expected job to be created")
	}
	if client.createdJob.Namespace != "test-ns" {
		t.Errorf("expected namespace=test-ns, got %s", client.createdJob.Namespace)
	}
	container := client.createdJob.Spec.Template.Spec.Containers[0]
	if container.Image != "alpine:latest" {
		t.Errorf("expected image=alpine:latest, got %s", container.Image)
	}
}

func TestKubernetesToolRuntimeHTTPSuccess(t *testing.T) {
	watchCh := make(chan watch.Event, 1)
	watchCh <- newCompletedJobEvent("test-job")

	client := &fakeKubernetesJobClient{
		watchCh: watchCh,
		pods:    []corev1.Pod{{ObjectMeta: metav1.ObjectMeta{Name: "test-pod"}}},
		logs:    `{"result": "ok"}`,
	}

	registry := NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
		"web_search": {
			Type:     "http",
			Endpoint: "https://api.example.com/search",
		},
	})

	rt := NewKubernetesToolRuntimeWithClient(client, KubernetesToolConfig{
		Namespace:    "default",
		DefaultImage: "curlimages/curl:8.8.0",
		JobTTLSeconds: 300,
	}, nil)
	rt = rt.WithRegistry(registry).(*KubernetesToolRuntime)

	result, err := rt.Call(context.Background(), "web_search", `{"q":"test"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "ok") {
		t.Fatalf("expected result containing 'ok', got %q", result)
	}
}

func TestKubernetesToolRuntimeJobFailed(t *testing.T) {
	watchCh := make(chan watch.Event, 1)
	watchCh <- newFailedJobEvent("test-job", "BackoffLimitExceeded")

	client := &fakeKubernetesJobClient{
		watchCh: watchCh,
	}

	registry := NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
		"my-tool": {
			Type: "cli",
			Cli: resources.ToolCliSpec{
				Image:   "alpine:latest",
				Command: "false",
			},
		},
	})

	rt := NewKubernetesToolRuntimeWithClient(client, KubernetesToolConfig{}, nil)
	rt = rt.WithRegistry(registry).(*KubernetesToolRuntime)

	_, err := rt.Call(context.Background(), "my-tool", `{}`)
	if err == nil {
		t.Fatal("expected error for failed job")
	}
	var toolErr *ToolError
	if !errors.As(err, &toolErr) {
		t.Fatalf("expected ToolError, got %T: %v", err, err)
	}
	if toolErr.Code != ToolCodeExecutionFailed {
		t.Errorf("expected code=%s, got %s", ToolCodeExecutionFailed, toolErr.Code)
	}
}

func TestKubernetesToolRuntimeContextTimeout(t *testing.T) {
	watchCh := make(chan watch.Event) // never sends

	client := &fakeKubernetesJobClient{
		watchCh: watchCh,
	}

	registry := NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
		"my-tool": {
			Type: "cli",
			Cli: resources.ToolCliSpec{
				Image:   "alpine:latest",
				Command: "sleep",
				Args:    []string{"3600"},
			},
		},
	})

	rt := NewKubernetesToolRuntimeWithClient(client, KubernetesToolConfig{}, nil)
	rt = rt.WithRegistry(registry).(*KubernetesToolRuntime)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := rt.Call(ctx, "my-tool", `{}`)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	var toolErr *ToolError
	if !errors.As(err, &toolErr) {
		t.Fatalf("expected ToolError, got %T: %v", err, err)
	}
	if toolErr.Code != ToolCodeTimeout {
		t.Errorf("expected code=%s, got %s", ToolCodeTimeout, toolErr.Code)
	}
}

func TestKubernetesToolRuntimeMissingTool(t *testing.T) {
	registry := NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{})
	client := &fakeKubernetesJobClient{}
	rt := NewKubernetesToolRuntimeWithClient(client, KubernetesToolConfig{}, nil)
	rt = rt.WithRegistry(registry).(*KubernetesToolRuntime)

	_, err := rt.Call(context.Background(), "nonexistent", `{}`)
	if err == nil {
		t.Fatal("expected error for missing tool")
	}
	var toolErr *ToolError
	if !errors.As(err, &toolErr) {
		t.Fatalf("expected ToolError, got %T: %v", err, err)
	}
	if toolErr.Code != ToolCodeUnsupportedTool {
		t.Errorf("expected code=%s, got %s", ToolCodeUnsupportedTool, toolErr.Code)
	}
}

func TestKubernetesToolRuntimeNoClient(t *testing.T) {
	registry := NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
		"my-tool": {
			Type: "cli",
			Cli: resources.ToolCliSpec{
				Image:   "alpine:latest",
				Command: "echo",
			},
		},
	})

	rt := NewKubernetesToolRuntimeWithClient(nil, KubernetesToolConfig{}, nil)
	rt = rt.WithRegistry(registry).(*KubernetesToolRuntime)

	_, err := rt.Call(context.Background(), "my-tool", `{}`)
	if err == nil {
		t.Fatal("expected error for nil client")
	}
	var toolErr *ToolError
	if !errors.As(err, &toolErr) {
		t.Fatalf("expected ToolError, got %T: %v", err, err)
	}
	if toolErr.Code != ToolCodeIsolationUnavailable {
		t.Errorf("expected code=%s, got %s", ToolCodeIsolationUnavailable, toolErr.Code)
	}
}

func TestKubernetesToolRuntimeMissingImage(t *testing.T) {
	registry := NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
		"my-tool": {
			Type: "cli",
			Cli: resources.ToolCliSpec{
				Command: "echo",
			},
		},
	})

	client := &fakeKubernetesJobClient{}
	rt := NewKubernetesToolRuntimeWithClient(client, KubernetesToolConfig{}, nil)
	rt = rt.WithRegistry(registry).(*KubernetesToolRuntime)

	_, err := rt.Call(context.Background(), "my-tool", `{}`)
	if err == nil {
		t.Fatal("expected error for missing image")
	}
}

func TestKubernetesToolRuntimeResourceLimits(t *testing.T) {
	watchCh := make(chan watch.Event, 1)
	watchCh <- newCompletedJobEvent("test-job")

	client := &fakeKubernetesJobClient{
		watchCh: watchCh,
		pods:    []corev1.Pod{{ObjectMeta: metav1.ObjectMeta{Name: "test-pod"}}},
		logs:    "ok",
	}

	registry := NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
		"my-tool": {
			Type: "cli",
			Cli: resources.ToolCliSpec{
				Image:   "alpine:latest",
				Command: "echo",
				Resources: resources.ContainerResources{
					Memory: "256Mi",
					CPUs:   "500m",
				},
			},
		},
	})

	rt := NewKubernetesToolRuntimeWithClient(client, KubernetesToolConfig{
		Namespace:      "test-ns",
		ServiceAccount: "tool-sa",
		JobTTLSeconds:  120,
	}, nil)
	rt = rt.WithRegistry(registry).(*KubernetesToolRuntime)

	_, err := rt.Call(context.Background(), "my-tool", `{}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	job := client.createdJob
	if job == nil {
		t.Fatal("expected job to be created")
	}

	container := job.Spec.Template.Spec.Containers[0]
	if container.Resources.Limits.Memory().String() != "256Mi" {
		t.Errorf("expected memory limit=256Mi, got %s", container.Resources.Limits.Memory().String())
	}
	if container.Resources.Limits.Cpu().String() != "500m" {
		t.Errorf("expected cpu limit=500m, got %s", container.Resources.Limits.Cpu().String())
	}

	if job.Spec.Template.Spec.ServiceAccountName != "tool-sa" {
		t.Errorf("expected service account=tool-sa, got %s", job.Spec.Template.Spec.ServiceAccountName)
	}
	if *job.Spec.TTLSecondsAfterFinished != 120 {
		t.Errorf("expected TTL=120, got %d", *job.Spec.TTLSecondsAfterFinished)
	}
}

func TestKubernetesToolRuntimeWithNamespace(t *testing.T) {
	config := KubernetesToolConfig{Namespace: "original-ns"}
	rt := NewKubernetesToolRuntimeWithClient(&fakeKubernetesJobClient{}, config, nil)
	
	scoped := rt.WithNamespace("custom-ns").(*KubernetesToolRuntime)
	if scoped.namespace != "custom-ns" {
		t.Errorf("expected namespace=custom-ns, got %s", scoped.namespace)
	}
	if rt.namespace != "" {
		t.Error("original runtime namespace should not be mutated")
	}
}

func TestKubernetesToolRuntimeCreateJobError(t *testing.T) {
	client := &fakeKubernetesJobClient{
		createErr: errors.New("forbidden"),
	}

	registry := NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
		"my-tool": {
			Type: "cli",
			Cli: resources.ToolCliSpec{
				Image:   "alpine:latest",
				Command: "echo",
			},
		},
	})

	rt := NewKubernetesToolRuntimeWithClient(client, KubernetesToolConfig{}, nil)
	rt = rt.WithRegistry(registry).(*KubernetesToolRuntime)

	_, err := rt.Call(context.Background(), "my-tool", `{}`)
	if err == nil {
		t.Fatal("expected error")
	}
	var toolErr *ToolError
	if !errors.As(err, &toolErr) {
		t.Fatalf("expected ToolError, got %T", err)
	}
	if !toolErr.Retryable {
		t.Error("expected retryable error for job creation failure")
	}
}

func TestSanitizeK8sName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"UPPER-CASE", "upper-case"},
		{"with spaces", "with-spaces"},
		{"with_underscores", "with-underscores"},
		{"trailing---", "trailing"},
	}
	for _, tc := range tests {
		got := sanitizeK8sName(tc.input)
		if got != tc.expected {
			t.Errorf("sanitizeK8sName(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestSanitizeK8sLabelValue(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"UPPER", "upper"},
		{"with spaces", "with-spaces"},
		{"---trimmed---", "trimmed"},
	}
	for _, tc := range tests {
		got := sanitizeK8sLabelValue(tc.input)
		if got != tc.expected {
			t.Errorf("sanitizeK8sLabelValue(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}
