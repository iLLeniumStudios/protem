package controller

import (
	"context"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
	"github.com/iLLeniumStudios/protem/internal/fleet"
)

var _ = Describe("agent identity under config/rbac", func() {
	It("is provisioned by the operator, works only in its namespace, and ends with its TargetCluster", func() {
		ctx := context.Background()
		for _, file := range []string{"role.yaml", "role_binding.yaml"} {
			for _, obj := range readManifests(filepath.Join("..", "..", "config", "rbac", file)) {
				Expect(k8sClient.Create(ctx, obj)).To(Succeed(), obj.GetName())
				DeferCleanup(func() { _ = client.IgnoreNotFound(k8sClient.Delete(ctx, obj)) })
			}
		}
		operator := impersonate("system:serviceaccount:system:controller-manager", "system:serviceaccounts", "system:serviceaccounts:system")

		cluster := &protemv1alpha1.TargetCluster{ObjectMeta: metav1.ObjectMeta{Name: "enrol-rbac"}, Spec: protemv1alpha1.TargetClusterSpec{DisplayName: "Enrol"}}
		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Patch(ctx, cluster, client.RawPatch(types.MergePatchType, []byte(`{"metadata":{"finalizers":null}}`)))
			_ = client.IgnoreNotFound(k8sClient.Delete(ctx, cluster))
		})
		ns := fleet.ClusterNamespace(cluster.Name)
		reconciler := &TargetClusterReconciler{Client: operator, Scheme: k8sClient.Scheme(), HubClusterName: "local"}
		reconcileAsOperator := func() error {
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: cluster.Name}})
			return err
		}
		Eventually(reconcileAsOperator).Should(Succeed())
		account := client.ObjectKey{Namespace: ns, Name: fleet.AgentName}
		for _, obj := range []client.Object{&corev1.ServiceAccount{}, &rbacv1.Role{}, &rbacv1.RoleBinding{}} {
			Expect(k8sClient.Get(ctx, account, obj)).To(Succeed(), "%T", obj)
		}

		mint := func(serviceAccount string) (string, error) {
			seconds := int64(600)
			req := &authenticationv1.TokenRequest{Spec: authenticationv1.TokenRequestSpec{ExpirationSeconds: &seconds}}
			sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: serviceAccount}}
			err := operator.SubResource("token").Create(ctx, sa, req)
			return req.Status.Token, err
		}
		token, err := mint(fleet.AgentName)
		Expect(err).NotTo(HaveOccurred())
		agent, err := client.New(&rest.Config{Host: cfg.Host, TLSClientConfig: rest.TLSClientConfig{CAData: cfg.CAData}, BearerToken: token},
			client.Options{Scheme: k8sClient.Scheme()})
		Expect(err).NotTo(HaveOccurred())
		Expect(agent.List(ctx, &protemv1alpha1.ClusterGrantList{}, client.InNamespace(ns))).To(Succeed())
		err = agent.List(ctx, &protemv1alpha1.ClusterGrantList{}, client.InNamespace(defaultNs))
		Expect(apierrors.IsForbidden(err)).To(BeTrue(), "listing ClusterGrants in %s: %v", defaultNs, err)
		err = agent.List(ctx, &protemv1alpha1.ClusterGrantList{})
		Expect(apierrors.IsForbidden(err)).To(BeTrue(), "listing ClusterGrants everywhere: %v", err)

		other := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "someone-else"}}
		Expect(k8sClient.Create(ctx, other)).To(Succeed())
		_, err = mint(other.Name)
		Expect(apierrors.IsForbidden(err)).To(BeTrue(), "operator minting a token for %s: %v", other.Name, err)

		Expect(k8sClient.Delete(ctx, cluster)).To(Succeed())
		Eventually(reconcileAsOperator).Should(Succeed())
		Expect(apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(cluster), cluster))).To(BeTrue(), "TargetCluster still present")
		Expect(apierrors.IsNotFound(k8sClient.Get(ctx, account, &corev1.ServiceAccount{}))).To(BeTrue(), "agent ServiceAccount still present")
		Expect(apierrors.IsNotFound(k8sClient.Get(ctx, account, &rbacv1.RoleBinding{}))).To(BeTrue(), "agent RoleBinding still present")
		Expect(k8sClient.Get(ctx, account, &rbacv1.Role{})).To(Succeed(), "agent Role should be left in place")
		Eventually(func() bool {
			return apierrors.IsUnauthorized(agent.List(ctx, &protemv1alpha1.ClusterGrantList{}, client.InNamespace(ns)))
		}).WithTimeout(30*time.Second).Should(BeTrue(), "the old token still works")
	})
})
