package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	authorizationv1 "k8s.io/api/authorization/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
)

const (
	teamA       = "team-a"
	teamB       = "team-b"
	eveEmail    = "eve@company.com"
	lastModKey  = "protem.dev/last-modified-by"
	argoTracker = "argocd.argoproj.io/tracking-id"
	sourceHub   = "hub"
)

type sarPolicy func(user string, a authorizationv1.ResourceAttributes) bool

type sarRule struct{ user, verb, resource, namespace, name string }

func allowAll(string, authorizationv1.ResourceAttributes) bool { return true }
func denyAll(string, authorizationv1.ResourceAttributes) bool  { return false }

func allowOnly(rules ...sarRule) sarPolicy {
	return func(user string, a authorizationv1.ResourceAttributes) bool {
		return slices.Contains(rules, sarRule{user, a.Verb, a.Resource, a.Namespace, a.Name})
	}
}

func newMgmtServer(t *testing.T, policy sarPolicy, opt ServerOptions, objs ...client.Object) (*Server, client.Client) {
	t.Helper()
	c := fake.NewClientBuilder().WithScheme(setupTestScheme(t)).WithObjects(objs...).WithStatusSubresource(&protemv1alpha1.AccessRequest{}).
		WithInterceptorFuncs(interceptor.Funcs{Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			if sar, ok := obj.(*authorizationv1.SubjectAccessReview); ok {
				sar.Status.Allowed = policy(sar.Spec.User, *sar.Spec.ResourceAttributes)
				return nil
			}
			return c.Create(ctx, obj, opts...)
		}}).Build()
	opt.Addr, opt.DevAuth = testAddr, true
	s, err := NewServer(c, opt)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return s, c
}

func call(s *Server, method, path, as, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("X-Test-User", as)
	w := httptest.NewRecorder()
	s.Mux.ServeHTTP(w, r)
	return w
}

func validSpec() protemv1alpha1.AccessProfileSpec {
	return protemv1alpha1.AccessProfileSpec{
		DisplayName: "Team A view",
		Role:        protemv1alpha1.TargetRoleRef{Kind: "ClusterRole", Name: "view"},
		Scope:       protemv1alpha1.ProfileScopeNamespaced,
		Namespaces:  []string{productionNs},
		Clusters:    protemv1alpha1.ClusterTargeting{Names: []string{clusterA}},
		Duration:    protemv1alpha1.ProfileDurationConfig{Default: metav1.Duration{Duration: time.Hour}, Max: metav1.Duration{Duration: 2 * time.Hour}},
		Requesters:  protemv1alpha1.ProfileRequesters{Groups: []string{devsGroup}},
		Approval:    protemv1alpha1.ProfileApprovalConfig{Rules: []protemv1alpha1.ApprovalRule{{Name: "auto", Mode: protemv1alpha1.ApprovalModeAuto}}},
	}
}

func profileBody(name, resourceVersion, displayName string) string {
	spec := validSpec()
	spec.DisplayName = displayName
	b, _ := json.Marshal(ProfilePayload{Name: name, ResourceVersion: resourceVersion, Spec: spec})
	return string(b)
}

func testProfile(ns, name string, labels, annotations map[string]string) *protemv1alpha1.AccessProfile {
	return &protemv1alpha1.AccessProfile{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name, Labels: labels, Annotations: annotations},
		Spec:       validSpec(),
	}
}

func testCluster(labels, annotations map[string]string) *protemv1alpha1.TargetCluster {
	return &protemv1alpha1.TargetCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Labels: labels, Annotations: annotations},
		Spec:       protemv1alpha1.TargetClusterSpec{DisplayName: "c1", Subject: "{{.Email}}"},
	}
}

type objectState struct {
	exists          bool
	resourceVersion string
}

func stateOf(t *testing.T, c client.Client, key client.ObjectKey, obj client.Object) objectState {
	t.Helper()
	err := c.Get(context.Background(), key, obj)
	if apierrors.IsNotFound(err) {
		return objectState{}
	}
	if err != nil {
		t.Fatalf("get %v: %v", key, err)
	}
	return objectState{exists: true, resourceVersion: obj.GetResourceVersion()}
}

type writeCase struct {
	name, method, path, body string
	key                      client.ObjectKey
	obj                      func() client.Object
	sar                      sarRule
	success                  int
}

func writeCases(as string) []writeCase {
	profile := func() client.Object { return &protemv1alpha1.AccessProfile{} }
	cluster := func() client.Object { return &protemv1alpha1.TargetCluster{} }
	return []writeCase{
		{"create", http.MethodPost, "/api/namespaces/team-a/profiles", profileBody("new", "", "New"),
			client.ObjectKey{Namespace: teamA, Name: "new"}, profile, sarRule{as, "create", "accessprofiles", teamA, ""}, http.StatusCreated},
		{"update", http.MethodPut, "/api/namespaces/team-a/profiles/p1", profileBody("", "999", "Renamed"),
			client.ObjectKey{Namespace: teamA, Name: "p1"}, profile, sarRule{as, "update", "accessprofiles", teamA, "p1"}, http.StatusOK},
		{"delete", http.MethodDelete, "/api/namespaces/team-a/profiles/p1", "",
			client.ObjectKey{Namespace: teamA, Name: "p1"}, profile, sarRule{as, "delete", "accessprofiles", teamA, "p1"}, http.StatusNoContent},
		{"patch cluster", http.MethodPatch, "/api/clusters/c1", `{"paused":true}`,
			client.ObjectKey{Name: "c1"}, cluster, sarRule{as, "update", "targetclusters", "", "c1"}, http.StatusOK},
	}
}

func TestManagement_WriteRoutesOnlyWhenEnabled(t *testing.T) {
	if _, err := NewServer(fake.NewClientBuilder().WithScheme(setupTestScheme(t)).Build(), ServerOptions{UIManagement: "yes"}); err == nil {
		t.Fatal("expected an unknown --ui-management value to fail startup")
	}
	for _, mode := range []string{"", "readonly", "enabled"} {
		for _, tc := range writeCases(aliceEmail) {
			s, c := newMgmtServer(t, allowOnly(tc.sar), ServerOptions{UIManagement: mode},
				testProfile(teamA, "p1", nil, nil), testCluster(nil, nil))
			before := stateOf(t, c, tc.key, tc.obj())
			w := call(s, tc.method, tc.path, aliceEmail, tc.body)
			if mode != "enabled" {
				if w.Code != http.StatusNotFound {
					t.Errorf("mode %q %s: expected 404, got %d: %s", mode, tc.name, w.Code, w.Body.String())
				}
				if after := stateOf(t, c, tc.key, tc.obj()); after != before {
					t.Errorf("mode %q %s: object changed from %+v to %+v", mode, tc.name, before, after)
				}
				continue
			}
			if w.Code != tc.success {
				t.Errorf("enabled %s: expected %d, got %d: %s", tc.name, tc.success, w.Code, w.Body.String())
			}
			if after := stateOf(t, c, tc.key, tc.obj()); after == before {
				t.Errorf("enabled %s: expected the object to change, still %+v", tc.name, after)
			}
		}
	}
}

func TestManagement_WritesRefusedWhenSARDenied(t *testing.T) {
	for _, tc := range writeCases(aliceEmail) {
		s, c := newMgmtServer(t, denyAll, ServerOptions{UIManagement: "enabled"},
			testProfile(teamA, "p1", nil, nil), testCluster(nil, nil))
		before := stateOf(t, c, tc.key, tc.obj())
		if w := call(s, tc.method, tc.path, aliceEmail, tc.body); w.Code != http.StatusForbidden {
			t.Errorf("%s: expected 403, got %d: %s", tc.name, w.Code, w.Body.String())
		}
		if after := stateOf(t, c, tc.key, tc.obj()); after != before {
			t.Errorf("%s: object changed from %+v to %+v", tc.name, before, after)
		}
	}
}

func TestManagement_GitOpsManagedObjectsRefused(t *testing.T) {
	for _, owner := range []struct {
		want                string
		labels, annotations map[string]string
	}{
		{"Argo CD", nil, map[string]string{argoTracker: "app:protem/AccessProfile:team-a/p1"}},
		{"Argo CD", map[string]string{"argocd.argoproj.io/instance": "protem"}, nil},
		{"Flux", map[string]string{"kustomize.toolkit.fluxcd.io/name": "protem"}, nil},
		{"platform-team", map[string]string{"protem.dev/managed-by": "platform-team"}, nil},
	} {
		for _, tc := range writeCases(aliceEmail)[1:] {
			s, c := newMgmtServer(t, allowAll, ServerOptions{UIManagement: "enabled"},
				testProfile(teamA, "p1", owner.labels, owner.annotations), testCluster(owner.labels, owner.annotations))
			before := stateOf(t, c, tc.key, tc.obj())
			w := call(s, tc.method, tc.path, aliceEmail, tc.body)
			if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), owner.want) || !strings.Contains(w.Body.String(), "Git") {
				t.Errorf("%s %s: expected 409 naming %q and Git, got %d: %s", owner.want, tc.name, owner.want, w.Code, w.Body.String())
			}
			if after := stateOf(t, c, tc.key, tc.obj()); after != before {
				t.Errorf("%s %s: object changed from %+v to %+v", owner.want, tc.name, before, after)
			}
		}
	}
}

func TestManagement_UpdateProfileNeedsCurrentResourceVersion(t *testing.T) {
	s, c := newMgmtServer(t, allowAll, ServerOptions{UIManagement: "enabled"}, testProfile(teamA, "p1", nil, nil))
	for _, tc := range []struct {
		body string
		want int
	}{
		{profileBody("", "1", "Stale"), http.StatusConflict},
		{profileBody("", "", "Blind"), http.StatusBadRequest},
		{`{"resourceVersion":"999","spec":{},"metadata":{}}`, http.StatusBadRequest},
	} {
		if w := call(s, http.MethodPut, "/api/namespaces/team-a/profiles/p1", aliceEmail, tc.body); w.Code != tc.want {
			t.Errorf("%s: expected %d, got %d: %s", tc.body, tc.want, w.Code, w.Body.String())
		}
	}
	var stored protemv1alpha1.AccessProfile
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: teamA, Name: "p1"}, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Spec.DisplayName != validSpec().DisplayName || stored.ResourceVersion != "999" {
		t.Fatalf("expected the profile unchanged, got %q rv %s", stored.Spec.DisplayName, stored.ResourceVersion)
	}
}

func TestManagement_PatchClusterValidatesSubjectAndLabels(t *testing.T) {
	reserved := map[string]string{"protem.dev/region": "eu", "app.kubernetes.io/part-of": "fleet"}
	labels := map[string]string{"env": "dev"}
	for k, v := range reserved {
		labels[k] = v
	}
	s, c := newMgmtServer(t, allowAll, ServerOptions{UIManagement: "enabled"}, testCluster(labels, nil))
	for _, tc := range []struct {
		body string
		want int
	}{
		{`{"subject":"shared-admin"}`, http.StatusBadRequest},
		{`{"labels":{"protem.dev/team":"x"}}`, http.StatusBadRequest},
		{`{"labels":{"helm.toolkit.fluxcd.io/name":"x"}}`, http.StatusBadRequest},
		{`{"paused":true,"enforcement":"Authorizer"}`, http.StatusBadRequest},
		{`{"subject":"oidc:{{.Username}}","labels":{"env":"prod","tier":"1"}}`, http.StatusOK},
	} {
		if w := call(s, http.MethodPatch, "/api/clusters/c1", aliceEmail, tc.body); w.Code != tc.want {
			t.Errorf("%s: expected %d, got %d: %s", tc.body, tc.want, w.Code, w.Body.String())
		}
	}
	var stored protemv1alpha1.TargetCluster
	if err := c.Get(context.Background(), client.ObjectKey{Name: "c1"}, &stored); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"env": "prod", "tier": "1"}
	for k, v := range reserved {
		want[k] = v
	}
	if !mapsEqual(stored.Labels, want) {
		t.Errorf("expected labels %v, got %v", want, stored.Labels)
	}
	if stored.Spec.Subject != "oidc:{{.Username}}" || stored.Spec.Paused {
		t.Errorf("expected only the subject changed, got %+v", stored.Spec)
	}
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

func TestManagement_WritesStampAuthenticatedUser(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	s, c := newMgmtServer(t, allowAll, ServerOptions{UIManagement: "enabled", Recorder: recorder},
		testProfile(teamA, "p1", nil, map[string]string{lastModKey: "mallory@company.com", "team": "a"}), testCluster(nil, nil))

	for _, tc := range []struct {
		method, path, body, event string
		key                       client.ObjectKey
		obj                       client.Object
		want                      int
	}{
		{http.MethodPost, "/api/namespaces/team-a/profiles", profileBody("new", "", "New"), "ProfileCreated",
			client.ObjectKey{Namespace: teamA, Name: "new"}, &protemv1alpha1.AccessProfile{}, http.StatusCreated},
		{http.MethodPut, "/api/namespaces/team-a/profiles/p1", profileBody("", "999", "Renamed"), "ProfileUpdated",
			client.ObjectKey{Namespace: teamA, Name: "p1"}, &protemv1alpha1.AccessProfile{}, http.StatusOK},
		{http.MethodPatch, "/api/clusters/c1", `{"paused":true}`, "ClusterSettingsChanged",
			client.ObjectKey{Name: "c1"}, &protemv1alpha1.TargetCluster{}, http.StatusOK},
	} {
		if w := call(s, tc.method, tc.path, bobEmail, tc.body); w.Code != tc.want {
			t.Fatalf("%s %s: expected %d, got %d: %s", tc.method, tc.path, tc.want, w.Code, w.Body.String())
		}
		if err := c.Get(context.Background(), tc.key, tc.obj); err != nil {
			t.Fatal(err)
		}
		if got := tc.obj.GetAnnotations()[lastModKey]; got != bobEmail {
			t.Errorf("%s: expected last-modified-by %s, got %q", tc.event, bobEmail, got)
		}
		select {
		case e := <-recorder.Events:
			if !strings.HasPrefix(e, "Normal "+tc.event+" ") || !strings.Contains(e, bobEmail) {
				t.Errorf("expected a Normal %s event naming bob, got %q", tc.event, e)
			}
		default:
			t.Errorf("expected a %s event", tc.event)
		}
	}
	var p1 protemv1alpha1.AccessProfile
	_ = c.Get(context.Background(), client.ObjectKey{Namespace: teamA, Name: "p1"}, &p1)
	if p1.Annotations["team"] != "a" || p1.Spec.DisplayName != "Renamed" {
		t.Errorf("expected the update to keep other annotations and apply the spec, got %v %q", p1.Annotations, p1.Spec.DisplayName)
	}

	if w := call(s, http.MethodDelete, "/api/namespaces/team-a/profiles/p1", bobEmail, ""); w.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestManagedBy(t *testing.T) {
	for _, tc := range []struct {
		name                string
		labels, annotations map[string]string
		want                string
	}{
		{"plain", map[string]string{"env": "prod"}, nil, ""},
		{"argo tracking annotation", nil, map[string]string{argoTracker: "x"}, "Argo CD"},
		{"argo instance label", map[string]string{"argocd.argoproj.io/instance": "x"}, nil, "Argo CD"},
		{"flux kustomize", map[string]string{"kustomize.toolkit.fluxcd.io/name": "x"}, nil, "Flux"},
		{"flux helm", map[string]string{"helm.toolkit.fluxcd.io/name": "x"}, nil, "Flux"},
		{"helm", map[string]string{"app.kubernetes.io/managed-by": "Helm"}, nil, "Helm"},
		{"kustomize is not an owner", map[string]string{"app.kubernetes.io/managed-by": "kustomize"}, nil, ""},
		{"lock label", map[string]string{"protem.dev/managed-by": "platform-team"}, nil, "platform-team"},
		{"argo beats flux", map[string]string{"argocd.argoproj.io/instance": "x", "kustomize.toolkit.fluxcd.io/name": "x"}, nil, "Argo CD"},
		{"flux beats helm", map[string]string{"helm.toolkit.fluxcd.io/name": "x", "app.kubernetes.io/managed-by": "Helm"}, nil, "Flux"},
		{"helm beats lock", map[string]string{"app.kubernetes.io/managed-by": "Helm", "protem.dev/managed-by": "team"}, nil, "Helm"},
	} {
		if got := managedBy(&metav1.ObjectMeta{Labels: tc.labels, Annotations: tc.annotations}); got != tc.want {
			t.Errorf("%s: expected %q, got %q", tc.name, tc.want, got)
		}
	}
}

func TestRuleRisks(t *testing.T) {
	rule := func(groups, resources, verbs []string) rbacv1.PolicyRule {
		return rbacv1.PolicyRule{APIGroups: groups, Resources: resources, Verbs: verbs}
	}
	core, rbacGroup, all := []string{""}, []string{"rbac.authorization.k8s.io"}, []string{"*"}
	for _, tc := range []struct {
		name string
		rule rbacv1.PolicyRule
		want []string
	}{
		{"read pods", rule(core, []string{"pods"}, []string{"get", "list"}), nil},
		{"cluster admin", rule(all, all, all), []string{riskAllVerbs, riskAllResources, riskSecrets}},
		{"list secrets", rule(core, []string{"secrets"}, []string{"list"}), []string{riskSecrets}},
		{"create secrets only", rule(core, []string{"secrets"}, []string{"create"}), nil},
		{"exec", rule(core, []string{"pods/exec"}, []string{"create"}), []string{riskPodAccess}},
		{"portforward get", rule(core, []string{"pods/portforward"}, []string{"get"}), []string{riskPodAccess}},
		{"exec list only", rule(core, []string{"pods/exec"}, []string{"list"}), nil},
		{"nodes proxy", rule(core, []string{"nodes/proxy"}, []string{"get"}), []string{riskKubelet}},
		{"token", rule(core, []string{"serviceaccounts/token"}, []string{"create"}), []string{riskTokens}},
		{"impersonate", rule(core, []string{"users", "groups"}, []string{"impersonate"}), []string{riskImpersonate}},
		{"bind", rule(rbacGroup, []string{"clusterroles"}, []string{"bind"}), []string{riskRBAC}},
		{"write rolebindings", rule(rbacGroup, []string{"rolebindings"}, []string{"patch"}), []string{riskRBAC}},
		{"write roles any group", rule(all, []string{"roles"}, []string{"create"}), []string{riskRBAC}},
		{"read roles", rule(rbacGroup, []string{"roles"}, []string{"get"}), nil},
		{"roles in another group", rule([]string{"example.com"}, []string{"roles"}, []string{"update"}), nil},
		{"all verbs on secrets", rule(core, []string{"secrets"}, all), []string{riskAllVerbs, riskSecrets}},
		{"all verbs on rbac", rule(rbacGroup, []string{"roles"}, all), []string{riskAllVerbs, riskRBAC}},
		{"all non-resource urls", rbacv1.PolicyRule{NonResourceURLs: all, Verbs: []string{"get"}}, []string{riskNonResourceURLs}},
		{"healthz", rbacv1.PolicyRule{NonResourceURLs: []string{"/healthz"}, Verbs: []string{"get"}}, nil},
	} {
		if got := ruleRisks(tc.rule); !slices.Equal(got, tc.want) {
			t.Errorf("%s: expected %q, got %q", tc.name, tc.want, got)
		}
	}
}

func TestCapabilities(t *testing.T) {
	objs := []client.Object{testProfile(teamA, "p1", nil, nil), testProfile(teamB, "p2", nil, nil), testProfile(teamB, "p3", nil, nil)}
	for _, tc := range []struct {
		name   string
		mode   string
		policy sarPolicy
		want   CapabilitiesView
	}{
		{"no rights", "enabled", denyAll, CapabilitiesView{Management: true, ProfileNamespaces: []string{}}},
		{"one namespace", "enabled", allowOnly(
			sarRule{aliceEmail, "update", "accessprofiles", teamB, ""},
			sarRule{aliceEmail, "list", "targetclusters", "", ""},
		), CapabilitiesView{Management: true, ProfileNamespaces: []string{teamB}, ViewFleet: true}},
		{"all namespaces", "enabled", allowOnly(
			sarRule{aliceEmail, "create", "accessprofiles", "", ""},
			sarRule{aliceEmail, "update", "targetclusters", "", ""},
			sarRule{aliceEmail, "get", "accessrequests", "", ""},
		), CapabilitiesView{Management: true, ProfileNamespaces: []string{"*"}, ManageClusters: true, ViewAll: true}},
		{"readonly admin", "", allowAll, CapabilitiesView{ProfileNamespaces: []string{}, ViewFleet: true, ViewAll: true}},
	} {
		s, _ := newMgmtServer(t, tc.policy, ServerOptions{UIManagement: tc.mode}, objs...)
		w := call(s, http.MethodGet, "/api/capabilities", aliceEmail, "")
		if w.Code != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d: %s", tc.name, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"profileNamespaces":[`) {
			t.Errorf("%s: expected profileNamespaces to be an array, got %s", tc.name, w.Body.String())
		}
		var got CapabilitiesView
		_ = json.Unmarshal(w.Body.Bytes(), &got)
		if got.Management != tc.want.Management || got.ManageClusters != tc.want.ManageClusters || got.ViewFleet != tc.want.ViewFleet ||
			got.ViewAll != tc.want.ViewAll || !slices.Equal(got.ProfileNamespaces, tc.want.ProfileNamespaces) {
			t.Errorf("%s: expected %+v, got %+v", tc.name, tc.want, got)
		}
	}
}

func TestGrantPermissions(t *testing.T) {
	grant := &protemv1alpha1.AccessGrant{
		ObjectMeta: metav1.ObjectMeta{Namespace: teamA, Name: "g1"},
		Spec: protemv1alpha1.AccessGrantSpec{
			Requester: protemv1alpha1.RequesterInfo{Username: aliceUsername, Email: aliceEmail},
			Role:      protemv1alpha1.TargetRoleRef{Kind: "ClusterRole", Name: "edit"},
			Clusters:  []string{clusterA, clusterB},
			ExpiresAt: metav1.NewTime(time.Now().Add(time.Hour)),
		},
		Status: protemv1alpha1.AccessGrantStatus{Clusters: []protemv1alpha1.ClusterBindingState{
			{ClusterName: clusterB, State: protemv1alpha1.ClusterBindingStateFailed, Message: "agent unreachable"},
		}},
	}
	hubRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "edit"},
		Rules:      []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}}},
	}
	clusterGrant := &protemv1alpha1.ClusterGrant{
		ObjectMeta: metav1.ObjectMeta{Namespace: "protem-cluster-cluster-a", Name: "team-a.g1"},
		Spec:       protemv1alpha1.ClusterGrantSpec{Subject: "oidc:alice@company.com"},
		Status: protemv1alpha1.ClusterGrantStatus{
			Phase:    protemv1alpha1.ClusterGrantPhaseActive,
			Bindings: []protemv1alpha1.ManagedBindingRef{{Kind: "ClusterRoleBinding", Name: "protem-g1"}},
			Rules:    []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"get"}}},
		},
	}
	auditor := "auditor@company.com"
	s, _ := newMgmtServer(t, allowOnly(sarRule{auditor, "get", "accessgrants", teamA, ""}), ServerOptions{}, grant, hubRole, clusterGrant)
	const path = "/api/namespaces/team-a/grants/g1/permissions"

	if w := call(s, http.MethodGet, path, eveEmail, ""); w.Code != http.StatusForbidden {
		t.Fatalf("stranger: expected 403, got %d: %s", w.Code, w.Body.String())
	}
	if w := call(s, http.MethodGet, "/api/namespaces/team-a/grants/nope/permissions", aliceEmail, ""); w.Code != http.StatusNotFound {
		t.Fatalf("missing grant: expected 404, got %d", w.Code)
	}
	if w := call(s, http.MethodGet, path, auditor, ""); w.Code != http.StatusOK {
		t.Fatalf("auditor: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	w := call(s, http.MethodGet, path, aliceEmail, "")
	var got []ClusterPermissionsView
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || w.Code != http.StatusOK || len(got) != 2 {
		t.Fatalf("requester: expected 200 with two clusters, got %d %v: %s", w.Code, err, w.Body.String())
	}
	a, b := got[0], got[1]
	if a.Cluster != clusterA || a.State != "Active" || a.Subject != "oidc:alice@company.com" || len(a.Bindings) != 1 ||
		a.Permissions.Source != "cluster" || !a.Permissions.Found || len(a.Permissions.Rules) != 1 ||
		!slices.Equal(a.Permissions.Rules[0].Resources, []string{"secrets"}) || !slices.Equal(a.Permissions.Rules[0].Risks, []string{riskSecrets}) {
		t.Errorf("expected cluster-a to report the cluster's own rules, got %+v", a)
	}
	if b.Cluster != clusterB || b.State != "Failed" || b.Message != "agent unreachable" ||
		b.Permissions.Source != sourceHub || !b.Permissions.Found || len(b.Permissions.Rules) != 1 || !slices.Equal(b.Permissions.Rules[0].Resources, []string{"pods"}) {
		t.Errorf("expected cluster-b to fall back to grant status and hub role, got %+v", b)
	}
}

func TestProfileDetailAndPermissions(t *testing.T) {
	p := testProfile(teamA, "p1", map[string]string{"tier": "gold"}, map[string]string{lastModKey: bobEmail})
	p.ManagedFields = []metav1.ManagedFieldsEntry{{Manager: "kubectl"}}
	p.Status.ActiveGrantsCount = 3
	p.Spec.Role = protemv1alpha1.TargetRoleRef{Kind: "ClusterRole", Name: "debugger"}
	roleProfile := testProfile(teamA, "p2", nil, nil)
	roleProfile.Spec.Role = protemv1alpha1.TargetRoleRef{Kind: roleKind, Name: "deployer"}
	debugger := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "debugger"},
		Rules:      []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"pods/exec"}, Verbs: []string{"create"}}},
	}
	s, _ := newMgmtServer(t, denyAll, ServerOptions{}, p, roleProfile, debugger)

	w := call(s, http.MethodGet, "/api/namespaces/team-a/profiles/p1", aliceEmail, "")
	var detail ProfileDetailView
	if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil || w.Code != http.StatusOK {
		t.Fatalf("detail: expected 200, got %d %v: %s", w.Code, err, w.Body.String())
	}
	if detail.Name != "p1" || detail.ResourceVersion != "999" || detail.Spec.Role.Name != "debugger" {
		t.Errorf("expected the profile with its resourceVersion and spec, got %+v", detail)
	}
	for _, want := range []string{"apiVersion: protem.dev/v1alpha1", "kind: AccessProfile", "name: p1", "namespace: team-a", "tier: gold", "displayName: Team A view"} {
		if !strings.Contains(detail.YAML, want) {
			t.Errorf("expected YAML to contain %q, got:\n%s", want, detail.YAML)
		}
	}
	for _, banned := range []string{"status", "annotations", "managedFields", "resourceVersion", "creationTimestamp", lastModKey} {
		if strings.Contains(detail.YAML, banned) {
			t.Errorf("expected YAML without %q, got:\n%s", banned, detail.YAML)
		}
	}
	if w := call(s, http.MethodGet, "/api/namespaces/team-a/profiles/nope", aliceEmail, ""); w.Code != http.StatusNotFound {
		t.Errorf("absent profile: expected 404, got %d", w.Code)
	}

	w = call(s, http.MethodGet, "/api/namespaces/team-a/profiles/p1/permissions", aliceEmail, "")
	var perms PermissionsView
	_ = json.Unmarshal(w.Body.Bytes(), &perms)
	if w.Code != http.StatusOK || perms.Source != sourceHub || !perms.Found || len(perms.Rules) != 1 || !slices.Equal(perms.Rules[0].Risks, []string{riskPodAccess}) {
		t.Errorf("expected the hub ClusterRole's rules with the exec risk, got %d %+v", w.Code, perms)
	}

	w = call(s, http.MethodGet, "/api/namespaces/team-a/profiles/p2/permissions", aliceEmail, "")
	perms = PermissionsView{}
	_ = json.Unmarshal(w.Body.Bytes(), &perms)
	if w.Code != http.StatusOK || perms.Found || perms.Source != sourceHub || !strings.Contains(w.Body.String(), `"rules":[]`) ||
		!strings.Contains(perms.Message, sourceHub) || !strings.Contains(perms.Message, "target clusters") {
		t.Errorf("expected a missing hub Role to be reported as not found, got %d %s", w.Code, w.Body.String())
	}
}

func TestListViewsCarryNewFields(t *testing.T) {
	resolved := &protemv1alpha1.AccessRequest{
		ObjectMeta: metav1.ObjectMeta{Namespace: teamA, Name: "resolved"},
		Spec:       protemv1alpha1.AccessRequestSpec{Requester: protemv1alpha1.RequesterInfo{Email: aliceEmail}},
		Status: protemv1alpha1.AccessRequestStatus{Resolved: &protemv1alpha1.ResolvedScope{
			Role: &protemv1alpha1.TargetRoleRef{Kind: roleKind, Name: "deployer"}, Scope: protemv1alpha1.ProfileScopeNamespaced,
		}},
	}
	unresolved := &protemv1alpha1.AccessRequest{
		ObjectMeta: metav1.ObjectMeta{Namespace: teamA, Name: "unresolved"},
		Spec:       protemv1alpha1.AccessRequestSpec{Requester: protemv1alpha1.RequesterInfo{Email: aliceEmail}},
	}
	cluster := testCluster(map[string]string{"kustomize.toolkit.fluxcd.io/name": "fleet"}, nil)
	cluster.Spec.Paused, cluster.Spec.Enforcement = true, protemv1alpha1.EnforcementModeAuthorizer
	s, _ := newMgmtServer(t, denyAll, ServerOptions{SignOutURL: "https://sso.example/logout"}, resolved, unresolved, cluster,
		testProfile(teamA, "p1", map[string]string{"argocd.argoproj.io/instance": "protem"}, nil))

	var requests struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.Unmarshal(call(s, http.MethodGet, "/api/requests", aliceEmail, "").Body.Bytes(), &requests)
	if len(requests.Items) != 2 {
		t.Errorf("expected both requests, got %v", requests)
	}
	for _, r := range requests.Items {
		_, hasRole := r["role"]
		_, hasScope := r["scope"]
		if r["name"] == "unresolved" && (hasRole || hasScope) {
			t.Errorf("expected no role or scope before resolution, got %v", r)
		}
		if r["name"] == "resolved" && (!hasRole || r["scope"] != "Namespaced") {
			t.Errorf("expected the frozen role and scope, got %v", r)
		}
	}
	var clusters []ClusterView
	_ = json.Unmarshal(call(s, http.MethodGet, "/api/clusters", aliceEmail, "").Body.Bytes(), &clusters)
	if len(clusters) != 1 || !clusters[0].Paused || clusters[0].Subject != "{{.Email}}" || clusters[0].Enforcement != "Authorizer" || clusters[0].ManagedBy != "Flux" {
		t.Errorf("expected cluster settings and owner, got %+v", clusters)
	}
	var profiles []ProfileView
	_ = json.Unmarshal(call(s, http.MethodGet, "/api/profiles", aliceEmail, "").Body.Bytes(), &profiles)
	if len(profiles) != 1 || profiles[0].ManagedBy != "Argo CD" {
		t.Errorf("expected the profile owner, got %+v", profiles)
	}
	var me MeView
	_ = json.Unmarshal(call(s, http.MethodGet, "/api/me", aliceEmail, "").Body.Bytes(), &me)
	if me.SignOutURL != "https://sso.example/logout" {
		t.Errorf("expected the sign-out URL, got %+v", me)
	}
}
