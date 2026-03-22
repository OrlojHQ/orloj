package agentruntime

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
)

// ModelProviderPlugin builds model gateways for a provider family.
type ModelProviderPlugin interface {
	Name() string
	Aliases() []string
	RequiresAPIKey() bool
	BuildGateway(cfg ModelGatewayConfig) (ModelGateway, error)
}

// ModelProviderRegistry stores model provider plugins by name and alias.
type ModelProviderRegistry struct {
	mu      sync.RWMutex
	plugins map[string]ModelProviderPlugin
}

func NewModelProviderRegistry() *ModelProviderRegistry {
	return &ModelProviderRegistry{
		plugins: make(map[string]ModelProviderPlugin),
	}
}

func (r *ModelProviderRegistry) Register(plugin ModelProviderPlugin) error {
	if r == nil {
		return fmt.Errorf("model provider registry is nil")
	}
	if plugin == nil {
		return fmt.Errorf("model provider plugin is nil")
	}
	keys := make([]string, 0, 1+len(plugin.Aliases()))
	name := strings.ToLower(strings.TrimSpace(plugin.Name()))
	if name == "" {
		return fmt.Errorf("model provider plugin name is required")
	}
	keys = append(keys, name)
	for _, alias := range plugin.Aliases() {
		key := strings.ToLower(strings.TrimSpace(alias))
		if key == "" {
			continue
		}
		keys = append(keys, key)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for _, key := range keys {
		if existing, ok := r.plugins[key]; ok {
			return fmt.Errorf("model provider key %q already registered by %q", key, existing.Name())
		}
	}
	for _, key := range keys {
		r.plugins[key] = plugin
	}
	return nil
}

func (r *ModelProviderRegistry) Lookup(provider string) (ModelProviderPlugin, bool) {
	if r == nil {
		return nil, false
	}
	key := strings.ToLower(strings.TrimSpace(provider))
	if key == "" {
		return nil, false
	}
	r.mu.RLock()
	plugin, ok := r.plugins[key]
	r.mu.RUnlock()
	return plugin, ok
}

var (
	defaultModelProviderRegistry     = NewModelProviderRegistry()
	defaultModelProviderRegistryOnce sync.Once
)

func DefaultModelProviderRegistry() *ModelProviderRegistry {
	defaultModelProviderRegistryOnce.Do(func() {
		mustRegisterBuiltinModelProviders(defaultModelProviderRegistry)
	})
	return defaultModelProviderRegistry
}

// RegisterModelProvider registers a plugin globally for model gateway and router usage.
func RegisterModelProvider(plugin ModelProviderPlugin) error {
	return DefaultModelProviderRegistry().Register(plugin)
}

func mustRegisterBuiltinModelProviders(registry *ModelProviderRegistry) {
	builtins := []ModelProviderPlugin{
		&mockModelProviderPlugin{},
		&openAIModelProviderPlugin{},
		&anthropicModelProviderPlugin{},
		&azureOpenAIModelProviderPlugin{},
		&ollamaModelProviderPlugin{},
	}
	for _, plugin := range builtins {
		if err := registry.Register(plugin); err != nil {
			log.Fatalf("FATAL: register builtin model provider %q failed: %v", plugin.Name(), err)
			os.Exit(1)
		}
	}
}
