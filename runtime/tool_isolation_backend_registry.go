package agentruntime

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

type ToolIsolationBackendOptions struct {
	Mode                string
	ContainerConfig     ContainerToolRuntimeConfig
	SecretResolver      SecretResolver
	WASMConfig          WASMToolRuntimeConfig
	WASMExecutorFactory WASMToolExecutorFactory
}

type ToolIsolationBackendFactory func(options ToolIsolationBackendOptions) (ToolRuntime, error)

// ToolIsolationBackendRegistry resolves isolated runtime backends by mode.
// New backends can be registered without editing core switch logic.
type ToolIsolationBackendRegistry struct {
	mu        sync.RWMutex
	factories map[string]ToolIsolationBackendFactory
}

func NewToolIsolationBackendRegistry() *ToolIsolationBackendRegistry {
	return &ToolIsolationBackendRegistry{
		factories: make(map[string]ToolIsolationBackendFactory),
	}
}

func (r *ToolIsolationBackendRegistry) Register(mode string, factory ToolIsolationBackendFactory) error {
	if r == nil {
		return fmt.Errorf("tool isolation backend registry is nil")
	}
	mode = normalizeToolIsolationMode(mode)
	if mode == "" {
		return fmt.Errorf("tool isolation backend mode is required")
	}
	if factory == nil {
		return fmt.Errorf("tool isolation backend factory is required for mode=%s", mode)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[mode] = factory
	return nil
}

func (r *ToolIsolationBackendRegistry) Build(options ToolIsolationBackendOptions) (ToolRuntime, error) {
	if r == nil {
		return nil, fmt.Errorf("tool isolation backend registry is nil")
	}
	mode := normalizeToolIsolationMode(options.Mode)
	r.mu.RLock()
	factory, ok := r.factories[mode]
	modes := make([]string, 0, len(r.factories))
	for name := range r.factories {
		modes = append(modes, name)
	}
	r.mu.RUnlock()
	if !ok {
		sort.Strings(modes)
		return nil, fmt.Errorf("unsupported tool isolation backend %q; expected one of %s", strings.TrimSpace(options.Mode), strings.Join(modes, ", "))
	}
	options.Mode = mode
	return factory(options)
}

func (r *ToolIsolationBackendRegistry) Modes() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.factories))
	for mode := range r.factories {
		out = append(out, mode)
	}
	sort.Strings(out)
	return out
}

var (
	defaultToolIsolationBackendRegistryOnce sync.Once
	defaultToolIsolationBackendRegistry     *ToolIsolationBackendRegistry
)

func DefaultToolIsolationBackendRegistry() *ToolIsolationBackendRegistry {
	defaultToolIsolationBackendRegistryOnce.Do(func() {
		registry := NewToolIsolationBackendRegistry()
		_ = registry.Register("none", func(_ ToolIsolationBackendOptions) (ToolRuntime, error) {
			return nil, nil
		})
		_ = registry.Register("container", func(options ToolIsolationBackendOptions) (ToolRuntime, error) {
			cfg := options.ContainerConfig.normalized()
			resolver := options.SecretResolver
			if resolver == nil {
				resolver = NewEnvSecretResolver("ORLOJ_SECRET_")
			}
			return NewContainerToolRuntimeWithRunnerAndSecrets(nil, cfg, nil, resolver), nil
		})
		_ = registry.Register("wasm", func(options ToolIsolationBackendOptions) (ToolRuntime, error) {
			return NewWASMToolRuntimeWithFactory(nil, options.WASMExecutorFactory, options.WASMConfig), nil
		})
		_ = registry.Register("external", func(options ToolIsolationBackendOptions) (ToolRuntime, error) {
			resolver := options.SecretResolver
			if resolver == nil {
				resolver = NewEnvSecretResolver("ORLOJ_SECRET_")
			}
			return NewExternalToolRuntime(nil, resolver, nil), nil
		})
		_ = registry.Register("grpc", func(options ToolIsolationBackendOptions) (ToolRuntime, error) {
			resolver := options.SecretResolver
			if resolver == nil {
				resolver = NewEnvSecretResolver("ORLOJ_SECRET_")
			}
			return NewGRPCToolRuntime(nil, resolver, nil), nil
		})
		_ = registry.Register("webhook-callback", func(options ToolIsolationBackendOptions) (ToolRuntime, error) {
			resolver := options.SecretResolver
			if resolver == nil {
				resolver = NewEnvSecretResolver("ORLOJ_SECRET_")
			}
			return NewWebhookCallbackToolRuntime(nil, resolver, nil, 0), nil
		})
		defaultToolIsolationBackendRegistry = registry
	})
	return defaultToolIsolationBackendRegistry
}

func RegisterToolIsolationBackend(mode string, factory ToolIsolationBackendFactory) error {
	return DefaultToolIsolationBackendRegistry().Register(mode, factory)
}

func BuildToolIsolationRuntime(options ToolIsolationBackendOptions) (ToolRuntime, error) {
	return DefaultToolIsolationBackendRegistry().Build(options)
}

func normalizeToolIsolationMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		return "none"
	}
	return mode
}
