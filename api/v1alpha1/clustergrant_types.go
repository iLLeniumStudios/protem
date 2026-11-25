/*
Copyright 2026 The Protem Authors.

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
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterGrantSpec defines the per-cluster elevation lease dispatched to a spoke cluster's namespace.
type ClusterGrantSpec struct {
	// GrantRef references the parent AccessGrant in its origin namespace
	GrantRef string `json:"grantRef"`

	// ProfileRef is the AccessProfile name
	ProfileRef string `json:"profileRef"`

	// Requester is the snapshot of the user identity
	Requester RequesterInfo `json:"requester"`

	// Role snapshot to bind
	Role TargetRoleRef `json:"role"`

	// Scope indicates Namespaced or Cluster
	Scope ProfileScope `json:"scope"`

	// Namespaces targeted by this grant on this cluster
	// +optional
	Namespaces []string `json:"namespaces,omitempty"`

	// Subject is the resolved username/subject mapped for this cluster
	// +optional
	Subject string `json:"subject,omitempty"`

	// ExpiresAt is the absolute time when access elevation expires
	ExpiresAt metav1.Time `json:"expiresAt"`

	// Revocation details if revoked early
	// +optional
	Revocation *RevocationDetails `json:"revocation,omitempty"`
}

// ClusterGrantPhase defines the lifecycle phase of a ClusterGrant on a spoke cluster.
// +kubebuilder:validation:Enum=Pending;Active;Expired;Revoked;Failed
type ClusterGrantPhase string

const (
	ClusterGrantPhasePending ClusterGrantPhase = "Pending"
	ClusterGrantPhaseActive  ClusterGrantPhase = "Active"
	ClusterGrantPhaseExpired ClusterGrantPhase = "Expired"
	ClusterGrantPhaseRevoked ClusterGrantPhase = "Revoked"
	ClusterGrantPhaseFailed  ClusterGrantPhase = "Failed"
)

// ClusterGrantStatus defines the observed state reported by the spoke agent.
type ClusterGrantStatus struct {
	// Phase is Pending, Active, Expired, Revoked, or Failed
	// +optional
	Phase ClusterGrantPhase `json:"phase,omitempty"`

	// Bindings created on the spoke cluster
	// +optional
	Bindings []ManagedBindingRef `json:"bindings,omitempty"`

	// Message describes error or enforcement status from spoke
	// +optional
	Message string `json:"message,omitempty"`

	// Rules are the rules of the ClusterRole the agent bound, as read on that cluster.
	// Empty for a namespaced Role, whose rules can differ per namespace.
	// +optional
	// +listType=atomic
	Rules []rbacv1.PolicyRule `json:"rules,omitempty"`

	// LastTransitionTime timestamp of last status change
	// +optional
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=cgrant;clgrant
// +kubebuilder:printcolumn:name="Profile",type=string,JSONPath=`.spec.profileRef`
// +kubebuilder:printcolumn:name="Requester",type=string,JSONPath=`.spec.requester.username`
// +kubebuilder:printcolumn:name="Role",type=string,JSONPath=`.spec.role.name`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Expires At",type=string,JSONPath=`.spec.expiresAt`

// ClusterGrant represents a single-cluster slice of an AccessGrant, living in protem-cluster-<name>.
type ClusterGrant struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterGrantSpec   `json:"spec,omitempty"`
	Status ClusterGrantStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterGrantList contains a list of ClusterGrant.
type ClusterGrantList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterGrant `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterGrant{}, &ClusterGrantList{})
}
