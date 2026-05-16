// Package crds contains CRD types for the orloj.dev API group.
// These thin wrappers embed existing Orloj Spec types with K8s
// metav1.ObjectMeta, enabling controller-gen to produce CRD manifests.
//
// +groupName=orloj.dev
// +versionName=v1
package crds

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	GroupVersion = schema.GroupVersion{Group: "orloj.dev", Version: "v1"}

	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	AddToScheme = SchemeBuilder.AddToScheme
)
