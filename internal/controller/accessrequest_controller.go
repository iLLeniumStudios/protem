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
	"errors"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
)

// AccessRequestReconciler reconciles an AccessRequest object
type AccessRequestReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=protem.dev,resources=accessrequests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=protem.dev,resources=accessrequests/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=protem.dev,resources=accessrequests/finalizers,verbs=update
// +kubebuilder:rbac:groups=protem.dev,resources=accessprofiles,verbs=get;list;watch
// +kubebuilder:rbac:groups=protem.dev,resources=targetclusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=protem.dev,resources=accessgrants,verbs=get;list;watch;create;update;patch;delete

func (r *AccessRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("accessrequest", req.NamespacedName)

	var accessReq protemv1alpha1.AccessRequest
	if err := r.Get(ctx, req.NamespacedName, &accessReq); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	switch accessReq.Status.Phase {
	case protemv1alpha1.RequestPhaseRejected, protemv1alpha1.RequestPhaseCancelled, protemv1alpha1.RequestPhaseTimedOut:
		return ctrl.Result{}, nil
	}

	profile := &protemv1alpha1.AccessProfile{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: accessReq.Namespace, Name: accessReq.Spec.ProfileRef}, profile); apierrors.IsNotFound(err) {
		profile = nil
	} else if err != nil {
		return ctrl.Result{}, err
	}

	var clusters protemv1alpha1.TargetClusterList
	var requests protemv1alpha1.AccessRequestList
	var grants protemv1alpha1.AccessGrantList
	if accessReq.Status.Phase == "" {
		inNamespace := client.InNamespace(accessReq.Namespace)
		if err := errors.Join(r.List(ctx, &clusters), r.List(ctx, &requests, inNamespace), r.List(ctx, &grants, inNamespace)); err != nil {
			return ctrl.Result{}, err
		}
	}

	next := Advance(&accessReq, profile, clusters.Items, Others{Requests: requests.Items, Grants: grants.Items}, time.Now())
	if next.Grant != nil {
		if err := controllerutil.SetControllerReference(&accessReq, next.Grant, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("Creating AccessGrant", "grant", next.Grant.Name, "expiresAt", next.Grant.Spec.ExpiresAt)
		if err := r.Create(ctx, next.Grant); apierrors.IsAlreadyExists(err) {
			var existing protemv1alpha1.AccessGrant
			if err := r.Get(ctx, client.ObjectKeyFromObject(next.Grant), &existing); err != nil {
				return ctrl.Result{}, err
			}
			if !metav1.IsControlledBy(&existing, &accessReq) {
				return ctrl.Result{}, fmt.Errorf("AccessGrant %s already exists and is not controlled by AccessRequest %s", client.ObjectKeyFromObject(&existing), accessReq.Name)
			}
		} else if err != nil {
			return ctrl.Result{}, err
		}
	}
	if next.Status != nil {
		log.Info("Advancing AccessRequest", "phase", next.Status.Phase)
		accessReq.Status = *next.Status
		// The API (decisions) and the audit controller (its cursor) write this status too. Losing that
		// race is routine: read again and redo the step instead of reporting an error.
		if err := r.Status().Update(ctx, &accessReq); apierrors.IsConflict(err) {
			return ctrl.Result{RequeueAfter: 50 * time.Millisecond}, nil
		} else if err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{RequeueAfter: next.RequeueAfter}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AccessRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&protemv1alpha1.AccessRequest{}).
		Named("accessrequest").
		Complete(r)
}
