package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
)

const (
	testAddr         = "127.0.0.1:8080"
	defaultNs        = "default"
	productionNs     = "production"
	clusterA         = "cluster-a"
	clusterB         = "cluster-b"
	sreRuleName      = "sre-rule"
	prodDbAdmin      = "prod-db-admin"
	roleKind         = "Role"
	hubClusterName   = "hub-cluster"
	prodProfileName  = "prod-profile"
	prodProfileTitle = "Prod Profile"
)

func setupTestScheme(t *testing.T) *runtime.Scheme {
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatalf("failed to add clientgo: %v", err)
	}
	if err := protemv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("failed to add protemv1alpha1: %v", err)
	}
	return s
}

func TestAPI_Routes(t *testing.T) {
	fakeClient := fake.NewClientBuilder().WithScheme(setupTestScheme(t)).Build()
	server, err := NewServer(fakeClient, ServerOptions{Addr: testAddr})
	if err != nil {
		t.Fatalf("failed to initialize server: %v", err)
	}
	handler := server.cors(server.Mux)

	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/api/me", http.StatusUnauthorized},
		{http.MethodGet, "/api/clusters", http.StatusUnauthorized},
		{http.MethodGet, "/api/profiles", http.StatusUnauthorized},
		{http.MethodGet, "/api/requests", http.StatusUnauthorized},
		{http.MethodGet, "/api/namespaces/x/requests", http.StatusUnauthorized},
		{http.MethodPost, "/api/namespaces/x/requests", http.StatusUnauthorized},
		{http.MethodPost, "/api/namespaces/x/requests/y/approve", http.StatusUnauthorized},
		{http.MethodPost, "/api/namespaces/x/requests/y/reject", http.StatusUnauthorized},
		{http.MethodPost, "/api/namespaces/x/requests/y/cancel", http.StatusUnauthorized},
		{http.MethodGet, "/api/grants", http.StatusUnauthorized},
		{http.MethodPost, "/api/namespaces/x/grants/y/revoke", http.StatusUnauthorized},
		{http.MethodGet, "/api/stats", http.StatusUnauthorized},
		{http.MethodGet, "/api/audit", http.StatusUnauthorized},
		{http.MethodGet, "/api/namespaces/x/requests/y/approve", http.StatusNotFound},
		{http.MethodPost, "/api/requests", http.StatusNotFound},
		{http.MethodPost, "/api/requests/y/approve", http.StatusNotFound},
		{http.MethodGet, "/api/nope", http.StatusNotFound},
	} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != tc.want {
			t.Errorf("%s %s: expected %d, got %d: %s", tc.method, tc.path, tc.want, w.Code, w.Body.String())
		}
		if w.Code == http.StatusUnauthorized || w.Code == http.StatusNotFound {
			var body map[string]string
			if err := json.NewDecoder(w.Body).Decode(&body); err != nil || body["error"] == "" {
				t.Errorf("%s %s: expected JSON error body, got %v", tc.method, tc.path, err)
			}
		}
	}
}

func TestAPI_Me(t *testing.T) {
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	server, err := NewServer(fakeClient, ServerOptions{Addr: testAddr, DevAuth: true})
	if err != nil {
		t.Fatalf("failed to initialize server: %v", err)
	}

	// Test default Alice
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.Header.Set("X-Test-User", aliceEmail)
	w := httptest.NewRecorder()
	server.Mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var user UserInfo
	if err := json.NewDecoder(w.Body).Decode(&user); err != nil {
		t.Fatalf("failed to decode user: %v", err)
	}
	if user.Email != aliceEmail {
		t.Fatalf("expected alice@company.com, got %s", user.Email)
	}
	if len(user.Groups) == 0 || user.Groups[0] != devsGroup {
		t.Fatalf("expected group devs, got %v", user.Groups)
	}

	// Test persona switch to Bob (Approver)
	reqBob := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	reqBob.Header.Set("X-Test-User", bobEmail)
	wBob := httptest.NewRecorder()
	server.Mux.ServeHTTP(wBob, reqBob)

	var userBob UserInfo
	_ = json.NewDecoder(wBob.Body).Decode(&userBob)
	if userBob.Email != bobEmail {
		t.Fatalf("expected bob@company.com, got %s", userBob.Email)
	}
	if len(userBob.Groups) == 0 || userBob.Groups[0] != sreLeadsGroup {
		t.Fatalf("expected group sre-leads, got %v", userBob.Groups)
	}
}

func TestAPI_RequestsAndApprovals(t *testing.T) {
	scheme := setupTestScheme(t)

	profile := &protemv1alpha1.AccessProfile{
		ObjectMeta: metav1.ObjectMeta{
			Name:      prodDbAdmin,
			Namespace: defaultNs,
		},
		Spec: protemv1alpha1.AccessProfileSpec{
			DisplayName: "Prod Database Admin",
			Scope:       protemv1alpha1.ProfileScopeNamespaced,
			Namespaces:  []string{productionNs},
			Role: protemv1alpha1.TargetRoleRef{
				Kind: "ClusterRole",
				Name: adminName,
			},
			Clusters: protemv1alpha1.ClusterTargeting{
				Names: []string{clusterA},
			},
			Duration: protemv1alpha1.ProfileDurationConfig{
				Default: metav1.Duration{Duration: 30 * time.Minute},
				Max:     metav1.Duration{Duration: 2 * time.Hour},
			},
			Requesters: protemv1alpha1.ProfileRequesters{
				Groups: []string{devsGroup},
			},
			Justification: protemv1alpha1.ProfileJustification{
				Reason: protemv1alpha1.JustificationRequired,
			},
			Approval: protemv1alpha1.ProfileApprovalConfig{
				Rules: []protemv1alpha1.ApprovalRule{
					{
						Name: sreRuleName,
						Mode: protemv1alpha1.ApprovalModeManual,
						Approvers: &protemv1alpha1.ApprovalApprovers{
							Groups: []string{sreLeadsGroup},
						},
						RequiredApprovals: 1,
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(profile).
		WithStatusSubresource(&protemv1alpha1.AccessRequest{}).
		Build()

	server, err := NewServer(fakeClient, ServerOptions{Addr: testAddr, DevAuth: true})
	if err != nil {
		t.Fatalf("failed to initialize server: %v", err)
	}

	// 1. Submit Request as Alice
	payload := CreateRequestPayload{
		ProfileRef: prodDbAdmin,
		Duration:   "1h",
		Reason:     "Emergency patch",
		Ticket:     "INC-1234",
	}
	body, _ := json.Marshal(payload)
	postReq := httptest.NewRequest(http.MethodPost, "/api/namespaces/default/requests", bytes.NewReader(body))
	postReq.Header.Set("X-Test-User", aliceEmail)
	postReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.Mux.ServeHTTP(w, postReq)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
	}

	var created RequestView
	_ = json.NewDecoder(w.Body).Decode(&created)
	if created.Requester.Email != aliceEmail {
		t.Fatalf("expected requester alice@company.com, got %s", created.Requester.Email)
	}

	// Populate Status.Phase and Status.Resolved as controller does
	var stored protemv1alpha1.AccessRequest
	if err := fakeClient.Get(context.Background(), client.ObjectKey{Namespace: created.Namespace, Name: created.Name}, &stored); err != nil {
		t.Fatalf("failed to get created request: %v", err)
	}
	stored.Status.Phase = protemv1alpha1.RequestPhasePending
	stored.Status.Resolved = &protemv1alpha1.ResolvedScope{
		Requirements: []protemv1alpha1.ClusterApprovalRequirement{
			{
				ClusterName:       clusterA,
				RuleName:          sreRuleName,
				Mode:              protemv1alpha1.ApprovalModeManual,
				RequiredApprovals: 1,
				ApproverGroups:    []string{sreLeadsGroup},
			},
		},
	}
	_ = fakeClient.Status().Update(context.Background(), &stored)

	// 2. Approve Request as Bob (sre-leads)
	approveReq := httptest.NewRequest(http.MethodPost, "/api/namespaces/default/requests/"+created.Name+"/approve", nil)
	approveReq.Header.Set("X-Test-User", bobEmail)
	wApprove := httptest.NewRecorder()
	server.Mux.ServeHTTP(wApprove, approveReq)

	if wApprove.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on approve, got %d: %s", wApprove.Code, wApprove.Body.String())
	}

	var approved RequestView
	_ = json.NewDecoder(wApprove.Body).Decode(&approved)
	if len(approved.Approvals) == 0 || approved.Approvals[0].User != bobEmail {
		t.Fatalf("expected approval decision by bob@company.com, got %v", approved.Approvals)
	}

	// 3. Verify Stats & Audit
	statsReq := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	statsReq.Header.Set("X-Test-User", aliceEmail)
	wStats := httptest.NewRecorder()
	server.Mux.ServeHTTP(wStats, statsReq)
	var stats StatsView
	_ = json.NewDecoder(wStats.Body).Decode(&stats)
	if stats.TotalRequests != 1 {
		t.Fatalf("expected 1 total request, got %d", stats.TotalRequests)
	}
}

func TestAPI_GrantsAndRevoke(t *testing.T) {
	scheme := setupTestScheme(t)

	grant := &protemv1alpha1.AccessGrant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "grant-test-api",
			Namespace: defaultNs,
		},
		Spec: protemv1alpha1.AccessGrantSpec{
			RequestRef: "req-1",
			ProfileRef: "prof-1",
			Requester: protemv1alpha1.RequesterInfo{
				Username: aliceUsername,
				Email:    aliceEmail,
			},
			Role: protemv1alpha1.TargetRoleRef{
				Kind: roleKind,
				Name: adminName,
			},
			Scope:      protemv1alpha1.ProfileScopeNamespaced,
			Namespaces: []string{productionNs},
			ExpiresAt:  metav1.NewTime(time.Now().Add(30 * time.Minute)),
		},
		Status: protemv1alpha1.AccessGrantStatus{
			Phase: protemv1alpha1.GrantPhaseActive,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant).
		Build()

	server, err := NewServer(fakeClient, ServerOptions{Addr: testAddr, DevAuth: true})
	if err != nil {
		t.Fatalf("failed to initialize server: %v", err)
	}

	// List grants
	getReq := httptest.NewRequest(http.MethodGet, "/api/grants", nil)
	getReq.Header.Set("X-Test-User", aliceEmail)
	wGet := httptest.NewRecorder()
	server.Mux.ServeHTTP(wGet, getReq)

	var grants GrantPage
	_ = json.NewDecoder(wGet.Body).Decode(&grants)
	if len(grants.Items) != 1 || grants.Items[0].RemainingSeconds <= 0 || grants.Total != 1 {
		t.Fatalf("expected 1 active grant with positive remaining seconds, got %+v", grants)
	}

	// Revoke grant (by requester Alice)
	revokePayload := map[string]string{"reason": "Incident mitigated"}
	body, _ := json.Marshal(revokePayload)
	revokeReq := httptest.NewRequest(http.MethodPost, "/api/namespaces/default/grants/grant-test-api/revoke", bytes.NewReader(body))
	revokeReq.Header.Set("X-Test-User", aliceEmail)
	wRevoke := httptest.NewRecorder()
	server.Mux.ServeHTTP(wRevoke, revokeReq)

	if wRevoke.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on revoke, got %d: %s", wRevoke.Code, wRevoke.Body.String())
	}

	var updated GrantView
	_ = json.NewDecoder(wRevoke.Body).Decode(&updated)
	if updated.Revocation == nil || updated.Revocation.By != aliceEmail {
		t.Fatalf("expected revoked by alice@company.com, got revocation=%v", updated.Revocation)
	}

	again := httptest.NewRequest(http.MethodPost, "/api/namespaces/default/grants/grant-test-api/revoke", strings.NewReader(`{"reason":"second"}`))
	again.Header.Set("X-Test-User", aliceEmail)
	wAgain := httptest.NewRecorder()
	server.Mux.ServeHTTP(wAgain, again)
	if wAgain.Code != http.StatusBadRequest || !strings.Contains(wAgain.Body.String(), "Grant is already revoked or expired") {
		t.Fatalf("expected 400 on second revoke, got %d: %s", wAgain.Code, wAgain.Body.String())
	}
	var stored protemv1alpha1.AccessGrant
	if err := fakeClient.Get(context.Background(), client.ObjectKey{Namespace: defaultNs, Name: "grant-test-api"}, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Spec.Revocation == nil || stored.Spec.Revocation.By != updated.Revocation.By || stored.Spec.Revocation.Reason != "Incident mitigated" {
		t.Fatalf("expected original revocation kept, got %+v", stored.Spec.Revocation)
	}

	wGet = httptest.NewRecorder()
	server.Mux.ServeHTTP(wGet, getReq)
	grants = GrantPage{}
	_ = json.NewDecoder(wGet.Body).Decode(&grants)
	if len(grants.Items) != 1 || grants.Items[0].Actions.Revoke {
		t.Fatalf("expected revoked grant not to offer revoke, got %+v", grants)
	}
}

func TestAPI_ViewsCarryActions(t *testing.T) {
	scheme := setupTestScheme(t)
	accessReq := &protemv1alpha1.AccessRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "req-views", Namespace: defaultNs},
		Spec: protemv1alpha1.AccessRequestSpec{
			ProfileRef: prodDbAdmin,
			Requester:  protemv1alpha1.RequesterInfo{Username: aliceUsername, Email: aliceEmail, Groups: []string{devsGroup}},
		},
		Status: protemv1alpha1.AccessRequestStatus{
			Phase: protemv1alpha1.RequestPhasePending,
			Resolved: &protemv1alpha1.ResolvedScope{
				Requirements: []protemv1alpha1.ClusterApprovalRequirement{{
					ClusterName:       clusterA,
					RuleName:          sreRuleName,
					Mode:              protemv1alpha1.ApprovalModeManual,
					RequiredApprovals: 1,
					ApproverGroups:    []string{sreLeadsGroup},
				}},
			},
		},
	}
	grant := &protemv1alpha1.AccessGrant{
		ObjectMeta: metav1.ObjectMeta{Name: "grant-views", Namespace: defaultNs},
		Spec: protemv1alpha1.AccessGrantSpec{
			ProfileRef: prodDbAdmin,
			Requester:  protemv1alpha1.RequesterInfo{Username: aliceUsername, Email: aliceEmail},
			ExpiresAt:  metav1.NewTime(time.Now().Add(30 * time.Minute)),
		},
		Status: protemv1alpha1.AccessGrantStatus{Phase: protemv1alpha1.GrantPhaseActive},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(accessReq, grant).Build()
	server, err := NewServer(fakeClient, ServerOptions{Addr: testAddr, DevAuth: true})
	if err != nil {
		t.Fatalf("failed NewServer: %v", err)
	}
	get := func(path, as string, out any) {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.Header.Set("X-Test-User", as)
		w := httptest.NewRecorder()
		server.Mux.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s as %s: expected 200, got %d: %s", path, as, w.Code, w.Body.String())
		}
		if err := json.NewDecoder(w.Body).Decode(out); err != nil {
			t.Fatalf("GET %s: decode: %v", path, err)
		}
	}

	type requestActions struct{ Approve, Reject, Cancel bool }
	for _, tc := range []struct {
		as   string
		want requestActions
	}{
		{bobEmail, requestActions{Approve: true, Reject: true}},
		{aliceEmail, requestActions{Cancel: true}},
	} {
		var requests struct {
			Items []struct {
				Name    string         `json:"name"`
				Phase   string         `json:"phase"`
				Actions requestActions `json:"actions"`
			} `json:"items"`
		}
		get("/api/requests", tc.as, &requests)
		if len(requests.Items) != 1 || requests.Items[0].Name != "req-views" || requests.Items[0].Phase != "Pending" {
			t.Fatalf("as %s: expected the pending req-views, got %+v", tc.as, requests)
		}
		if requests.Items[0].Actions != tc.want {
			t.Errorf("as %s: expected actions %+v, got %+v", tc.as, tc.want, requests.Items[0].Actions)
		}
	}

	var grants struct {
		Items []struct {
			Phase            string `json:"phase"`
			RemainingSeconds int64  `json:"remainingSeconds"`
			Actions          struct {
				Revoke bool `json:"revoke"`
			} `json:"actions"`
		} `json:"items"`
	}
	get("/api/grants", aliceEmail, &grants)
	if g := grants.Items; len(g) != 1 || g[0].Phase != "Active" || g[0].RemainingSeconds <= 0 || !g[0].Actions.Revoke {
		t.Fatalf("expected requester to see an active, revocable grant with time left, got %+v", grants)
	}
}

func TestAPI_MeReportsDevAuth(t *testing.T) {
	for _, devAuth := range []bool{true, false} {
		server, err := NewServer(fake.NewClientBuilder().WithScheme(setupTestScheme(t)).Build(), ServerOptions{Addr: testAddr, DevAuth: devAuth})
		if err != nil {
			t.Fatalf("failed NewServer: %v", err)
		}
		w := httptest.NewRecorder()
		server.handleMe(w, httptest.NewRequest(http.MethodGet, "/api/me", nil), UserInfo{Username: aliceUsername, Email: aliceEmail})
		var me struct {
			Email   string `json:"email"`
			DevAuth *bool  `json:"devAuth"`
		}
		if err := json.NewDecoder(w.Body).Decode(&me); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if me.Email != aliceEmail || me.DevAuth == nil || *me.DevAuth != devAuth {
			t.Errorf("DevAuth=%v: expected email and devAuth %v, got %+v", devAuth, devAuth, me)
		}
	}
}

func TestAPI_RequestViewCarriesScopeAndProgress(t *testing.T) {
	resolved := &protemv1alpha1.AccessRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "req-resolved", Namespace: defaultNs},
		Spec: protemv1alpha1.AccessRequestSpec{
			ProfileRef: prodDbAdmin,
			Requester:  protemv1alpha1.RequesterInfo{Username: aliceUsername, Email: aliceEmail, Groups: []string{devsGroup}},
		},
		Status: protemv1alpha1.AccessRequestStatus{
			Phase: protemv1alpha1.RequestPhasePending,
			Resolved: &protemv1alpha1.ResolvedScope{
				Clusters:   []string{clusterA, clusterB},
				Namespaces: []string{productionNs},
				Requirements: []protemv1alpha1.ClusterApprovalRequirement{
					{ClusterName: clusterA, RuleName: sreRuleName, Mode: protemv1alpha1.ApprovalModeManual, RequiredApprovals: 2, ApproverGroups: []string{sreLeadsGroup}, ApproverUsers: []string{"dave"}},
					{ClusterName: clusterB, RuleName: "auto-rule", Mode: protemv1alpha1.ApprovalModeAuto},
				},
			},
		},
	}
	unresolved := &protemv1alpha1.AccessRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "req-unresolved", Namespace: defaultNs},
		Spec: protemv1alpha1.AccessRequestSpec{
			ProfileRef: prodDbAdmin,
			Requester:  protemv1alpha1.RequesterInfo{Username: aliceUsername, Email: aliceEmail, Groups: []string{devsGroup}},
			Clusters:   []string{"cluster-c"},
			Namespaces: []string{"staging"},
		},
		Status: protemv1alpha1.AccessRequestStatus{Phase: protemv1alpha1.RequestPhasePending, GrantRef: "grant-x"},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(setupTestScheme(t)).
		WithObjects(resolved, unresolved).
		WithStatusSubresource(&protemv1alpha1.AccessRequest{}).
		Build()
	server, err := NewServer(fakeClient, ServerOptions{Addr: testAddr, DevAuth: true})
	if err != nil {
		t.Fatalf("failed NewServer: %v", err)
	}
	serve := func(method, path, as string) string {
		t.Helper()
		r := httptest.NewRequest(method, path, nil)
		r.Header.Set("X-Test-User", as)
		w := httptest.NewRecorder()
		server.Mux.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("%s %s: expected 200, got %d: %s", method, path, w.Code, w.Body.String())
		}
		return w.Body.String()
	}

	approved := serve(http.MethodPost, "/api/namespaces/default/requests/req-resolved/approve", bobEmail)
	if !strings.Contains(approved, `"approvedBy":[]`) {
		t.Errorf("expected an empty approvedBy array for the auto requirement, got %s", approved)
	}
	type requirement struct {
		Cluster        string   `json:"cluster"`
		Rule           string   `json:"rule"`
		Mode           string   `json:"mode"`
		Required       int      `json:"required"`
		ApprovedBy     []string `json:"approvedBy"`
		ApproverGroups []string `json:"approverGroups"`
		ApproverUsers  []string `json:"approverUsers"`
	}
	var view struct {
		Phase        string        `json:"phase"`
		Clusters     []string      `json:"clusters"`
		Namespaces   []string      `json:"namespaces"`
		Requirements []requirement `json:"requirements"`
		Actions      struct {
			Approve, Reject bool
		} `json:"actions"`
	}
	if err := json.Unmarshal([]byte(approved), &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if view.Phase != "Pending" || strings.Join(view.Clusters, ",") != "cluster-a,cluster-b" || strings.Join(view.Namespaces, ",") != productionNs {
		t.Fatalf("expected pending request scoped to resolved clusters and namespaces, got %+v", view)
	}
	if len(view.Requirements) != 2 {
		t.Fatalf("expected 2 requirements, got %+v", view.Requirements)
	}
	if view.Actions.Approve || !view.Actions.Reject {
		t.Errorf("expected bob to have no approve action after approving but still reject, got %+v", view.Actions)
	}
	again := httptest.NewRequest(http.MethodPost, "/api/namespaces/default/requests/req-resolved/approve", nil)
	again.Header.Set("X-Test-User", bobEmail)
	w := httptest.NewRecorder()
	server.Mux.ServeHTTP(w, again)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "You have already approved this request") {
		t.Errorf("expected 409 on repeat approval, got %d: %s", w.Code, w.Body.String())
	}
	sre := view.Requirements[0]
	if sre.Cluster != clusterA || sre.Rule != sreRuleName || sre.Mode != "Manual" || sre.Required != 2 ||
		strings.Join(sre.ApprovedBy, ",") != bobEmail || strings.Join(sre.ApproverGroups, ",") != sreLeadsGroup || strings.Join(sre.ApproverUsers, ",") != "dave" {
		t.Errorf("expected sre-rule 1 of 2 approved by bob, got %+v", sre)
	}
	if auto := view.Requirements[1]; auto.Cluster != clusterB || auto.Mode != "Auto" || len(auto.ApprovedBy) != 0 {
		t.Errorf("expected auto requirement with no approvals, got %+v", auto)
	}

	assertSpecScopeFallback(t, serve(http.MethodGet, "/api/requests", aliceEmail))
}

func assertSpecScopeFallback(t *testing.T, body string) {
	t.Helper()
	var page struct {
		Items []struct {
			Name         string            `json:"name"`
			Clusters     []string          `json:"clusters"`
			Namespaces   []string          `json:"namespaces"`
			Requirements []json.RawMessage `json:"requirements"`
			GrantRef     string            `json:"grantRef"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(body), &page); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	list := page.Items
	for _, v := range list {
		if v.Name == "req-unresolved" &&
			(strings.Join(v.Clusters, ",") != "cluster-c" || strings.Join(v.Namespaces, ",") != "staging" || v.Requirements != nil || v.GrantRef != "grant-x") {
			t.Errorf("expected unresolved request to fall back to spec scope and carry grantRef, got %+v", v)
		}
	}
	if len(list) != 2 {
		t.Fatalf("expected both requests listed, got %+v", list)
	}
}

func TestAPI_ProfilesListRequestableSpokes(t *testing.T) {
	spoke := func(name, env string, ready bool) *protemv1alpha1.TargetCluster {
		c := &protemv1alpha1.TargetCluster{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{"env": env}}}
		if ready {
			c.Status.Conditions = []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}}
		}
		return c
	}
	profile := func(name string, clusters protemv1alpha1.ClusterTargeting) *protemv1alpha1.AccessProfile {
		return &protemv1alpha1.AccessProfile{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: defaultNs},
			Spec:       protemv1alpha1.AccessProfileSpec{Clusters: clusters, Requesters: protemv1alpha1.ProfileRequesters{Groups: []string{devsGroup}}},
		}
	}
	fakeClient := fake.NewClientBuilder().WithScheme(setupTestScheme(t)).WithObjects(
		spoke("prod-b", "prod", true), spoke("prod-a", "prod", true), spoke("prod-down", "prod", false), spoke("dev-a", "dev", true),
		profile("by-label", protemv1alpha1.ClusterTargeting{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}}}),
		profile("by-name", protemv1alpha1.ClusterTargeting{Names: []string{"dev-a", "prod-down"}}),
		profile("none-ready", protemv1alpha1.ClusterTargeting{Names: []string{"prod-down"}}),
		profile("misconfigured", protemv1alpha1.ClusterTargeting{}),
	).Build()
	server, err := NewServer(fakeClient, ServerOptions{Addr: testAddr, DevAuth: true})
	if err != nil {
		t.Fatalf("failed NewServer: %v", err)
	}
	r := httptest.NewRequest(http.MethodGet, "/api/profiles", nil)
	r.Header.Set("X-Test-User", aliceEmail)
	w := httptest.NewRecorder()
	server.Mux.ServeHTTP(w, r)
	var profiles []struct {
		Name     string    `json:"name"`
		Clusters *[]string `json:"clusters"`
	}
	if err := json.NewDecoder(w.Body).Decode(&profiles); err != nil {
		t.Fatalf("decode: %v (%d %s)", err, w.Code, w.Body.String())
	}
	got := map[string]string{}
	for _, p := range profiles {
		if p.Clusters == nil {
			t.Fatalf("profile %s: expected clusters to be an array, got null or missing", p.Name)
		}
		got[p.Name] = strings.Join(*p.Clusters, ",")
	}
	if got["by-label"] != "prod-a,prod-b" || got["by-name"] != "dev-a" || got["none-ready"] != "" || got["misconfigured"] != "" {
		t.Fatalf("expected ready spokes each profile can target, got %v", got)
	}
}

func TestAPI_ClustersCountActiveGrants(t *testing.T) {
	grant := func(name string, phase protemv1alpha1.GrantPhase, clusters ...string) *protemv1alpha1.AccessGrant {
		return &protemv1alpha1.AccessGrant{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: defaultNs},
			Spec:       protemv1alpha1.AccessGrantSpec{Clusters: clusters},
			Status:     protemv1alpha1.AccessGrantStatus{Phase: phase},
		}
	}
	fakeClient := fake.NewClientBuilder().WithScheme(setupTestScheme(t)).WithObjects(
		&protemv1alpha1.TargetCluster{ObjectMeta: metav1.ObjectMeta{Name: "spoke-a"}},
		&protemv1alpha1.TargetCluster{ObjectMeta: metav1.ObjectMeta{Name: "spoke-b"}},
		&protemv1alpha1.TargetCluster{ObjectMeta: metav1.ObjectMeta{Name: "spoke-c"}},
		grant("g1", protemv1alpha1.GrantPhaseActive, "spoke-a"),
		grant("g2", protemv1alpha1.GrantPhaseActive, "spoke-a", "spoke-b"),
		grant("g3", protemv1alpha1.GrantPhaseExpired, "spoke-a"),
	).Build()
	server, err := NewServer(fakeClient, ServerOptions{Addr: testAddr, DevAuth: true})
	if err != nil {
		t.Fatalf("failed NewServer: %v", err)
	}
	r := httptest.NewRequest(http.MethodGet, "/api/clusters", nil)
	r.Header.Set("X-Test-User", "admin@company.com")
	w := httptest.NewRecorder()
	server.Mux.ServeHTTP(w, r)
	var clusters []ClusterView
	if err := json.NewDecoder(w.Body).Decode(&clusters); err != nil {
		t.Fatalf("decode: %v (%d %s)", err, w.Code, w.Body.String())
	}
	got := map[string]int{}
	for _, c := range clusters {
		got[c.Name] = c.ActiveGrantsCount
	}
	if len(got) != 3 || got["spoke-a"] != 2 || got["spoke-b"] != 1 || got["spoke-c"] != 0 {
		t.Fatalf("expected active grants a=2 b=1 c=0, got %v", got)
	}
}
