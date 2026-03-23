package agentruntime

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/OrlojHQ/orloj/resources"
)

// ModelEndpointLookup resolves namespaced ModelEndpoint resources.
type ModelEndpointLookup interface {
	Get(ctx context.Context, name string) (resources.ModelEndpoint, bool, error)
}

// ModelRouterConfig configures model routing between default and referenced endpoints.
type ModelRouterConfig struct {
	Fallback        ModelGateway
	Endpoints       ModelEndpointLookup
	Secrets         SecretResourceLookup
	FallbackAPIKey  string
	SecretEnvPrefix string
}

type cachedModelGateway struct {
	ResourceVersion string
	Gateway         ModelGateway
}

// ModelRouter routes model requests by ModelRequest.ModelRef when provided.
type ModelRouter struct {
	fallback       ModelGateway
	endpoints      ModelEndpointLookup
	secretResolver SecretResolver
	fallbackAPIKey string

	mu    sync.RWMutex
	cache map[string]cachedModelGateway
}

func NewModelRouter(cfg ModelRouterConfig) *ModelRouter {
	fallback := cfg.Fallback
	if fallback == nil {
		fallback = &MockModelGateway{}
	}

	prefix := strings.TrimSpace(cfg.SecretEnvPrefix)
	if prefix == "" {
		prefix = "ORLOJ_SECRET_"
	}
	storeResolver := NewStoreSecretResolver(cfg.Secrets, "value")
	envResolver := NewEnvSecretResolver(prefix)
	resolver := NewChainSecretResolver(storeResolver, envResolver)

	return &ModelRouter{
		fallback:       fallback,
		endpoints:      cfg.Endpoints,
		secretResolver: resolver,
		fallbackAPIKey: strings.TrimSpace(cfg.FallbackAPIKey),
		cache:          make(map[string]cachedModelGateway),
	}
}

func (r *ModelRouter) Complete(ctx context.Context, req ModelRequest) (ModelResponse, error) {
	if r == nil {
		return (&MockModelGateway{}).Complete(ctx, req)
	}
	modelRef := strings.TrimSpace(req.ModelRef)
	if modelRef == "" || r.endpoints == nil {
		return r.fallback.Complete(ctx, req)
	}

	endpoint, endpointKey, ok, err := r.resolveEndpoint(ctx, req.Namespace, modelRef)
	if err != nil {
		return ModelResponse{}, fmt.Errorf("model endpoint %q lookup failed: %w", modelRef, err)
	}
	if !ok {
		return ModelResponse{}, fmt.Errorf("model endpoint %q not found in namespace %q", modelRef, resources.NormalizeNamespace(req.Namespace))
	}
	gateway, err := r.gatewayForEndpoint(ctx, endpoint, endpointKey)
	if err != nil {
		return ModelResponse{}, err
	}

	routedReq := req
	if strings.TrimSpace(routedReq.Model) == "" {
		routedReq.Model = strings.TrimSpace(endpoint.Spec.DefaultModel)
	}
	return gateway.Complete(ctx, routedReq)
}

func (r *ModelRouter) resolveEndpoint(ctx context.Context, namespace string, modelRef string) (resources.ModelEndpoint, string, bool, error) {
	lookupNamespace, lookupName := parseModelEndpointRef(namespace, modelRef)
	lookupKey := scopedName(lookupNamespace, lookupName)
	endpoint, ok, err := r.endpoints.Get(ctx, lookupKey)
	if err != nil {
		return resources.ModelEndpoint{}, lookupKey, false, err
	}
	return endpoint, lookupKey, ok, nil
}

func (r *ModelRouter) gatewayForEndpoint(ctx context.Context, endpoint resources.ModelEndpoint, endpointKey string) (ModelGateway, error) {
	r.mu.RLock()
	cached, ok := r.cache[endpointKey]
	r.mu.RUnlock()
	if ok && cached.Gateway != nil && strings.TrimSpace(cached.ResourceVersion) == strings.TrimSpace(endpoint.Metadata.ResourceVersion) {
		return cached.Gateway, nil
	}

	provider := strings.ToLower(strings.TrimSpace(endpoint.Spec.Provider))
	if provider == "" {
		provider = "openai"
	}
	registry := DefaultModelProviderRegistry()
	plugin, ok := registry.Lookup(provider)
	if !ok {
		return nil, fmt.Errorf("unsupported model endpoint provider %q for %s", endpoint.Spec.Provider, endpointKey)
	}
	apiKey := ""
	if plugin.RequiresAPIKey() {
		var err error
		apiKey, err = r.resolveEndpointAPIKey(ctx, endpoint)
		if err != nil {
			return nil, err
		}
	}

	cfg := DefaultModelGatewayConfig()
	cfg.Provider = provider
	cfg.APIKey = apiKey
	cfg.BaseURL = strings.TrimSpace(endpoint.Spec.BaseURL)
	cfg.DefaultModel = strings.TrimSpace(endpoint.Spec.DefaultModel)
	cfg.Options = endpoint.Spec.Options

	gateway, err := newModelGatewayFromConfigWithRegistry(cfg, registry)
	if err != nil {
		return nil, fmt.Errorf("configure model endpoint %s failed: %w", endpointKey, err)
	}

	r.mu.Lock()
	r.cache[endpointKey] = cachedModelGateway{
		ResourceVersion: strings.TrimSpace(endpoint.Metadata.ResourceVersion),
		Gateway:         gateway,
	}
	r.mu.Unlock()
	return gateway, nil
}

func (r *ModelRouter) resolveEndpointAPIKey(ctx context.Context, endpoint resources.ModelEndpoint) (string, error) {
	secretRef := strings.TrimSpace(endpoint.Spec.Auth.SecretRef)
	if secretRef == "" {
		if strings.TrimSpace(r.fallbackAPIKey) == "" {
			return "", fmt.Errorf("model endpoint %q requires auth.secretRef or fallback API key", endpoint.Metadata.Name)
		}
		return strings.TrimSpace(r.fallbackAPIKey), nil
	}
	resolver := r.secretResolver
	if aware, ok := resolver.(namespaceAwareSecretResolver); ok {
		resolver = aware.WithNamespace(endpoint.Metadata.Namespace)
	}
	if resolver == nil {
		return "", fmt.Errorf("model endpoint %q has auth.secretRef but no resolver is configured", endpoint.Metadata.Name)
	}
	value, err := resolver.Resolve(ctx, secretRef)
	if err != nil {
		return "", fmt.Errorf("resolve model endpoint secret failed endpoint=%s secretRef=%s: %w", endpoint.Metadata.Name, secretRef, err)
	}
	return value, nil
}

func parseModelEndpointRef(namespace string, ref string) (string, string) {
	ref = strings.TrimSpace(ref)
	namespace = resources.NormalizeNamespace(namespace)
	if strings.Contains(ref, "/") {
		parts := strings.SplitN(ref, "/", 2)
		ns := resources.NormalizeNamespace(strings.TrimSpace(parts[0]))
		name := strings.TrimSpace(parts[1])
		return ns, name
	}
	return namespace, ref
}

func scopedName(namespace, name string) string {
	return resources.NormalizeNamespace(namespace) + "/" + strings.TrimSpace(name)
}
