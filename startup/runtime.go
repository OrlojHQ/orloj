package startup

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	agentruntime "github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/store"
	"github.com/tetratelabs/wazero"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type IsolatedToolRuntimeConfig struct {
	Backend          string
	ContainerRuntime string
	ContainerImage   string
	ContainerNetwork string
	ContainerMemory  string
	ContainerCPUs    string
	ContainerPids    int
	ContainerUser    string
	SecretEnvPrefix  string
	WASMModule       string
	WASMEntrypoint   string
	WASMMemoryBytes  int64
	WASMFuel         uint64
	WASMWASI         bool
	WASMCacheDir     string
	Secrets          agentruntime.SecretResourceLookup
}

type McpRuntimeConfig struct {
	ContainerRuntime string
	ContainerNetwork string
	ContainerMemory  string
	ContainerCPUs    string
	ContainerPids    int
	SecretEnvPrefix  string
	Secrets          agentruntime.SecretResourceLookup
}

func NewIsolatedToolRuntime(cfg IsolatedToolRuntimeConfig, logger *log.Logger) (agentruntime.ToolRuntime, error) {
	mode := strings.ToLower(strings.TrimSpace(cfg.Backend))
	if mode == "" {
		mode = "none"
	}
	containerCfg := agentruntime.DefaultContainerToolRuntimeConfig()
	containerCfg.RuntimeBinary = strings.TrimSpace(cfg.ContainerRuntime)
	containerCfg.Image = strings.TrimSpace(cfg.ContainerImage)
	containerCfg.Network = strings.TrimSpace(cfg.ContainerNetwork)
	containerCfg.Memory = strings.TrimSpace(cfg.ContainerMemory)
	containerCfg.CPUs = strings.TrimSpace(cfg.ContainerCPUs)
	containerCfg.PidsLimit = cfg.ContainerPids
	containerCfg.User = strings.TrimSpace(cfg.ContainerUser)

	storeResolver := agentruntime.NewStoreSecretResolver(cfg.Secrets, "value")
	envResolver := agentruntime.NewEnvSecretResolver(strings.TrimSpace(cfg.SecretEnvPrefix))
	resolver := agentruntime.NewChainSecretResolver(storeResolver, envResolver)

	runtime, err := agentruntime.BuildToolIsolationRuntime(agentruntime.ToolIsolationBackendOptions{
		Mode:            mode,
		ContainerConfig: containerCfg,
		SecretResolver:  resolver,
	})
	if err != nil {
		return nil, err
	}
	if logger != nil {
		switch mode {
		case "none":
			logger.Printf("tool isolation backend=%s", "none")
		case "container":
			logger.Printf("tool isolation backend=%s runtime=%s image=%s network=%s",
				"container", containerCfg.RuntimeBinary, containerCfg.Image, containerCfg.Network)
		}
	}
	return runtime, nil
}

func NewMcpSessionManager(cfg McpRuntimeConfig) *agentruntime.McpSessionManager {
	storeResolver := agentruntime.NewStoreSecretResolver(cfg.Secrets, "value")
	envPrefix := strings.TrimSpace(cfg.SecretEnvPrefix)
	if envPrefix == "" {
		envPrefix = "ORLOJ_SECRET_"
	}
	envResolver := agentruntime.NewEnvSecretResolver(envPrefix)
	resolver := agentruntime.NewChainSecretResolver(storeResolver, envResolver)

	sessionManager := agentruntime.NewMcpSessionManager(resolver)
	network := strings.TrimSpace(cfg.ContainerNetwork)
	if network == "" {
		network = "bridge"
	}
	sessionManager.SetContainerConfig(agentruntime.ContainerToolRuntimeConfig{
		RuntimeBinary: strings.TrimSpace(cfg.ContainerRuntime),
		Network:       network,
		Memory:        strings.TrimSpace(cfg.ContainerMemory),
		CPUs:          strings.TrimSpace(cfg.ContainerCPUs),
		PidsLimit:     cfg.ContainerPids,
	})
	return sessionManager
}

// NewWASMToolRuntime creates an embedded wazero-backed WASM tool runtime.
// This is always initialized regardless of --tool-isolation-backend because
// the wazero engine is pure Go with zero external dependencies.
func NewWASMToolRuntime(cfg IsolatedToolRuntimeConfig, logger *log.Logger) (agentruntime.ToolRuntime, func(), error) {
	wasmEngine := wazero.NewRuntimeWithConfig(context.Background(), wazero.NewRuntimeConfigInterpreter())

	// Build module resolver for remote (HTTPS/OCI) module references.
	storeResolver := agentruntime.NewStoreSecretResolver(cfg.Secrets, "value")
	envResolver := agentruntime.NewEnvSecretResolver(strings.TrimSpace(cfg.SecretEnvPrefix))
	secretResolver := agentruntime.NewChainSecretResolver(storeResolver, envResolver)

	moduleResolver, err := agentruntime.NewWASMModuleResolver(agentruntime.WASMModuleResolverConfig{
		CacheDir:       strings.TrimSpace(cfg.WASMCacheDir),
		AllowPrivate:   false,
		SecretResolver: secretResolver,
	})
	if err != nil && logger != nil {
		logger.Printf("WARNING: wasm module resolver init failed: %v (remote modules will not be available)", err)
	}

	wasmFactory := agentruntime.NewWazeroExecutorFactoryWithResolver(wasmEngine, moduleResolver)
	wasmCfg := agentruntime.WASMToolRuntimeConfig{
		ModulePath:     strings.TrimSpace(cfg.WASMModule),
		Entrypoint:     strings.TrimSpace(cfg.WASMEntrypoint),
		MaxMemoryBytes: cfg.WASMMemoryBytes,
		Fuel:           cfg.WASMFuel,
		EnableWASI:     cfg.WASMWASI,
	}
	rt := agentruntime.NewWASMToolRuntimeWithFactory(nil, wasmFactory, wasmCfg)
	cleanup := func() { wasmEngine.Close(context.Background()) }
	if logger != nil {
		cacheDir := strings.TrimSpace(cfg.WASMCacheDir)
		if cacheDir == "" {
			cacheDir = "(default)"
		}
		logger.Printf("wasm runtime=wazero (embedded) module=%s entrypoint=%s wasi=%t memory_bytes=%d fuel=%d cache_dir=%s",
			wasmCfg.ModulePath, wasmCfg.Entrypoint, wasmCfg.EnableWASI, wasmCfg.MaxMemoryBytes, wasmCfg.Fuel, cacheDir)
	}
	return rt, cleanup, nil
}

// KubernetesToolRuntimeConfig holds flag-level configuration for the K8s backend.
type KubernetesToolRuntimeConfig struct {
	Namespace      string
	ServiceAccount string
	JobTTLSeconds  int32
	DefaultImage   string
	SecretEnvPrefix string
	Secrets        agentruntime.SecretResourceLookup
}

// NewKubernetesToolRuntime creates a K8s Job-based tool runtime.
// Uses in-cluster config when available, falling back to kubeconfig.
func NewKubernetesToolRuntime(cfg KubernetesToolRuntimeConfig, logger *log.Logger) (agentruntime.ToolRuntime, error) {
	storeResolver := agentruntime.NewStoreSecretResolver(cfg.Secrets, "value")
	envResolver := agentruntime.NewEnvSecretResolver(strings.TrimSpace(cfg.SecretEnvPrefix))
	secretResolver := agentruntime.NewChainSecretResolver(storeResolver, envResolver)

	k8sConfig := agentruntime.KubernetesToolConfig{
		Namespace:      strings.TrimSpace(cfg.Namespace),
		ServiceAccount: strings.TrimSpace(cfg.ServiceAccount),
		DefaultImage:   strings.TrimSpace(cfg.DefaultImage),
		JobTTLSeconds:  cfg.JobTTLSeconds,
	}

	clientset, err := buildKubernetesClientset()
	if err != nil {
		return nil, fmt.Errorf("kubernetes client initialization failed: %w", err)
	}

	k8sSecretResolver := agentruntime.NewKubernetesSecretResolver(clientset, "value")
	combinedResolver := agentruntime.NewChainSecretResolver(k8sSecretResolver, secretResolver)

	rt := agentruntime.NewKubernetesToolRuntime(clientset, k8sConfig, combinedResolver)
	if logger != nil {
		ns := strings.TrimSpace(cfg.Namespace)
		if ns == "" {
			ns = "(pod-namespace or default)"
		}
		logger.Printf("kubernetes tool isolation enabled namespace=%s service_account=%s job_ttl=%d default_image=%s",
			ns, strings.TrimSpace(cfg.ServiceAccount), cfg.JobTTLSeconds, strings.TrimSpace(cfg.DefaultImage))
	}
	return rt, nil
}

// KubernetesAgentRuntimeConfig holds flag-level configuration for agent K8s execution.
type KubernetesAgentRuntimeConfig struct {
	Namespace      string
	ServiceAccount string
	Image          string
	JobTTLSeconds  int32
	DefaultMemory  string
	DefaultCPU     string
	EnvConfigMap   string
	EnvSecretName  string
}

// NewKubernetesAgentRuntime creates a K8s Job-based agent runtime.
func NewKubernetesAgentRuntime(cfg KubernetesAgentRuntimeConfig, store agentruntime.AgentJobStore, logger *log.Logger) (*agentruntime.KubernetesAgentRuntime, error) {
	clientset, err := buildKubernetesClientset()
	if err != nil {
		return nil, fmt.Errorf("kubernetes client initialization failed: %w", err)
	}
	client := agentruntime.NewDefaultKubernetesJobClient(clientset)
	agentCfg := agentruntime.KubernetesAgentConfig{
		Namespace:      strings.TrimSpace(cfg.Namespace),
		ServiceAccount: strings.TrimSpace(cfg.ServiceAccount),
		Image:          strings.TrimSpace(cfg.Image),
		JobTTLSeconds:  cfg.JobTTLSeconds,
		DefaultMemory:  strings.TrimSpace(cfg.DefaultMemory),
		DefaultCPU:     strings.TrimSpace(cfg.DefaultCPU),
		EnvConfigMap:   strings.TrimSpace(cfg.EnvConfigMap),
		EnvSecretName:  strings.TrimSpace(cfg.EnvSecretName),
	}
	rt := agentruntime.NewKubernetesAgentRuntime(client, agentCfg, store, logger)
	if logger != nil {
		ns := strings.TrimSpace(cfg.Namespace)
		if ns == "" {
			ns = "(pod-namespace or default)"
		}
		logger.Printf("kubernetes agent execution enabled namespace=%s service_account=%s image=%s job_ttl=%d default_memory=%s default_cpu=%s",
			ns, strings.TrimSpace(cfg.ServiceAccount), strings.TrimSpace(cfg.Image), cfg.JobTTLSeconds, strings.TrimSpace(cfg.DefaultMemory), strings.TrimSpace(cfg.DefaultCPU))
	}
	return rt, nil
}

func buildKubernetesClientset() (kubernetes.Interface, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		home := os.Getenv("HOME")
		if home == "" {
			return nil, fmt.Errorf("not in cluster and HOME not set: %w", err)
		}
		kubeconfigPath := home + "/.kube/config"
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to build kubeconfig from %s: %w", kubeconfigPath, err)
		}
	}
	return kubernetes.NewForConfig(cfg)
}

func NewAgentMessageBus(
	logger *log.Logger,
	backend string,
	natsURL string,
	subjectPrefix string,
	streamName string,
	historyMax int,
	dedupeWindow time.Duration,
	fatalLoggers ...*log.Logger,
) (agentruntime.AgentMessageBus, func()) {
	fatalLogger := logger
	if len(fatalLoggers) > 0 && fatalLoggers[0] != nil {
		fatalLogger = fatalLoggers[0]
	}
	mode := strings.ToLower(strings.TrimSpace(backend))
	switch mode {
	case "", "none":
		if logger != nil {
			logger.Printf("runtime agent message bus backend=%s", "none")
		}
		return nil, nil
	case "memory":
		bus := agentruntime.NewMemoryAgentMessageBus(subjectPrefix, historyMax, dedupeWindow)
		if logger != nil {
			logger.Printf("runtime agent message bus backend=%s prefix=%s history_max=%d dedupe_window=%s",
				"memory", subjectPrefix, historyMax, dedupeWindow)
		}
		return bus, func() { _ = bus.Close() }
	case "nats", "nats-jetstream":
		bus, err := agentruntime.NewNATSJetStreamAgentMessageBus(natsURL, subjectPrefix, streamName, logger)
		if err != nil && fatalLogger != nil {
			fatalLogger.Fatalf("failed to initialize runtime agent message bus: %v", err)
		}
		return bus, func() { _ = bus.Close() }
	default:
		if fatalLogger != nil {
			fatalLogger.Fatalf("unsupported runtime agent message bus backend %q; expected none, memory, or nats-jetstream", backend)
		}
		return nil, nil
	}
}

func LogSecretEncryption(logger *log.Logger, key []byte) {
	if logger == nil {
		return
	}
	if len(key) == 0 {
		logger.Printf("WARNING: secret encryption at rest is DISABLED — secrets will be stored as base64 plaintext; set ORLOJ_SECRET_ENCRYPTION_KEY to enable encryption")
		return
	}
	logger.Printf("secret encryption at rest enabled (AES-256-GCM)")
}

func ParseSecretEncryptionKey(raw string) ([]byte, error) {
	key, err := store.ParseEncryptionKey(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid --secret-encryption-key: %w", err)
	}
	return key, nil
}
