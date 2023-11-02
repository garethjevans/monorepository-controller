/*
Copyright 2023 VMware Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"github.com/fluxcd/pkg/apis/meta"
	"github.com/vmware-labs/reconciler-runtime/apis"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MonoRepositorySpec defines the structure of the mono repository.
type MonoRepositorySpec struct {
	// GitRepository the spec of the git repository to search for changes
	// +required
	GitRepository GitRepositorySpec `json:"gitRepository"`

	// SubPath the subPath in the repository to examine for changes
	// +kubebuilder:validation:Type=string
	// +optional
	SubPath string `json:"subPath"`

	// Interval at which to check the MonoRepository for updates.
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Pattern="^([0-9]+(\\.[0-9]+)?(ms|s|m|h))+$"
	// +required
	Interval metav1.Duration `json:"interval"`
}

// GitRepositorySpec defines the structure of a git repository.
type GitRepositorySpec struct {
	// +required
	URL string `json:"url"`

	// +optional
	Kind string `json:"kind"`

	// +required
	Branch string `json:"branch"`
}

// MonoRepositoryStatus defines the observed state of MonoRepository.
type MonoRepositoryStatus struct {
	apis.Status `json:",inline"`

	// +optional
	SHA string `json:"sha,omitempty"`

	// Artifact represents the last successful GitRepository reconciliation.
	// +optional
	Artifact *Artifact `json:"artifact,omitempty"`

	meta.ReconcileRequestStatus `json:",inline"`
}

// Artifact represents the output of a Source reconciliation.
type Artifact struct {
	// URL is the HTTP address of the Artifact as exposed by the controller
	// managing the Source. It can be used to retrieve the Artifact for
	// consumption, e.g. by another controller applying the Artifact contents.
	// +required
	URL string `json:"url"`

	// Revision is a human-readable identifier traceable in the origin source
	// system. It can be a Git commit SHA, Git tag, a Helm chart version, etc.
	// +optional
	Revision string `json:"revision"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:shortName=monorepo
//+kubebuilder:printcolumn:name="URL",type="string",JSONPath=`.spec.gitRepository.url`
//+kubebuilder:printcolumn:name="Kind",type="string",JSONPath=`.spec.gitRepository.kind`
//+kubebuilder:printcolumn:name="Branch",type="string",JSONPath=`.spec.gitRepository.branch`
//+kubebuilder:printcolumn:name="SubPath",type="string",JSONPath=`.spec.subPath`
//+kubebuilder:printcolumn:name="SHA",type="string",JSONPath=`.status.sha`
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
//+kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].status"

// MonoRepository is the Schema for the mono repository API.
type MonoRepository struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MonoRepositorySpec   `json:"spec,omitempty"`
	Status MonoRepositoryStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// MonoRepositoryList contains a list of MonoRepository.
type MonoRepositoryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MonoRepository `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MonoRepository{}, &MonoRepositoryList{})
}
