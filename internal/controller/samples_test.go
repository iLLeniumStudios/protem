package controller

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
)

var _ = Describe("config/samples", func() {
	It("is accepted by the CRDs and its request ends in a grant", func() {
		ctx := context.Background()
		dir := filepath.Join("..", "..", "config", "samples")

		var kustomization struct {
			Resources []string `json:"resources"`
		}
		data, err := os.ReadFile(filepath.Join(dir, "kustomization.yaml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(yaml.Unmarshal(data, &kustomization)).To(Succeed())
		Expect(kustomization.Resources).NotTo(BeEmpty())

		for _, file := range kustomization.Resources {
			data, err := os.ReadFile(filepath.Join(dir, file))
			Expect(err).NotTo(HaveOccurred())
			decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
			for {
				obj := &unstructured.Unstructured{}
				if err := decoder.Decode(obj); errors.Is(err, io.EOF) {
					break
				} else {
					Expect(err).NotTo(HaveOccurred(), file)
				}
				if len(obj.Object) == 0 {
					continue
				}
				Expect(k8sClient.Create(ctx, obj)).To(Succeed(), "%s: %s %s", file, obj.GetKind(), obj.GetName())
				DeferCleanup(func() { _ = client.IgnoreNotFound(k8sClient.Delete(ctx, obj)) })
			}
		}

		clusters := &TargetClusterReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), HubClusterName: "local"}
		_, err = clusters.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: "local"}})
		Expect(err).NotTo(HaveOccurred())

		key := types.NamespacedName{Namespace: "default", Name: "alice-dev-view"}
		requests := &AccessRequestReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		request := &protemv1alpha1.AccessRequest{}
		for range 5 {
			_, err = requests.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Get(ctx, key, request)).To(Succeed())
			if request.Status.GrantRef != "" {
				break
			}
		}
		Expect(request.Status.Phase).To(Equal(protemv1alpha1.RequestPhaseApproved), "conditions: %+v", request.Status.Conditions)

		grant := &protemv1alpha1.AccessGrant{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: request.Status.GrantRef}, grant)).To(Succeed())
		DeferCleanup(func() { _ = client.IgnoreNotFound(k8sClient.Delete(ctx, grant)) })
		Expect(grant.Spec.Role.Name).To(Equal("view"))
		Expect(grant.Spec.Clusters).To(Equal([]string{"local"}))
	})
})

var _ = Describe("README.md", func() {
	It("shows only manifests the CRDs accept", func() {
		ctx := context.Background()
		readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
		Expect(err).NotTo(HaveOccurred())

		blocks := regexp.MustCompile("(?s)```yaml\n(.*?)```").FindAllSubmatch(readme, -1)
		Expect(blocks).NotTo(BeEmpty(), "the README has no yaml example")
		for _, block := range blocks {
			decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(block[1]), 4096)
			for {
				obj := &unstructured.Unstructured{}
				if err := decoder.Decode(obj); errors.Is(err, io.EOF) {
					break
				} else {
					Expect(err).NotTo(HaveOccurred())
				}
				if len(obj.Object) == 0 {
					continue
				}
				Expect(k8sClient.Create(ctx, obj)).To(Succeed(), "%s %s", obj.GetKind(), obj.GetName())
				DeferCleanup(func() { _ = client.IgnoreNotFound(k8sClient.Delete(ctx, obj)) })
			}
		}
	})
})
