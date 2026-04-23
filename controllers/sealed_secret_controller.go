package controllers

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/store"
)

type SealedSecretController struct {
	store          *store.SealedSecretStore
	secrets        *store.SecretStore
	sealingKeys    *store.SealingKeyStore
	reconcileEvery time.Duration
	orphanSweep    time.Duration
	logger         *log.Logger
}

func NewSealedSecretController(
	store *store.SealedSecretStore,
	secrets *store.SecretStore,
	sealingKeys *store.SealingKeyStore,
	logger *log.Logger,
	reconcileEvery time.Duration,
	orphanSweep time.Duration,
) *SealedSecretController {
	if reconcileEvery <= 0 {
		reconcileEvery = 5 * time.Second
	}
	if orphanSweep <= 0 {
		orphanSweep = 60 * time.Second
	}
	return &SealedSecretController{
		store:          store,
		secrets:        secrets,
		sealingKeys:    sealingKeys,
		reconcileEvery: reconcileEvery,
		orphanSweep:    orphanSweep,
		logger:         logger,
	}
}

func (c *SealedSecretController) Start(ctx context.Context) {
	queue := newKeyQueue(1024)
	go c.runWorker(ctx, queue)

	reconcileTicker := time.NewTicker(c.reconcileEvery)
	defer reconcileTicker.Stop()
	orphanTicker := time.NewTicker(c.orphanSweep)
	defer orphanTicker.Stop()

	for {
		c.enqueueAll(ctx, queue)
		select {
		case <-ctx.Done():
			return
		case <-reconcileTicker.C:
		case <-orphanTicker.C:
			if err := c.cleanupOrphans(ctx); err != nil && c.logger != nil {
				c.logger.Printf("sealedsecret controller orphan cleanup error: %v", err)
			}
		}
	}
}

func (c *SealedSecretController) runWorker(ctx context.Context, queue *keyQueue) {
	for {
		key, ok := queue.Pop(ctx)
		if !ok {
			return
		}
		if err := c.reconcileByName(ctx, key); err != nil && c.logger != nil {
			c.logger.Printf("sealedsecret controller reconcile error: %v", err)
		}
		queue.Done(key)
	}
}

func (c *SealedSecretController) enqueueAll(ctx context.Context, queue *keyQueue) {
	if c == nil || c.store == nil {
		return
	}
	items, err := c.store.List(ctx)
	if err != nil {
		return
	}
	for _, item := range items {
		queue.Enqueue(store.ScopedName(item.Metadata.Namespace, item.Metadata.Name))
	}
}

func (c *SealedSecretController) cleanupOrphans(ctx context.Context) error {
	if c == nil || c.secrets == nil || c.store == nil {
		return nil
	}
	secrets, err := c.secrets.List(ctx)
	if err != nil {
		return err
	}
	for _, secret := range secrets {
		owner := strings.TrimSpace(secret.Metadata.Annotations[resources.SealedSecretOwnerAnnotation])
		if owner == "" {
			continue
		}
		_, ok, err := c.store.Get(ctx, owner)
		if err != nil {
			return err
		}
		if ok {
			continue
		}
		if err := c.secrets.Delete(ctx, store.ScopedName(secret.Metadata.Namespace, secret.Metadata.Name)); err != nil && c.logger != nil {
			c.logger.Printf("sealedsecret controller orphan delete failed secret=%s/%s: %v", secret.Metadata.Namespace, secret.Metadata.Name, err)
		}
	}
	return nil
}

func (c *SealedSecretController) reconcileByName(ctx context.Context, name string) error {
	if c == nil || c.store == nil || c.secrets == nil || c.sealingKeys == nil {
		return nil
	}
	item, ok, err := c.store.Get(ctx, name)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	active, ok, err := c.sealingKeys.GetActive(ctx)
	if err != nil {
		return c.markError(ctx, item, "sealing key unavailable: "+err.Error())
	}
	if !ok {
		return c.markError(ctx, item, "sealing key unavailable: no active sealing key is configured")
	}
	privateKey, keyID, err := resources.ParseSealingPrivateKeyPEM(active.PrivateKeyPEM)
	if err != nil {
		return c.markError(ctx, item, "sealing key unavailable: "+err.Error())
	}

	secret, err := resources.UnsealSealedSecret(item, keyID, privateKey)
	if err != nil {
		return c.markError(ctx, item, err.Error())
	}
	secret.Metadata.Annotations = mergeStringMaps(secret.Metadata.Annotations, item.Spec.Template.Annotations)
	if secret.Metadata.Annotations == nil {
		secret.Metadata.Annotations = make(map[string]string)
	}
	secret.Metadata.Annotations[resources.SealedSecretOwnerAnnotation] = store.ScopedName(item.Metadata.Namespace, item.Metadata.Name)
	secret.Metadata.Labels = mergeStringMaps(secret.Metadata.Labels, item.Spec.Template.Labels)

	existing, exists, err := c.secrets.Get(ctx, store.ScopedName(secret.Metadata.Namespace, secret.Metadata.Name))
	if err != nil {
		return err
	}
	if exists {
		owner := strings.TrimSpace(existing.Metadata.Annotations[resources.SealedSecretOwnerAnnotation])
		expectedOwner := store.ScopedName(item.Metadata.Namespace, item.Metadata.Name)
		if owner == "" || owner != expectedOwner {
			return c.markError(ctx, item, fmt.Sprintf("target secret %q already exists and is not managed by this SealedSecret", secret.Metadata.Name))
		}
		secret.Metadata.ResourceVersion = existing.Metadata.ResourceVersion
		secret.Metadata.Generation = existing.Metadata.Generation
		secret.Metadata.CreatedAt = existing.Metadata.CreatedAt
	}

	if _, err := c.secrets.Upsert(ctx, secret); err != nil {
		return c.markError(ctx, item, err.Error())
	}
	item.Status.Phase = "Ready"
	item.Status.LastError = ""
	item.Status.ObservedGeneration = item.Metadata.Generation
	_, err = c.store.Upsert(ctx, item)
	return err
}

func (c *SealedSecretController) markError(ctx context.Context, item resources.SealedSecret, msg string) error {
	item.Status.Phase = "Error"
	item.Status.LastError = strings.TrimSpace(msg)
	item.Status.ObservedGeneration = item.Metadata.Generation
	_, err := c.store.Upsert(ctx, item)
	return err
}

func mergeStringMaps(left, right map[string]string) map[string]string {
	if len(left) == 0 && len(right) == 0 {
		return nil
	}
	out := make(map[string]string, len(left)+len(right))
	for key, value := range left {
		out[key] = value
	}
	for key, value := range right {
		out[key] = value
	}
	return out
}
