package enforcer

import (
	"context"
	"testing"
	"time"

	authorizationv1 "k8s.io/api/authorization/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/iLLeniumStudios/protem/internal/fleet"
)

// shippedCeiling is the bind rule's resourceNames in config/agent/spoke/cluster_role.yaml.
const shippedCeiling = "edit,view"

func TestAllowedRolesAnnotation(t *testing.T) {
	bind := func(groups, verbs, resources, names []string) authorizationv1.ResourceRule {
		return authorizationv1.ResourceRule{APIGroups: groups, Verbs: verbs, Resources: resources, ResourceNames: names}
	}
	rbac := []string{rbacv1.GroupName}
	roles := []string{"roles", "clusterroles"}
	for _, tc := range []struct {
		name  string
		rules []authorizationv1.ResourceRule
		want  string
	}{
		{"named roles from two rules are merged and sorted", []authorizationv1.ResourceRule{
			bind(rbac, []string{"bind"}, roles, []string{"view", "edit"}),
			bind(rbac, []string{"get", "bind"}, []string{"clusterroles"}, []string{"db-operator", "edit"}),
		}, "db-operator,edit,view"},
		{"wildcard verb", []authorizationv1.ResourceRule{bind(rbac, []string{"*"}, []string{"roles"}, []string{"view"})}, "view"},
		{"wildcard resource", []authorizationv1.ResourceRule{bind(rbac, []string{"bind"}, []string{"*"}, []string{"view"})}, "view"},
		{"no resourceNames means any role", []authorizationv1.ResourceRule{
			bind(rbac, []string{"bind"}, roles, []string{"view"}),
			bind(rbac, []string{"bind"}, []string{"clusterroles"}, nil),
		}, "*"},
		{"cluster-admin binds any role", []authorizationv1.ResourceRule{bind([]string{"*"}, []string{"*"}, []string{"*"}, nil)}, "*"},
		{"other API group is ignored", []authorizationv1.ResourceRule{bind([]string{"apps"}, []string{"bind"}, roles, nil)}, ""},
		{"bind on an unrelated resource is ignored", []authorizationv1.ResourceRule{bind(rbac, []string{"bind"}, []string{"rolebindings"}, nil)}, ""},
		{"read-only rules are ignored", []authorizationv1.ResourceRule{bind(rbac, []string{"get", "list"}, roles, nil)}, ""},
		{"nothing", nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := allowedRolesAnnotation(tc.rules); got != tc.want {
				t.Errorf("allowedRolesAnnotation = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLeaseReportsTheRolesTheTargetLetsTheAgentBind(t *testing.T) {
	hub, _ := syncOnce(t, nil)
	reviews := 0
	incomplete, withRules := false, true
	spoke := fake.NewClientBuilder().WithScheme(hub.Scheme()).WithInterceptorFuncs(interceptor.Funcs{
		Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			review, ok := obj.(*authorizationv1.SelfSubjectRulesReview)
			if !ok {
				return c.Create(ctx, obj, opts...)
			}
			reviews++
			review.Status = authorizationv1.SubjectRulesReviewStatus{Incomplete: incomplete}
			if withRules {
				review.Status.ResourceRules = []authorizationv1.ResourceRule{{
					APIGroups: []string{rbacv1.GroupName}, Verbs: []string{"bind"},
					Resources: []string{"roles", "clusterroles"}, ResourceNames: []string{"view", "edit"},
				}}
			}
			return nil
		},
	}).Build()
	agent := NewAgent(AgentConfig{ClusterName: "spoke-1", HubClient: hub, SpokeClient: spoke})
	annotations := func() (roles, partial string) {
		t.Helper()
		var lease coordinationv1.Lease
		if err := hub.Get(t.Context(), client.ObjectKey{Namespace: spokeNS, Name: fleet.AgentLeaseName}, &lease); err != nil {
			t.Fatal(err)
		}
		return lease.Annotations[fleet.AllowedRolesAnnotation], lease.Annotations[fleet.AllowedRolesPartialAnnotation]
	}
	renew := func() {
		t.Helper()
		if err := agent.renewLease(t.Context()); err != nil {
			t.Fatalf("renewLease: %v", err)
		}
	}
	expect := func(step, wantRoles, wantPartial string, wantReviews int) {
		t.Helper()
		if roles, partial := annotations(); roles != wantRoles || partial != wantPartial || reviews != wantReviews {
			t.Fatalf("%s: allowed roles %q, partial %q after %d reviews; want %q, %q after %d",
				step, roles, partial, reviews, wantRoles, wantPartial, wantReviews)
		}
	}
	recheck := func() {
		agent.rolesCheckedAt = agent.rolesCheckedAt.Add(-time.Minute)
		renew()
	}

	renew()
	expect("complete review", shippedCeiling, "", 1)

	incomplete = true
	renew()
	expect("within a minute", shippedCeiling, "", 1)

	recheck()
	expect("incomplete review with rules", shippedCeiling, "true", 2)

	incomplete = false
	recheck()
	expect("complete review after a partial one", shippedCeiling, "", 3)

	incomplete = true
	recheck()
	expect("incomplete review with rules again", shippedCeiling, "true", 4)

	withRules = false
	recheck()
	expect("incomplete review without rules", "", "", 5)
}
