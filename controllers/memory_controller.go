package controllers

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"strings"
	"time"

	agentruntime "github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/store"
)

// MemoryController reconciles Memory resources.
type MemoryController struct {
	store          *store.MemoryStore
	secrets        *store.SecretStore
	modelEndpoints *store.ModelEndpointStore
	reconcileEvery time.Duration
	logger         *log.Logger
	backends       *agentruntime.PersistentMemoryBackendRegistry
}

func NewMemoryController(memStore *store.MemoryStore, logger *log.Logger, reconcileEvery time.Duration) *MemoryController {
	if reconcileEvery <= 0 {
		reconcileEvery = 5 * time.Second
	}
	return &MemoryController{store: memStore, logger: logger, reconcileEvery: reconcileEvery}
}

// SetBackendRegistry configures the controller to register persistent backends on reconcile.
func (c *MemoryController) SetBackendRegistry(registry *agentruntime.PersistentMemoryBackendRegistry) {
	c.backends = registry
}

// SetSecretStore configures the controller to resolve auth credentials from secrets.
func (c *MemoryController) SetSecretStore(secrets *store.SecretStore) {
	c.secrets = secrets
}

// SetModelEndpointStore configures the controller to resolve embedding_model
// references as ModelEndpoint resources for building embedding providers.
func (c *MemoryController) SetModelEndpointStore(eps *store.ModelEndpointStore) {
	c.modelEndpoints = eps
}

func (c *MemoryController) Start(ctx context.Context) {
	queue := newKeyQueue(1024)
	go c.runWorker(ctx, queue)

	ticker := time.NewTicker(c.reconcileEvery)
	defer ticker.Stop()

	for {
		c.enqueueAll(queue)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (c *MemoryController) runWorker(ctx context.Context, queue *keyQueue) {
	for {
		key, ok := queue.Pop(ctx)
		if !ok {
			return
		}
		if err := c.reconcileByName(key); err != nil && c.logger != nil {
			c.logger.Printf("memory controller reconcile error: %v", err)
		}
		queue.Done(key)
	}
}

func (c *MemoryController) enqueueAll(queue *keyQueue) {
	for _, item := range c.store.List() {
		queue.Enqueue(store.ScopedName(item.Metadata.Namespace, item.Metadata.Name))
	}
}

func (c *MemoryController) ReconcileOnce(_ context.Context) error {
	for _, item := range c.store.List() {
		if err := c.reconcileByName(store.ScopedName(item.Metadata.Namespace, item.Metadata.Name)); err != nil {
			return err
		}
	}
	return nil
}

func (c *MemoryController) reconcileByName(name string) error {
	item, ok := c.store.Get(name)
	if !ok {
		return nil
	}
	if item.Status.Phase == "Ready" && item.Status.ObservedGeneration == item.Metadata.Generation {
		return nil
	}

	provider := strings.ToLower(strings.TrimSpace(item.Spec.Provider))
	if c.backends != nil {
		scopedKey := store.ScopedName(item.Metadata.Namespace, item.Metadata.Name)
		if _, exists := c.backends.Get(scopedKey); !exists {
			cfg := agentruntime.MemoryProviderConfig{
				Type:           item.Spec.Type,
				Provider:       item.Spec.Provider,
				EmbeddingModel: item.Spec.EmbeddingModel,
				Endpoint:       item.Spec.Endpoint,
			}
			if ref := strings.TrimSpace(item.Spec.Auth.SecretRef); ref != "" {
				token, err := c.resolveAuthToken(item.Metadata.Namespace, ref)
				if err != nil {
					item.Status.Phase = "Error"
					item.Status.LastError = "auth secret resolution failed: " + err.Error()
					item.Status.ObservedGeneration = item.Metadata.Generation
					_, upsertErr := c.store.Upsert(item)
					return upsertErr
				}
				cfg.AuthToken = token
			}

			if embRef := strings.TrimSpace(item.Spec.EmbeddingModel); embRef != "" {
				embedder, err := c.resolveEmbeddingProvider(item.Metadata.Namespace, embRef)
				if err != nil {
					item.Status.Phase = "Error"
					item.Status.LastError = "embedding model resolution failed: " + err.Error()
					item.Status.ObservedGeneration = item.Metadata.Generation
					_, upsertErr := c.store.Upsert(item)
					return upsertErr
				}
				cfg.Embedder = embedder
			}

			backend, err := agentruntime.DefaultMemoryProviderRegistry().Create(cfg)
			if err != nil {
				item.Status.Phase = "Error"
				item.Status.LastError = err.Error()
				item.Status.ObservedGeneration = item.Metadata.Generation
				_, upsertErr := c.store.Upsert(item)
				return upsertErr
			}
			if err := backend.Ping(context.Background()); err != nil {
				item.Status.Phase = "Error"
				item.Status.LastError = "backend connectivity failed: " + err.Error()
				item.Status.ObservedGeneration = item.Metadata.Generation
				_, upsertErr := c.store.Upsert(item)
				return upsertErr
			}
			c.backends.Register(scopedKey, backend)
			if c.logger != nil {
				c.logger.Printf("memory controller: registered backend %s (provider=%s)", scopedKey, provider)
			}
		}
	}

	item.Status.Phase = "Ready"
	item.Status.LastError = ""
	item.Status.ObservedGeneration = item.Metadata.Generation
	_, err := c.store.Upsert(item)
	return err
}

// resolveEmbeddingProvider looks up a ModelEndpoint by the given reference and
// builds an EmbeddingProvider from its base_url, auth, and default_model.
func (c *MemoryController) resolveEmbeddingProvider(namespace, embeddingModelRef string) (agentruntime.EmbeddingProvider, error) {
	if c.modelEndpoints == nil {
		return nil, fmt.Errorf("no model endpoint store configured")
	}
	key := embeddingModelRef
	if !strings.Contains(key, "/") {
		key = store.ScopedName(namespace, embeddingModelRef)
	}
	ep, ok := c.modelEndpoints.Get(key)
	if !ok {
		return nil, fmt.Errorf("model endpoint %q not found", embeddingModelRef)
	}

	apiKey := ""
	if ref := strings.TrimSpace(ep.Spec.Auth.SecretRef); ref != "" {
		var err error
		apiKey, err = c.resolveAuthToken(ep.Metadata.Namespace, ref)
		if err != nil {
			return nil, fmt.Errorf("resolve embedding endpoint auth: %w", err)
		}
	}

	baseURL := strings.TrimSpace(ep.Spec.BaseURL)
	model := strings.TrimSpace(ep.Spec.DefaultModel)
	return agentruntime.NewOpenAIEmbeddingProvider(baseURL, apiKey, model), nil
}

// resolveAuthToken looks up a Secret by scoped name and returns the first
// base64-decoded data value as a bearer token.
func (c *MemoryController) resolveAuthToken(namespace, secretRef string) (string, error) {
	if c.secrets == nil {
		return "", fmt.Errorf("no secret store configured")
	}
	key := secretRef
	if !strings.Contains(key, "/") {
		key = store.ScopedName(namespace, secretRef)
	}
	secret, ok := c.secrets.Get(key)
	if !ok {
		return "", fmt.Errorf("secret %q not found", secretRef)
	}
	for _, v := range secret.Spec.Data {
		decoded, err := base64.StdEncoding.DecodeString(v)
		if err != nil {
			return strings.TrimSpace(v), nil
		}
		return strings.TrimSpace(string(decoded)), nil
	}
	return "", fmt.Errorf("secret %q has no data entries", secretRef)
}
