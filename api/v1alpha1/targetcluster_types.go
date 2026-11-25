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

// EnforcementMode defines how privileged access is enforced on the target cluster.
// +kubebuilder:validation:Enum=RoleBinding;Authorizer
type EnforcementMode string

const (
	EnforcementModeRoleBinding EnforcementMode = "RoleBinding"
	EnforcementModeAuthorizer  EnforcementMode = "Authorizer"
)

// ClusterPhase defines the connection and lifecycle state of a TargetCluster.
// +kubebuilder:validation:Enum=Ready;Disconnected;Maintenance
type ClusterPhase string

const (
	ClusterPhaseReady        ClusterPhase = "Ready"
	ClusterPhaseDisconnected ClusterPhase = "Disconnected"
	ClusterPhaseMaintenance  ClusterPhase = "Maintenance"
)

// TargetClusterSpec defines the desired configuration and connection settings for a cluster.
type TargetClusterSpec struct {
	DisplayName string `json:"displayName"`

	// +optional
	Description string `json:"description,omitempty"`

	// Enforcement selects the enforcement mechanism on this cluster (default: RoleBinding)
	// +kubebuilder:default=RoleBinding
	// +optional
	Enforcement EnforcementMode `json:"enforcement,omitempty"`

	// Subject is the template for the user name people are bound as on this cluster, e.g. {{.Email}} or
	// oidc:{{.Username}}. Empty means the operator's subject template. Ignored for the hub's own cluster.
	// +optional
	Subject string `json:"subject,omitempty"`

	// Paused suspends new elevation leases on this cluster for maintenance
	// +optional
	// +kubebuilder:default=false
	Paused bool `json:"paused,omitempty"`
}

// TargetClusterStatus defines the observed health and connectivity of the cluster.
type TargetClusterStatus struct {
	// Phase is Ready, Disconnected, or Maintenance
	// +optional
	Phase ClusterPhase `json:"phase,omitempty"`

	// LastHeartbeat timestamp of last contact from spoke agent
	// +optional
	LastHeartbeat *metav1.Time `json:"lastHeartbeat,omitempty"`

	// AgentVersion reports the running spoke agent version
	// +optional
	AgentVersion string `json:"agentVersion,omitempty"`

	// ActiveGrantsCount tracks currently active leases on this cluster
	// +optional
	ActiveGrantsCount int `json:"activeGrantsCount,omitempty"`

	// Conditions represent observations of cluster readiness and agent connectivity
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=tcluster
// +kubebuilder:printcolumn:name="Display Name",type=string,JSONPath=`.spec.displayName`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Agent Version",type=string,JSONPath=`.status.agentVersion`
// +kubebuilder:printcolumn:name="Active Grants",type=integer,JSONPath=`.status.activeGrantsCount`
// +kubebuilder:printcolumn:name="Last Heartbeat",type=string,JSONPath=`.status.lastHeartbeat`

// TargetCluster represents a managed spoke Kubernetes cluster registered with the Protem fleet Hub.
type TargetCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TargetClusterSpec   `json:"spec,omitempty"`
	Status TargetClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TargetClusterList contains a list of TargetCluster.
type TargetClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TargetCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TargetCluster{}, &TargetClusterList{})
}
