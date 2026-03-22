package startup

import (
	"fmt"
	"log"
	"strings"
	"time"

	agentruntime "github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/store"
)

type ModelGatewayConfig struct {
	Provider     string
	APIKey       string
	BaseURL      string
	DefaultModel string
	Timeout      time.Duration
}

func NewModelGateway(cfg ModelGatewayConfig) (agentruntime.ModelGateway, error) {
	gwCfg := agentruntime.DefaultModelGatewayConfig()
	gwCfg.Provider = strings.TrimSpace(cfg.Provider)
	gwCfg.APIKey = strings.TrimSpace(cfg.APIKey)
	gwCfg.BaseURL = strings.TrimSpace(cfg.BaseURL)
	gwCfg.DefaultModel = strings.TrimSpace(cfg.DefaultModel)
	gwCfg.Timeout = cfg.Timeout
	return agentruntime.NewModelGatewayFromConfig(gwCfg)
}

func ResolveModelGatewayAPIKey(provider string, explicit string) string {
	key := strings.TrimSpace(explicit)
	if key != "" {
		return key
	}
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "openai", "openai-compatible", "openai_compatible":
		return EnvOrDefault("OPENAI_API_KEY", "")
	case "anthropic":
		return EnvOrDefault("ANTHROPIC_API_KEY", "")
	case "azure-openai", "azure_openai", "azure":
		key := EnvOrDefault("AZURE_OPENAI_API_KEY", "")
		if key != "" {
			return key
		}
		return EnvOrDefault("OPENAI_API_KEY", "")
	default:
		return ""
	}
}

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
	WASMRuntimeBin   string
	WASMRuntimeArgs  []string
	WASMMemoryBytes  int64
	WASMFuel         uint64
	WASMWASI         bool
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

	wasmCfg := agentruntime.WASMToolRuntimeConfig{
		ModulePath:     strings.TrimSpace(cfg.WASMModule),
		Entrypoint:     strings.TrimSpace(cfg.WASMEntrypoint),
		RuntimeBinary:  strings.TrimSpace(cfg.WASMRuntimeBin),
		RuntimeArgs:    append([]string(nil), cfg.WASMRuntimeArgs...),
		MaxMemoryBytes: cfg.WASMMemoryBytes,
		Fuel:           cfg.WASMFuel,
		EnableWASI:     cfg.WASMWASI,
	}
	wasmFactory := agentruntime.NewWASMCommandExecutorFactory()
	runtime, err := agentruntime.BuildToolIsolationRuntime(agentruntime.ToolIsolationBackendOptions{
		Mode:                mode,
		ContainerConfig:     containerCfg,
		SecretResolver:      resolver,
		WASMConfig:          wasmCfg,
		WASMExecutorFactory: wasmFactory,
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
		case "wasm":
			logger.Printf("tool isolation backend=%s module=%s entrypoint=%s runtime=%s runtime_args=%d wasi=%t memory_bytes=%d fuel=%d",
				"wasm", wasmCfg.ModulePath, wasmCfg.Entrypoint, wasmCfg.RuntimeBinary,
				len(wasmCfg.RuntimeArgs), wasmCfg.EnableWASI, wasmCfg.MaxMemoryBytes, wasmCfg.Fuel)
		}
	}
	return runtime, nil
}

func NewAgentMessageBus(
	logger *log.Logger,
	backend string,
	natsURL string,
	subjectPrefix string,
	streamName string,
	historyMax int,
	dedupeWindow time.Duration,
) (agentruntime.AgentMessageBus, func()) {
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
		if err != nil && logger != nil {
			logger.Fatalf("failed to initialize runtime agent message bus: %v", err)
		}
		return bus, func() { _ = bus.Close() }
	default:
		if logger != nil {
			logger.Fatalf("unsupported runtime agent message bus backend %q; expected none, memory, or nats-jetstream", backend)
		}
		return nil, nil
	}
}

func LogModelGatewayConfig(logger *log.Logger, provider string, timeout time.Duration, baseURL, defaultModel, secretPrefix string) {
	if logger == nil {
		return
	}
	logger.Printf("task model gateway provider=%s timeout=%s base_url=%s default_model=%s model_secret_env_prefix=%s",
		strings.ToLower(strings.TrimSpace(provider)),
		timeout.String(),
		strings.TrimSpace(baseURL),
		strings.TrimSpace(defaultModel),
		strings.TrimSpace(secretPrefix),
	)
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
