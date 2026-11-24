package controller

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("config/rbac", func() {
	It("lets the operator bind on the hub only the roles listed as bindable", func() {
		ctx := context.Background()
		for _, file := range []string{"role.yaml", "role_binding.yaml", "hub_bindable_roles.yaml"} {
			for _, obj := range readManifests(filepath.Join("..", "..", "config", "rbac", file)) {
				Expect(k8sClient.Create(ctx, obj)).To(Succeed(), obj.GetName())
				DeferCleanup(func() { _ = client.IgnoreNotFound(k8sClient.Delete(ctx, obj)) })
			}
		}
		operator := impersonate("system:serviceaccount:system:controller-manager", "system:serviceaccounts", "system:serviceaccounts:system")

		bindTo := func(role string) error {
			binding := &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{GenerateName: "hub-rbac-", Namespace: defaultNs},
				RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: role},
				Subjects:   []rbacv1.Subject{{APIGroup: rbacv1.GroupName, Kind: rbacv1.UserKind, Name: aliceUsername}},
			}
			err := operator.Create(ctx, binding)
			if err == nil {
				DeferCleanup(func() { _ = client.IgnoreNotFound(k8sClient.Delete(ctx, binding)) })
			}
			return err
		}
		Eventually(func() error { return bindTo("view") }).Should(Succeed())
		err := bindTo("cluster-admin")
		Expect(apierrors.IsForbidden(err)).To(BeTrue(), "binding cluster-admin: %v", err)
	})
})

func readManifests(path string) []*unstructured.Unstructured {
	data, err := os.ReadFile(path)
	Expect(err).NotTo(HaveOccurred())
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
	var objs []*unstructured.Unstructured
	for {
		obj := &unstructured.Unstructured{}
		if err := decoder.Decode(obj); errors.Is(err, io.EOF) {
			return objs
		} else {
			Expect(err).NotTo(HaveOccurred(), path)
		}
		if len(obj.Object) > 0 {
			objs = append(objs, obj)
		}
	}
}

func impersonate(user string, groups ...string) client.Client {
	impersonated := rest.CopyConfig(cfg)
	impersonated.Impersonate = rest.ImpersonationConfig{UserName: user, Groups: append(groups, "system:authenticated")}
	c, err := client.New(impersonated, client.Options{Scheme: k8sClient.Scheme()})
	Expect(err).NotTo(HaveOccurred())
	return c
}
