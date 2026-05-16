package crds

import (
	"context"
	"fmt"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/OrlojHQ/orloj/resources"
)

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = AddToScheme(s)
	return s
}

// --- Conversion Tests ---

func TestAgentToOrloj(t *testing.T) {
	crd := &Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-agent",
			Namespace: "prod",
			Labels:    map[string]string{"team": "ml"},
		},
		Spec: resources.AgentSpec{
			ModelRef: "gpt-4",
			Prompt:   "You are a helpful assistant",
		},
	}
	orloj := AgentToOrloj(crd)
	if orloj.Metadata.Name != "my-agent" {
		t.Errorf("expected name 'my-agent', got %q", orloj.Metadata.Name)
	}
	if orloj.Metadata.Namespace != "prod" {
		t.Errorf("expected namespace 'prod', got %q", orloj.Metadata.Namespace)
	}
	if orloj.Metadata.Annotations[AnnotationManagedBy] != ManagedByCRDSync {
		t.Errorf("expected managed-by annotation, got %v", orloj.Metadata.Annotations)
	}
	if orloj.APIVersion != "orloj.dev/v1" {
		t.Errorf("expected apiVersion 'orloj.dev/v1', got %q", orloj.APIVersion)
	}
	if orloj.Kind != "Agent" {
		t.Errorf("expected kind 'Agent', got %q", orloj.Kind)
	}
	if orloj.Spec.ModelRef != "gpt-4" {
		t.Errorf("expected model_ref 'gpt-4', got %q", orloj.Spec.ModelRef)
	}
	if orloj.Metadata.Labels["team"] != "ml" {
		t.Errorf("expected label team=ml, got %v", orloj.Metadata.Labels)
	}
}

func TestIsCRDManaged(t *testing.T) {
	tests := []struct {
		name     string
		meta     resources.ObjectMeta
		expected bool
	}{
		{"nil annotations", resources.ObjectMeta{}, false},
		{"empty annotations", resources.ObjectMeta{Annotations: map[string]string{}}, false},
		{"wrong value", resources.ObjectMeta{Annotations: map[string]string{AnnotationManagedBy: "other"}}, false},
		{"correct", resources.ObjectMeta{Annotations: map[string]string{AnnotationManagedBy: ManagedByCRDSync}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsCRDManaged(tt.meta); got != tt.expected {
				t.Errorf("IsCRDManaged() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestScopedName(t *testing.T) {
	tests := []struct {
		name     string
		meta     metav1.ObjectMeta
		expected string
	}{
		{"k8s namespace used", metav1.ObjectMeta{Name: "foo", Namespace: "bar"}, "bar/foo"},
		{"empty namespace defaults", metav1.ObjectMeta{Name: "foo", Namespace: ""}, "default/foo"},
		{"target-namespace annotation overrides", metav1.ObjectMeta{
			Name:        "foo",
			Namespace:   "k8s-ns",
			Annotations: map[string]string{AnnotationTargetNamespace: "production"},
		}, "production/foo"},
		{"empty target-namespace falls back to k8s namespace", metav1.ObjectMeta{
			Name:        "foo",
			Namespace:   "team-a",
			Annotations: map[string]string{AnnotationTargetNamespace: ""},
		}, "team-a/foo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ScopedName(tt.meta); got != tt.expected {
				t.Errorf("ScopedName() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// --- Reconciler Tests ---

type mockSyncer struct {
	upserted []resources.Agent
	deleted  []string
	upsertFn func(ctx context.Context, item resources.Agent) (resources.Agent, error)
}

func (m *mockSyncer) upsert(ctx context.Context, item resources.Agent) (resources.Agent, error) {
	if m.upsertFn != nil {
		return m.upsertFn(ctx, item)
	}
	m.upserted = append(m.upserted, item)
	return item, nil
}

func (m *mockSyncer) delete(ctx context.Context, name string) error {
	m.deleted = append(m.deleted, name)
	return nil
}

func TestSyncReconciler_CreateOrUpdate(t *testing.T) {
	s := testScheme()
	mock := &mockSyncer{}

	crd := &Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-agent",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: resources.AgentSpec{
			ModelRef: "gpt-4",
			Prompt:   "test",
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).
		WithObjects(crd).
		WithStatusSubresource(crd).
		Build()

	rec := &SyncReconciler[*Agent, resources.Agent]{
		Client: fakeClient,
		Scheme: s,
		NewCRD: func() *Agent { return &Agent{} },
		Syncer: ResourceSyncer[*Agent, resources.Agent]{
			Kind:       "Agent",
			ToOrloj:    AgentToOrloj,
			Upsert:     mock.upsert,
			Delete:     mock.delete,
			ScopedName: func(a *Agent) string { return ScopedName(a.ObjectMeta) },
		},
	}

	result, err := rec.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-agent", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Errorf("expected no requeue, got %v", result.RequeueAfter)
	}
	if len(mock.upserted) != 1 {
		t.Fatalf("expected 1 upsert, got %d", len(mock.upserted))
	}
	if mock.upserted[0].Metadata.Name != "test-agent" {
		t.Errorf("expected name test-agent, got %q", mock.upserted[0].Metadata.Name)
	}
	if mock.upserted[0].Metadata.Annotations[AnnotationManagedBy] != ManagedByCRDSync {
		t.Error("expected managed-by annotation on upserted resource")
	}

	// Verify the finalizer was added
	updated := &Agent{}
	_ = fakeClient.Get(context.Background(), types.NamespacedName{Name: "test-agent", Namespace: "default"}, updated)
	hasFinalizer := false
	for _, f := range updated.Finalizers {
		if f == FinalizerSync {
			hasFinalizer = true
		}
	}
	if !hasFinalizer {
		t.Error("expected finalizer to be added")
	}
}

func TestSyncReconciler_UpsertError(t *testing.T) {
	s := testScheme()
	mock := &mockSyncer{
		upsertFn: func(ctx context.Context, item resources.Agent) (resources.Agent, error) {
			return resources.Agent{}, fmt.Errorf("validation failed: spec.model_ref is required")
		},
	}

	crd := &Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "bad-agent",
			Namespace:  "default",
			Generation: 1,
			Finalizers: []string{FinalizerSync},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).
		WithObjects(crd).
		WithStatusSubresource(crd).
		Build()

	rec := &SyncReconciler[*Agent, resources.Agent]{
		Client: fakeClient,
		Scheme: s,
		NewCRD: func() *Agent { return &Agent{} },
		Syncer: ResourceSyncer[*Agent, resources.Agent]{
			Kind:       "Agent",
			ToOrloj:    AgentToOrloj,
			Upsert:     mock.upsertFn,
			Delete:     mock.delete,
			ScopedName: func(a *Agent) string { return ScopedName(a.ObjectMeta) },
		},
	}

	result, err := rec.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "bad-agent", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != 10*time.Second {
		t.Errorf("expected requeue after 10s, got %v", result.RequeueAfter)
	}

	// Status should show sync error
	updated := &Agent{}
	_ = fakeClient.Get(context.Background(), types.NamespacedName{Name: "bad-agent", Namespace: "default"}, updated)
	if updated.Status.Phase != "SyncError" {
		t.Errorf("expected phase SyncError, got %q", updated.Status.Phase)
	}
	if updated.Status.SyncError == "" {
		t.Error("expected syncError to be set")
	}
}

func TestSyncReconciler_Delete(t *testing.T) {
	s := testScheme()
	mock := &mockSyncer{}

	now := metav1.Now()
	crd := &Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "delete-me",
			Namespace:         "default",
			DeletionTimestamp: &now,
			Finalizers:        []string{FinalizerSync},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).
		WithObjects(crd).
		Build()

	rec := &SyncReconciler[*Agent, resources.Agent]{
		Client: fakeClient,
		Scheme: s,
		NewCRD: func() *Agent { return &Agent{} },
		Syncer: ResourceSyncer[*Agent, resources.Agent]{
			Kind:       "Agent",
			ToOrloj:    AgentToOrloj,
			Upsert:     mock.upsert,
			Delete:     mock.delete,
			ScopedName: func(a *Agent) string { return ScopedName(a.ObjectMeta) },
		},
	}

	result, err := rec.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "delete-me", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Errorf("expected no requeue, got %v", result.RequeueAfter)
	}
	if len(mock.deleted) != 1 {
		t.Fatalf("expected 1 delete, got %d", len(mock.deleted))
	}
	if mock.deleted[0] != "default/delete-me" {
		t.Errorf("expected delete of 'default/delete-me', got %q", mock.deleted[0])
	}
}

func TestSyncReconciler_NotFound(t *testing.T) {
	s := testScheme()

	fakeClient := fake.NewClientBuilder().WithScheme(s).Build()

	rec := &SyncReconciler[*Agent, resources.Agent]{
		Client: fakeClient,
		Scheme: s,
		NewCRD: func() *Agent { return &Agent{} },
		Syncer: ResourceSyncer[*Agent, resources.Agent]{
			Kind: "Agent",
		},
	}

	result, err := rec.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Errorf("expected no requeue for not-found, got %v", result.RequeueAfter)
	}
}

// --- DeepCopy Tests ---

func TestDeepCopy(t *testing.T) {
	original := &Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "prod",
			Labels:    map[string]string{"env": "prod"},
		},
		Spec: resources.AgentSpec{
			ModelRef: "gpt-4",
			Prompt:   "hello",
			Tools:    []string{"tool1", "tool2"},
		},
		Status: CRDStatus{Phase: "Synced"},
	}

	copied := original.DeepCopy()
	if copied.Name != "test" || copied.Namespace != "prod" {
		t.Error("name/namespace not copied")
	}
	if copied.Spec.ModelRef != "gpt-4" {
		t.Error("spec not copied")
	}

	// Mutating the copy should not affect the original
	copied.Labels["env"] = "staging"
	if original.Labels["env"] != "prod" {
		t.Error("deep copy shares label map")
	}
}

func TestMergeAnnotations(t *testing.T) {
	input := map[string]string{"existing": "value"}
	result := mergeAnnotations(input)
	if result["existing"] != "value" {
		t.Error("existing annotation lost")
	}
	if result[AnnotationManagedBy] != ManagedByCRDSync {
		t.Error("managed-by annotation not added")
	}
	// Mutating result should not affect input
	result["new"] = "key"
	if _, ok := input["new"]; ok {
		t.Error("mergeAnnotations mutated input map")
	}
}

func TestAllConversions(t *testing.T) {
	tests := []struct {
		name string
		fn   func() (string, string, string)
	}{
		{"AgentSystem", func() (string, string, string) {
			crd := &AgentSystem{ObjectMeta: metav1.ObjectMeta{Name: "sys1", Namespace: "ns1"}}
			o := AgentSystemToOrloj(crd)
			return o.Metadata.Name, o.Kind, o.Metadata.Annotations[AnnotationManagedBy]
		}},
		{"Tool", func() (string, string, string) {
			crd := &Tool{ObjectMeta: metav1.ObjectMeta{Name: "tool1", Namespace: "ns1"}}
			o := ToolToOrloj(crd)
			return o.Metadata.Name, o.Kind, o.Metadata.Annotations[AnnotationManagedBy]
		}},
		{"McpServer", func() (string, string, string) {
			crd := &McpServer{ObjectMeta: metav1.ObjectMeta{Name: "mcp1", Namespace: "ns1"}}
			o := McpServerToOrloj(crd)
			return o.Metadata.Name, o.Kind, o.Metadata.Annotations[AnnotationManagedBy]
		}},
		{"ModelEndpoint", func() (string, string, string) {
			crd := &ModelEndpoint{ObjectMeta: metav1.ObjectMeta{Name: "ep1", Namespace: "ns1"}}
			o := ModelEndpointToOrloj(crd)
			return o.Metadata.Name, o.Kind, o.Metadata.Annotations[AnnotationManagedBy]
		}},
		{"Memory", func() (string, string, string) {
			crd := &Memory{ObjectMeta: metav1.ObjectMeta{Name: "mem1", Namespace: "ns1"}}
			o := MemoryToOrloj(crd)
			return o.Metadata.Name, o.Kind, o.Metadata.Annotations[AnnotationManagedBy]
		}},
		{"AgentPolicy", func() (string, string, string) {
			crd := &AgentPolicy{ObjectMeta: metav1.ObjectMeta{Name: "pol1", Namespace: "ns1"}}
			o := AgentPolicyToOrloj(crd)
			return o.Metadata.Name, o.Kind, o.Metadata.Annotations[AnnotationManagedBy]
		}},
		{"Secret", func() (string, string, string) {
			crd := &Secret{ObjectMeta: metav1.ObjectMeta{Name: "sec1", Namespace: "ns1"}}
			o := SecretToOrloj(crd)
			return o.Metadata.Name, o.Kind, o.Metadata.Annotations[AnnotationManagedBy]
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, kind, managedBy := tt.fn()
			if name == "" {
				t.Error("name not set")
			}
			if kind == "" {
				t.Error("kind not set")
			}
			if managedBy != ManagedByCRDSync {
				t.Errorf("managed-by annotation not set, got %q", managedBy)
			}
		})
	}
}
