package api

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
	"github.com/iLLeniumStudios/protem/internal/approval"
)

func TestProfileActivation(t *testing.T) {
	const oncall = "oncall"
	cluster := func(name, env string, ready bool) protemv1alpha1.TargetCluster {
		c := protemv1alpha1.TargetCluster{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{"env": env}}}
		if ready {
			c.Status.Conditions = []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}}
		}
		return c
	}
	ready := []protemv1alpha1.TargetCluster{cluster("prod-1", "prod", true), cluster("dev-1", "dev", true)}
	notReady := []protemv1alpha1.TargetCluster{cluster("prod-1", "prod", false), cluster("dev-1", "dev", false)}
	envIs := func(env string) *protemv1alpha1.ApprovalRuleWhen {
		return &protemv1alpha1.ApprovalRuleWhen{ClusterSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": env}}}
	}
	profile := func(rules ...protemv1alpha1.ApprovalRule) *protemv1alpha1.AccessProfile {
		return &protemv1alpha1.AccessProfile{
			ObjectMeta: metav1.ObjectMeta{Namespace: teamA, Name: "p"},
			Spec: protemv1alpha1.AccessProfileSpec{
				Role:       protemv1alpha1.TargetRoleRef{Kind: "ClusterRole", Name: "view"},
				Clusters:   protemv1alpha1.ClusterTargeting{Names: []string{"prod-1", "dev-1"}},
				Requesters: protemv1alpha1.ProfileRequesters{Groups: []string{devsGroup}},
				Approval:   protemv1alpha1.ProfileApprovalConfig{Rules: rules},
			},
		}
	}
	auto := protemv1alpha1.ApprovalRule{Name: "auto", Mode: protemv1alpha1.ApprovalModeAuto}
	byUsers := protemv1alpha1.ApprovalRule{Name: "users", Mode: protemv1alpha1.ApprovalModeManual,
		Approvers: &protemv1alpha1.ApprovalApprovers{Users: []string{bobEmail}}}
	oncallAuto := protemv1alpha1.ApprovalRule{Name: oncall, Mode: protemv1alpha1.ApprovalModeAuto,
		When: &protemv1alpha1.ApprovalRuleWhen{RequesterGroups: []string{oncall}}}
	prodManual := protemv1alpha1.ApprovalRule{Name: "prod", Mode: protemv1alpha1.ApprovalModeManual, When: envIs("prod"),
		Approvers: &protemv1alpha1.ApprovalApprovers{Groups: []string{sreLeadsGroup}, Users: []string{bobEmail}}}
	defaultAuto := protemv1alpha1.ApprovalRule{Name: "default", Mode: protemv1alpha1.ApprovalModeAuto,
		Approvers: &protemv1alpha1.ApprovalApprovers{Groups: []string{sreLeadsGroup, devsGroup}, Users: []string{bobEmail}}}
	devOnly := protemv1alpha1.ApprovalRule{Name: "dev", Mode: protemv1alpha1.ApprovalModeAuto, When: envIs("dev")}

	dev := approval.Person{Email: aliceEmail, Groups: []string{devsGroup}}
	devOncall := approval.Person{Email: aliceEmail, Groups: []string{devsGroup, oncall}}
	oncallOnly := approval.Person{Email: eveEmail, Groups: []string{oncall}}
	for _, tc := range []struct {
		name       string
		profile    *protemv1alpha1.AccessProfile
		person     approval.Person
		spokes     []protemv1alpha1.TargetCluster
		want       string
		wantGroups []string
		wantUsers  []string
	}{
		{"single automatic rule", profile(auto), dev, ready, "at-once", nil, nil},
		{"single manual rule naming only users", profile(byUsers), dev, ready, "approval", nil, []string{bobEmail}},
		{"manual on prod, automatic elsewhere", profile(oncallAuto, prodManual, defaultAuto), dev, ready, "mixed",
			[]string{sreLeadsGroup, devsGroup}, []string{bobEmail}},
		{"caller's group matches the automatic rule first", profile(oncallAuto, prodManual, defaultAuto), devOncall, ready, "at-once",
			[]string{sreLeadsGroup, devsGroup}, []string{bobEmail}},
		{"a cluster no rule matches needs approval", profile(devOnly), dev, ready, "mixed", nil, nil},
		{"caller not a requester: the rules alone decide", profile(oncallAuto, prodManual, defaultAuto), oncallOnly, ready, "mixed",
			[]string{sreLeadsGroup, devsGroup}, []string{bobEmail}},
		{"no ready cluster: the rules alone decide", profile(oncallAuto, prodManual, defaultAuto), devOncall, notReady, "mixed",
			[]string{sreLeadsGroup, devsGroup}, []string{bobEmail}},
		{"no ready cluster, every rule automatic", profile(auto, devOnly), dev, notReady, "at-once", nil, nil},
		{"no ready cluster, no rule automatic", profile(byUsers), dev, notReady, "approval", nil, []string{bobEmail}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := toProfileView(tc.profile, tc.person, tc.spokes)
			if v.Activation != tc.want || !slices.Equal(v.ApproverGroups, tc.wantGroups) || !slices.Equal(v.ApproverUsers, tc.wantUsers) {
				t.Errorf("activation %q, approver groups %q, users %q; want %q, %q, %q",
					v.Activation, v.ApproverGroups, v.ApproverUsers, tc.want, tc.wantGroups, tc.wantUsers)
			}
		})
	}
}

func TestRequestsCountFlaggedRulesOfTheirRole(t *testing.T) {
	clusterRole := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "admin-ish"}, Rules: []rbacv1.PolicyRule{
		{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"get"}},
		{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"*"}},
		{APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"get"}},
	}}
	role := &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Namespace: productionNs, Name: "ops"}, Rules: []rbacv1.PolicyRule{
		{APIGroups: []string{""}, Resources: []string{"pods/exec"}, Verbs: []string{"create"}},
		{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"list"}},
	}}
	profile := &protemv1alpha1.AccessProfile{ObjectMeta: metav1.ObjectMeta{Namespace: teamA, Name: "db-admin"}, Spec: validSpec()}
	profile.Spec.Role = protemv1alpha1.TargetRoleRef{Kind: roleKind, Name: "ops"}
	resolved := func(name, roleName string) *protemv1alpha1.AccessRequest {
		req := historyRequest(teamA, name, aliceEmail, protemv1alpha1.RequestPhasePending, devsGroup)
		req.Status.Resolved.Role = &protemv1alpha1.TargetRoleRef{Kind: "ClusterRole", Name: roleName}
		return req
	}
	gets := map[string]int{}
	c := fake.NewClientBuilder().WithScheme(setupTestScheme(t)).WithObjects(
		clusterRole, role, profile,
		resolved("first", "admin-ish"), resolved("second", "admin-ish"), resolved("hidden", "secret-role"),
		historyRequest(teamA, "unresolved-1", aliceEmail, protemv1alpha1.RequestPhasePending, ""),
		historyRequest(teamA, "unresolved-2", aliceEmail, protemv1alpha1.RequestPhasePending, ""),
	).WithInterceptorFuncs(interceptor.Funcs{Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
		switch obj.(type) {
		case *rbacv1.ClusterRole, *rbacv1.Role:
			gets[key.Name]++
			if key.Name == "secret-role" {
				return apierrors.NewForbidden(schema.GroupResource{Group: rbacv1.GroupName, Resource: "clusterroles"}, key.Name, nil)
			}
		}
		return c.Get(ctx, key, obj, opts...)
	}}).Build()
	s, err := NewServer(c, ServerOptions{Addr: testAddr, DevAuth: true})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	items, _ := decodePage[RequestView](t, s, "/api/requests", aliceEmail)
	got := map[string]int{}
	for _, v := range items {
		got[v.Name] = v.FlaggedRules
	}
	want := map[string]int{"first": 2, "second": 2, "hidden": 0, "unresolved-1": 1, "unresolved-2": 1}
	if len(got) != len(want) {
		t.Fatalf("flagged rules = %v, want %v", got, want)
	}
	for name, n := range want {
		if got[name] != n {
			t.Errorf("%s: flagged rules = %d, want %d", name, got[name], n)
		}
	}
	for name, n := range gets {
		if n != 1 {
			t.Errorf("role %s read %d times in one list, want once", name, n)
		}
	}

	w := call(s, http.MethodGet, "/api/namespaces/team-a/requests/hidden", aliceEmail, "")
	if w.Code != http.StatusOK {
		t.Errorf("GET a request whose role the hub cannot read = %d, want 200: %s", w.Code, w.Body.String())
	}
	var v RequestView
	if w := call(s, http.MethodGet, "/api/namespaces/team-a/requests/first", aliceEmail, ""); json.Unmarshal(w.Body.Bytes(), &v) != nil || v.FlaggedRules != 2 {
		t.Errorf("GET one request: flagged rules = %d, want 2: %s", v.FlaggedRules, w.Body.String())
	}
}
