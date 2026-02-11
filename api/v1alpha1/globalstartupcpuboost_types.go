// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ManagedByGlobalBoostLabel is the label key applied to child StartupCPUBoost
	// resources to indicate they are managed by a GlobalStartupCPUBoost.
	// The label value is the name of the GlobalStartupCPUBoost.
	ManagedByGlobalBoostLabel = "autoscaling.x-k8s.io/managed-by-global"

	// GlobalBoostChildPrefix is the naming prefix for child StartupCPUBoost
	// resources created by a GlobalStartupCPUBoost controller.
	GlobalBoostChildPrefix = "global-"

	// GlobalBoostFinalizer is the finalizer added to GlobalStartupCPUBoost
	// resources to ensure child cleanup on deletion.
	GlobalBoostFinalizer = "autoscaling.x-k8s.io/global-boost-cleanup"
)

// GlobalStartupCPUBoostTemplate defines the template for child StartupCPUBoost
// resources created in each target namespace.
type GlobalStartupCPUBoostTemplate struct {
	// Selector defines the pod label selector for the child StartupCPUBoost
	// +kubebuilder:validation:Required
	Selector metav1.LabelSelector `json:"selector,omitempty"`
	// Spec defines the boost specification for the child StartupCPUBoost
	// +kubebuilder:validation:Required
	Spec StartupCPUBoostSpec `json:"spec,omitempty"`
}

// GlobalStartupCPUBoostSpec defines the desired state of GlobalStartupCPUBoost
type GlobalStartupCPUBoostSpec struct {
	// NamespaceSelector defines the label selector for target namespaces.
	// Child StartupCPUBoost resources will be created in all matching namespaces.
	// +kubebuilder:validation:Required
	NamespaceSelector metav1.LabelSelector `json:"namespaceSelector,omitempty"`
	// Template defines the StartupCPUBoost template to create in each target namespace.
	// +kubebuilder:validation:Required
	Template GlobalStartupCPUBoostTemplate `json:"template,omitempty"`
}

// GlobalStartupCPUBoostStatus defines the observed state of GlobalStartupCPUBoost
type GlobalStartupCPUBoostStatus struct {
	// NamespaceCount is the number of namespaces where child boosts are active.
	// +kubebuilder:validation:Optional
	NamespaceCount int32 `json:"namespaceCount,omitempty"`
	// ActiveNamespaces is the list of namespaces where child StartupCPUBoost
	// resources have been created.
	// +kubebuilder:validation:Optional
	ActiveNamespaces []string `json:"activeNamespaces,omitempty"`
	// SkippedNamespaces is the list of namespaces that matched the selector but
	// were skipped because they already contain user-created StartupCPUBoost resources.
	// +kubebuilder:validation:Optional
	SkippedNamespaces []string `json:"skippedNamespaces,omitempty"`
	// Conditions hold the latest available observations of the GlobalStartupCPUBoost
	// current state.
	// +optional
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster

// GlobalStartupCPUBoost is the Schema for the globalstartupcpuboosts API.
// It distributes StartupCPUBoost resources across namespaces matching a selector.
type GlobalStartupCPUBoost struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GlobalStartupCPUBoostSpec   `json:"spec,omitempty"`
	Status GlobalStartupCPUBoostStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// GlobalStartupCPUBoostList contains a list of GlobalStartupCPUBoost
type GlobalStartupCPUBoostList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GlobalStartupCPUBoost `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GlobalStartupCPUBoost{}, &GlobalStartupCPUBoostList{})
}
