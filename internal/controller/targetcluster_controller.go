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

package controller

import (
	"context"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
	"github.com/iLLeniumStudios/protem/internal/fleet"
)

var targetClusterLog = logf.Log.WithName("targetcluster-controller")

const (
	HeartbeatTimeout   = 90 * time.Second
	readyConditionType = "Ready"
)

// TargetClusterReconciler reconciles a TargetCluster object
type TargetClusterReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	HubClusterName string
}

// +kubebuilder:rbac:groups=protem.dev,resources=targetclusters,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups=protem.dev,resources=targetclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=protem.dev,resources=targetclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups="",resources=serviceaccounts/token,verbs=create,resourceNames=protem-agent
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=create;update

func (r *TargetClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := targetClusterLog.WithValues("targetcluster", req.NamespacedName)

	var cluster protemv1alpha1.TargetCluster
	if err := r.Get(ctx, req.NamespacedName, &cluster); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch TargetCluster")
		return ctrl.Result{}, err
	}
	if !cluster.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.removeAgent(ctx, &cluster)
	}

	// 1. Ensure cluster namespace exists on Hub
	clusterNS := fleet.ClusterNamespace(cluster.Name)
	var nsObj corev1.Namespace
	if err := r.Get(ctx, client.ObjectKey{Name: clusterNS}, &nsObj); err != nil {
		if apierrors.IsNotFound(err) {
			nsObj = corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: clusterNS}}
			if createErr := r.Create(ctx, &nsObj); createErr != nil && !apierrors.IsAlreadyExists(createErr) {
				log.Error(createErr, "Failed to create cluster namespace", "namespace", clusterNS)
			}
		}
	}

	now := time.Now()
	origStatus := cluster.Status.DeepCopy()

	hubName := r.HubClusterName
	if hubName == "" {
		hubName = "local"
	}

	if cluster.Name != hubName {
		if err := r.ensureAgent(ctx, &cluster); err != nil {
			log.Error(err, "Failed to provision the agent's identity on the hub")
			return ctrl.Result{}, err
		}
	}

	// 2. Hub cluster itself is inherently active and Ready
	if cluster.Name == hubName {
		cluster.Status.Phase = protemv1alpha1.ClusterPhaseReady
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{Type: readyConditionType, Status: metav1.ConditionTrue, Reason: "HubControlPlane", Message: "Management hub cluster is active", LastTransitionTime: metav1.NewTime(now)})
		if !r.statusEqual(origStatus, &cluster.Status) {
			if err := r.Status().Update(ctx, &cluster); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// 2. Check heartbeat from Lease or status.lastHeartbeat
	var lastHeartbeatTime *time.Time
	if cluster.Status.LastHeartbeat != nil {
		t := cluster.Status.LastHeartbeat.Time
		lastHeartbeatTime = &t
	}

	var lease coordinationv1.Lease
	if err := r.Get(ctx, client.ObjectKey{Namespace: clusterNS, Name: fleet.AgentLeaseName}, &lease); err == nil && lease.Spec.RenewTime != nil {
		leaseTime := lease.Spec.RenewTime.Time
		if lastHeartbeatTime == nil || leaseTime.After(*lastHeartbeatTime) {
			lastHeartbeatTime = &leaseTime
			cluster.Status.LastHeartbeat = &metav1.Time{Time: leaseTime}
		}
	}

	if lastHeartbeatTime == nil {
		cluster.Status.Phase = protemv1alpha1.ClusterPhaseDisconnected
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{Type: readyConditionType, Status: metav1.ConditionFalse, Reason: "AwaitingFirstHeartbeat", Message: "No heartbeat received from spoke agent yet", LastTransitionTime: metav1.NewTime(now)})
		if !r.statusEqual(origStatus, &cluster.Status) {
			if err := r.Status().Update(ctx, &cluster); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	sinceHeartbeat := now.Sub(*lastHeartbeatTime)
	if sinceHeartbeat > HeartbeatTimeout {
		// Agent timed out
		cluster.Status.Phase = protemv1alpha1.ClusterPhaseDisconnected
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{Type: readyConditionType, Status: metav1.ConditionFalse, Reason: "HeartbeatTimeout", Message: "Spoke agent heartbeat exceeded 90s timeout threshold", LastTransitionTime: metav1.NewTime(now)})
		if !r.statusEqual(origStatus, &cluster.Status) {
			if err := r.Status().Update(ctx, &cluster); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Active and Healthy
	cluster.Status.Phase = protemv1alpha1.ClusterPhaseReady
	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{Type: readyConditionType, Status: metav1.ConditionTrue, Reason: "HeartbeatActive", Message: "Spoke agent is actively sending heartbeats", LastTransitionTime: metav1.NewTime(now)})

	if !r.statusEqual(origStatus, &cluster.Status) {
		if err := r.Status().Update(ctx, &cluster); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Requeue when this heartbeat will expire
	requeueIn := HeartbeatTimeout - sinceHeartbeat + time.Second
	return ctrl.Result{RequeueAfter: requeueIn}, nil
}

func (r *TargetClusterReconciler) ensureAgent(ctx context.Context, cluster *protemv1alpha1.TargetCluster) error {
	if !controllerutil.ContainsFinalizer(cluster, protemFinalizer) {
		patch := client.MergeFromWithOptions(cluster.DeepCopy(), client.MergeFromWithOptimisticLock{})
		controllerutil.AddFinalizer(cluster, protemFinalizer)
		if err := r.Patch(ctx, cluster, patch); err != nil {
			return err
		}
	}
	return fleet.EnsureAgentIdentity(ctx, r.Client, cluster.Name)
}

// removeAgent cuts the agent off and deletes what it left on the hub. The namespace stays: the operator
// may not delete namespaces.
func (r *TargetClusterReconciler) removeAgent(ctx context.Context, cluster *protemv1alpha1.TargetCluster) error {
	if !controllerutil.ContainsFinalizer(cluster, protemFinalizer) {
		return nil
	}
	if err := fleet.RemoveAgentIdentity(ctx, r.Client, cluster.Name); err != nil {
		return err
	}
	ns := fleet.ClusterNamespace(cluster.Name)
	lease := &coordinationv1.Lease{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: fleet.AgentLeaseName}}
	if err := r.Delete(ctx, lease); client.IgnoreNotFound(err) != nil {
		return err
	}
	var grants protemv1alpha1.ClusterGrantList
	if err := r.List(ctx, &grants, client.InNamespace(ns)); err != nil {
		return err
	}
	for i := range grants.Items {
		if err := r.Delete(ctx, &grants.Items[i]); client.IgnoreNotFound(err) != nil {
			return err
		}
	}
	patch := client.MergeFromWithOptions(cluster.DeepCopy(), client.MergeFromWithOptimisticLock{})
	controllerutil.RemoveFinalizer(cluster, protemFinalizer)
	return r.Patch(ctx, cluster, patch)
}

func (r *TargetClusterReconciler) statusEqual(a, b *protemv1alpha1.TargetClusterStatus) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Phase == b.Phase &&
		len(a.Conditions) == len(b.Conditions) &&
		a.AgentVersion == b.AgentVersion &&
		a.ActiveGrantsCount == b.ActiveGrantsCount
}

func (r *TargetClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&protemv1alpha1.TargetCluster{}).
		Named("targetcluster").
		Complete(r)
}
