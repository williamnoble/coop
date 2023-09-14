/*
Copyright 2023.

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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SourceSpec defines the source type
type SourceSpec struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// DestinationSpec defines the destination Namespace
// TODO: Update this to Namespaces
type DestinationSpec struct {
	Namespace string `json:"namespace,omitempty"`
}

// CoopSpec defines the desired state of Coop
// Coop defines the specification for a Copy Operation from `Source` to
// `Destination` for a given `Type`.
type CoopSpec struct {
	//Type        runtime.Object  `json:"type"`
	Source      SourceSpec      `json:"source"`
	Destination DestinationSpec `json:"destination"`
}

// CoopStatus defines the observed state of Coop
type CoopStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Coop is the Schema for the coops API
type Coop struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CoopSpec   `json:"spec,omitempty"`
	Status CoopStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CoopList contains a list of Coop
type CoopList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Coop `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Coop{}, &CoopList{})
}
