package crds

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/startup"
	"github.com/OrlojHQ/orloj/store"
)

func testStoresWithAgents(agents ...resources.Agent) *startup.StoreSet {
	s := &startup.StoreSet{
		Agents:       store.NewAgentStore(),
		AgentSystems: store.NewAgentSystemStore(),
		ModelEPs:     store.NewModelEndpointStore(),
		Tools:        store.NewToolStore(),
		Secrets:      store.NewSecretStore(),
		Memories:     store.NewMemoryStore(),
		Policies:     store.NewAgentPolicyStore(),
		McpServers:   store.NewMcpServerStore(),
	}
	for _, a := range agents {
		s.Agents.Upsert(context.Background(), a)
	}
	return s
}

func TestStatusWriter_NeedLeaderElection(t *testing.T) {
	sw := NewStatusWriter(nil, nil, time.Second)
	if !sw.NeedLeaderElection() {
		t.Error("StatusWriter must require leader election")
	}
}

func TestStatusWriter_SyncAgents_UpdatesPhase(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = AddToScheme(scheme)

	crd := &Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sw-agent",
			Namespace: "default",
			Annotations: map[string]string{
				AnnotationManagedBy: ManagedByCRDSync,
			},
		},
		Spec:   resources.AgentSpec{ModelRef: "gpt-4", Prompt: "test"},
		Status: CRDStatus{Phase: "Synced", ObservedGeneration: 1},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(crd).
		WithStatusSubresource(crd).
		Build()

	stores := testStoresWithAgents(resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata: resources.ObjectMeta{
			Name:      "sw-agent",
			Namespace: "default",
			Annotations: map[string]string{
				AnnotationManagedBy: ManagedByCRDSync,
			},
			Generation: 2,
		},
		Spec:   resources.AgentSpec{ModelRef: "gpt-4", Prompt: "test"},
		Status: resources.AgentStatus{Phase: "Ready"},
	})

	sw := NewStatusWriter(fakeClient, stores, time.Second)
	ctx := context.Background()

	if err := sw.syncAgents(ctx); err != nil {
		t.Fatalf("syncAgents: %v", err)
	}

	updated := &Agent{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: "sw-agent", Namespace: "default"}, updated); err != nil {
		t.Fatalf("get: %v", err)
	}
	if updated.Status.Phase != "Ready" {
		t.Errorf("expected phase Ready, got %q", updated.Status.Phase)
	}
	if updated.Status.ObservedGeneration != 2 {
		t.Errorf("expected generation 2, got %d", updated.Status.ObservedGeneration)
	}
}

func TestStatusWriter_SyncAgents_SkipsNonCRDManaged(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = AddToScheme(scheme)

	crd := &Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "manual-agent",
			Namespace: "default",
		},
		Spec:   resources.AgentSpec{ModelRef: "gpt-4", Prompt: "test"},
		Status: CRDStatus{Phase: "Synced"},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(crd).
		WithStatusSubresource(crd).
		Build()

	stores := testStoresWithAgents(resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata: resources.ObjectMeta{
			Name:      "manual-agent",
			Namespace: "default",
		},
		Spec:   resources.AgentSpec{ModelRef: "gpt-4", Prompt: "test"},
		Status: resources.AgentStatus{Phase: "Ready"},
	})

	sw := NewStatusWriter(fakeClient, stores, time.Second)
	ctx := context.Background()

	if err := sw.syncAgents(ctx); err != nil {
		t.Fatalf("syncAgents: %v", err)
	}

	// Status should be unchanged because it's not CRD-managed
	updated := &Agent{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: "manual-agent", Namespace: "default"}, updated); err != nil {
		t.Fatalf("get: %v", err)
	}
	if updated.Status.Phase != "Synced" {
		t.Errorf("expected phase unchanged (Synced), got %q", updated.Status.Phase)
	}
}

func TestStatusWriter_SyncAgents_SkipsWhenUnchanged(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = AddToScheme(scheme)

	crd := &Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "unchanged-agent",
			Namespace: "default",
			Annotations: map[string]string{
				AnnotationManagedBy: ManagedByCRDSync,
			},
		},
		Spec:   resources.AgentSpec{ModelRef: "gpt-4", Prompt: "test"},
		Status: CRDStatus{Phase: "Ready", ObservedGeneration: 1},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(crd).
		WithStatusSubresource(crd).
		Build()

	stores := testStoresWithAgents(resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata: resources.ObjectMeta{
			Name:      "unchanged-agent",
			Namespace: "default",
			Annotations: map[string]string{
				AnnotationManagedBy: ManagedByCRDSync,
			},
			Generation: 1,
		},
		Spec:   resources.AgentSpec{ModelRef: "gpt-4", Prompt: "test"},
		Status: resources.AgentStatus{Phase: "Ready"},
	})

	sw := NewStatusWriter(fakeClient, stores, time.Second)
	ctx := context.Background()

	// Should not error — just skip the update because phase+generation match
	if err := sw.syncAgents(ctx); err != nil {
		t.Fatalf("syncAgents: %v", err)
	}
}

func TestStatusWriter_SyncAgents_TargetNamespace(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = AddToScheme(scheme)

	// CRD lives in K8s namespace "orloj" but targets Orloj namespace "production"
	crd := &Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cross-ns-agent",
			Namespace: "orloj",
			Annotations: map[string]string{
				AnnotationManagedBy:       ManagedByCRDSync,
				AnnotationTargetNamespace: "production",
			},
		},
		Spec:   resources.AgentSpec{ModelRef: "gpt-4o", Prompt: "test"},
		Status: CRDStatus{Phase: "Synced", ObservedGeneration: 1},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(crd).
		WithStatusSubresource(crd).
		Build()

	// Store has the agent in Orloj namespace "production" with source-namespace annotation
	stores := testStoresWithAgents(resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata: resources.ObjectMeta{
			Name:      "cross-ns-agent",
			Namespace: "production",
			Annotations: map[string]string{
				AnnotationManagedBy:       ManagedByCRDSync,
				AnnotationTargetNamespace: "production",
				AnnotationSourceNamespace: "orloj",
			},
			Generation: 3,
		},
		Spec:   resources.AgentSpec{ModelRef: "gpt-4o", Prompt: "test"},
		Status: resources.AgentStatus{Phase: "Ready"},
	})

	sw := NewStatusWriter(fakeClient, stores, time.Second)
	ctx := context.Background()

	if err := sw.syncAgents(ctx); err != nil {
		t.Fatalf("syncAgents: %v", err)
	}

	// Verify it looked up the CRD in K8s namespace "orloj" (not "production")
	updated := &Agent{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: "cross-ns-agent", Namespace: "orloj"}, updated); err != nil {
		t.Fatalf("get: %v", err)
	}
	if updated.Status.Phase != "Ready" {
		t.Errorf("expected phase Ready, got %q", updated.Status.Phase)
	}
	if updated.Status.ObservedGeneration != 3 {
		t.Errorf("expected generation 3, got %d", updated.Status.ObservedGeneration)
	}
}

func TestStatusWriter_SyncSecrets_UsesHardcodedSyncedPhase(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = AddToScheme(scheme)

	crd := &Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sw-secret",
			Namespace: "default",
			Annotations: map[string]string{
				AnnotationManagedBy: ManagedByCRDSync,
			},
		},
		Spec:   resources.SecretSpec{StringData: map[string]string{"key": "val"}},
		Status: CRDStatus{Phase: ""},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(crd).
		WithStatusSubresource(&Secret{}).
		Build()

	secretStore := store.NewSecretStore()
	_, err := secretStore.Upsert(context.Background(), resources.Secret{
		APIVersion: "orloj.dev/v1",
		Kind:       "Secret",
		Metadata: resources.ObjectMeta{
			Name:      "sw-secret",
			Namespace: "default",
			Annotations: map[string]string{
				AnnotationManagedBy: ManagedByCRDSync,
			},
			Generation: 1,
		},
		Spec: resources.SecretSpec{StringData: map[string]string{"key": "val"}},
	})
	if err != nil {
		t.Fatalf("store upsert: %v", err)
	}

	stores := &startup.StoreSet{
		Agents:       store.NewAgentStore(),
		AgentSystems: store.NewAgentSystemStore(),
		ModelEPs:     store.NewModelEndpointStore(),
		Tools:        store.NewToolStore(),
		Secrets:      secretStore,
		Memories:     store.NewMemoryStore(),
		Policies:     store.NewAgentPolicyStore(),
		McpServers:   store.NewMcpServerStore(),
	}

	sw := NewStatusWriter(fakeClient, stores, time.Second)
	ctx := context.Background()

	if err := sw.syncSecrets(ctx); err != nil {
		t.Fatalf("syncSecrets: %v", err)
	}

	updated := &Secret{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: "sw-secret", Namespace: "default"}, updated); err != nil {
		t.Fatalf("get: %v", err)
	}
	if updated.Status.Phase != "Synced" {
		t.Errorf("expected phase Synced for secrets, got %q", updated.Status.Phase)
	}
}
