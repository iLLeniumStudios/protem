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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
)

var _ = Describe("AccessGrant Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: defaultNs,
		}
		accessgrant := &protemv1alpha1.AccessGrant{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind AccessGrant")
			err := k8sClient.Get(ctx, typeNamespacedName, accessgrant)
			if err != nil && errors.IsNotFound(err) {
				resource := &protemv1alpha1.AccessGrant{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: defaultNs,
					},
					Spec: protemv1alpha1.AccessGrantSpec{
						RequestRef: reqOne,
						ProfileRef: testProfileReq,
						Requester: protemv1alpha1.RequesterInfo{
							Username: aliceUsername,
							Email:    aliceEmail,
							Groups:   []string{devsGroup},
						},
						Role: protemv1alpha1.TargetRoleRef{
							Kind: clusterRoleKind,
							Name: adminName,
						},
						Scope:      protemv1alpha1.ProfileScopeNamespaced,
						Namespaces: []string{defaultNs},
						Clusters:   []string{testCluster},
						ExpiresAt:  metav1.NewTime(time.Now().Add(1 * time.Hour)),
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &protemv1alpha1.AccessGrant{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance AccessGrant")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &AccessGrantReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
