package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
)

var _ = Describe("TargetCluster CRD", func() {
	// An unset subject means "use the operator's subject template". A default written by the API
	// server would pin every cluster to one format and make that template apply to the hub only.
	It("leaves an unset subject unset", func() {
		ctx := context.Background()
		cluster := &protemv1alpha1.TargetCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "crd-no-subject"},
			Spec:       protemv1alpha1.TargetClusterSpec{DisplayName: "No subject"},
		}
		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
		DeferCleanup(func() { _ = client.IgnoreNotFound(k8sClient.Delete(ctx, cluster)) })

		var stored protemv1alpha1.TargetCluster
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cluster), &stored)).To(Succeed())
		Expect(stored.Spec.Subject).To(BeEmpty())
	})
})
