package enforcer

import (
	"context"
	"strings"
	"testing"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
	"github.com/iLLeniumStudios/protem/internal/fleet"
)

const (
	spokeNS = "protem-cluster-spoke-1"
	devNs   = "dev"
)

func clusterGrant() *protemv1alpha1.ClusterGrant {
	return &protemv1alpha1.ClusterGrant{
		ObjectMeta: metav1.ObjectMeta{Name: "team-a.g1", Namespace: spokeNS},
		Spec: protemv1alpha1.ClusterGrantSpec{
			GrantRef:   "g1",
			Requester:  protemv1alpha1.RequesterInfo{Username: "alice", Email: "alice@company.com"},
			Role:       protemv1alpha1.TargetRoleRef{Kind: "Role", Name: "edit"},
			Scope:      protemv1alpha1.ProfileScopeNamespaced,
			Namespaces: []string{devNs},
			Subject:    "oidc:alice@company.com",
			ExpiresAt:  metav1.NewTime(time.Now().Add(time.Hour)),
		},
	}
}

func spokeBinding(name string, labels map[string]string) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: devNs, Labels: labels},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: "edit"},
		Subjects:   []rbacv1.Subject{{Kind: rbacv1.UserKind, Name: "someone"}},
	}
}

func enforcerLabels(grant string) map[string]string {
	return map[string]string{
		"protem.dev/managed-by": "protem-agent",
		"protem.dev/grant":      grant,
	}
}

func syncOnce(t *testing.T, grants []client.Object, spokeObjs ...client.Object) (hub, spoke client.Client) {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := protemv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	hub = fake.NewClientBuilder().WithScheme(scheme).WithObjects(grants...).WithStatusSubresource(&protemv1alpha1.ClusterGrant{}).Build()
	spoke = fake.NewClientBuilder().WithScheme(scheme).WithObjects(spokeObjs...).Build()

	agent := NewAgent(AgentConfig{ClusterName: "spoke-1", HubClient: hub, SpokeClient: spoke})
	if err := agent.SyncOnce(context.Background()); err != nil {
		t.Fatalf("SyncOnce: %v", err)
	}
	return hub, spoke
}

func hubStatus(t *testing.T, hub client.Client) protemv1alpha1.ClusterGrantStatus {
	t.Helper()
	var cg protemv1alpha1.ClusterGrant
	if err := hub.Get(context.Background(), client.ObjectKey{Namespace: spokeNS, Name: "team-a.g1"}, &cg); err != nil {
		t.Fatal(err)
	}
	return cg.Status
}

func spokeRoleBinding(t *testing.T, spoke client.Client, name string) (*rbacv1.RoleBinding, bool) {
	t.Helper()
	var rb rbacv1.RoleBinding
	err := spoke.Get(context.Background(), client.ObjectKey{Namespace: devNs, Name: name}, &rb)
	if err != nil && !apierrors.IsNotFound(err) {
		t.Fatal(err)
	}
	return &rb, err == nil
}

func TestSyncOnceAppliesActiveGrantAndReportsActive(t *testing.T) {
	hub, spoke := syncOnce(t, []client.Object{clusterGrant()}, editRole())

	rb, ok := spokeRoleBinding(t, spoke, "protem-grant-team-a.g1")
	if !ok {
		t.Fatal("spoke RoleBinding not created")
	}
	if len(rb.Subjects) != 1 || rb.Subjects[0].Name != "oidc:alice@company.com" {
		t.Fatalf("subjects = %v", rb.Subjects)
	}
	status := hubStatus(t, hub)
	if status.Phase != protemv1alpha1.ClusterGrantPhaseActive {
		t.Fatalf("phase = %q, want Active", status.Phase)
	}
	if len(status.Bindings) != 1 || status.Bindings[0] != (protemv1alpha1.ManagedBindingRef{Kind: "RoleBinding", Namespace: devNs, Name: "protem-grant-team-a.g1"}) {
		t.Fatalf("bindings = %v", status.Bindings)
	}
}

func TestSyncOnceRemovesRevokedGrantAndReportsRevoked(t *testing.T) {
	grant := clusterGrant()
	grant.Spec.Revocation = &protemv1alpha1.RevocationDetails{By: "admin", At: metav1.Now()}

	hub, spoke := syncOnce(t, []client.Object{grant}, spokeBinding("protem-grant-team-a.g1", enforcerLabels("team-a.g1")))

	if _, ok := spokeRoleBinding(t, spoke, "protem-grant-team-a.g1"); ok {
		t.Fatal("revoked grant's spoke RoleBinding still exists")
	}
	if phase := hubStatus(t, hub).Phase; phase != protemv1alpha1.ClusterGrantPhaseRevoked {
		t.Fatalf("phase = %q, want Revoked", phase)
	}
}

func TestSyncOnceReportsFailedForUnmanagedBinding(t *testing.T) {
	hub, spoke := syncOnce(t, []client.Object{clusterGrant()}, editRole(), spokeBinding("protem-grant-team-a.g1", nil))

	if phase := hubStatus(t, hub).Phase; phase != protemv1alpha1.ClusterGrantPhaseFailed {
		t.Fatalf("phase = %q, want Failed", phase)
	}
	rb, _ := spokeRoleBinding(t, spoke, "protem-grant-team-a.g1")
	if rb.Subjects[0].Name != "someone" {
		t.Fatalf("unmanaged binding was modified: %v", rb.Subjects)
	}
}

func TestSyncOnceReportsTheBindingsAFailedGrantStillHolds(t *testing.T) {
	grant := clusterRoleGrant("view")
	grant.Spec.Namespaces = []string{devNs, "ops"}
	foreign := spokeBinding("protem-grant-team-a.g1", nil)
	foreign.Namespace = "ops"
	hub, spoke := syncOnce(t, []client.Object{grant},
		&rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "view"}, Rules: viewRules}, foreign)

	status := hubStatus(t, hub)
	if status.Phase != protemv1alpha1.ClusterGrantPhaseFailed {
		t.Fatalf("phase = %q, want Failed", status.Phase)
	}
	_, devBound := spokeRoleBinding(t, spoke, "protem-grant-team-a.g1")
	reported := len(status.Bindings) == 1 && status.Bindings[0].Namespace == devNs
	if devBound != reported {
		t.Fatalf("binding in %s exists = %v, but status reports bindings %v", devNs, devBound, status.Bindings)
	}
}

func TestSyncOnceDoesNotRewriteAnUnchangedFailure(t *testing.T) {
	hub, spoke := syncOnce(t, []client.Object{clusterRoleGrant("missing")})
	version := func() string {
		var cg protemv1alpha1.ClusterGrant
		if err := hub.Get(context.Background(), client.ObjectKey{Namespace: spokeNS, Name: "team-a.g1"}, &cg); err != nil {
			t.Fatal(err)
		}
		return cg.ResourceVersion
	}
	first := version()

	agent := NewAgent(AgentConfig{ClusterName: "spoke-1", HubClient: hub, SpokeClient: spoke})
	if err := agent.SyncOnce(context.Background()); err != nil {
		t.Fatalf("second SyncOnce: %v", err)
	}
	if second := version(); second != first {
		t.Fatalf("second sync rewrote an unchanged Failed status: resourceVersion %s -> %s", first, second)
	}
}

func TestSyncOnceSweepsBindingWhoseGrantIsGone(t *testing.T) {
	_, spoke := syncOnce(t, nil, spokeBinding("protem-grant-gone", enforcerLabels("gone")))

	if _, ok := spokeRoleBinding(t, spoke, "protem-grant-gone"); ok {
		t.Fatal("orphaned spoke RoleBinding was not swept")
	}
}

func TestSyncOnceRefreshesStaleBindingsOfSameLength(t *testing.T) {
	grant := clusterGrant()
	grant.Status = protemv1alpha1.ClusterGrantStatus{
		Phase:    protemv1alpha1.ClusterGrantPhaseActive,
		Bindings: []protemv1alpha1.ManagedBindingRef{{Kind: "RoleBinding", Namespace: "prod", Name: "protem-grant-team-a.g1"}},
	}

	hub, _ := syncOnce(t, []client.Object{grant}, editRole())

	bindings := hubStatus(t, hub).Bindings
	if len(bindings) != 1 || bindings[0].Namespace != devNs {
		t.Fatalf("bindings = %v, want the dev RoleBinding", bindings)
	}
}

func editRole() *rbacv1.Role {
	return &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: "edit", Namespace: devNs}}
}

var viewRules = []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list"}}}

func clusterRoleGrant(name string) *protemv1alpha1.ClusterGrant {
	grant := clusterGrant()
	grant.Spec.Role = protemv1alpha1.TargetRoleRef{Kind: "ClusterRole", Name: name}
	return grant
}

func TestSyncOnceReportsClusterRoleRules(t *testing.T) {
	grant := clusterRoleGrant("view")
	grant.Status = protemv1alpha1.ClusterGrantStatus{
		Phase:    protemv1alpha1.ClusterGrantPhaseActive,
		Bindings: []protemv1alpha1.ManagedBindingRef{{Kind: "RoleBinding", Namespace: devNs, Name: "protem-grant-team-a.g1"}},
		Rules:    []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"get"}}},
	}
	view := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "view"}, Rules: viewRules}

	hub, _ := syncOnce(t, []client.Object{grant}, view)

	status := hubStatus(t, hub)
	if status.Phase != protemv1alpha1.ClusterGrantPhaseActive || !equality.Semantic.DeepEqual(status.Rules, viewRules) {
		t.Fatalf("status = %+v, want Active with the view rules", status)
	}
}

func TestSyncOnceFailsAndUnbindsWhenClusterRoleIsMissing(t *testing.T) {
	grant := clusterRoleGrant("ghost")
	grant.Status.Rules = viewRules

	hub, spoke := syncOnce(t, []client.Object{grant}, spokeBinding("protem-grant-team-a.g1", enforcerLabels("team-a.g1")))

	status := hubStatus(t, hub)
	if status.Phase != protemv1alpha1.ClusterGrantPhaseFailed || !strings.HasPrefix(status.Message, "RoleNotFound:") || !strings.Contains(status.Message, "ghost") {
		t.Fatalf("status = %+v, want Failed with RoleNotFound naming ghost", status)
	}
	if len(status.Rules) != 0 || len(status.Bindings) != 0 {
		t.Fatalf("rules = %v, bindings = %v, want both empty", status.Rules, status.Bindings)
	}
	if _, ok := spokeRoleBinding(t, spoke, "protem-grant-team-a.g1"); ok {
		t.Fatal("binding of a grant whose role is missing still exists")
	}
}

func TestSyncOnceFailsWhenRoleIsMissingInOneNamespace(t *testing.T) {
	grant := clusterGrant()
	grant.Spec.Namespaces = []string{devNs, "prod"}

	hub, spoke := syncOnce(t, []client.Object{grant}, editRole())

	status := hubStatus(t, hub)
	if status.Phase != protemv1alpha1.ClusterGrantPhaseFailed || !strings.HasPrefix(status.Message, "RoleNotFound:") || !strings.Contains(status.Message, "prod") {
		t.Fatalf("status = %+v, want Failed with RoleNotFound naming prod", status)
	}
	if _, ok := spokeRoleBinding(t, spoke, "protem-grant-team-a.g1"); ok {
		t.Fatal("binding created although the Role is missing in prod")
	}
}

func TestSyncOnceClearsRulesOfRevokedGrant(t *testing.T) {
	grant := clusterRoleGrant("view")
	grant.Spec.Revocation = &protemv1alpha1.RevocationDetails{By: "admin", At: metav1.Now()}
	grant.Status = protemv1alpha1.ClusterGrantStatus{Phase: protemv1alpha1.ClusterGrantPhaseActive, Rules: viewRules}

	hub, _ := syncOnce(t, []client.Object{grant})

	if rules := hubStatus(t, hub).Rules; len(rules) != 0 {
		t.Fatalf("rules = %v, want empty after revocation", rules)
	}
}

func TestSweepExpiredDeletesBindingsWithoutParsableExpiry(t *testing.T) {
	future := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	valid := spokeBinding("valid", enforcerLabels("g"))
	valid.Annotations = map[string]string{"protem.dev/expires-at": future}
	garbled := spokeBinding("garbled", enforcerLabels("g"))
	garbled.Annotations = map[string]string{"protem.dev/expires-at": "soon"}
	missing := spokeBinding("missing", enforcerLabels("g"))
	_, spoke := syncOnce(t, nil)
	for _, obj := range []client.Object{valid, garbled, missing} {
		if err := spoke.Create(context.Background(), obj); err != nil {
			t.Fatal(err)
		}
	}

	agent := NewAgent(AgentConfig{ClusterName: "spoke-1", SpokeClient: spoke})
	if err := agent.sweepExpired(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}

	if _, ok := spokeRoleBinding(t, spoke, "valid"); !ok {
		t.Fatal("binding with a future expiry was deleted")
	}
	for _, name := range []string{"garbled", "missing"} {
		if _, ok := spokeRoleBinding(t, spoke, name); ok {
			t.Fatalf("binding %q without a parsable expiry was kept", name)
		}
	}
}

func TestStartRenewsTheLeaseBeforeTheFirstTick(t *testing.T) {
	hub, spoke := syncOnce(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() {
		done <- NewAgent(AgentConfig{ClusterName: "spoke-1", HubClient: hub, SpokeClient: spoke, SyncInterval: time.Hour}).Start(ctx)
	}()
	defer func() {
		cancel()
		<-done
	}()

	if err := wait.PollUntilContextTimeout(ctx, 20*time.Millisecond, 2*time.Second, true, func(ctx context.Context) (bool, error) {
		return hub.Get(ctx, client.ObjectKey{Namespace: spokeNS, Name: fleet.AgentLeaseName}, &coordinationv1.Lease{}) == nil, nil
	}); err != nil {
		t.Fatalf("Lease not created at start: %v", err)
	}
}
