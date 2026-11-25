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
	"slices"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
	"github.com/iLLeniumStudios/protem/internal/binding"
	"github.com/iLLeniumStudios/protem/internal/fleet"
)

const (
	protemFinalizer = "protem.dev/finalizer"
	hubBindingOwner = "protem"
)

// AccessGrantReconciler reconciles an AccessGrant object
type AccessGrantReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	SubjectTemplate string
	HubClusterName  string
}

// +kubebuilder:rbac:groups=protem.dev,resources=accessgrants,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=protem.dev,resources=accessgrants/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=protem.dev,resources=accessgrants/finalizers,verbs=update
// +kubebuilder:rbac:groups=protem.dev,resources=clustergrants,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=protem.dev,resources=clustergrants/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings;clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;clusterroles,verbs=get;list;watch
// The UI creates requests on behalf of the person logged in (config/admission/requester_identity.yaml).
// +kubebuilder:rbac:groups=protem.dev,resources=accessrequests,verbs=create;delegate

func (r *AccessGrantReconciler) isHubTargeted(grant *protemv1alpha1.AccessGrant) bool {
	if r.HubClusterName == "" || len(grant.Spec.Clusters) == 0 {
		return true // Standalone single-cluster mode
	}
	for _, c := range grant.Spec.Clusters {
		if c == r.HubClusterName {
			return true
		}
	}
	return false
}

func (r *AccessGrantReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("accessgrant", req.NamespacedName)

	var grant protemv1alpha1.AccessGrant
	if err := r.Get(ctx, req.NamespacedName, &grant); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle Deletion with Finalizer
	if !grant.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&grant, protemFinalizer) {
			log.Info("Cleaning up managed RBAC bindings before grant deletion")
			if err := r.cleanupBindings(ctx, &grant); err != nil {
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(&grant, protemFinalizer)
			return ctrl.Result{}, r.Update(ctx, &grant)
		}
		return ctrl.Result{}, nil
	}

	// Ensure Finalizer is registered
	if !controllerutil.ContainsFinalizer(&grant, protemFinalizer) {
		controllerutil.AddFinalizer(&grant, protemFinalizer)
		if err := r.Update(ctx, &grant); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Check if Revoked (presence of Revocation field)
	if grant.Spec.Revocation != nil {
		changed := grant.Status.Phase != protemv1alpha1.GrantPhaseRevoked
		if changed {
			log.Info("Grant has been revoked, removing RBAC bindings", "revokedBy", grant.Spec.Revocation.By)
			if err := r.cleanupBindings(ctx, &grant); err != nil {
				return ctrl.Result{}, err
			}
			r.cleanupClusterGrants(ctx, &grant)
			grant.Status.Phase = protemv1alpha1.GrantPhaseRevoked
			grant.Status.ManagedBindings = nil
		}
		states, waiting := r.revokedClusterStates(ctx, &grant)
		if changed || !equality.Semantic.DeepEqual(grant.Status.Clusters, states) {
			grant.Status.Clusters = states
			if err := r.Status().Update(ctx, &grant); err != nil {
				return ctrl.Result{}, err
			}
		}
		if waiting {
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
		return ctrl.Result{}, nil
	}

	// Check if Expired
	now := time.Now()
	if now.After(grant.Spec.ExpiresAt.Time) || now.Equal(grant.Spec.ExpiresAt.Time) {
		if grant.Status.Phase != protemv1alpha1.GrantPhaseExpired {
			log.Info("Grant has expired, removing RBAC bindings", "expiredAt", grant.Spec.ExpiresAt)
			if err := r.cleanupBindings(ctx, &grant); err != nil {
				return ctrl.Result{}, err
			}
			r.cleanupClusterGrants(ctx, &grant)
			grant.Status.Phase = protemv1alpha1.GrantPhaseExpired
			grant.Status.ManagedBindings = nil
			for i, c := range grant.Status.Clusters {
				grant.Status.Clusters[i] = protemv1alpha1.ClusterBindingState{ClusterName: c.ClusterName, State: protemv1alpha1.ClusterBindingStateExpired, LastTransitionTime: metav1.NewTime(now)}
			}
			return ctrl.Result{}, r.Status().Update(ctx, &grant)
		}
		return ctrl.Result{}, nil
	}

	// Active Grant: Ensure RBAC bindings are created on Hub ONLY if Hub is targeted (C3)
	var managedBindings []protemv1alpha1.ManagedBindingRef
	var hubErr error
	if r.isHubTargeted(&grant) {
		managedBindings, hubErr = r.ensureBindings(ctx, &grant)
		// A failure on the hub is reported as that cluster's state, when the grant lists it.
		if hubErr != nil && !slices.Contains(grant.Spec.Clusters, r.HubClusterName) {
			return ctrl.Result{}, hubErr
		}
	}

	clusterStates := r.ensureClusterGrants(ctx, &grant, managedBindings, hubErr)

	requeueAfter := time.Until(grant.Spec.ExpiresAt.Time)
	if requeueAfter > 30*time.Second {
		requeueAfter = 30 * time.Second
	}
	if requeueAfter < time.Second {
		requeueAfter = time.Second
	}

	if grant.Status.Phase != protemv1alpha1.GrantPhaseActive ||
		!equality.Semantic.DeepEqual(grant.Status.ManagedBindings, managedBindings) ||
		!equality.Semantic.DeepEqual(grant.Status.Clusters, clusterStates) {
		grant.Status.Phase = protemv1alpha1.GrantPhaseActive
		grant.Status.ManagedBindings = managedBindings
		grant.Status.Clusters = clusterStates
		if err := r.Status().Update(ctx, &grant); err != nil {
			return ctrl.Result{}, err
		}
	}

	if hubErr != nil {
		return ctrl.Result{}, hubErr
	}
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

func (r *AccessGrantReconciler) ensureBindings(ctx context.Context, grant *protemv1alpha1.AccessGrant) ([]protemv1alpha1.ManagedBindingRef, error) {
	return binding.Apply(ctx, r.Client, hubBindingOwner, binding.Desired{
		Grant:          grant.Name,
		GrantNamespace: grant.Namespace,
		Subject:        binding.FormatSubject(r.SubjectTemplate, grant.Spec.Requester),
		Role:           grant.Spec.Role,
		Scope:          grant.Spec.Scope,
		Namespaces:     grant.Spec.Namespaces,
		ExpiresAt:      grant.Spec.ExpiresAt.Time,
	})
}

func (r *AccessGrantReconciler) cleanupBindings(ctx context.Context, grant *protemv1alpha1.AccessGrant) error {
	if !r.isHubTargeted(grant) {
		return nil
	}
	return binding.Remove(ctx, r.Client, hubBindingOwner, grant.Namespace, grant.Name)
}

func (r *AccessGrantReconciler) ensureClusterGrants(ctx context.Context, grant *protemv1alpha1.AccessGrant, hubBindings []protemv1alpha1.ManagedBindingRef, hubErr error) []protemv1alpha1.ClusterBindingState {
	log := logf.FromContext(ctx).WithValues("accessgrant", grant.Name)
	clusterStates := make([]protemv1alpha1.ClusterBindingState, 0, len(grant.Spec.Clusters))
	transitionTime := func(cluster string, state protemv1alpha1.ClusterBindingStatePhase) metav1.Time {
		for _, prev := range grant.Status.Clusters {
			if prev.ClusterName == cluster && prev.State == state {
				return prev.LastTransitionTime
			}
		}
		return metav1.Now()
	}
	for _, clusterName := range grant.Spec.Clusters {
		// The hub has no agent to read a ClusterGrant or report on one: the operator bound there itself.
		if clusterName == r.HubClusterName {
			// Versions before 0.1.4 wrote one.
			stale := &protemv1alpha1.ClusterGrant{}
			if err := r.Get(ctx, client.ObjectKey{Namespace: fleet.ClusterNamespace(clusterName), Name: fleet.GrantKey(grant.Namespace, grant.Name)}, stale); err == nil {
				_ = r.Delete(ctx, stale)
			}
			state, message := protemv1alpha1.ClusterBindingStateActive, "Applied by the hub"
			if hubErr != nil {
				state, message = protemv1alpha1.ClusterBindingStateFailed, hubErr.Error()
			}
			clusterStates = append(clusterStates, protemv1alpha1.ClusterBindingState{
				ClusterName:        clusterName,
				State:              state,
				Bindings:           hubBindings,
				Message:            message,
				LastTransitionTime: transitionTime(clusterName, state),
			})
			continue
		}
		clusterNS := fleet.ClusterNamespace(clusterName)

		// Ensure cluster namespace exists on Hub
		var nsObj corev1.Namespace
		if err := r.Get(ctx, client.ObjectKey{Name: clusterNS}, &nsObj); err != nil {
			if apierrors.IsNotFound(err) {
				nsObj = corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: clusterNS}}
				if createErr := r.Create(ctx, &nsObj); createErr != nil && !apierrors.IsAlreadyExists(createErr) {
					log.Error(createErr, "Failed to create cluster namespace on Hub", "namespace", clusterNS)
				}
			}
		}

		// Resolve subject template for this specific cluster from TargetCluster on Hub
		subjectTemplate := r.SubjectTemplate
		var tc protemv1alpha1.TargetCluster
		if err := r.Get(ctx, client.ObjectKey{Name: clusterName}, &tc); err == nil && tc.Spec.Subject != "" {
			subjectTemplate = tc.Spec.Subject
		}
		resolvedSubject := binding.FormatSubject(subjectTemplate, grant.Spec.Requester)

		cg := &protemv1alpha1.ClusterGrant{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fleet.GrantKey(grant.Namespace, grant.Name),
				Namespace: clusterNS,
			},
		}

		_, err := controllerutil.CreateOrUpdate(ctx, r.Client, cg, func() error {
			if cg.Labels == nil {
				cg.Labels = make(map[string]string)
			}
			cg.Labels[binding.LabelManagedBy] = "protem-hub"
			cg.Labels[binding.LabelGrant] = grant.Name
			cg.Labels[binding.LabelGrantNamespace] = grant.Namespace
			cg.Spec = protemv1alpha1.ClusterGrantSpec{
				GrantRef:   grant.Name,
				ProfileRef: grant.Spec.ProfileRef,
				Requester:  grant.Spec.Requester,
				Role:       grant.Spec.Role,
				Scope:      grant.Spec.Scope,
				Namespaces: grant.Spec.Namespaces,
				Subject:    resolvedSubject,
				ExpiresAt:  grant.Spec.ExpiresAt,
				Revocation: grant.Spec.Revocation,
			}
			return nil
		})
		if err != nil {
			clusterStates = append(clusterStates, protemv1alpha1.ClusterBindingState{
				ClusterName:        clusterName,
				State:              protemv1alpha1.ClusterBindingStateFailed,
				Message:            err.Error(),
				LastTransitionTime: transitionTime(clusterName, protemv1alpha1.ClusterBindingStateFailed),
			})
			continue
		}

		state := protemv1alpha1.ClusterBindingStatePhase(cg.Status.Phase)
		if state == "" {
			state = protemv1alpha1.ClusterBindingStatePending
		}
		clusterStates = append(clusterStates, protemv1alpha1.ClusterBindingState{
			ClusterName:        clusterName,
			State:              state,
			Bindings:           cg.Status.Bindings,
			Message:            cg.Status.Message,
			LastTransitionTime: transitionTime(clusterName, state),
		})
	}
	return clusterStates
}

func (r *AccessGrantReconciler) cleanupClusterGrants(ctx context.Context, grant *protemv1alpha1.AccessGrant) {
	for _, clusterName := range grant.Spec.Clusters {
		clusterNS := fleet.ClusterNamespace(clusterName)
		var cg protemv1alpha1.ClusterGrant
		if err := r.Get(ctx, client.ObjectKey{Namespace: clusterNS, Name: fleet.GrantKey(grant.Namespace, grant.Name)}, &cg); err == nil {
			if grant.Spec.Revocation != nil && clusterName != r.HubClusterName {
				cg.Spec.Revocation = grant.Spec.Revocation
				_ = r.Update(ctx, &cg)
			} else {
				_ = r.Delete(ctx, &cg)
			}
		}
	}
}

func grantForBinding(_ context.Context, obj client.Object) []reconcile.Request {
	labels := obj.GetLabels()
	name, namespace := labels[binding.LabelGrant], labels[binding.LabelGrantNamespace]
	if name == "" || namespace == "" {
		return nil
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: namespace, Name: name}}}
}

// SetupWithManager sets up the controller with the Manager.
func (r *AccessGrantReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&protemv1alpha1.AccessGrant{}).
		Watches(&rbacv1.RoleBinding{}, handler.EnqueueRequestsFromMapFunc(grantForBinding)).
		Watches(&rbacv1.ClusterRoleBinding{}, handler.EnqueueRequestsFromMapFunc(grantForBinding)).
		Named("accessgrant").
		Complete(r)
}

// revokedClusterStates reports a cluster of a revoked grant as Revoked once nothing can still be
// bound there: on the hub at once, on a target cluster when its agent says so or the grant's own
// time is up, which the agent enforces without the hub. It also reports whether any is still open.
func (r *AccessGrantReconciler) revokedClusterStates(ctx context.Context, grant *protemv1alpha1.AccessGrant) ([]protemv1alpha1.ClusterBindingState, bool) {
	expired := !time.Now().Before(grant.Spec.ExpiresAt.Time)
	states := make([]protemv1alpha1.ClusterBindingState, 0, len(grant.Status.Clusters))
	waiting := false
	for _, c := range grant.Status.Clusters {
		done := expired || c.ClusterName == r.HubClusterName
		if !done {
			var cg protemv1alpha1.ClusterGrant
			err := r.Get(ctx, client.ObjectKey{Namespace: fleet.ClusterNamespace(c.ClusterName), Name: fleet.GrantKey(grant.Namespace, grant.Name)}, &cg)
			done = apierrors.IsNotFound(err) || (err == nil && cg.Status.Phase == protemv1alpha1.ClusterGrantPhaseRevoked)
		}
		if done && c.State != protemv1alpha1.ClusterBindingStateRevoked {
			c = protemv1alpha1.ClusterBindingState{ClusterName: c.ClusterName, State: protemv1alpha1.ClusterBindingStateRevoked, LastTransitionTime: metav1.Now()}
		}
		waiting = waiting || !done
		states = append(states, c)
	}
	return states, waiting
}
