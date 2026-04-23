package controllers

import (
	"context"
	"io"
	"log"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/store"
)

func newSealedSecretControllerHarness(t *testing.T) (*SealedSecretController, *store.SealedSecretStore, *store.SecretStore, *store.SealingKeyStore) {
	t.Helper()

	sealedStore := store.NewSealedSecretStore()
	secretStore := store.NewSecretStore()
	keyStore := store.NewSealingKeyStore()
	material, err := resources.GenerateSealingKeyMaterial()
	if err != nil {
		t.Fatalf("generate sealing key material: %v", err)
	}
	if _, err := keyStore.CreateActive(context.Background(), store.SealingKey{
		KeyID:         material.KeyID,
		PublicKeyPEM:  material.PublicKeyPEM,
		PrivateKeyPEM: material.PrivateKeyPEM,
	}); err != nil {
		t.Fatalf("create active sealing key: %v", err)
	}

	controller := NewSealedSecretController(
		sealedStore,
		secretStore,
		keyStore,
		log.New(io.Discard, "", 0),
		5*time.Millisecond,
		50*time.Millisecond,
	)
	return controller, sealedStore, secretStore, keyStore
}

func TestSealedSecretControllerReconcileCreatesManagedSecret(t *testing.T) {
	ctx := context.Background()
	controller, sealedStore, secretStore, keyStore := newSealedSecretControllerHarness(t)
	active, ok, err := keyStore.GetActive(ctx)
	if err != nil || !ok {
		t.Fatalf("get active sealing key: ok=%v err=%v", ok, err)
	}
	publicKey, _, err := resources.ParseSealingPublicKeyPEM(active.PublicKeyPEM)
	if err != nil {
		t.Fatalf("parse sealing public key: %v", err)
	}

	sealed, err := resources.SealSecret(resources.Secret{
		APIVersion: "orloj.dev/v1",
		Kind:       "Secret",
		Metadata: resources.ObjectMeta{
			Name:      "db-creds",
			Namespace: "default",
		},
		Spec: resources.SecretSpec{
			StringData: map[string]string{
				"username": "alice",
				"password": "s3cr3t",
			},
		},
	}, active.KeyID, publicKey)
	if err != nil {
		t.Fatalf("seal secret: %v", err)
	}

	if _, err := sealedStore.Upsert(ctx, sealed); err != nil {
		t.Fatalf("upsert sealed secret: %v", err)
	}
	if err := controller.reconcileByName(ctx, store.ScopedName("default", "db-creds")); err != nil {
		t.Fatalf("reconcile sealed secret: %v", err)
	}

	generated, ok, err := secretStore.Get(ctx, store.ScopedName("default", "db-creds"))
	if err != nil || !ok {
		t.Fatalf("get generated secret: ok=%v err=%v", ok, err)
	}
	if got := generated.Spec.Data["username"]; got != "YWxpY2U=" {
		t.Fatalf("expected username data to be present, got %q", got)
	}
	if got := generated.Metadata.Annotations[resources.SealedSecretOwnerAnnotation]; got != "default/db-creds" {
		t.Fatalf("expected owner annotation default/db-creds, got %q", got)
	}

	stored, ok, err := sealedStore.Get(ctx, store.ScopedName("default", "db-creds"))
	if err != nil || !ok {
		t.Fatalf("get stored sealed secret: ok=%v err=%v", ok, err)
	}
	if stored.Status.Phase != "Ready" {
		t.Fatalf("expected Ready status, got %q", stored.Status.Phase)
	}
	if stored.Status.LastError != "" {
		t.Fatalf("expected empty lastError, got %q", stored.Status.LastError)
	}
}

func TestSealedSecretControllerConflictFailsClosed(t *testing.T) {
	ctx := context.Background()
	controller, sealedStore, secretStore, keyStore := newSealedSecretControllerHarness(t)
	active, ok, err := keyStore.GetActive(ctx)
	if err != nil || !ok {
		t.Fatalf("get active sealing key: ok=%v err=%v", ok, err)
	}
	publicKey, _, err := resources.ParseSealingPublicKeyPEM(active.PublicKeyPEM)
	if err != nil {
		t.Fatalf("parse sealing public key: %v", err)
	}

	if _, err := secretStore.Upsert(ctx, resources.Secret{
		APIVersion: "orloj.dev/v1",
		Kind:       "Secret",
		Metadata: resources.ObjectMeta{
			Name:      "db-creds",
			Namespace: "default",
		},
		Spec: resources.SecretSpec{
			StringData: map[string]string{"username": "manual"},
		},
	}); err != nil {
		t.Fatalf("upsert unmanaged secret: %v", err)
	}

	sealed, err := resources.SealSecret(resources.Secret{
		APIVersion: "orloj.dev/v1",
		Kind:       "Secret",
		Metadata: resources.ObjectMeta{
			Name:      "db-creds",
			Namespace: "default",
		},
		Spec: resources.SecretSpec{
			StringData: map[string]string{"username": "sealed"},
		},
	}, active.KeyID, publicKey)
	if err != nil {
		t.Fatalf("seal secret: %v", err)
	}
	if _, err := sealedStore.Upsert(ctx, sealed); err != nil {
		t.Fatalf("upsert sealed secret: %v", err)
	}

	if err := controller.reconcileByName(ctx, store.ScopedName("default", "db-creds")); err != nil {
		t.Fatalf("reconcile sealed secret: %v", err)
	}

	existing, ok, err := secretStore.Get(ctx, store.ScopedName("default", "db-creds"))
	if err != nil || !ok {
		t.Fatalf("get existing secret: ok=%v err=%v", ok, err)
	}
	if got := existing.Spec.Data["username"]; got != "bWFudWFs" {
		t.Fatalf("expected unmanaged secret to remain unchanged, got %q", got)
	}

	stored, ok, err := sealedStore.Get(ctx, store.ScopedName("default", "db-creds"))
	if err != nil || !ok {
		t.Fatalf("get stored sealed secret: ok=%v err=%v", ok, err)
	}
	if stored.Status.Phase != "Error" {
		t.Fatalf("expected Error status, got %q", stored.Status.Phase)
	}
}

func TestSealedSecretControllerCleanupDeletesOrphanedManagedSecret(t *testing.T) {
	ctx := context.Background()
	controller, _, secretStore, _ := newSealedSecretControllerHarness(t)

	if _, err := secretStore.Upsert(ctx, resources.Secret{
		APIVersion: "orloj.dev/v1",
		Kind:       "Secret",
		Metadata: resources.ObjectMeta{
			Name:      "orphan",
			Namespace: "default",
			Annotations: map[string]string{
				resources.SealedSecretOwnerAnnotation: "default/missing",
			},
		},
		Spec: resources.SecretSpec{
			StringData: map[string]string{"token": "orphaned"},
		},
	}); err != nil {
		t.Fatalf("upsert orphaned secret: %v", err)
	}

	if err := controller.cleanupOrphans(ctx); err != nil {
		t.Fatalf("cleanup orphans: %v", err)
	}

	if _, ok, err := secretStore.Get(ctx, store.ScopedName("default", "orphan")); err != nil {
		t.Fatalf("get orphaned secret after cleanup: %v", err)
	} else if ok {
		t.Fatal("expected orphaned managed secret to be deleted")
	}
}
