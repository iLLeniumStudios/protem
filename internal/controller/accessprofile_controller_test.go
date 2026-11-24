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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
)

var _ = Describe("AccessProfile Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: defaultNs,
		}
		accessprofile := &protemv1alpha1.AccessProfile{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind AccessProfile")
			err := k8sClient.Get(ctx, typeNamespacedName, accessprofile)
			if err != nil && errors.IsNotFound(err) {
				resource := &protemv1alpha1.AccessProfile{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: defaultNs,
					},
					Spec: protemv1alpha1.AccessProfileSpec{
						DisplayName: "Test Profile",
						Role: protemv1alpha1.TargetRoleRef{
							Kind: clusterRoleKind,
							Name: adminName,
						},
						Scope:      protemv1alpha1.ProfileScopeNamespaced,
						Namespaces: []string{defaultNs},
						Clusters: protemv1alpha1.ClusterTargeting{
							Names: []string{testCluster},
						},
						Duration: protemv1alpha1.ProfileDurationConfig{
							Default: metav1.Duration{Duration: 30 * time.Minute},
							Max:     metav1.Duration{Duration: 2 * time.Hour},
						},
						Requesters: protemv1alpha1.ProfileRequesters{
							Groups: []string{devsGroup},
						},
						Justification: protemv1alpha1.ProfileJustification{
							Reason: protemv1alpha1.JustificationRequired,
						},
						Approval: protemv1alpha1.ProfileApprovalConfig{
							Rules: []protemv1alpha1.ApprovalRule{
								{
									Name: "rule-1",
									Mode: protemv1alpha1.ApprovalModeAuto,
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &protemv1alpha1.AccessProfile{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance AccessProfile")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &AccessProfileReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	It("admits a profile requested only by users and rejects one with no groups and no users", func() {
		ctx := context.Background()
		profile := func(name string, requesters protemv1alpha1.ProfileRequesters) *protemv1alpha1.AccessProfile {
			return &protemv1alpha1.AccessProfile{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: defaultNs},
				Spec: protemv1alpha1.AccessProfileSpec{
					DisplayName:   "Requesters",
					Role:          protemv1alpha1.TargetRoleRef{Kind: clusterRoleKind, Name: adminName},
					Scope:         protemv1alpha1.ProfileScopeCluster,
					Clusters:      protemv1alpha1.ClusterTargeting{Names: []string{testCluster}},
					Duration:      protemv1alpha1.ProfileDurationConfig{Default: metav1.Duration{Duration: time.Hour}, Max: metav1.Duration{Duration: time.Hour}},
					Requesters:    requesters,
					Justification: protemv1alpha1.ProfileJustification{Reason: protemv1alpha1.JustificationOptional},
					Approval:      protemv1alpha1.ProfileApprovalConfig{Rules: []protemv1alpha1.ApprovalRule{{Name: "auto", Mode: protemv1alpha1.ApprovalModeAuto}}},
				},
			}
		}

		usersOnly := profile("requesters-users-only", protemv1alpha1.ProfileRequesters{Users: []string{aliceEmail}})
		Expect(k8sClient.Create(ctx, usersOnly)).To(Succeed())
		DeferCleanup(func() { Expect(k8sClient.Delete(ctx, usersOnly)).To(Succeed()) })

		nobody := profile("requesters-nobody", protemv1alpha1.ProfileRequesters{})
		DeferCleanup(func() { _ = client.IgnoreNotFound(k8sClient.Delete(ctx, nobody)) })
		Expect(k8sClient.Create(ctx, nobody)).To(MatchError(ContainSubstring("requesters must name at least one group or user")))
	})
})
