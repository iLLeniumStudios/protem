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

// RequestPhase defines the lifecycle state of an AccessRequest.
// +kubebuilder:validation:Enum=Pending;Approved;Rejected;Cancelled;TimedOut
type RequestPhase string

const (
	RequestPhasePending   RequestPhase = "Pending"
	RequestPhaseApproved  RequestPhase = "Approved"
	RequestPhaseRejected  RequestPhase = "Rejected"
	RequestPhaseCancelled RequestPhase = "Cancelled"
	RequestPhaseTimedOut  RequestPhase = "TimedOut"
)

// RequesterInfo contains authenticated identity info of the requester.
type RequesterInfo struct {
	Username string `json:"username"`
	// +optional
	Email  string   `json:"email,omitempty"`
	Groups []string `json:"groups,omitempty"`
}

// RequestJustification records the business justification and ticket reference.
type RequestJustification struct {
	// Reason explains why access elevation is necessary
	// +optional
	Reason string `json:"reason,omitempty"`

	// Ticket is an incident or change ticket ID (e.g. INC-1234)
	// +optional
	Ticket string `json:"ticket,omitempty"`
}

// AccessRequestSpec defines the requested elevation.
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="AccessRequest spec is immutable after creation"
type AccessRequestSpec struct {
	// ProfileRef is the name of the AccessProfile in the same namespace
	ProfileRef string `json:"profileRef"`

	// Requester is stamped from request.userInfo by admission/operator
	Requester RequesterInfo `json:"requester"`

	// Namespaces requests a subset of the profile's namespaces
	// +optional
	Namespaces []string `json:"namespaces,omitempty"`

	// Clusters requests a subset of the profile's matched clusters
	// +optional
	Clusters []string `json:"clusters,omitempty"`

	// Duration requested (optional, defaults to profile default)
	// +optional
	Duration *metav1.Duration `json:"duration,omitempty"`

	// Justification for the requested access
	// +optional
	Justification RequestJustification `json:"justification,omitempty"`
}

// ClusterApprovalRequirement records the required approvals per target cluster.
type ClusterApprovalRequirement struct {
	ClusterName       string       `json:"clusterName"`
	RuleName          string       `json:"ruleName"`
	Mode              ApprovalMode `json:"mode"`
	RequiredApprovals int          `json:"requiredApprovals"`
	// +optional
	ApproverGroups []string `json:"approverGroups,omitempty"`
	// +optional
	ApproverUsers []string `json:"approverUsers,omitempty"`
	// +optional
	AllowSelfApproval bool `json:"allowSelfApproval,omitempty"`
}

// ResolvedScope contains the frozen target scope evaluated at creation.
type ResolvedScope struct {
	// Role frozen at request creation; the grant binds this role, not the profile's current one
	// +optional
	Role *TargetRoleRef `json:"role,omitempty"`

	// Scope frozen at request creation
	// +optional
	Scope ProfileScope `json:"scope,omitempty"`

	// Clusters frozen at request creation
	Clusters []string `json:"clusters"`

	// Namespaces frozen at request creation
	// +optional
	Namespaces []string `json:"namespaces,omitempty"`

	// Duration determined from request, profile max, and matched rules
	Duration metav1.Duration `json:"duration"`

	// Requirements per cluster computed from approval rules
	// +optional
	Requirements []ClusterApprovalRequirement `json:"requirements,omitempty"`
}

// ApprovalDecisionType defines whether a person approved, rejected, or cancelled a request.
// +kubebuilder:validation:Enum=Approved;Rejected;Cancelled
type ApprovalDecisionType string

const (
	ApprovalDecisionApproved  ApprovalDecisionType = "Approved"
	ApprovalDecisionRejected  ApprovalDecisionType = "Rejected"
	ApprovalDecisionCancelled ApprovalDecisionType = "Cancelled"
)

// ApprovalDecision records an individual approver's action.
type ApprovalDecision struct {
	User     string               `json:"user"`
	Decision ApprovalDecisionType `json:"decision"`
	At       metav1.Time          `json:"at"`
	// +optional
	Comment string `json:"comment,omitempty"`
	// +optional
	Rule string `json:"rule,omitempty"`
}

// AccessRequestStatus defines the observed state of AccessRequest.
type AccessRequestStatus struct {
	// Phase of the request workflow
	// +optional
	Phase RequestPhase `json:"phase,omitempty"`

	// Resolved frozen target clusters, namespaces, and duration
	// +optional
	Resolved *ResolvedScope `json:"resolved,omitempty"`

	// Approvals records decisions made on this request
	// +optional
	Approvals []ApprovalDecision `json:"approvals,omitempty"`

	// GrantRef references the provisioned AccessGrant in the same namespace
	// +optional
	GrantRef string `json:"grantRef,omitempty"`

	// Conditions represent observations of the request lifecycle
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// AuditedEvents counts this request's audit events the operator has already written to its
	// audit log, so each is written once and none is skipped across restarts.
	// +optional
	AuditedEvents int32 `json:"auditedEvents,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=areq
// +kubebuilder:printcolumn:name="Profile",type=string,JSONPath=`.spec.profileRef`
// +kubebuilder:printcolumn:name="Requester",type=string,JSONPath=`.spec.requester.username`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Duration",type=string,JSONPath=`.status.resolved.duration`
// +kubebuilder:printcolumn:name="Grant",type=string,JSONPath=`.status.grantRef`

// AccessRequest represents a user request for temporary privileged access.
type AccessRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AccessRequestSpec   `json:"spec,omitempty"`
	Status AccessRequestStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AccessRequestList contains a list of AccessRequest.
type AccessRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AccessRequest `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AccessRequest{}, &AccessRequestList{})
}
