package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
)

// C1: Dev auth must default to false and reject unauthenticated requests out-of-the-box
func TestEscalation_3_1_UnauthenticatedReturns401(t *testing.T) {
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	// Constructed server with default options -> DevAuth defaults to false!
	server, err := NewServer(fakeClient)
	if err != nil {
		t.Fatalf("unexpected NewServer error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	w := httptest.NewRecorder()
	server.Mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("VULNERABILITY C1/§3.1: Expected 401 Unauthorized for unauthenticated request on default server, got %d", w.Code)
	}
}

// C1: X-Test-User header must be rejected on default constructed server (DevAuth=false)
func TestEscalation_3_1_ForgedTestUserHeaderIgnored(t *testing.T) {
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	server, err := NewServer(fakeClient)
	if err != nil {
		t.Fatalf("unexpected NewServer error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.Header.Set("X-Test-User", "admin@company.com")
	w := httptest.NewRecorder()
	server.Mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("VULNERABILITY C1/§3.1: Expected 401 Unauthorized when X-Test-User is passed with default server, got %d", w.Code)
	}
}

// C1: NewServer must refuse to enable DevAuth unless bound to a loopback address
func TestEscalation_C1_DevAuthRefusedOnNonLoopback(t *testing.T) {
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	// 1. Non-loopback 0.0.0.0 -> must fail
	_, err := NewServer(fakeClient, ServerOptions{Addr: "0.0.0.0:8080", DevAuth: true})
	if err == nil {
		t.Fatalf("VULNERABILITY C1: NewServer allowed enabling dev-auth on non-loopback address 0.0.0.0:8080!")
	}

	// 2. Wildcard :8080 -> must fail
	_, err = NewServer(fakeClient, ServerOptions{Addr: ":8080", DevAuth: true})
	if err == nil {
		t.Fatalf("VULNERABILITY C1: NewServer allowed enabling dev-auth on wildcard address :8080!")
	}

	// 3. Loopback 127.0.0.1:8080 -> permitted
	server, err := NewServer(fakeClient, ServerOptions{Addr: testAddr, DevAuth: true})
	if err != nil {
		t.Fatalf("unexpected error enabling dev-auth on 127.0.0.1:8080: %v", err)
	}
	if !server.DevAuth {
		t.Fatalf("expected DevAuth to be enabled on loopback")
	}
}

// C2: Cryptographic OIDC verification must reject forged or unverified JWTs
type mockOIDCVerifier struct {
	fail bool
}

func (m *mockOIDCVerifier) Verify(ctx context.Context, rawIDToken string) (*oidc.IDToken, error) {
	if m.fail {
		return nil, fmt.Errorf("crypto/rsa: verification error: invalid signature")
	}
	return &oidc.IDToken{}, nil
}

func TestEscalation_C2_CryptographicOIDCVerification(t *testing.T) {
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	server, err := NewServer(fakeClient, ServerOptions{Addr: testAddr, DevAuth: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Inject verifier that enforces real signature check
	server.Authenticator.verifier = &mockOIDCVerifier{fail: true}

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.Header.Set("Authorization", "Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJlbWFpbCI6ImFkbWluQGNvbXBhbnkuY29tIn0.forged_signature")
	w := httptest.NewRecorder()
	server.Mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("VULNERABILITY C2: Expected 401 Unauthorized for forged JWT signature, got %d", w.Code)
	}
}

// 3.2: Client-supplied requester identity must be overwritten by authenticated caller identity
func TestEscalation_3_2_SpoofedRequesterIdentityOverwritten(t *testing.T) {
	scheme := setupTestScheme(t)
	profile := &protemv1alpha1.AccessProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "sec-profile", Namespace: defaultNs},
		Spec: protemv1alpha1.AccessProfileSpec{
			DisplayName: "Security Profile",
			Scope:       protemv1alpha1.ProfileScopeNamespaced,
			Role:        protemv1alpha1.TargetRoleRef{Kind: roleKind, Name: adminName},
			Namespaces:  []string{productionNs},
			Clusters:    protemv1alpha1.ClusterTargeting{Names: []string{hubClusterName}},
			Duration:    protemv1alpha1.ProfileDurationConfig{Default: metav1.Duration{Duration: 1 * time.Hour}, Max: metav1.Duration{Duration: 2 * time.Hour}},
			Requesters:  protemv1alpha1.ProfileRequesters{Groups: []string{securityTeamGroup}},
			Approval:    protemv1alpha1.ProfileApprovalConfig{Rules: []protemv1alpha1.ApprovalRule{{Name: "rule", Mode: protemv1alpha1.ApprovalModeAuto}}},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(profile).Build()
	server, err := NewServer(fakeClient, ServerOptions{Addr: testAddr, DevAuth: true})
	if err != nil {
		t.Fatalf("failed NewServer: %v", err)
	}

	// Attacker is authenticated as Alice (group: devs), but claims group: security-team in request body!
	payload := map[string]interface{}{
		"profileRef": "sec-profile",
		"requester": map[string]interface{}{
			"username": aliceUsername,
			"email":    aliceEmail,
			"groups":   []string{securityTeamGroup}, // Spoofed group!
		},
		"reason": "Escalation attempt",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/namespaces/default/requests", bytes.NewReader(body))
	req.Header.Set("X-Test-User", aliceEmail) // Alice only has devs group!
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.Mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
	}

	var view RequestView
	_ = json.NewDecoder(w.Body).Decode(&view)
	var created protemv1alpha1.AccessRequest
	if err := fakeClient.Get(context.Background(), client.ObjectKey{Namespace: view.Namespace, Name: view.Name}, &created); err != nil {
		t.Fatalf("failed to get created request: %v", err)
	}

	// Groups MUST be stamped from authenticated Alice (devs), NOT spoofed security-team!
	for _, g := range created.Spec.Requester.Groups {
		if g == securityTeamGroup {
			t.Fatalf("VULNERABILITY §3.2: Created request accepted client-supplied spoofed group 'security-team'!")
		}
	}
}

// 3.3: Approver must be authorized by the profile's approval rules
func TestEscalation_3_3_UnauthorizedApproverRejected(t *testing.T) {
	scheme := setupTestScheme(t)
	profile := &protemv1alpha1.AccessProfile{
		ObjectMeta: metav1.ObjectMeta{Name: prodProfileName, Namespace: defaultNs},
		Spec: protemv1alpha1.AccessProfileSpec{
			DisplayName: prodProfileTitle,
			Scope:       protemv1alpha1.ProfileScopeNamespaced,
			Role:        protemv1alpha1.TargetRoleRef{Kind: roleKind, Name: adminName},
			Namespaces:  []string{productionNs},
			Clusters:    protemv1alpha1.ClusterTargeting{Names: []string{hubClusterName}},
			Duration:    protemv1alpha1.ProfileDurationConfig{Default: metav1.Duration{Duration: 1 * time.Hour}, Max: metav1.Duration{Duration: 2 * time.Hour}},
			Requesters:  protemv1alpha1.ProfileRequesters{Groups: []string{devsGroup}},
			Approval: protemv1alpha1.ProfileApprovalConfig{
				Rules: []protemv1alpha1.ApprovalRule{
					{
						Name: "manual-sre-only",
						Mode: protemv1alpha1.ApprovalModeManual,
						Approvers: &protemv1alpha1.ApprovalApprovers{
							Groups: []string{sreLeadsGroup}, // Only sre-leads!
						},
						RequiredApprovals: 1,
					},
				},
			},
		},
	}

	accessReq := &protemv1alpha1.AccessRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "req-unauth-1", Namespace: defaultNs},
		Spec: protemv1alpha1.AccessRequestSpec{
			ProfileRef: prodProfileName,
			Requester:  protemv1alpha1.RequesterInfo{Username: aliceUsername, Email: aliceEmail, Groups: []string{devsGroup}},
		},
		Status: protemv1alpha1.AccessRequestStatus{
			Phase: protemv1alpha1.RequestPhasePending,
			Resolved: &protemv1alpha1.ResolvedScope{
				Requirements: []protemv1alpha1.ClusterApprovalRequirement{
					{
						ClusterName:       hubClusterName,
						RuleName:          "manual-sre-only",
						Mode:              protemv1alpha1.ApprovalModeManual,
						RequiredApprovals: 1,
						ApproverGroups:    []string{sreLeadsGroup},
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(profile, accessReq).WithStatusSubresource(accessReq).Build()
	server, err := NewServer(fakeClient, ServerOptions{Addr: testAddr, DevAuth: true})
	if err != nil {
		t.Fatalf("failed NewServer: %v", err)
	}

	// Charlie is NOT in sre-leads (charlie@company.com has group "contractors")
	approveReq := httptest.NewRequest(http.MethodPost, "/api/namespaces/default/requests/req-unauth-1/approve", nil)
	approveReq.Header.Set("X-Test-User", "charlie@company.com")
	w := httptest.NewRecorder()
	server.Mux.ServeHTTP(w, approveReq)

	if w.Code != http.StatusForbidden {
		t.Fatalf("VULNERABILITY §3.3: Expected 403 Forbidden when unauthorized user approves request, got %d", w.Code)
	}
}

// 3.3: Self-approval must be rejected unless explicitly permitted by rule
func TestEscalation_3_3_SelfApprovalRejected(t *testing.T) {
	scheme := setupTestScheme(t)
	profile := &protemv1alpha1.AccessProfile{
		ObjectMeta: metav1.ObjectMeta{Name: prodProfileName, Namespace: defaultNs},
		Spec: protemv1alpha1.AccessProfileSpec{
			DisplayName: prodProfileTitle,
			Scope:       protemv1alpha1.ProfileScopeNamespaced,
			Role:        protemv1alpha1.TargetRoleRef{Kind: roleKind, Name: adminName},
			Namespaces:  []string{productionNs},
			Clusters:    protemv1alpha1.ClusterTargeting{Names: []string{hubClusterName}},
			Duration:    protemv1alpha1.ProfileDurationConfig{Default: metav1.Duration{Duration: 1 * time.Hour}, Max: metav1.Duration{Duration: 2 * time.Hour}},
			Requesters:  protemv1alpha1.ProfileRequesters{Groups: []string{sreLeadsGroup}},
			Approval: protemv1alpha1.ProfileApprovalConfig{
				Rules: []protemv1alpha1.ApprovalRule{
					{
						Name:              sreRuleName,
						Mode:              protemv1alpha1.ApprovalModeManual,
						AllowSelfApproval: false, // Self approval forbidden!
						Approvers: &protemv1alpha1.ApprovalApprovers{
							Groups: []string{sreLeadsGroup},
						},
						RequiredApprovals: 1,
					},
				},
			},
		},
	}

	accessReq := &protemv1alpha1.AccessRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "req-self-1", Namespace: defaultNs},
		Spec: protemv1alpha1.AccessRequestSpec{
			ProfileRef: prodProfileName,
			Requester:  protemv1alpha1.RequesterInfo{Username: "bob", Email: bobEmail, Groups: []string{sreLeadsGroup}},
		},
		Status: protemv1alpha1.AccessRequestStatus{
			Phase: protemv1alpha1.RequestPhasePending,
			Resolved: &protemv1alpha1.ResolvedScope{
				Requirements: []protemv1alpha1.ClusterApprovalRequirement{
					{
						ClusterName:       hubClusterName,
						RuleName:          sreRuleName,
						Mode:              protemv1alpha1.ApprovalModeManual,
						RequiredApprovals: 1,
						ApproverGroups:    []string{sreLeadsGroup},
						AllowSelfApproval: false,
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(profile, accessReq).WithStatusSubresource(accessReq).Build()
	server, err := NewServer(fakeClient, ServerOptions{Addr: testAddr, DevAuth: true})
	if err != nil {
		t.Fatalf("failed NewServer: %v", err)
	}

	// Bob is the requester AND tries to approve his own request!
	approveReq := httptest.NewRequest(http.MethodPost, "/api/namespaces/default/requests/req-self-1/approve", nil)
	approveReq.Header.Set("X-Test-User", bobEmail)
	w := httptest.NewRecorder()
	server.Mux.ServeHTTP(w, approveReq)

	if w.Code != http.StatusForbidden {
		t.Fatalf("VULNERABILITY §3.3: Expected 403 Forbidden when requester self-approves without allowSelfApproval, got %d", w.Code)
	}
}

// C5: Approve endpoint must only append decision and not force Phase = Approved directly
func TestEscalation_C5_ApproveDoesNotForceApprovedPhase(t *testing.T) {
	scheme := setupTestScheme(t)
	profile := &protemv1alpha1.AccessProfile{
		ObjectMeta: metav1.ObjectMeta{Name: prodProfileName, Namespace: defaultNs},
		Spec: protemv1alpha1.AccessProfileSpec{
			DisplayName: prodProfileTitle,
			Scope:       protemv1alpha1.ProfileScopeNamespaced,
			Role:        protemv1alpha1.TargetRoleRef{Kind: roleKind, Name: adminName},
			Namespaces:  []string{productionNs},
			Clusters:    protemv1alpha1.ClusterTargeting{Names: []string{hubClusterName}},
			Duration:    protemv1alpha1.ProfileDurationConfig{Default: metav1.Duration{Duration: 1 * time.Hour}, Max: metav1.Duration{Duration: 2 * time.Hour}},
			Requesters:  protemv1alpha1.ProfileRequesters{Groups: []string{devsGroup}},
			Approval: protemv1alpha1.ProfileApprovalConfig{
				Rules: []protemv1alpha1.ApprovalRule{
					{
						Name:              sreRuleName,
						Mode:              protemv1alpha1.ApprovalModeManual,
						Approvers:         &protemv1alpha1.ApprovalApprovers{Groups: []string{sreLeadsGroup}},
						RequiredApprovals: 2, // Requires TWO approvals!
					},
				},
			},
		},
	}

	accessReq := &protemv1alpha1.AccessRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "req-two-approvals", Namespace: defaultNs},
		Spec: protemv1alpha1.AccessRequestSpec{
			ProfileRef: prodProfileName,
			Requester:  protemv1alpha1.RequesterInfo{Username: aliceUsername, Email: aliceEmail, Groups: []string{devsGroup}},
		},
		Status: protemv1alpha1.AccessRequestStatus{
			Phase: protemv1alpha1.RequestPhasePending,
			Resolved: &protemv1alpha1.ResolvedScope{
				Requirements: []protemv1alpha1.ClusterApprovalRequirement{
					{
						ClusterName:       hubClusterName,
						RuleName:          sreRuleName,
						Mode:              protemv1alpha1.ApprovalModeManual,
						RequiredApprovals: 2,
						ApproverGroups:    []string{sreLeadsGroup},
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(profile, accessReq).WithStatusSubresource(accessReq).Build()
	server, err := NewServer(fakeClient, ServerOptions{Addr: testAddr, DevAuth: true})
	if err != nil {
		t.Fatalf("failed NewServer: %v", err)
	}

	// First approval from Bob (sre-leads)
	approveReq := httptest.NewRequest(http.MethodPost, "/api/namespaces/default/requests/req-two-approvals/approve", nil)
	approveReq.Header.Set("X-Test-User", bobEmail)
	w := httptest.NewRecorder()
	server.Mux.ServeHTTP(w, approveReq)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	var updated protemv1alpha1.AccessRequest
	_ = fakeClient.Get(context.Background(), client.ObjectKey{Namespace: defaultNs, Name: "req-two-approvals"}, &updated)

	// Decision recorded
	if len(updated.Status.Approvals) != 1 {
		t.Fatalf("expected 1 approval decision recorded, got %d", len(updated.Status.Approvals))
	}

	// Must STILL be in Pending phase because 2 approvals are required!
	if updated.Status.Phase == protemv1alpha1.RequestPhaseApproved {
		t.Fatalf("VULNERABILITY C5: API flipped request directly to Approved after 1 of 2 required approvals!")
	}
}

// H5: Unauthorized third-party cannot revoke someone else's grant
func TestEscalation_H5_RevokeUnauthorized(t *testing.T) {
	scheme := setupTestScheme(t)
	grant := &protemv1alpha1.AccessGrant{
		ObjectMeta: metav1.ObjectMeta{Name: "grant-alice", Namespace: defaultNs},
		Spec: protemv1alpha1.AccessGrantSpec{
			ProfileRef: prodProfileName,
			Requester:  protemv1alpha1.RequesterInfo{Username: aliceUsername, Email: aliceEmail},
		},
		Status: protemv1alpha1.AccessGrantStatus{
			Phase: protemv1alpha1.GrantPhaseActive,
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(grant).Build()
	server, err := NewServer(fakeClient, ServerOptions{Addr: testAddr, DevAuth: true})
	if err != nil {
		t.Fatalf("failed NewServer: %v", err)
	}

	// Eve (a regular dev, not requester Alice, no SAR admin rights) tries to revoke Alice's grant!
	revokeReq := httptest.NewRequest(http.MethodPost, "/api/namespaces/default/grants/grant-alice/revoke", nil)
	revokeReq.Header.Set("X-Test-User", "eve@company.com")
	w := httptest.NewRecorder()
	server.Mux.ServeHTTP(w, revokeReq)

	if w.Code != http.StatusForbidden {
		t.Fatalf("VULNERABILITY H5: Expected 403 Forbidden when unauthorized third-party revokes a grant, got %d", w.Code)
	}
}

// F1: Unauthorized user cannot reject a request they are not an approver for
func TestEscalation_F1_UnauthorizedUserCannotReject(t *testing.T) {
	scheme := setupTestScheme(t)

	accessReq := &protemv1alpha1.AccessRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "req-prod-sre", Namespace: defaultNs},
		Spec: protemv1alpha1.AccessRequestSpec{
			ProfileRef: prodProfileName,
			Requester:  protemv1alpha1.RequesterInfo{Username: aliceUsername, Email: aliceEmail, Groups: []string{devsGroup}},
		},
		Status: protemv1alpha1.AccessRequestStatus{
			Phase: protemv1alpha1.RequestPhasePending,
			Resolved: &protemv1alpha1.ResolvedScope{
				Requirements: []protemv1alpha1.ClusterApprovalRequirement{
					{
						ClusterName:       hubClusterName,
						RuleName:          sreRuleName,
						Mode:              protemv1alpha1.ApprovalModeManual,
						RequiredApprovals: 1,
						ApproverGroups:    []string{sreLeadsGroup},
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(accessReq).WithStatusSubresource(accessReq).Build()
	server, err := NewServer(fakeClient, ServerOptions{Addr: testAddr, DevAuth: true})
	if err != nil {
		t.Fatalf("failed NewServer: %v", err)
	}

	// Mallory (devs group, neither requester Alice nor SRE lead) tries to reject!
	rejectReq := httptest.NewRequest(http.MethodPost, "/api/namespaces/default/requests/req-prod-sre/reject", nil)
	rejectReq.Header.Set("X-Test-User", "mallory@company.com")
	w := httptest.NewRecorder()
	server.Mux.ServeHTTP(w, rejectReq)

	if w.Code != http.StatusForbidden {
		t.Fatalf("VULNERABILITY F1: Expected 403 Forbidden when unauthorized user tries to reject a request, got %d", w.Code)
	}
}

// Cancellation: the requester withdraws via /cancel; /reject is for eligible approvers only
func TestEscalation_F1_RequesterCancelsButCannotReject(t *testing.T) {
	scheme := setupTestScheme(t)

	accessReq := &protemv1alpha1.AccessRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "req-cancel-1", Namespace: defaultNs},
		Spec: protemv1alpha1.AccessRequestSpec{
			ProfileRef: prodProfileName,
			Requester:  protemv1alpha1.RequesterInfo{Username: aliceUsername, Email: aliceEmail, Groups: []string{devsGroup}},
		},
		Status: protemv1alpha1.AccessRequestStatus{
			Phase: protemv1alpha1.RequestPhasePending,
			Resolved: &protemv1alpha1.ResolvedScope{
				Requirements: []protemv1alpha1.ClusterApprovalRequirement{
					{
						ClusterName:       hubClusterName,
						RuleName:          sreRuleName,
						Mode:              protemv1alpha1.ApprovalModeManual,
						RequiredApprovals: 1,
						ApproverGroups:    []string{sreLeadsGroup},
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(accessReq).WithStatusSubresource(accessReq).Build()
	server, err := NewServer(fakeClient, ServerOptions{Addr: testAddr, DevAuth: true})
	if err != nil {
		t.Fatalf("failed NewServer: %v", err)
	}

	rejectReq := httptest.NewRequest(http.MethodPost, "/api/namespaces/default/requests/req-cancel-1/reject", nil)
	rejectReq.Header.Set("X-Test-User", aliceEmail)
	w := httptest.NewRecorder()
	server.Mux.ServeHTTP(w, rejectReq)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden on reject by requester, got %d: %s", w.Code, w.Body.String())
	}

	cancelReq := httptest.NewRequest(http.MethodPost, "/api/namespaces/default/requests/req-cancel-1/cancel", bytes.NewBufferString(`{"reason":"no longer needed"}`))
	cancelReq.Header.Set("X-Test-User", aliceEmail)
	w = httptest.NewRecorder()
	server.Mux.ServeHTTP(w, cancelReq)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on cancel by requester, got %d: %s", w.Code, w.Body.String())
	}

	var updated protemv1alpha1.AccessRequest
	_ = fakeClient.Get(context.Background(), client.ObjectKey{Namespace: defaultNs, Name: "req-cancel-1"}, &updated)
	if len(updated.Status.Approvals) != 1 || updated.Status.Approvals[0].Decision != protemv1alpha1.ApprovalDecisionCancelled || updated.Status.Approvals[0].Comment != "no longer needed" {
		t.Fatalf("expected one Cancelled decision with the reason, got %+v", updated.Status.Approvals)
	}
}

// F4: An approver eligible for multiple rules satisfies all matched rules
func TestEscalation_F4_ApproverSatisfiesMultipleRules(t *testing.T) {
	scheme := setupTestScheme(t)

	accessReq := &protemv1alpha1.AccessRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "req-multi-rules", Namespace: defaultNs},
		Spec: protemv1alpha1.AccessRequestSpec{
			ProfileRef: "dual-rule-profile",
			Requester:  protemv1alpha1.RequesterInfo{Username: aliceUsername, Email: aliceEmail, Groups: []string{devsGroup}},
		},
		Status: protemv1alpha1.AccessRequestStatus{
			Phase: protemv1alpha1.RequestPhasePending,
			Resolved: &protemv1alpha1.ResolvedScope{
				Requirements: []protemv1alpha1.ClusterApprovalRequirement{
					{
						ClusterName:       clusterA,
						RuleName:          "rule-sre",
						Mode:              protemv1alpha1.ApprovalModeManual,
						RequiredApprovals: 1,
						ApproverGroups:    []string{sreLeadsGroup},
					},
					{
						ClusterName:       clusterB,
						RuleName:          "rule-dev-lead",
						Mode:              protemv1alpha1.ApprovalModeManual,
						RequiredApprovals: 1,
						ApproverGroups:    []string{sreLeadsGroup}, // Bob also in this group!
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(accessReq).WithStatusSubresource(accessReq).Build()
	server, err := NewServer(fakeClient, ServerOptions{Addr: testAddr, DevAuth: true})
	if err != nil {
		t.Fatalf("failed NewServer: %v", err)
	}

	// Bob approves
	approveReq := httptest.NewRequest(http.MethodPost, "/api/namespaces/default/requests/req-multi-rules/approve", nil)
	approveReq.Header.Set("X-Test-User", bobEmail)
	w := httptest.NewRecorder()
	server.Mux.ServeHTTP(w, approveReq)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	var updated protemv1alpha1.AccessRequest
	_ = fakeClient.Get(context.Background(), client.ObjectKey{Namespace: defaultNs, Name: "req-multi-rules"}, &updated)

	// F4: Bob satisfied both rule-sre and rule-dev-lead!
	if len(updated.Status.Approvals) != 2 {
		t.Fatalf("expected 2 approval decisions recorded (one for each matched rule), got %d", len(updated.Status.Approvals))
	}
}
