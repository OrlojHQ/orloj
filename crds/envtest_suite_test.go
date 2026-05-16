package crds_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/OrlojHQ/orloj/crds"
	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/store"
	"github.com/OrlojHQ/orloj/startup"
)

func ptr[T any](v T) *T { return &v }

func setupEnvtest(t *testing.T) (client.Client, *runtime.Scheme, *envtest.Environment) {
	t.Helper()
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("skipping envtest: KUBEBUILDER_ASSETS not set (run via 'make test-operator')")
	}
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add client-go scheme: %v", err)
	}
	if err := crds.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add CRD scheme: %v", err)
	}

	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
		Scheme:                scheme,
	}

	cfg, err := testEnv.Start()
	if err != nil {
		t.Fatalf("failed to start envtest: %v", err)
	}

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		testEnv.Stop()
		t.Fatalf("failed to create client: %v", err)
	}

	t.Cleanup(func() {
		testEnv.Stop()
	})

	return k8sClient, scheme, testEnv
}

func newMemoryStores() *startup.StoreSet {
	return &startup.StoreSet{
		Agents:       store.NewAgentStore(),
		AgentSystems: store.NewAgentSystemStore(),
		ModelEPs:     store.NewModelEndpointStore(),
		Tools:        store.NewToolStore(),
		Secrets:      store.NewSecretStore(),
		Memories:     store.NewMemoryStore(),
		Policies:     store.NewAgentPolicyStore(),
		McpServers:   store.NewMcpServerStore(),
	}
}

// TestEnvtest_AgentFullLifecycle exercises create → sync → update → delete
// using a real K8s API server (envtest).
func TestEnvtest_AgentFullLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping envtest in short mode")
	}

	k8sClient, scheme, testEnv := setupEnvtest(t)
	stores := newMemoryStores()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mgr, err := ctrl.NewManager(testEnv.Config, ctrl.Options{
		Scheme:         scheme,
		LeaderElection: false,
		Metrics:        metricsserver.Options{BindAddress: "0"},
		Controller:     config.Controller{SkipNameValidation: ptr(true)},
	})
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	rec := &crds.SyncReconciler[*crds.Agent, resources.Agent]{
		Client: mgr.GetClient(),
		Scheme: scheme,
		NewCRD: func() *crds.Agent { return &crds.Agent{} },
		Syncer: crds.ResourceSyncer[*crds.Agent, resources.Agent]{
			Kind:       "Agent",
			ToOrloj:    crds.AgentToOrloj,
			Upsert:     stores.Agents.Upsert,
			Delete:     stores.Agents.Delete,
			ScopedName: func(a *crds.Agent) string { return crds.ScopedName(a.ObjectMeta) },
		},
	}
	if err := rec.SetupWithManager(mgr); err != nil {
		t.Fatalf("failed to register reconciler: %v", err)
	}

	go func() {
		if err := mgr.Start(ctx); err != nil {
			t.Logf("manager exited: %v", err)
		}
	}()

	// Wait for manager cache to sync
	if !mgr.GetCache().WaitForCacheSync(ctx) {
		t.Fatal("cache never synced")
	}

	// --- CREATE ---
	agent := &crds.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "envtest-agent",
			Namespace: "default",
		},
		Spec: resources.AgentSpec{
			ModelRef: "gpt-4o",
			Prompt:   "You are a test agent.",
		},
	}
	if err := k8sClient.Create(ctx, agent); err != nil {
		t.Fatalf("failed to create Agent CRD: %v", err)
	}

	// Poll until synced to store
	var storeAgent resources.Agent
	waitFor(t, ctx, 5*time.Second, func() bool {
		a, ok, _ := stores.Agents.Get(ctx, "default/envtest-agent")
		if ok {
			storeAgent = a
			return true
		}
		return false
	})

	if storeAgent.Spec.ModelRef != "gpt-4o" {
		t.Errorf("expected model_ref gpt-4o, got %q", storeAgent.Spec.ModelRef)
	}
	if storeAgent.Metadata.Annotations[crds.AnnotationManagedBy] != crds.ManagedByCRDSync {
		t.Error("expected managed-by annotation on store resource")
	}

	// Verify finalizer was added
	updated := &crds.Agent{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: "envtest-agent", Namespace: "default"}, updated); err != nil {
		t.Fatalf("failed to get agent: %v", err)
	}
	if !containsFinalizer(updated.Finalizers, crds.FinalizerSync) {
		t.Error("expected finalizer to be present")
	}

	// Verify status was set
	waitFor(t, ctx, 5*time.Second, func() bool {
		a := &crds.Agent{}
		_ = k8sClient.Get(ctx, types.NamespacedName{Name: "envtest-agent", Namespace: "default"}, a)
		return a.Status.Phase == "Synced"
	})

	// --- UPDATE ---
	fresh := &crds.Agent{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: "envtest-agent", Namespace: "default"}, fresh); err != nil {
		t.Fatalf("failed to get fresh agent: %v", err)
	}
	fresh.Spec.Prompt = "Updated prompt"
	if err := k8sClient.Update(ctx, fresh); err != nil {
		t.Fatalf("failed to update Agent CRD: %v", err)
	}

	waitFor(t, ctx, 5*time.Second, func() bool {
		a, ok, _ := stores.Agents.Get(ctx, "default/envtest-agent")
		return ok && a.Spec.Prompt == "Updated prompt"
	})

	// --- DELETE ---
	toDelete := &crds.Agent{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: "envtest-agent", Namespace: "default"}, toDelete); err != nil {
		t.Fatalf("failed to get agent for delete: %v", err)
	}
	if err := k8sClient.Delete(ctx, toDelete); err != nil {
		t.Fatalf("failed to delete Agent CRD: %v", err)
	}

	waitFor(t, ctx, 5*time.Second, func() bool {
		_, ok, _ := stores.Agents.Get(ctx, "default/envtest-agent")
		return !ok
	})
}

// TestEnvtest_AllKindsSync verifies all 8 CRD kinds reconcile to the store.
func TestEnvtest_AllKindsSync(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping envtest in short mode")
	}

	k8sClient, scheme, testEnv := setupEnvtest(t)
	stores := newMemoryStores()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	mgr, err := ctrl.NewManager(testEnv.Config, ctrl.Options{
		Scheme:         scheme,
		LeaderElection: false,
		Metrics:        metricsserver.Options{BindAddress: "0"},
		Controller:     config.Controller{SkipNameValidation: ptr(true)},
	})
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	registerAllReconcilers(t, mgr, stores, scheme)

	go func() {
		if err := mgr.Start(ctx); err != nil {
			t.Logf("manager exited: %v", err)
		}
	}()
	if !mgr.GetCache().WaitForCacheSync(ctx) {
		t.Fatal("cache never synced")
	}

	t.Run("Agent", func(t *testing.T) {
		obj := &crds.Agent{
			ObjectMeta: metav1.ObjectMeta{Name: "kind-test-agent", Namespace: "default"},
			Spec:       resources.AgentSpec{ModelRef: "gpt-4", Prompt: "test"},
		}
		if err := k8sClient.Create(ctx, obj); err != nil {
			t.Fatalf("create: %v", err)
		}
		waitFor(t, ctx, 5*time.Second, func() bool {
			_, ok, _ := stores.Agents.Get(ctx, "default/kind-test-agent")
			return ok
		})
	})

	t.Run("AgentSystem", func(t *testing.T) {
		obj := &crds.AgentSystem{
			ObjectMeta: metav1.ObjectMeta{Name: "kind-test-sys", Namespace: "default"},
			Spec:       resources.AgentSystemSpec{Agents: []string{"a1"}},
		}
		if err := k8sClient.Create(ctx, obj); err != nil {
			t.Fatalf("create: %v", err)
		}
		waitFor(t, ctx, 5*time.Second, func() bool {
			_, ok, _ := stores.AgentSystems.Get(ctx, "default/kind-test-sys")
			return ok
		})
	})

	t.Run("Tool", func(t *testing.T) {
		obj := &crds.Tool{
			ObjectMeta: metav1.ObjectMeta{Name: "kind-test-tool", Namespace: "default"},
			Spec:       resources.ToolSpec{Description: "test tool"},
		}
		if err := k8sClient.Create(ctx, obj); err != nil {
			t.Fatalf("create: %v", err)
		}
		waitFor(t, ctx, 5*time.Second, func() bool {
			_, ok, _ := stores.Tools.Get(ctx, "default/kind-test-tool")
			return ok
		})
	})

	t.Run("McpServer", func(t *testing.T) {
		obj := &crds.McpServer{
			ObjectMeta: metav1.ObjectMeta{Name: "kind-test-mcp", Namespace: "default"},
			Spec:       resources.McpServerSpec{Transport: "stdio", Command: "echo"},
		}
		if err := k8sClient.Create(ctx, obj); err != nil {
			t.Fatalf("create: %v", err)
		}
		waitFor(t, ctx, 5*time.Second, func() bool {
			_, ok, _ := stores.McpServers.Get(ctx, "default/kind-test-mcp")
			return ok
		})
	})

	t.Run("ModelEndpoint", func(t *testing.T) {
		obj := &crds.ModelEndpoint{
			ObjectMeta: metav1.ObjectMeta{Name: "kind-test-ep", Namespace: "default"},
			Spec:       resources.ModelEndpointSpec{Provider: "openai", DefaultModel: "gpt-4o"},
		}
		if err := k8sClient.Create(ctx, obj); err != nil {
			t.Fatalf("create: %v", err)
		}
		waitFor(t, ctx, 5*time.Second, func() bool {
			_, ok, _ := stores.ModelEPs.Get(ctx, "default/kind-test-ep")
			return ok
		})
	})

	t.Run("Memory", func(t *testing.T) {
		obj := &crds.Memory{
			ObjectMeta: metav1.ObjectMeta{Name: "kind-test-mem", Namespace: "default"},
			Spec:       resources.MemoryConfig{Type: "vector", Provider: "builtin"},
		}
		if err := k8sClient.Create(ctx, obj); err != nil {
			t.Fatalf("create: %v", err)
		}
		waitFor(t, ctx, 5*time.Second, func() bool {
			_, ok, _ := stores.Memories.Get(ctx, "default/kind-test-mem")
			return ok
		})
	})

	t.Run("AgentPolicy", func(t *testing.T) {
		obj := &crds.AgentPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "kind-test-policy", Namespace: "default"},
			Spec:       resources.AgentPolicySpec{},
		}
		if err := k8sClient.Create(ctx, obj); err != nil {
			t.Fatalf("create: %v", err)
		}
		waitFor(t, ctx, 5*time.Second, func() bool {
			_, ok, _ := stores.Policies.Get(ctx, "default/kind-test-policy")
			return ok
		})
	})

	t.Run("Secret", func(t *testing.T) {
		obj := &crds.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "kind-test-secret", Namespace: "default"},
			Spec:       resources.SecretSpec{StringData: map[string]string{"api-key": "test-value"}},
		}
		if err := k8sClient.Create(ctx, obj); err != nil {
			t.Fatalf("create: %v", err)
		}
		waitFor(t, ctx, 5*time.Second, func() bool {
			_, ok, _ := stores.Secrets.Get(ctx, "default/kind-test-secret")
			return ok
		})
	})
}

// TestEnvtest_UpsertErrorSetsStatus verifies that store validation errors
// propagate to .status.phase = SyncError on the CRD object.
func TestEnvtest_UpsertErrorSetsStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping envtest in short mode")
	}

	k8sClient, scheme, testEnv := setupEnvtest(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mgr, err := ctrl.NewManager(testEnv.Config, ctrl.Options{
		Scheme:         scheme,
		LeaderElection: false,
		Metrics:        metricsserver.Options{BindAddress: "0"},
		Controller:     config.Controller{SkipNameValidation: ptr(true)},
	})
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	failingUpsert := func(_ context.Context, _ resources.Agent) (resources.Agent, error) {
		return resources.Agent{}, fmt.Errorf("validation: model_ref is required")
	}

	rec := &crds.SyncReconciler[*crds.Agent, resources.Agent]{
		Client: mgr.GetClient(),
		Scheme: scheme,
		NewCRD: func() *crds.Agent { return &crds.Agent{} },
		Syncer: crds.ResourceSyncer[*crds.Agent, resources.Agent]{
			Kind:    "Agent",
			ToOrloj: crds.AgentToOrloj,
			Upsert:  failingUpsert,
			Delete: func(_ context.Context, _ string) error { return nil },
			ScopedName: func(a *crds.Agent) string { return crds.ScopedName(a.ObjectMeta) },
		},
	}
	if err := rec.SetupWithManager(mgr); err != nil {
		t.Fatalf("setup: %v", err)
	}

	go func() {
		if err := mgr.Start(ctx); err != nil {
			t.Logf("manager exited: %v", err)
		}
	}()
	if !mgr.GetCache().WaitForCacheSync(ctx) {
		t.Fatal("cache never synced")
	}

	agent := &crds.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "bad-agent", Namespace: "default"},
		Spec:       resources.AgentSpec{Prompt: "no model ref"},
	}
	if err := k8sClient.Create(ctx, agent); err != nil {
		t.Fatalf("create: %v", err)
	}

	waitFor(t, ctx, 10*time.Second, func() bool {
		a := &crds.Agent{}
		_ = k8sClient.Get(ctx, types.NamespacedName{Name: "bad-agent", Namespace: "default"}, a)
		return a.Status.Phase == "SyncError" && a.Status.SyncError != ""
	})
}

// TestEnvtest_StatusWriter tests the periodic status writeback loop.
func TestEnvtest_StatusWriter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping envtest in short mode")
	}

	k8sClient, scheme, testEnv := setupEnvtest(t)
	stores := newMemoryStores()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Pre-populate the store with a CRD-managed agent that has a specific phase
	_, err := stores.Agents.Upsert(ctx, resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata: resources.ObjectMeta{
			Name:      "status-test",
			Namespace: "default",
			Annotations: map[string]string{
				crds.AnnotationManagedBy: crds.ManagedByCRDSync,
			},
		},
		Spec: resources.AgentSpec{ModelRef: "gpt-4", Prompt: "test"},
		Status: resources.AgentStatus{Phase: "Ready"},
	})
	if err != nil {
		t.Fatalf("store upsert: %v", err)
	}

	// Create the CRD object in K8s (simulates what the reconciler would have done)
	agent := &crds.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "status-test",
			Namespace: "default",
			Annotations: map[string]string{
				crds.AnnotationManagedBy: crds.ManagedByCRDSync,
			},
		},
		Spec: resources.AgentSpec{ModelRef: "gpt-4", Prompt: "test"},
	}
	if err := k8sClient.Create(ctx, agent); err != nil {
		t.Fatalf("create CRD: %v", err)
	}

	mgr, err := ctrl.NewManager(testEnv.Config, ctrl.Options{
		Scheme:         scheme,
		LeaderElection: false,
		Metrics:        metricsserver.Options{BindAddress: "0"},
		Controller:     config.Controller{SkipNameValidation: ptr(true)},
	})
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	sw := crds.NewStatusWriter(mgr.GetClient(), stores, 500*time.Millisecond)
	if err := mgr.Add(sw); err != nil {
		t.Fatalf("add status writer: %v", err)
	}

	go func() {
		if err := mgr.Start(ctx); err != nil {
			t.Logf("manager exited: %v", err)
		}
	}()
	if !mgr.GetCache().WaitForCacheSync(ctx) {
		t.Fatal("cache never synced")
	}

	// Wait for the status writer to propagate "Ready" phase to the CRD
	waitFor(t, ctx, 5*time.Second, func() bool {
		a := &crds.Agent{}
		_ = k8sClient.Get(ctx, types.NamespacedName{Name: "status-test", Namespace: "default"}, a)
		return a.Status.Phase == "Ready"
	})
}

// TestEnvtest_DeleteStoreUnavailable verifies retry on delete failure.
func TestEnvtest_DeleteStoreUnavailable(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping envtest in short mode")
	}

	k8sClient, scheme, testEnv := setupEnvtest(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	deleteCalls := 0
	failingDelete := func(_ context.Context, _ string) error {
		deleteCalls++
		if deleteCalls <= 2 {
			return fmt.Errorf("store unavailable")
		}
		return nil
	}

	mgr, err := ctrl.NewManager(testEnv.Config, ctrl.Options{
		Scheme:         scheme,
		LeaderElection: false,
		Metrics:        metricsserver.Options{BindAddress: "0"},
		Controller:     config.Controller{SkipNameValidation: ptr(true)},
	})
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	rec := &crds.SyncReconciler[*crds.Agent, resources.Agent]{
		Client: mgr.GetClient(),
		Scheme: scheme,
		NewCRD: func() *crds.Agent { return &crds.Agent{} },
		Syncer: crds.ResourceSyncer[*crds.Agent, resources.Agent]{
			Kind:    "Agent",
			ToOrloj: crds.AgentToOrloj,
			Upsert: func(ctx context.Context, a resources.Agent) (resources.Agent, error) {
				return a, nil
			},
			Delete:     failingDelete,
			ScopedName: func(a *crds.Agent) string { return crds.ScopedName(a.ObjectMeta) },
		},
	}
	if err := rec.SetupWithManager(mgr); err != nil {
		t.Fatalf("setup: %v", err)
	}

	go func() {
		if err := mgr.Start(ctx); err != nil {
			t.Logf("manager exited: %v", err)
		}
	}()
	if !mgr.GetCache().WaitForCacheSync(ctx) {
		t.Fatal("cache never synced")
	}

	// Create and wait for sync
	agent := &crds.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "retry-agent", Namespace: "default"},
		Spec:       resources.AgentSpec{ModelRef: "gpt-4", Prompt: "test"},
	}
	if err := k8sClient.Create(ctx, agent); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Wait for finalizer
	waitFor(t, ctx, 5*time.Second, func() bool {
		a := &crds.Agent{}
		_ = k8sClient.Get(ctx, types.NamespacedName{Name: "retry-agent", Namespace: "default"}, a)
		return containsFinalizer(a.Finalizers, crds.FinalizerSync)
	})

	// Delete - should retry and eventually succeed
	toDelete := &crds.Agent{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: "retry-agent", Namespace: "default"}, toDelete); err != nil {
		t.Fatalf("get: %v", err)
	}
	if err := k8sClient.Delete(ctx, toDelete); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// Wait until the object is fully gone (finalizer removed → K8s garbage collects)
	waitFor(t, ctx, 15*time.Second, func() bool {
		a := &crds.Agent{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: "retry-agent", Namespace: "default"}, a)
		return err != nil // NotFound
	})

	if deleteCalls < 3 {
		t.Errorf("expected at least 3 delete calls (2 failures + 1 success), got %d", deleteCalls)
	}
}

// --- helpers ---

func registerAllReconcilers(t *testing.T, mgr ctrl.Manager, stores *startup.StoreSet, scheme *runtime.Scheme) {
	t.Helper()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("register reconciler: %v", err)
		}
	}

	must((&crds.SyncReconciler[*crds.Agent, resources.Agent]{
		Client: mgr.GetClient(), Scheme: scheme,
		NewCRD: func() *crds.Agent { return &crds.Agent{} },
		Syncer: crds.ResourceSyncer[*crds.Agent, resources.Agent]{
			Kind: "Agent", ToOrloj: crds.AgentToOrloj,
			Upsert: stores.Agents.Upsert, Delete: stores.Agents.Delete,
			ScopedName: func(a *crds.Agent) string { return crds.ScopedName(a.ObjectMeta) },
		},
	}).SetupWithManager(mgr))

	must((&crds.SyncReconciler[*crds.AgentSystem, resources.AgentSystem]{
		Client: mgr.GetClient(), Scheme: scheme,
		NewCRD: func() *crds.AgentSystem { return &crds.AgentSystem{} },
		Syncer: crds.ResourceSyncer[*crds.AgentSystem, resources.AgentSystem]{
			Kind: "AgentSystem", ToOrloj: crds.AgentSystemToOrloj,
			Upsert: stores.AgentSystems.Upsert, Delete: stores.AgentSystems.Delete,
			ScopedName: func(a *crds.AgentSystem) string { return crds.ScopedName(a.ObjectMeta) },
		},
	}).SetupWithManager(mgr))

	must((&crds.SyncReconciler[*crds.Tool, resources.Tool]{
		Client: mgr.GetClient(), Scheme: scheme,
		NewCRD: func() *crds.Tool { return &crds.Tool{} },
		Syncer: crds.ResourceSyncer[*crds.Tool, resources.Tool]{
			Kind: "Tool", ToOrloj: crds.ToolToOrloj,
			Upsert: stores.Tools.Upsert, Delete: stores.Tools.Delete,
			ScopedName: func(t *crds.Tool) string { return crds.ScopedName(t.ObjectMeta) },
		},
	}).SetupWithManager(mgr))

	must((&crds.SyncReconciler[*crds.McpServer, resources.McpServer]{
		Client: mgr.GetClient(), Scheme: scheme,
		NewCRD: func() *crds.McpServer { return &crds.McpServer{} },
		Syncer: crds.ResourceSyncer[*crds.McpServer, resources.McpServer]{
			Kind: "McpServer", ToOrloj: crds.McpServerToOrloj,
			Upsert: stores.McpServers.Upsert, Delete: stores.McpServers.Delete,
			ScopedName: func(m *crds.McpServer) string { return crds.ScopedName(m.ObjectMeta) },
		},
	}).SetupWithManager(mgr))

	must((&crds.SyncReconciler[*crds.ModelEndpoint, resources.ModelEndpoint]{
		Client: mgr.GetClient(), Scheme: scheme,
		NewCRD: func() *crds.ModelEndpoint { return &crds.ModelEndpoint{} },
		Syncer: crds.ResourceSyncer[*crds.ModelEndpoint, resources.ModelEndpoint]{
			Kind: "ModelEndpoint", ToOrloj: crds.ModelEndpointToOrloj,
			Upsert: stores.ModelEPs.Upsert, Delete: stores.ModelEPs.Delete,
			ScopedName: func(m *crds.ModelEndpoint) string { return crds.ScopedName(m.ObjectMeta) },
		},
	}).SetupWithManager(mgr))

	must((&crds.SyncReconciler[*crds.Memory, resources.Memory]{
		Client: mgr.GetClient(), Scheme: scheme,
		NewCRD: func() *crds.Memory { return &crds.Memory{} },
		Syncer: crds.ResourceSyncer[*crds.Memory, resources.Memory]{
			Kind: "Memory", ToOrloj: crds.MemoryToOrloj,
			Upsert: stores.Memories.Upsert, Delete: stores.Memories.Delete,
			ScopedName: func(m *crds.Memory) string { return crds.ScopedName(m.ObjectMeta) },
		},
	}).SetupWithManager(mgr))

	must((&crds.SyncReconciler[*crds.AgentPolicy, resources.AgentPolicy]{
		Client: mgr.GetClient(), Scheme: scheme,
		NewCRD: func() *crds.AgentPolicy { return &crds.AgentPolicy{} },
		Syncer: crds.ResourceSyncer[*crds.AgentPolicy, resources.AgentPolicy]{
			Kind: "AgentPolicy", ToOrloj: crds.AgentPolicyToOrloj,
			Upsert: stores.Policies.Upsert, Delete: stores.Policies.Delete,
			ScopedName: func(p *crds.AgentPolicy) string { return crds.ScopedName(p.ObjectMeta) },
		},
	}).SetupWithManager(mgr))

	must((&crds.SyncReconciler[*crds.Secret, resources.Secret]{
		Client: mgr.GetClient(), Scheme: scheme,
		NewCRD: func() *crds.Secret { return &crds.Secret{} },
		Syncer: crds.ResourceSyncer[*crds.Secret, resources.Secret]{
			Kind: "Secret", ToOrloj: crds.SecretToOrloj,
			Upsert: stores.Secrets.Upsert, Delete: stores.Secrets.Delete,
			ScopedName: func(s *crds.Secret) string { return crds.ScopedName(s.ObjectMeta) },
		},
	}).SetupWithManager(mgr))
}

func waitFor(t *testing.T, ctx context.Context, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("context cancelled while waiting")
		case <-time.After(100 * time.Millisecond):
		}
	}
	t.Fatalf("condition not met within %v", timeout)
}

func containsFinalizer(finalizers []string, target string) bool {
	for _, f := range finalizers {
		if f == target {
			return true
		}
	}
	return false
}
