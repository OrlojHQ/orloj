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

// ModelRouterConfig configures model routing via ModelEndpoint resources.
type ModelRouterConfig struct {
	Endpoints       ModelEndpointLookup
	Secrets         SecretResourceLookup
	SecretEnvPrefix string
}

type cachedModelGateway struct {
	ResourceVersion       string
	SecretResourceVersion string
	Gateway               ModelGateway
}

// ModelRouter routes model requests to ModelEndpoint-backed gateways by ModelRequest.ModelRef.
type ModelRouter struct {
	endpoints      ModelEndpointLookup
	secrets        SecretResourceLookup
	secretResolver SecretResolver

	mu    sync.RWMutex
	cache map[string]cachedModelGateway
}

func NewModelRouter(cfg ModelRouterConfig) *ModelRouter {
	prefix := strings.TrimSpace(cfg.SecretEnvPrefix)
	if prefix == "" {
		prefix = "ORLOJ_SECRET_"
	}
	storeResolver := NewStoreSecretResolver(cfg.Secrets, "value")
	envResolver := NewEnvSecretResolver(prefix)
	resolver := NewChainSecretResolver(storeResolver, envResolver)

	return &ModelRouter{
		endpoints:      cfg.Endpoints,
		secrets:        cfg.Secrets,
		secretResolver: resolver,
		cache:          make(map[string]cachedModelGateway),
	}
}

func (r *ModelRouter) Complete(ctx context.Context, req ModelRequest) (ModelResponse, error) {
	if r == nil {
		return ModelResponse{}, fmt.Errorf("model router is not configured")
	}
	modelRef := strings.TrimSpace(req.ModelRef)
	if modelRef == "" {
		return ModelResponse{}, fmt.Errorf("model_ref is required; configure spec.model_ref on the agent")
	}
	if r.endpoints == nil {
		return ModelResponse{}, fmt.Errorf("model endpoint store is not configured")
	}

	refs := make([]string, 0, 1+len(req.FallbackModelRefs))
	refs = append(refs, modelRef)
	for _, ref := range req.FallbackModelRefs {
		if r := strings.TrimSpace(ref); r != "" {
			refs = append(refs, r)
		}
	}

	var lastErr error
	for _, ref := range refs {
		if ctx.Err() != nil {
			if lastErr != nil {
				return ModelResponse{}, lastErr
			}
			return ModelResponse{}, ctx.Err()
		}

		endpoint, endpointKey, ok, err := r.resolveEndpoint(ctx, req.Namespace, ref)
		if err != nil {
			lastErr = fmt.Errorf("model endpoint %q lookup failed: %w", ref, err)
			continue
		}
		if !ok {
			lastErr = fmt.Errorf("model endpoint %q not found in namespace %q", ref, resources.NormalizeNamespace(req.Namespace))
			continue
		}

		gateway, err := r.gatewayForEndpoint(ctx, endpoint, endpointKey)
		if err != nil {
			lastErr = fmt.Errorf("configure model endpoint %s failed: %w", ref, err)
			continue
		}

		routedReq := req
		routedReq.ModelRef = ref
		if strings.TrimSpace(routedReq.Model) == "" {
			routedReq.Model = strings.TrimSpace(endpoint.Spec.DefaultModel)
		}

		resp, err := gateway.Complete(ctx, routedReq)
		if err == nil {
			return resp, nil
		}

		lastErr = err

		if mge, retryable := IsModelGatewayError(err); mge != nil && !retryable {
			return ModelResponse{}, err
		}
	}

	return ModelResponse{}, lastErr
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
	secretRV := r.resolveSecretResourceVersion(ctx, endpoint)

	r.mu.RLock()
	cached, ok := r.cache[endpointKey]
	r.mu.RUnlock()
	epRV := strings.TrimSpace(endpoint.Metadata.ResourceVersion)
	if ok && cached.Gateway != nil && cached.ResourceVersion == epRV && cached.SecretResourceVersion == secretRV {
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
	needsSecret := plugin.RequiresAPIKey() || strings.TrimSpace(endpoint.Spec.Auth.SecretRef) != ""
	if needsSecret {
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
	if endpoint.Spec.AllowPrivate != nil {
		cfg.AllowPrivate = *endpoint.Spec.AllowPrivate
	} else if provider == "ollama" {
		cfg.AllowPrivate = true
	}

	gateway, err := newModelGatewayFromConfigWithRegistry(cfg, registry)
	if err != nil {
		return nil, fmt.Errorf("configure model endpoint %s failed: %w", endpointKey, err)
	}

	r.mu.Lock()
	r.cache[endpointKey] = cachedModelGateway{
		ResourceVersion:       epRV,
		SecretResourceVersion: secretRV,
		Gateway:               gateway,
	}
	r.mu.Unlock()
	return gateway, nil
}

func (r *ModelRouter) resolveEndpointAPIKey(ctx context.Context, endpoint resources.ModelEndpoint) (string, error) {
	secretRef := strings.TrimSpace(endpoint.Spec.Auth.SecretRef)
	if secretRef == "" {
		return "", fmt.Errorf("model endpoint %q requires auth.secretRef", endpoint.Metadata.Name)
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

// resolveSecretResourceVersion returns the ResourceVersion of the secret
// referenced by the endpoint's auth.secretRef, or "" if unavailable. Used to
// detect secret changes so the gateway cache is invalidated when a key is
// rotated without changing the ModelEndpoint itself.
func (r *ModelRouter) resolveSecretResourceVersion(ctx context.Context, endpoint resources.ModelEndpoint) string {
	ref := strings.TrimSpace(endpoint.Spec.Auth.SecretRef)
	if ref == "" || r.secrets == nil {
		return ""
	}
	ns := resources.NormalizeNamespace(endpoint.Metadata.Namespace)
	key := ns + "/" + ref
	secret, ok, err := r.secrets.Get(ctx, key)
	if err != nil || !ok {
		return ""
	}
	return strings.TrimSpace(secret.Metadata.ResourceVersion)
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
