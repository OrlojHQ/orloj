package crds

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/OrlojHQ/orloj/resources"
)

const (
	AnnotationManagedBy       = "orloj.dev/managed-by"
	AnnotationTargetNamespace = "orloj.dev/target-namespace"
	AnnotationSourceNamespace = "orloj.dev/source-namespace"
	ManagedByCRDSync          = "crd-sync"
	FinalizerSync             = "orloj.dev/sync"
)

func crdMetaToOrloj(meta metav1.ObjectMeta) resources.ObjectMeta {
	annotations := mergeAnnotations(meta.Annotations)
	if meta.Annotations[AnnotationTargetNamespace] != "" {
		annotations[AnnotationSourceNamespace] = meta.Namespace
	}
	return resources.ObjectMeta{
		Name:        meta.Name,
		Namespace:   resolveOrlojNamespace(meta),
		Labels:      meta.Labels,
		Annotations: annotations,
	}
}

// resolveOrlojNamespace returns the Orloj namespace for a CRD object.
// If the orloj.dev/target-namespace annotation is set, it takes precedence
// over the Kubernetes namespace. This allows decoupling K8s governance
// namespaces from Orloj logical namespaces.
func resolveOrlojNamespace(meta metav1.ObjectMeta) string {
	if ns := meta.Annotations[AnnotationTargetNamespace]; ns != "" {
		return ns
	}
	return meta.Namespace
}

func mergeAnnotations(k8sAnnotations map[string]string) map[string]string {
	out := make(map[string]string, len(k8sAnnotations)+1)
	for k, v := range k8sAnnotations {
		out[k] = v
	}
	out[AnnotationManagedBy] = ManagedByCRDSync
	return out
}

func AgentToOrloj(crd *Agent) resources.Agent {
	return resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   crdMetaToOrloj(crd.ObjectMeta),
		Spec:       crd.Spec,
	}
}

func AgentSystemToOrloj(crd *AgentSystem) resources.AgentSystem {
	return resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   crdMetaToOrloj(crd.ObjectMeta),
		Spec:       crd.Spec,
	}
}

func ToolToOrloj(crd *Tool) resources.Tool {
	return resources.Tool{
		APIVersion: "orloj.dev/v1",
		Kind:       "Tool",
		Metadata:   crdMetaToOrloj(crd.ObjectMeta),
		Spec:       crd.Spec,
	}
}

func McpServerToOrloj(crd *McpServer) resources.McpServer {
	return resources.McpServer{
		APIVersion: "orloj.dev/v1",
		Kind:       "McpServer",
		Metadata:   crdMetaToOrloj(crd.ObjectMeta),
		Spec:       crd.Spec,
	}
}

func ModelEndpointToOrloj(crd *ModelEndpoint) resources.ModelEndpoint {
	return resources.ModelEndpoint{
		APIVersion: "orloj.dev/v1",
		Kind:       "ModelEndpoint",
		Metadata:   crdMetaToOrloj(crd.ObjectMeta),
		Spec:       crd.Spec,
	}
}

func MemoryToOrloj(crd *Memory) resources.Memory {
	return resources.Memory{
		APIVersion: "orloj.dev/v1",
		Kind:       "Memory",
		Metadata:   crdMetaToOrloj(crd.ObjectMeta),
		Spec:       crd.Spec,
	}
}

func AgentPolicyToOrloj(crd *AgentPolicy) resources.AgentPolicy {
	return resources.AgentPolicy{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentPolicy",
		Metadata:   crdMetaToOrloj(crd.ObjectMeta),
		Spec:       crd.Spec,
	}
}

func SecretToOrloj(crd *Secret) resources.Secret {
	return resources.Secret{
		APIVersion: "orloj.dev/v1",
		Kind:       "Secret",
		Metadata:   crdMetaToOrloj(crd.ObjectMeta),
		Spec:       crd.Spec,
	}
}

func IsCRDManaged(meta resources.ObjectMeta) bool {
	if meta.Annotations == nil {
		return false
	}
	return meta.Annotations[AnnotationManagedBy] == ManagedByCRDSync
}
