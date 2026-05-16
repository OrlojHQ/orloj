package crds

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/OrlojHQ/orloj/resources"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:shortName=osec
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type Secret struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              resources.SecretSpec `json:"spec,omitempty"`
	Status            CRDStatus            `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type SecretList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Secret `json:"items"`
}

func init() { SchemeBuilder.Register(&Secret{}, &SecretList{}) }
