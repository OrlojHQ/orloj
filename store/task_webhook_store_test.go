package store_test

import (
	"context"
	"testing"

	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/store"
)

func TestTaskWebhookGetByEndpointID(t *testing.T) {
	ctx := context.Background()
	s := store.NewTaskWebhookStore()

	hook := resources.TaskWebhook{
		APIVersion: "orloj.dev/v1",
		Kind:       "TaskWebhook",
		Metadata:   resources.ObjectMeta{Name: "build-events", Namespace: "default"},
		Spec: resources.TaskWebhookSpec{
			TaskRef: "weekly-report-template",
			Auth:    resources.TaskWebhookAuthSpec{SecretRef: "build-webhook-secret"},
		},
	}
	upserted, err := s.Upsert(ctx, hook)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if upserted.Status.EndpointID == "" {
		t.Fatal("expected endpoint ID to be assigned on upsert")
	}

	got, ok, err := s.GetByEndpointID(ctx, upserted.Status.EndpointID)
	if err != nil {
		t.Fatalf("GetByEndpointID: %v", err)
	}
	if !ok {
		t.Fatal("expected webhook lookup by endpoint ID to succeed")
	}
	if got.Metadata.Name != "build-events" {
		t.Fatalf("expected build-events, got %q", got.Metadata.Name)
	}

	_, ok, err = s.GetByEndpointID(ctx, "missing-endpoint-id")
	if err != nil {
		t.Fatalf("missing lookup err: %v", err)
	}
	if ok {
		t.Fatal("expected missing endpoint ID to not match")
	}
}
