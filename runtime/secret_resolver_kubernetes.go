package agentruntime

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// KubernetesSecretClient abstracts Kubernetes Secret API calls for testing.
type KubernetesSecretClient interface {
	GetSecret(ctx context.Context, namespace, name string) (map[string][]byte, error)
}

type defaultKubernetesSecretClient struct {
	clientset kubernetes.Interface
}

func (c *defaultKubernetesSecretClient) GetSecret(ctx context.Context, namespace, name string) (map[string][]byte, error) {
	secret, err := c.clientset.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return secret.Data, nil
}

// KubernetesSecretResolver resolves secretRef values from Kubernetes Secrets.
type KubernetesSecretResolver struct {
	client     KubernetesSecretClient
	namespace  string
	defaultKey string
}

func NewKubernetesSecretResolver(clientset kubernetes.Interface, defaultKey string) *KubernetesSecretResolver {
	if strings.TrimSpace(defaultKey) == "" {
		defaultKey = "value"
	}
	var client KubernetesSecretClient
	if clientset != nil {
		client = &defaultKubernetesSecretClient{clientset: clientset}
	}
	return NewKubernetesSecretResolverWithClient(client, defaultKey)
}

func NewKubernetesSecretResolverWithClient(client KubernetesSecretClient, defaultKey string) *KubernetesSecretResolver {
	if strings.TrimSpace(defaultKey) == "" {
		defaultKey = "value"
	}
	return &KubernetesSecretResolver{
		client:     client,
		defaultKey: defaultKey,
	}
}

func (r *KubernetesSecretResolver) WithNamespace(namespace string) SecretResolver {
	if r == nil {
		return nil
	}
	copy := *r
	copy.namespace = strings.TrimSpace(namespace)
	return &copy
}

func (r *KubernetesSecretResolver) Resolve(ctx context.Context, secretRef string) (string, error) {
	if r == nil || r.client == nil {
		return "", fmt.Errorf("%w: no kubernetes secret client configured", ErrToolSecretNotFound)
	}

	ns, name, key, err := parseSecretRef(secretRef, r.namespace, r.defaultKey)
	if err != nil {
		return "", err
	}

	data, err := r.client.GetSecret(ctx, ns, name)
	if err != nil {
		return "", fmt.Errorf("%w: kubernetes secret %q in namespace %q: %v", ErrToolSecretNotFound, name, ns, err)
	}
	if data == nil {
		return "", fmt.Errorf("%w: kubernetes secret %q has no data", ErrToolSecretNotFound, name)
	}

	raw, ok := data[key]
	if !ok {
		return "", fmt.Errorf("%w: kubernetes secret %q missing key %q", ErrToolSecretNotFound, name, key)
	}

	value := strings.TrimSpace(string(raw))
	if value == "" {
		return "", fmt.Errorf("kubernetes secret %q key %q resolved empty value", name, key)
	}
	return value, nil
}
