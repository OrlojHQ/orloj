package crds

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/OrlojHQ/orloj/resources"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:shortName=omcp
// +kubebuilder:printcolumn:name="Transport",type=string,JSONPath=`.spec.transport`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type McpServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              resources.McpServerSpec `json:"spec,omitempty"`
	Status            CRDStatus               `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type McpServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []McpServer `json:"items"`
}

func init() { SchemeBuilder.Register(&McpServer{}, &McpServerList{}) }
