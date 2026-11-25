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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GrantPhase defines the lifecycle state of an AccessGrant.
// +kubebuilder:validation:Enum=Active;Expired;Revoked
type GrantPhase string

const (
	GrantPhaseActive  GrantPhase = "Active"
	GrantPhaseExpired GrantPhase = "Expired"
	GrantPhaseRevoked GrantPhase = "Revoked"
)

// ManagedBindingRef tracks a RoleBinding or ClusterRoleBinding created by this grant.
type ManagedBindingRef struct {
	Kind string `json:"kind"`
	// +optional
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
}

// RevocationDetails captures revocation metadata when a lease is revoked early.
type RevocationDetails struct {
	// By is the identity that revoked the elevation
	// +optional
	By string `json:"by,omitempty"`

	// At is the timestamp when revocation took effect
	At metav1.Time `json:"at"`

	// Reason explains why elevation was revoked
	// +optional
	Reason string `json:"reason,omitempty"`
}

// ClusterBindingStatePhase defines the phase of a binding on a specific cluster.
// +kubebuilder:validation:Enum=Pending;Active;Revoked;Failed
type ClusterBindingStatePhase string

const (
	ClusterBindingStatePending ClusterBindingStatePhase = "Pending"
	ClusterBindingStateActive  ClusterBindingStatePhase = "Active"
	ClusterBindingStateRevoked ClusterBindingStatePhase = "Revoked"
	ClusterBindingStateFailed  ClusterBindingStatePhase = "Failed"
)

// ClusterBindingState tracks the status of bindings on a specific cluster.
type ClusterBindingState struct {
	ClusterName string                   `json:"clusterName"`
	State       ClusterBindingStatePhase `json:"state"` // Active | Expired | Revoked | Failed
	// +optional
	Bindings []ManagedBindingRef `json:"bindings,omitempty"`
	// +optional
	Message string `json:"message,omitempty"`
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
}

// AccessGrantSpec defines the immutable lease snapshot and revocation state.
// +kubebuilder:validation:XValidation:rule="oldSelf == null || (self.requestRef == oldSelf.requestRef && self.profileRef == oldSelf.profileRef && self.requester == oldSelf.requester && self.role == oldSelf.role && self.scope == oldSelf.scope && (has(self.namespaces) == has(oldSelf.namespaces) && (!has(self.namespaces) || self.namespaces == oldSelf.namespaces)) && (has(self.clusters) == has(oldSelf.clusters) && (!has(self.clusters) || self.clusters == oldSelf.clusters)) && self.expiresAt == oldSelf.expiresAt)",message="AccessGrant core spec is immutable after creation"
type AccessGrantSpec struct {
	// RequestRef is the name of the AccessRequest in the same namespace
	RequestRef string `json:"requestRef"`

	// ProfileRef is the name of the AccessProfile in the same namespace
	ProfileRef string `json:"profileRef"`

	// Requester is the snapshot of the user identity
	Requester RequesterInfo `json:"requester"`

	// Role snapshot to bind
	Role TargetRoleRef `json:"role"`

	// Scope indicates Namespaced or Cluster
	Scope ProfileScope `json:"scope"`

	// Namespaces targeted by this grant
	// +optional
	Namespaces []string `json:"namespaces,omitempty"`

	// Clusters targeted by this grant
	// +optional
	Clusters []string `json:"clusters,omitempty"`

	// ExpiresAt is the absolute time when access elevation expires
	ExpiresAt metav1.Time `json:"expiresAt"`

	// Revocation is set when access is revoked early; presence indicates revoked state
	// +optional
	Revocation *RevocationDetails `json:"revocation,omitempty"`
}

// AccessGrantStatus defines the observed state of AccessGrant.
type AccessGrantStatus struct {
	// Phase is Active, Expired, or Revoked
	// +optional
	Phase GrantPhase `json:"phase,omitempty"`

	// ManagedBindings tracks created bindings on the local cluster
	// +optional
	ManagedBindings []ManagedBindingRef `json:"managedBindings,omitempty"`

	// Clusters tracks per-cluster enforcement state in multi-cluster environments
	// +optional
	Clusters []ClusterBindingState `json:"clusters,omitempty"`

	// Conditions represent observations of the grant state
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=agrant
// +kubebuilder:printcolumn:name="Profile",type=string,JSONPath=`.spec.profileRef`
// +kubebuilder:printcolumn:name="Requester",type=string,JSONPath=`.spec.requester.username`
// +kubebuilder:printcolumn:name="Role",type=string,JSONPath=`.spec.role.name`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Expires At",type=string,JSONPath=`.spec.expiresAt`

// AccessGrant represents an active, time-bound access lease for a user.
type AccessGrant struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AccessGrantSpec   `json:"spec,omitempty"`
	Status AccessGrantStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AccessGrantList contains a list of AccessGrant.
type AccessGrantList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AccessGrant `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AccessGrant{}, &AccessGrantList{})
}
