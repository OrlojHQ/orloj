package agentruntime

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/OrlojHQ/orloj/resources"
)

var ErrToolSecretNotFound = errors.New("tool secret not found")

type SecretResourceLookup interface {
	Get(name string) (resources.Secret, bool, error)
}

type namespaceAwareSecretResolver interface {
	WithNamespace(namespace string) SecretResolver
}

type StoreSecretResolver struct {
	lookup     SecretResourceLookup
	namespace  string
	defaultKey string
}

func NewStoreSecretResolver(lookup SecretResourceLookup, defaultKey string) *StoreSecretResolver {
	if strings.TrimSpace(defaultKey) == "" {
		defaultKey = "value"
	}
	return &StoreSecretResolver{
		lookup:     lookup,
		defaultKey: defaultKey,
	}
}

func (r *StoreSecretResolver) WithNamespace(namespace string) SecretResolver {
	if r == nil {
		return nil
	}
	copy := *r
	copy.namespace = strings.TrimSpace(namespace)
	return &copy
}

func (r *StoreSecretResolver) Resolve(_ context.Context, secretRef string) (string, error) {
	if r == nil || r.lookup == nil {
		return "", fmt.Errorf("%w: no secret store configured", ErrToolSecretNotFound)
	}
	ns, name, key, err := parseSecretRef(secretRef, r.namespace, r.defaultKey)
	if err != nil {
		return "", err
	}
	lookupKey := resources.NormalizeNamespace(ns) + "/" + name
	secret, ok, err := r.lookup.Get(lookupKey)
	if err != nil {
		return "", fmt.Errorf("%w: secret %q lookup failed: %v", ErrToolSecretNotFound, name, err)
	}
	if !ok {
		return "", fmt.Errorf("%w: secret %q not found in namespace %q", ErrToolSecretNotFound, name, resources.NormalizeNamespace(ns))
	}
	if secret.Spec.Data == nil {
		return "", fmt.Errorf("%w: secret %q has no data", ErrToolSecretNotFound, name)
	}
	encoded, ok := secret.Spec.Data[key]
	if !ok {
		return "", fmt.Errorf("%w: secret %q missing key %q", ErrToolSecretNotFound, name, key)
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("secret %q key %q contains invalid base64 data: %w", name, key, err)
	}
	value := strings.TrimSpace(string(raw))
	if value == "" {
		return "", fmt.Errorf("secret %q key %q resolved empty value", name, key)
	}
	return value, nil
}

type ChainSecretResolver struct {
	resolvers []SecretResolver
}

func NewChainSecretResolver(resolvers ...SecretResolver) *ChainSecretResolver {
	filtered := make([]SecretResolver, 0, len(resolvers))
	for _, resolver := range resolvers {
		if resolver == nil {
			continue
		}
		filtered = append(filtered, resolver)
	}
	return &ChainSecretResolver{resolvers: filtered}
}

func (r *ChainSecretResolver) WithNamespace(namespace string) SecretResolver {
	if r == nil {
		return nil
	}
	out := make([]SecretResolver, 0, len(r.resolvers))
	for _, resolver := range r.resolvers {
		if aware, ok := resolver.(namespaceAwareSecretResolver); ok {
			out = append(out, aware.WithNamespace(namespace))
			continue
		}
		out = append(out, resolver)
	}
	return &ChainSecretResolver{resolvers: out}
}

func (r *ChainSecretResolver) Resolve(ctx context.Context, secretRef string) (string, error) {
	if r == nil || len(r.resolvers) == 0 {
		return "", fmt.Errorf("%w: no resolvers configured", ErrToolSecretNotFound)
	}
	var notFoundErr error
	for _, resolver := range r.resolvers {
		value, err := resolver.Resolve(ctx, secretRef)
		if err == nil {
			return value, nil
		}
		if errors.Is(err, ErrToolSecretNotFound) {
			notFoundErr = err
			continue
		}
		return "", err
	}
	if notFoundErr != nil {
		return "", notFoundErr
	}
	return "", fmt.Errorf("%w: %s", ErrToolSecretNotFound, strings.TrimSpace(secretRef))
}

func parseSecretRef(secretRef, defaultNamespace, defaultKey string) (namespace string, name string, key string, err error) {
	secretRef = strings.TrimSpace(secretRef)
	if secretRef == "" {
		return "", "", "", fmt.Errorf("secretRef is required")
	}
	if strings.TrimSpace(defaultKey) == "" {
		defaultKey = "value"
	}
	key = defaultKey
	namePart := secretRef
	if idx := strings.Index(secretRef, ":"); idx >= 0 {
		namePart = strings.TrimSpace(secretRef[:idx])
		k := strings.TrimSpace(secretRef[idx+1:])
		if k != "" {
			key = k
		}
	}
	if strings.Contains(namePart, "/") {
		parts := strings.SplitN(namePart, "/", 2)
		namespace = strings.TrimSpace(parts[0])
		name = strings.TrimSpace(parts[1])
	} else {
		namespace = strings.TrimSpace(defaultNamespace)
		name = strings.TrimSpace(namePart)
	}
	if name == "" {
		return "", "", "", fmt.Errorf("secretRef %q has empty secret name", secretRef)
	}
	if strings.TrimSpace(namespace) == "" {
		namespace = resources.DefaultNamespace
	}
	return namespace, name, key, nil
}
