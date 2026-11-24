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

// ProfileScope defines whether access is scoped to specific namespaces or cluster-wide.
// +kubebuilder:validation:Enum=Namespaced;Cluster
type ProfileScope string

const (
	ProfileScopeNamespaced ProfileScope = "Namespaced"
	ProfileScopeCluster    ProfileScope = "Cluster"
)

// ApprovalMode defines how an approval rule evaluates.
// +kubebuilder:validation:Enum=Auto;Manual
type ApprovalMode string

const (
	ApprovalModeAuto   ApprovalMode = "Auto"
	ApprovalModeManual ApprovalMode = "Manual"
)

// JustificationRequirement defines if business justification reason is required.
// +kubebuilder:validation:Enum=Required;Optional
type JustificationRequirement string

const (
	JustificationRequired JustificationRequirement = "Required"
	JustificationOptional JustificationRequirement = "Optional"
)

// TargetRoleRef identifies the target RBAC role to bind.
type TargetRoleRef struct {
	// Kind is either "Role" or "ClusterRole"
	// +kubebuilder:validation:Enum=Role;ClusterRole
	Kind string `json:"kind"`
	// Name is the name of the Role or ClusterRole
	Name string `json:"name"`
}

// ClusterTargeting defines which clusters in the fleet this profile applies to.
type ClusterTargeting struct {
	// Selector dynamically matches target clusters by label
	// +optional
	Selector *metav1.LabelSelector `json:"selector,omitempty"`

	// Names explicitly lists target cluster names
	// +optional
	Names []string `json:"names,omitempty"`
}

// ProfileDurationConfig defines duration boundaries.
type ProfileDurationConfig struct {
	// Default duration granted if requester does not specify
	Default metav1.Duration `json:"default"`
	// Maximum allowable duration
	Max metav1.Duration `json:"max"`
}

// ProfileRequesters defines who is eligible to request elevation.
type ProfileRequesters struct {
	// Groups allowed to request this profile
	// +optional
	Groups []string `json:"groups,omitempty"`

	// Users (usernames or emails) allowed to request this profile
	// +optional
	Users []string `json:"users,omitempty"`
}

// ProfileJustification defines justification constraints.
type ProfileJustification struct {
	// Reason requirement policy
	// +kubebuilder:default=Required
	Reason JustificationRequirement `json:"reason"`
	// TicketPattern is an optional regex pattern validating ticket IDs (e.g. ^(INC|OPS)-[0-9]+$)
	// +optional
	TicketPattern string `json:"ticketPattern,omitempty"`
}

// ApprovalRuleWhen defines when an approval rule matches a request.
type ApprovalRuleWhen struct {
	// ClusterSelector matches target cluster labels
	// +optional
	ClusterSelector *metav1.LabelSelector `json:"clusterSelector,omitempty"`

	// RequesterGroups matches requester groups
	// +optional
	RequesterGroups []string `json:"requesterGroups,omitempty"`

	// RequesterUsers matches requester usernames or emails
	// +optional
	RequesterUsers []string `json:"requesterUsers,omitempty"`
}

// ApprovalApprovers defines eligible approver users and groups.
type ApprovalApprovers struct {
	// Groups of approvers
	// +optional
	Groups []string `json:"groups,omitempty"`

	// Specific approver usernames/emails
	// +optional
	Users []string `json:"users,omitempty"`
}

// ApprovalRule defines a condition-based approval requirement.
// +kubebuilder:validation:XValidation:rule="self.mode != 'Manual' || has(self.approvers)",message="approvers is required when mode is Manual"
type ApprovalRule struct {
	// Name uniquely identifies this rule
	Name string `json:"name"`

	// When conditions under which this rule applies
	// +optional
	When *ApprovalRuleWhen `json:"when,omitempty"`

	// Mode determines if elevation is self-service (Auto) or requires approval (Manual)
	Mode ApprovalMode `json:"mode"`

	// MaxDuration can tighten the maximum duration for this rule
	// +optional
	MaxDuration *metav1.Duration `json:"maxDuration,omitempty"`

	// Approvers specifies who can approve this rule
	// +optional
	Approvers *ApprovalApprovers `json:"approvers,omitempty"`

	// RequiredApprovals is the number of distinct approvals needed (default 1)
	// +optional
	// +kubebuilder:default=1
	RequiredApprovals int `json:"requiredApprovals,omitempty"`

	// AllowSelfApproval permits a requester in an approver group to approve their own request
	// +optional
	// +kubebuilder:default=false
	AllowSelfApproval bool `json:"allowSelfApproval,omitempty"`
}

// ProfileApprovalConfig defines approval rules and timeout.
type ProfileApprovalConfig struct {
	// PendingTimeout defines when an unapproved request expires (e.g. 24h)
	// +optional
	PendingTimeout *metav1.Duration `json:"pendingTimeout,omitempty"`

	// Rules evaluated in order; first match per target cluster governs
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MinItems=1
	Rules []ApprovalRule `json:"rules"`
}

// AccessProfileSpec defines the desired state of AccessProfile.
// +kubebuilder:validation:XValidation:rule="self.scope != 'Cluster' || self.role.kind == 'ClusterRole'",message="Cluster scoped profiles require role.kind=ClusterRole"
// +kubebuilder:validation:XValidation:rule="self.scope != 'Namespaced' || (has(self.namespaces) && size(self.namespaces) > 0)",message="namespaces is required when scope is Namespaced"
// +kubebuilder:validation:XValidation:rule="(has(self.requesters.groups) && size(self.requesters.groups) > 0) || (has(self.requesters.users) && size(self.requesters.users) > 0)",message="requesters must name at least one group or user"
// +kubebuilder:validation:XValidation:rule="(has(self.clusters.selector) && !has(self.clusters.names)) || (!has(self.clusters.selector) && has(self.clusters.names))",message="clusters must specify exactly one of selector or names"
// +kubebuilder:validation:XValidation:rule="duration(self.duration.default) <= duration(self.duration.max)",message="duration.default must be less than or equal to duration.max"
// +kubebuilder:validation:XValidation:rule="duration(self.duration.default) > duration('0s')",message="duration.default must be greater than zero"
type AccessProfileSpec struct {
	DisplayName string `json:"displayName"`

	// +optional
	Description string `json:"description,omitempty"`

	// Role is the target Role or ClusterRole
	Role TargetRoleRef `json:"role"`

	// Scope is either Namespaced or Cluster
	// +kubebuilder:default=Namespaced
	Scope ProfileScope `json:"scope"`

	// Namespaces permitted for elevation (required iff Scope is Namespaced)
	// +optional
	Namespaces []string `json:"namespaces,omitempty"`

	// Clusters specifies targeted fleet clusters (either selector or names)
	Clusters ClusterTargeting `json:"clusters"`

	// Duration constraints
	Duration ProfileDurationConfig `json:"duration"`

	// Requesters permitted to request this profile
	Requesters ProfileRequesters `json:"requesters"`

	// Justification requirements
	Justification ProfileJustification `json:"justification"`

	// Approval rules and workflow
	Approval ProfileApprovalConfig `json:"approval"`
}

// AccessProfileStatus defines the observed state of AccessProfile.
type AccessProfileStatus struct {
	// +optional
	ActiveGrantsCount int `json:"activeGrantsCount,omitempty"`

	// Conditions represent observations of the profile's readiness
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=aprofile
// +kubebuilder:printcolumn:name="Display Name",type=string,JSONPath=`.spec.displayName`
// +kubebuilder:printcolumn:name="Role",type=string,JSONPath=`.spec.role.name`
// +kubebuilder:printcolumn:name="Scope",type=string,JSONPath=`.spec.scope`
// +kubebuilder:printcolumn:name="Max Duration",type=string,JSONPath=`.spec.duration.max`
// +kubebuilder:printcolumn:name="Active Grants",type=integer,JSONPath=`.status.activeGrantsCount`

// AccessProfile defines an eligible privileged role and its multi-cluster approval policy.
type AccessProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AccessProfileSpec   `json:"spec,omitempty"`
	Status AccessProfileStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AccessProfileList contains a list of AccessProfile.
type AccessProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AccessProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AccessProfile{}, &AccessProfileList{})
}
