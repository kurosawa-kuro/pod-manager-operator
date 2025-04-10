/*
Copyright 2025.

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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// PodManagerSpec defines the desired state of PodManager
type PodManagerSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Replicas is the number of pods to run
	Replicas int32 `json:"replicas,omitempty"`

	// RestartPolicy defines the restart policy for pods
	// +kubebuilder:validation:Enum=Always;OnFailure;Never
	RestartPolicy string `json:"restartPolicy,omitempty"`
}

// PodManagerStatus defines the observed state of PodManager
type PodManagerStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// AvailableReplicas represents the number of available pods
	AvailableReplicas int32 `json:"availableReplicas,omitempty"`

	// Status represents the current status of the PodManager
	Status string `json:"status,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// PodManager is the Schema for the podmanagers API
type PodManager struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PodManagerSpec   `json:"spec,omitempty"`
	Status PodManagerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PodManagerList contains a list of PodManager
type PodManagerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PodManager `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PodManager{}, &PodManagerList{})
}
