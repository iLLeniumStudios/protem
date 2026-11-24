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
	"fmt"
	"regexp"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
)

// AccessProfileReconciler reconciles an AccessProfile object
type AccessProfileReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=protem.dev,resources=accessprofiles,verbs=get;list;watch
// +kubebuilder:rbac:groups=protem.dev,resources=accessprofiles/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=protem.dev,resources=accessprofiles/finalizers,verbs=update

func (r *AccessProfileReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var profile protemv1alpha1.AccessProfile
	if err := r.Get(ctx, req.NamespacedName, &profile); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Validate ticket regex pattern if configured (jit-access-review.md §3.9)
	if profile.Spec.Justification.TicketPattern != "" {
		if _, err := regexp.Compile(profile.Spec.Justification.TicketPattern); err != nil {
			meta.SetStatusCondition(&profile.Status.Conditions, metav1.Condition{Type: readyConditionType, Status: metav1.ConditionFalse, Reason: "InvalidTicketPattern",
				Message: fmt.Sprintf("ticketPattern regex is invalid: %v", err), LastTransitionTime: metav1.Now()})
		} else {
			meta.SetStatusCondition(&profile.Status.Conditions, metav1.Condition{Type: readyConditionType, Status: metav1.ConditionTrue, Reason: "ValidProfile", Message: "AccessProfile is valid and ready", LastTransitionTime: metav1.Now()})
		}
	} else {
		meta.SetStatusCondition(&profile.Status.Conditions, metav1.Condition{Type: readyConditionType, Status: metav1.ConditionTrue, Reason: "ValidProfile", Message: "AccessProfile is valid and ready", LastTransitionTime: metav1.Now()})
	}

	// Track active grants count
	var grants protemv1alpha1.AccessGrantList
	if err := r.List(ctx, &grants, client.InNamespace(profile.Namespace), client.MatchingLabels{LabelProfileRef: profile.Name}); err == nil {
		activeCount := 0
		now := time.Now()
		for _, g := range grants.Items {
			if g.Status.Phase == protemv1alpha1.GrantPhaseActive && g.Spec.Revocation == nil && now.Before(g.Spec.ExpiresAt.Time) {
				activeCount++
			}
		}
		profile.Status.ActiveGrantsCount = activeCount
	}

	_ = r.Status().Update(ctx, &profile)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AccessProfileReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&protemv1alpha1.AccessProfile{}).
		Named("accessprofile").
		Complete(r)
}
