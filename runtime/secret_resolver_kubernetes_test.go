package agentruntime

import (
	"context"
	"errors"
	"testing"
)

type fakeKubernetesSecretClient struct {
	data map[string]map[string][]byte
	err  error
}

func (f *fakeKubernetesSecretClient) GetSecret(_ context.Context, namespace, name string) (map[string][]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	key := namespace + "/" + name
	data, ok := f.data[key]
	if !ok {
		return nil, errors.New("secret not found")
	}
	return data, nil
}

func TestKubernetesSecretResolverSuccess(t *testing.T) {
	client := &fakeKubernetesSecretClient{
		data: map[string]map[string][]byte{
			"default/my-secret": {
				"value": []byte("supersecret"),
			},
		},
	}

	resolver := NewKubernetesSecretResolverWithClient(client, "value")
	resolver = resolver.WithNamespace("default").(*KubernetesSecretResolver)

	val, err := resolver.Resolve(context.Background(), "my-secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "supersecret" {
		t.Fatalf("expected 'supersecret', got %q", val)
	}
}

func TestKubernetesSecretResolverCustomKey(t *testing.T) {
	client := &fakeKubernetesSecretClient{
		data: map[string]map[string][]byte{
			"default/my-secret": {
				"api-key": []byte("key123"),
			},
		},
	}

	resolver := NewKubernetesSecretResolverWithClient(client, "value")
	resolver = resolver.WithNamespace("default").(*KubernetesSecretResolver)

	val, err := resolver.Resolve(context.Background(), "my-secret:api-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "key123" {
		t.Fatalf("expected 'key123', got %q", val)
	}
}

func TestKubernetesSecretResolverNamespaced(t *testing.T) {
	client := &fakeKubernetesSecretClient{
		data: map[string]map[string][]byte{
			"prod/db-creds": {
				"password": []byte("dbpass"),
			},
		},
	}

	resolver := NewKubernetesSecretResolverWithClient(client, "value")

	val, err := resolver.Resolve(context.Background(), "prod/db-creds:password")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "dbpass" {
		t.Fatalf("expected 'dbpass', got %q", val)
	}
}

func TestKubernetesSecretResolverNotFound(t *testing.T) {
	client := &fakeKubernetesSecretClient{
		data: map[string]map[string][]byte{},
	}

	resolver := NewKubernetesSecretResolverWithClient(client, "value")
	resolver = resolver.WithNamespace("default").(*KubernetesSecretResolver)

	_, err := resolver.Resolve(context.Background(), "missing-secret")
	if err == nil {
		t.Fatal("expected error for missing secret")
	}
	if !errors.Is(err, ErrToolSecretNotFound) {
		t.Fatalf("expected ErrToolSecretNotFound, got %v", err)
	}
}

func TestKubernetesSecretResolverMissingKey(t *testing.T) {
	client := &fakeKubernetesSecretClient{
		data: map[string]map[string][]byte{
			"default/my-secret": {
				"other-key": []byte("val"),
			},
		},
	}

	resolver := NewKubernetesSecretResolverWithClient(client, "value")
	resolver = resolver.WithNamespace("default").(*KubernetesSecretResolver)

	_, err := resolver.Resolve(context.Background(), "my-secret:missing-key")
	if err == nil {
		t.Fatal("expected error for missing key")
	}
	if !errors.Is(err, ErrToolSecretNotFound) {
		t.Fatalf("expected ErrToolSecretNotFound, got %v", err)
	}
}

func TestKubernetesSecretResolverNilClient(t *testing.T) {
	resolver := NewKubernetesSecretResolverWithClient(nil, "value")

	_, err := resolver.Resolve(context.Background(), "my-secret")
	if err == nil {
		t.Fatal("expected error for nil client")
	}
	if !errors.Is(err, ErrToolSecretNotFound) {
		t.Fatalf("expected ErrToolSecretNotFound, got %v", err)
	}
}

func TestKubernetesSecretResolverEmptyValue(t *testing.T) {
	client := &fakeKubernetesSecretClient{
		data: map[string]map[string][]byte{
			"default/my-secret": {
				"value": []byte("   "),
			},
		},
	}

	resolver := NewKubernetesSecretResolverWithClient(client, "value")
	resolver = resolver.WithNamespace("default").(*KubernetesSecretResolver)

	_, err := resolver.Resolve(context.Background(), "my-secret")
	if err == nil {
		t.Fatal("expected error for empty value")
	}
}

func TestKubernetesSecretResolverWithNamespace(t *testing.T) {
	client := &fakeKubernetesSecretClient{
		data: map[string]map[string][]byte{
			"custom-ns/my-secret": {
				"value": []byte("namespaced-val"),
			},
		},
	}

	resolver := NewKubernetesSecretResolverWithClient(client, "value")
	scoped := resolver.WithNamespace("custom-ns").(*KubernetesSecretResolver)

	val, err := scoped.Resolve(context.Background(), "my-secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "namespaced-val" {
		t.Fatalf("expected 'namespaced-val', got %q", val)
	}
}
