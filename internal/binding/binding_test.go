package binding_test

import (
	"context"
	"strings"
	"testing"
	"time"

	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
	"github.com/iLLeniumStudios/protem/internal/binding"
)

const (
	aliceEmail    = "alice@company.com"
	aliceUsername = "alice"
	roleKind      = "Role"
	devNs         = "dev"
	editRoleName  = "edit"
	grantG1Name   = "protem-grant-g1"
)

var expiry = time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)

func newClient(objs ...client.Object) client.Client {
	return fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).WithObjects(objs...).Build()
}

func namespaced(namespaces ...string) binding.Desired {
	return binding.Desired{
		Grant:      "g1",
		Subject:    aliceEmail,
		Role:       protemv1alpha1.TargetRoleRef{Kind: roleKind, Name: editRoleName},
		Scope:      protemv1alpha1.ProfileScopeNamespaced,
		Namespaces: namespaces,
		ExpiresAt:  expiry,
	}
}

func labeled(owner, grant string) map[string]string {
	return map[string]string{binding.LabelManagedBy: owner, binding.LabelGrant: grant}
}

func roleBinding(ns, name string, labels map[string]string, expiresAt string) *rbacv1.RoleBinding {
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name, Labels: labels},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: roleKind, Name: editRoleName},
	}
	if expiresAt != "" {
		rb.Annotations = map[string]string{binding.AnnotationExpiry: expiresAt}
	}
	return rb
}

func exists(t *testing.T, c client.Client, obj client.Object, ns, name string) bool {
	t.Helper()
	err := c.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: name}, obj)
	if err != nil && !apierrors.IsNotFound(err) {
		t.Fatalf("get %s/%s: %v", ns, name, err)
	}
	return err == nil
}

func TestApplyNamespacedCreatesRoleBindingPerNamespaceSkippingWildcard(t *testing.T) {
	c := newClient()

	refs, err := binding.Apply(context.Background(), c, "protem", namespaced(devNs, "*", "", "prod"))
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	want := []protemv1alpha1.ManagedBindingRef{
		{Kind: "RoleBinding", Namespace: devNs, Name: grantG1Name},
		{Kind: "RoleBinding", Namespace: "prod", Name: grantG1Name},
	}
	if len(refs) != len(want) || refs[0] != want[0] || refs[1] != want[1] {
		t.Fatalf("refs = %v, want %v", refs, want)
	}
	var list rbacv1.RoleBindingList
	if err := c.List(context.Background(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 2 {
		t.Fatalf("got %d RoleBindings, want 2", len(list.Items))
	}
	var rb rbacv1.RoleBinding
	if !exists(t, c, &rb, devNs, grantG1Name) {
		t.Fatal("RoleBinding dev/protem-grant-g1 missing")
	}
	if rb.Labels[binding.LabelManagedBy] != "protem" || rb.Labels[binding.LabelGrant] != "g1" {
		t.Fatalf("labels = %v", rb.Labels)
	}
	if rb.Annotations[binding.AnnotationExpiry] != "2030-01-02T03:04:05Z" {
		t.Fatalf("expiry annotation = %q", rb.Annotations[binding.AnnotationExpiry])
	}
	if rb.RoleRef != (rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: roleKind, Name: editRoleName}) {
		t.Fatalf("roleRef = %v", rb.RoleRef)
	}
	if len(rb.Subjects) != 1 || rb.Subjects[0] != (rbacv1.Subject{Kind: rbacv1.UserKind, APIGroup: rbacv1.GroupName, Name: aliceEmail}) {
		t.Fatalf("subjects = %v", rb.Subjects)
	}
}

func TestApplyClusterScopeCreatesClusterRoleBinding(t *testing.T) {
	c := newClient()
	d := namespaced(devNs)
	d.Scope = protemv1alpha1.ProfileScopeCluster
	d.Role = protemv1alpha1.TargetRoleRef{Kind: "ClusterRole", Name: "cluster-admin"}

	refs, err := binding.Apply(context.Background(), c, "protem-agent", d)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	if len(refs) != 1 || refs[0] != (protemv1alpha1.ManagedBindingRef{Kind: "ClusterRoleBinding", Name: grantG1Name}) {
		t.Fatalf("refs = %v", refs)
	}
	var crb rbacv1.ClusterRoleBinding
	if !exists(t, c, &crb, "", grantG1Name) {
		t.Fatal("ClusterRoleBinding missing")
	}
	if crb.RoleRef.Kind != "ClusterRole" || crb.RoleRef.Name != "cluster-admin" || crb.Labels[binding.LabelManagedBy] != "protem-agent" {
		t.Fatalf("crb = %+v", crb)
	}
	if exists(t, c, &rbacv1.RoleBinding{}, devNs, grantG1Name) {
		t.Fatal("cluster scope must not create RoleBindings")
	}
}

func TestApplyRefusesToAdoptForeignBinding(t *testing.T) {
	for name, labels := range map[string]map[string]string{
		"unlabeled":     nil,
		"other owner":   labeled("protem-agent", "g1"),
		"other grant":   labeled("protem", "g2"),
		"missing grant": {binding.LabelManagedBy: "protem"},
	} {
		t.Run(name, func(t *testing.T) {
			existing := roleBinding(devNs, grantG1Name, labels, "")
			existing.Subjects = []rbacv1.Subject{{Kind: rbacv1.UserKind, Name: "intruder@evil.com"}}
			c := newClient(existing)

			_, err := binding.Apply(context.Background(), c, "protem", namespaced(devNs))
			if err == nil || !strings.Contains(err.Error(), "dev/protem-grant-g1") {
				t.Fatalf("err = %v, want refusal naming dev/protem-grant-g1", err)
			}

			var rb rbacv1.RoleBinding
			exists(t, c, &rb, devNs, grantG1Name)
			if rb.Subjects[0].Name != "intruder@evil.com" {
				t.Fatalf("foreign binding was modified: %v", rb.Subjects)
			}
		})
	}
}

func TestApplyCorrectsDriftedSubject(t *testing.T) {
	drifted := roleBinding(devNs, grantG1Name, labeled("protem", "g1"), "")
	drifted.Subjects = []rbacv1.Subject{
		{Kind: rbacv1.UserKind, Name: aliceEmail},
		{Kind: rbacv1.UserKind, Name: "attacker@evil.com"},
	}
	c := newClient(drifted)

	if _, err := binding.Apply(context.Background(), c, "protem", namespaced(devNs)); err != nil {
		t.Fatalf("apply: %v", err)
	}

	var rb rbacv1.RoleBinding
	exists(t, c, &rb, devNs, grantG1Name)
	if len(rb.Subjects) != 1 || rb.Subjects[0].Name != aliceEmail {
		t.Fatalf("subjects = %v, want only alice@company.com", rb.Subjects)
	}
	if rb.Annotations[binding.AnnotationExpiry] != "2030-01-02T03:04:05Z" {
		t.Fatalf("expiry annotation = %q", rb.Annotations[binding.AnnotationExpiry])
	}
}

func TestRemoveDeletesOnlyThatGrantsBindingsForOwner(t *testing.T) {
	c := newClient(
		roleBinding(devNs, grantG1Name, labeled("protem", "g1"), ""),
		roleBinding("prod", grantG1Name, labeled("protem", "g1"), ""),
		&rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: grantG1Name, Labels: labeled("protem", "g1")}},
		roleBinding(devNs, "protem-grant-g2", labeled("protem", "g2"), ""),
		roleBinding("qa", grantG1Name, labeled("protem-agent", "g1"), ""),
		roleBinding("qa", "unmanaged", nil, ""),
	)

	if err := binding.Remove(context.Background(), c, "protem", "", "g1"); err != nil {
		t.Fatalf("remove: %v", err)
	}

	for _, gone := range []client.ObjectKey{{Namespace: devNs, Name: grantG1Name}, {Namespace: "prod", Name: grantG1Name}} {
		if exists(t, c, &rbacv1.RoleBinding{}, gone.Namespace, gone.Name) {
			t.Errorf("%s should be deleted", gone)
		}
	}
	if exists(t, c, &rbacv1.ClusterRoleBinding{}, "", grantG1Name) {
		t.Error("ClusterRoleBinding should be deleted")
	}
	for _, kept := range []client.ObjectKey{{Namespace: devNs, Name: "protem-grant-g2"}, {Namespace: "qa", Name: grantG1Name}, {Namespace: "qa", Name: "unmanaged"}} {
		if !exists(t, c, &rbacv1.RoleBinding{}, kept.Namespace, kept.Name) {
			t.Errorf("%s should be kept", kept)
		}
	}
}

func TestSameGrantNameInOtherNamespacesCoexist(t *testing.T) {
	c := newClient()
	teamA, teamB := namespaced(devNs), namespaced(devNs)
	teamA.GrantNamespace, teamB.GrantNamespace = "team-a", "team-b"
	teamB.Subject = "bob@company.com"

	refsA, err := binding.Apply(context.Background(), c, "protem", teamA)
	if err != nil {
		t.Fatalf("apply team-a: %v", err)
	}
	refsB, err := binding.Apply(context.Background(), c, "protem", teamB)
	if err != nil {
		t.Fatalf("apply team-b: %v", err)
	}
	if len(refsA) != 1 || len(refsB) != 1 || refsA[0].Name == refsB[0].Name {
		t.Fatalf("refs team-a = %v, team-b = %v, want one distinct binding each", refsA, refsB)
	}

	if err := binding.Remove(context.Background(), c, "protem", "team-b", "g1"); err != nil {
		t.Fatalf("remove: %v", err)
	}

	var rb rbacv1.RoleBinding
	if !exists(t, c, &rb, devNs, refsA[0].Name) {
		t.Fatal("team-a's binding was removed by team-b's Remove")
	}
	if rb.Labels[binding.LabelGrantNamespace] != "team-a" || rb.Subjects[0].Name != aliceEmail {
		t.Fatalf("team-a's binding = %+v", rb)
	}
	if exists(t, c, &rbacv1.RoleBinding{}, devNs, refsB[0].Name) {
		t.Fatal("team-b's binding should be deleted")
	}
}

func TestSweepDeletesExpiredAndKeepsUnexpiredOrUnannotated(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := newClient(
		roleBinding(devNs, "expired", labeled("protem-agent", "g1"), "2025-12-31T23:00:00Z"),
		roleBinding(devNs, "live", labeled("protem-agent", "g2"), "2026-01-01T01:00:00Z"),
		roleBinding(devNs, "unannotated", labeled("protem-agent", "g3"), ""),
		roleBinding(devNs, "garbled", labeled("protem-agent", "g4"), "not-a-time"),
		roleBinding(devNs, "foreign-expired", labeled("protem", "g5"), "2025-12-31T23:00:00Z"),
		&rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{
			Name:        "expired-crb",
			Labels:      labeled("protem-agent", "g6"),
			Annotations: map[string]string{binding.AnnotationExpiry: "2025-12-31T23:00:00Z"},
		}},
	)
	var seen []string
	keep := func(grant string, expiresAt time.Time) bool {
		seen = append(seen, grant)
		return expiresAt.IsZero() || now.Before(expiresAt)
	}

	if err := binding.Sweep(context.Background(), c, "protem-agent", keep); err != nil {
		t.Fatalf("sweep: %v", err)
	}

	if len(seen) != 5 {
		t.Errorf("keep consulted for grants %v, want the 5 owned bindings", seen)
	}
	for name, want := range map[string]bool{"expired": false, "live": true, "unannotated": true, "garbled": true, "foreign-expired": true} {
		if got := exists(t, c, &rbacv1.RoleBinding{}, devNs, name); got != want {
			t.Errorf("RoleBinding %s exists = %v, want %v", name, got, want)
		}
	}
	if exists(t, c, &rbacv1.ClusterRoleBinding{}, "", "expired-crb") {
		t.Error("expired ClusterRoleBinding should be deleted")
	}
}

func TestFormatSubject(t *testing.T) {
	alice := protemv1alpha1.RequesterInfo{Username: aliceUsername, Email: aliceEmail}
	for _, tc := range []struct {
		name, tmpl string
		r          protemv1alpha1.RequesterInfo
		want       string
	}{
		{"default template is email", "", alice, aliceEmail},
		{"prefixed email", "oidc:{{.Email}}", alice, "oidc:alice@company.com"},
		{"username", "{{.Username}}", alice, aliceUsername},
		{"empty email falls back to username", "{{.Email}}", protemv1alpha1.RequesterInfo{Username: aliceUsername}, aliceUsername},
		{"template without placeholders falls back to email", "static", alice, aliceEmail},
		{"template without placeholders and no email falls back to username", "static", protemv1alpha1.RequesterInfo{Username: aliceUsername}, aliceUsername},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := binding.FormatSubject(tc.tmpl, tc.r); got != tc.want {
				t.Fatalf("FormatSubject(%q) = %q, want %q", tc.tmpl, got, tc.want)
			}
		})
	}
}
