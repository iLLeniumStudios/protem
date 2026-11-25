package controller

import (
	"context"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
)

var _ = Describe("config/admission", func() {
	It("lets a caller create a request only in their own name, unless RBAC allows delegate", func() {
		ctx := context.Background()
		const (
			operator   = "system:serviceaccount:protem-system:protem-controller-manager"
			automation = "system:serviceaccount:ci:request-bot"
		)

		policy := readManifests(filepath.Join("..", "..", "config", "admission", "requester_identity.yaml"))
		for _, obj := range policy {
			Expect(k8sClient.Create(ctx, obj)).To(Succeed(), obj.GetKind())
			DeferCleanup(func() { _ = client.IgnoreNotFound(k8sClient.Delete(ctx, obj)) })
		}

		requesters := &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{Name: "admission-test-requesters"},
			Rules:      []rbacv1.PolicyRule{{APIGroups: []string{protemv1alpha1.GroupVersion.Group}, Resources: []string{"accessrequests"}, Verbs: []string{"create"}}},
		}
		delegates := &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{Name: "admission-test-delegates"},
			Rules:      []rbacv1.PolicyRule{{APIGroups: []string{protemv1alpha1.GroupVersion.Group}, Resources: []string{"accessrequests"}, Verbs: []string{"delegate"}}},
		}
		subject := func(user string) rbacv1.Subject {
			return rbacv1.Subject{APIGroup: rbacv1.GroupName, Kind: rbacv1.UserKind, Name: user}
		}
		for _, obj := range []client.Object{
			requesters,
			delegates,
			&rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: requesters.Name},
				RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: requesters.Name},
				Subjects:   []rbacv1.Subject{{APIGroup: rbacv1.GroupName, Kind: rbacv1.GroupKind, Name: devsGroup}, subject(operator), subject(automation)},
			},
			&rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: delegates.Name},
				RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: delegates.Name},
				Subjects:   []rbacv1.Subject{subject(operator)},
			},
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: delegates.Name, Namespace: defaultNs},
				RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: delegates.Name},
				Subjects:   []rbacv1.Subject{subject(automation)},
			},
		} {
			Expect(k8sClient.Create(ctx, obj)).To(Succeed())
			DeferCleanup(func() { _ = client.IgnoreNotFound(k8sClient.Delete(ctx, obj)) })
		}

		create := func(c client.Client, requester protemv1alpha1.RequesterInfo) error {
			req := &protemv1alpha1.AccessRequest{
				ObjectMeta: metav1.ObjectMeta{GenerateName: "admission-", Namespace: defaultNs},
				Spec:       protemv1alpha1.AccessRequestSpec{ProfileRef: "any", Requester: requester},
			}
			err := c.Create(ctx, req)
			if err == nil {
				DeferCleanup(func() { _ = client.IgnoreNotFound(k8sClient.Delete(ctx, req)) })
			}
			return err
		}
		carol := impersonate("carol", devsGroup)

		By("refusing someone else's name from a caller without delegate, once the API server enforces the policy")
		Eventually(func() error {
			return create(carol, protemv1alpha1.RequesterInfo{Username: aliceUsername, Groups: []string{devsGroup}})
		}, 60*time.Second, time.Second).Should(MatchError(ContainSubstring("must be your own username, carol")))

		By("refusing someone else's email and groups the caller is not in")
		Expect(create(carol, protemv1alpha1.RequesterInfo{Username: "carol", Email: aliceEmail})).
			To(MatchError(ContainSubstring("spec.requester.email must be empty or carol")))
		Expect(create(carol, protemv1alpha1.RequesterInfo{Username: "carol", Groups: []string{devsGroup, sreLeadsGroup}})).
			To(MatchError(ContainSubstring("groups you belong to")))

		By("accepting the caller's own identity")
		Expect(create(carol, protemv1alpha1.RequesterInfo{Username: "carol", Groups: []string{devsGroup}})).To(Succeed())
		Expect(create(carol, protemv1alpha1.RequesterInfo{Username: "carol", Email: "carol"})).To(Succeed())

		By("letting callers RBAC allows to delegate create requests in someone else's name")
		alice := protemv1alpha1.RequesterInfo{Username: aliceUsername, Email: aliceEmail, Groups: []string{devsGroup, sreLeadsGroup}}
		Expect(create(impersonate(operator), alice)).To(Succeed())
		Expect(create(impersonate(automation), alice)).To(Succeed())

		By("removing the policy, and waiting until the API server stops enforcing it, so other specs are unaffected")
		for _, obj := range policy {
			Expect(k8sClient.Delete(ctx, obj)).To(Succeed())
		}
		Eventually(func() error {
			return create(carol, protemv1alpha1.RequesterInfo{Username: aliceUsername})
		}, 60*time.Second, time.Second).Should(Succeed())
	})

	// An API server that reads users from an OIDC token usually puts a prefix on their names and
	// groups. The login to the UI has none, so the policy must compare without it, or one person
	// becomes two requesters.
	It("compares names and groups without the API server's prefixes when they are configured", func() {
		ctx := context.Background()

		policy := readManifests(filepath.Join("..", "..", "config", "admission", "requester_identity.yaml"))
		for _, obj := range policy {
			if obj.GetKind() == "ValidatingAdmissionPolicy" {
				setPolicyVariable(obj, "usernamePrefix", "'oidc:'")
				setPolicyVariable(obj, "groupsPrefix", "'grp:'")
			}
			Expect(k8sClient.Create(ctx, obj)).To(Succeed(), obj.GetKind())
			DeferCleanup(func() { _ = client.IgnoreNotFound(k8sClient.Delete(ctx, obj)) })
		}

		requesters := &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{Name: "admission-prefix-requesters"},
			Rules:      []rbacv1.PolicyRule{{APIGroups: []string{protemv1alpha1.GroupVersion.Group}, Resources: []string{"accessrequests"}, Verbs: []string{"create"}}},
		}
		for _, obj := range []client.Object{
			requesters,
			&rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: requesters.Name},
				RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: requesters.Name},
				Subjects:   []rbacv1.Subject{{APIGroup: rbacv1.GroupName, Kind: rbacv1.GroupKind, Name: "grp:" + devsGroup}},
			},
		} {
			Expect(k8sClient.Create(ctx, obj)).To(Succeed())
			DeferCleanup(func() { _ = client.IgnoreNotFound(k8sClient.Delete(ctx, obj)) })
		}

		create := func(c client.Client, requester protemv1alpha1.RequesterInfo) error {
			req := &protemv1alpha1.AccessRequest{
				ObjectMeta: metav1.ObjectMeta{GenerateName: "admission-prefix-", Namespace: defaultNs},
				Spec:       protemv1alpha1.AccessRequestSpec{ProfileRef: "any", Requester: requester},
			}
			err := c.Create(ctx, req)
			if err == nil {
				DeferCleanup(func() { _ = client.IgnoreNotFound(k8sClient.Delete(ctx, req)) })
			}
			return err
		}
		dana := impersonate("oidc:dana", "grp:"+devsGroup, sreLeadsGroup)

		By("refusing the name with its prefix, once the API server enforces the policy")
		Eventually(func() error {
			return create(dana, protemv1alpha1.RequesterInfo{Username: "oidc:dana"})
		}, 60*time.Second, time.Second).Should(MatchError(ContainSubstring("must be your own username, dana")))

		By("accepting the caller's name and groups as the login to the UI would give them")
		Expect(create(dana, protemv1alpha1.RequesterInfo{Username: "dana", Email: "dana", Groups: []string{devsGroup}})).To(Succeed())

		By("refusing a group with its prefix, and a group the API server did not read from the token")
		Expect(create(dana, protemv1alpha1.RequesterInfo{Username: "dana", Groups: []string{"grp:" + devsGroup}})).
			To(MatchError(ContainSubstring("groups you belong to")))
		Expect(create(dana, protemv1alpha1.RequesterInfo{Username: "dana", Groups: []string{sreLeadsGroup}})).
			To(MatchError(ContainSubstring("groups you belong to")))

		By("refusing a caller whose name does not carry the prefix, so it cannot pass for a token user of the same name")
		Expect(create(impersonate("dana", "grp:"+devsGroup), protemv1alpha1.RequesterInfo{Username: "dana"})).
			To(MatchError(ContainSubstring("only users whose name starts with oidc:")))

		By("refusing a name or group that turns into one of Kubernetes' own once the prefix is removed")
		Expect(create(impersonate("oidc:system:serviceaccount:ci:deploy", "grp:"+devsGroup),
			protemv1alpha1.RequesterInfo{Username: "system:serviceaccount:ci:deploy"})).
			To(MatchError(ContainSubstring("may not start with system:")))
		Expect(create(impersonate("oidc:dana", "grp:"+devsGroup, "grp:system:masters"),
			protemv1alpha1.RequesterInfo{Username: "dana", Groups: []string{"system:masters"}})).
			To(MatchError(ContainSubstring("groups you belong to")))

		By("removing the policy, and waiting until the API server stops enforcing it, so other specs are unaffected")
		for _, obj := range policy {
			Expect(k8sClient.Delete(ctx, obj)).To(Succeed())
		}
		Eventually(func() error {
			return create(dana, protemv1alpha1.RequesterInfo{Username: aliceUsername})
		}, 60*time.Second, time.Second).Should(Succeed())
	})
})

func setPolicyVariable(policy *unstructured.Unstructured, name, expression string) {
	variables, found, err := unstructured.NestedSlice(policy.Object, "spec", "variables")
	Expect(err).NotTo(HaveOccurred())
	Expect(found).To(BeTrue(), "the policy declares no variables")
	set := false
	for _, v := range variables {
		if variable := v.(map[string]any); variable["name"] == name {
			variable["expression"] = expression
			set = true
		}
	}
	Expect(set).To(BeTrue(), "the policy has no variable %s", name)
	Expect(unstructured.SetNestedSlice(policy.Object, variables, "spec", "variables")).To(Succeed())
}
