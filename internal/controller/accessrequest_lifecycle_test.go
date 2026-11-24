package controller

import (
	"reflect"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
)

var lifecycleNow = time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)

const (
	req1Grant = "grant-req-1"
	prodAdmin = "prod-admin"
)

func lifecycleProfile() *v1.AccessProfile {
	return &v1.AccessProfile{
		ObjectMeta: metav1.ObjectMeta{Name: prodAdmin, Namespace: defaultNs},
		Spec: v1.AccessProfileSpec{
			Scope:      v1.ProfileScopeNamespaced,
			Role:       v1.TargetRoleRef{Kind: roleKind, Name: adminName},
			Namespaces: []string{productionNs},
			Clusters:   v1.ClusterTargeting{Names: []string{hubClusterName}},
			Duration: v1.ProfileDurationConfig{
				Default: metav1.Duration{Duration: time.Hour},
				Max:     metav1.Duration{Duration: 4 * time.Hour},
			},
			Requesters:    v1.ProfileRequesters{Groups: []string{devsGroup}},
			Justification: v1.ProfileJustification{Reason: v1.JustificationRequired},
			Approval: v1.ProfileApprovalConfig{
				Rules: []v1.ApprovalRule{{
					Name:              manualRule,
					Mode:              v1.ApprovalModeManual,
					Approvers:         &v1.ApprovalApprovers{Groups: []string{sreLeadsGroup}},
					RequiredApprovals: 1,
				}},
			},
		},
	}
}

func lifecycleRequest() *v1.AccessRequest {
	return &v1.AccessRequest{
		ObjectMeta: metav1.ObjectMeta{Name: reqOne, Namespace: defaultNs, CreationTimestamp: metav1.NewTime(lifecycleNow.Add(-time.Minute))},
		Spec: v1.AccessRequestSpec{
			ProfileRef:    prodAdmin,
			Requester:     v1.RequesterInfo{Username: aliceUsername, Email: aliceEmail, Groups: []string{devsGroup}},
			Justification: v1.RequestJustification{Reason: "Production fix"},
		},
	}
}

func pendingRequest() *v1.AccessRequest {
	req := lifecycleRequest()
	req.Status = v1.AccessRequestStatus{
		Phase: v1.RequestPhasePending,
		Resolved: &v1.ResolvedScope{
			Role:       &v1.TargetRoleRef{Kind: roleKind, Name: adminName},
			Scope:      v1.ProfileScopeNamespaced,
			Clusters:   []string{hubClusterName},
			Namespaces: []string{productionNs},
			Duration:   metav1.Duration{Duration: 2 * time.Hour},
			Requirements: []v1.ClusterApprovalRequirement{
				{ClusterName: hubClusterName, RuleName: manualRule, Mode: v1.ApprovalModeManual, RequiredApprovals: 1, ApproverGroups: []string{sreLeadsGroup}},
			},
		},
		Conditions: []metav1.Condition{
			{Type: "Decided", Status: metav1.ConditionFalse, Reason: "PendingApproval", LastTransitionTime: metav1.NewTime(lifecycleNow.Add(-time.Minute))},
		},
	}
	return req
}

func approvedRequest() *v1.AccessRequest {
	req := pendingRequest()
	req.Status.Phase = v1.RequestPhaseApproved
	return req
}

func decision(user string, kind v1.ApprovalDecisionType, at time.Time, comment string) v1.ApprovalDecision {
	return v1.ApprovalDecision{User: user, Decision: kind, At: metav1.NewTime(at), Comment: comment, Rule: manualRule}
}

func wantCondition(t *testing.T, tr Transition, condType string, status metav1.ConditionStatus, reason, message string) {
	t.Helper()
	if tr.Status == nil {
		t.Fatalf("expected a status write, got none")
	}
	c := meta.FindStatusCondition(tr.Status.Conditions, condType)
	if c == nil {
		t.Fatalf("missing %s condition in %v", condType, tr.Status.Conditions)
	}
	if c.Status != status || c.Reason != reason || (message != "" && c.Message != message) {
		t.Fatalf("%s condition = %s/%s %q, want %s/%s %q", condType, c.Status, c.Reason, c.Message, status, reason, message)
	}
}

func wantPhase(t *testing.T, tr Transition, phase v1.RequestPhase) {
	t.Helper()
	if tr.Status == nil || tr.Status.Phase != phase {
		t.Fatalf("expected phase %s, got %+v", phase, tr.Status)
	}
	if tr.Grant != nil {
		t.Fatalf("expected no grant, got %+v", tr.Grant)
	}
}

func TestAdvance(t *testing.T) {
	readyClusters := []v1.TargetCluster{*makeReadyCluster(hubClusterName)}

	cases := []struct {
		name     string
		req      func() *v1.AccessRequest
		profile  func() *v1.AccessProfile
		clusters []v1.TargetCluster
		others   Others
		check    func(t *testing.T, tr Transition)
	}{
		{
			name:    "missing profile rejects",
			req:     lifecycleRequest,
			profile: func() *v1.AccessProfile { return nil },
			check: func(t *testing.T, tr Transition) {
				wantPhase(t, tr, v1.RequestPhaseRejected)
				wantCondition(t, tr, "Validated", metav1.ConditionFalse, "ProfileNotFound", `AccessProfile "prod-admin" does not exist in namespace "default"`)
			},
		},
		{
			name: "requester outside requester groups is rejected",
			req: func() *v1.AccessRequest {
				req := lifecycleRequest()
				req.Spec.Requester.Groups = []string{"guests"}
				return req
			},
			profile:  lifecycleProfile,
			clusters: readyClusters,
			check: func(t *testing.T, tr Transition) {
				wantPhase(t, tr, v1.RequestPhaseRejected)
				wantCondition(t, tr, "Validated", metav1.ConditionFalse, "UnauthorizedRequester", `"alice" may not request this profile. It can be requested by the groups [devs].`)
			},
		},
		{
			name: "requester outside requester groups and users is rejected",
			req: func() *v1.AccessRequest {
				req := lifecycleRequest()
				req.Spec.Requester.Groups = []string{"guests"}
				return req
			},
			profile: func() *v1.AccessProfile {
				p := lifecycleProfile()
				p.Spec.Requesters.Users = []string{"bob", "carol@company.com"}
				return p
			},
			clusters: readyClusters,
			check: func(t *testing.T, tr Transition) {
				wantPhase(t, tr, v1.RequestPhaseRejected)
				wantCondition(t, tr, "Validated", metav1.ConditionFalse, "UnauthorizedRequester", `"alice" may not request this profile. It can be requested by the groups [devs] and the users [bob carol@company.com].`)
			},
		},
		{
			name: "requester outside requester users is rejected",
			req:  lifecycleRequest,
			profile: func() *v1.AccessProfile {
				p := lifecycleProfile()
				p.Spec.Requesters = v1.ProfileRequesters{Users: []string{"bob"}}
				return p
			},
			clusters: readyClusters,
			check: func(t *testing.T, tr Transition) {
				wantPhase(t, tr, v1.RequestPhaseRejected)
				wantCondition(t, tr, "Validated", metav1.ConditionFalse, "UnauthorizedRequester", `"alice" may not request this profile. It can be requested by the users [bob].`)
			},
		},
		{
			name: "ticket not matching pattern is rejected",
			req: func() *v1.AccessRequest {
				req := lifecycleRequest()
				req.Spec.Justification.Ticket = "nope"
				return req
			},
			profile: func() *v1.AccessProfile {
				p := lifecycleProfile()
				p.Spec.Justification.TicketPattern = "^INC-[0-9]+$"
				return p
			},
			clusters: readyClusters,
			check: func(t *testing.T, tr Transition) {
				wantPhase(t, tr, v1.RequestPhaseRejected)
				wantCondition(t, tr, "Validated", metav1.ConditionFalse, "InvalidTicket", `Ticket "nope" does not match required pattern (^INC-[0-9]+$)`)
			},
		},
		{
			name: "missing required reason is rejected",
			req: func() *v1.AccessRequest {
				req := lifecycleRequest()
				req.Spec.Justification.Reason = ""
				return req
			},
			profile:  lifecycleProfile,
			clusters: readyClusters,
			check: func(t *testing.T, tr Transition) {
				wantPhase(t, tr, v1.RequestPhaseRejected)
				wantCondition(t, tr, "Validated", metav1.ConditionFalse, "MissingJustification", "A business justification reason is required for this profile")
			},
		},
		{
			name:    "no ready cluster fails resolution",
			req:     lifecycleRequest,
			profile: lifecycleProfile,
			check: func(t *testing.T, tr Transition) {
				wantPhase(t, tr, v1.RequestPhaseRejected)
				wantCondition(t, tr, "Validated", metav1.ConditionFalse, "ResolutionFailed", `no ready clusters matched for profile "prod-admin"`)
			},
		},
		{
			name: "a new request waits for a targeted cluster that is not ready yet",
			req: func() *v1.AccessRequest {
				r := lifecycleRequest()
				r.CreationTimestamp = metav1.NewTime(lifecycleNow.Add(-10 * time.Second))
				return r
			},
			profile:  lifecycleProfile,
			clusters: []v1.TargetCluster{{ObjectMeta: metav1.ObjectMeta{Name: hubClusterName}}},
			check: func(t *testing.T, tr Transition) {
				if tr.Status != nil || tr.Grant != nil || tr.RequeueAfter <= 0 {
					t.Fatalf("expected only a requeue, got %+v", tr)
				}
			},
		},
		{
			name:     "a request whose cluster stays not ready is rejected saying so",
			req:      lifecycleRequest,
			profile:  lifecycleProfile,
			clusters: []v1.TargetCluster{{ObjectMeta: metav1.ObjectMeta{Name: hubClusterName}}},
			check: func(t *testing.T, tr Transition) {
				wantPhase(t, tr, v1.RequestPhaseRejected)
				wantCondition(t, tr, "Validated", metav1.ConditionFalse, "ClusterNotReady", "")
			},
		},
		{
			name: "auto rule approves with system decision",
			req:  lifecycleRequest,
			profile: func() *v1.AccessProfile {
				p := lifecycleProfile()
				p.Spec.Approval.Rules = []v1.ApprovalRule{{Name: "auto-rule", Mode: v1.ApprovalModeAuto}}
				return p
			},
			clusters: readyClusters,
			check: func(t *testing.T, tr Transition) {
				wantPhase(t, tr, v1.RequestPhaseApproved)
				wantCondition(t, tr, "Validated", metav1.ConditionTrue, "ScopeResolved", "")
				wantCondition(t, tr, "Decided", metav1.ConditionTrue, "AutoApproved", "")
				want := []v1.ApprovalDecision{{User: "system:auto-approval", Decision: v1.ApprovalDecisionApproved, At: metav1.NewTime(lifecycleNow), Rule: "auto"}}
				if !reflect.DeepEqual(tr.Status.Approvals, want) {
					t.Fatalf("approvals = %+v, want %+v", tr.Status.Approvals, want)
				}
				if got := meta.FindStatusCondition(tr.Status.Conditions, "Decided").LastTransitionTime.Time; !got.Equal(lifecycleNow) {
					t.Fatalf("Decided transition = %v, want %v", got, lifecycleNow)
				}
			},
		},
		{
			name:     "a profile the requester already waits for on the same cluster is rejected",
			req:      lifecycleRequest,
			profile:  lifecycleProfile,
			clusters: readyClusters,
			others: Others{Requests: []v1.AccessRequest{func() v1.AccessRequest {
				earlier := pendingRequest()
				earlier.Name = "req-earlier"
				return *earlier
			}()}},
			check: func(t *testing.T, tr Transition) {
				wantPhase(t, tr, v1.RequestPhaseRejected)
				wantCondition(t, tr, "Validated", metav1.ConditionFalse, "AlreadyHeld", "")
				if tr.Status.Resolved != nil {
					t.Fatalf("a refused request must not freeze a scope, got %+v", tr.Status.Resolved)
				}
			},
		},
		{
			name:     "manual rule freezes scope and waits",
			req:      lifecycleRequest,
			profile:  lifecycleProfile,
			clusters: readyClusters,
			check: func(t *testing.T, tr Transition) {
				wantPhase(t, tr, v1.RequestPhasePending)
				wantCondition(t, tr, "Decided", metav1.ConditionFalse, "PendingApproval", "")
				want := &v1.ResolvedScope{
					Role:       &v1.TargetRoleRef{Kind: roleKind, Name: adminName},
					Scope:      v1.ProfileScopeNamespaced,
					Clusters:   []string{hubClusterName},
					Namespaces: []string{productionNs},
					Duration:   metav1.Duration{Duration: time.Hour},
					Requirements: []v1.ClusterApprovalRequirement{
						{ClusterName: hubClusterName, RuleName: manualRule, Mode: v1.ApprovalModeManual, RequiredApprovals: 1, ApproverGroups: []string{sreLeadsGroup}},
					},
				}
				if !reflect.DeepEqual(tr.Status.Resolved, want) {
					t.Fatalf("resolved = %+v, want %+v", tr.Status.Resolved, want)
				}
			},
		},
		{
			name: "pending past timeout times out",
			req: func() *v1.AccessRequest {
				req := pendingRequest()
				req.CreationTimestamp = metav1.NewTime(lifecycleNow.Add(-2 * time.Hour))
				meta.SetStatusCondition(&req.Status.Conditions, metav1.Condition{
					Type: "Decided", Status: metav1.ConditionFalse, Reason: "PendingApproval",
					LastTransitionTime: metav1.NewTime(lifecycleNow.Add(-2 * time.Hour)),
				})
				return req
			},
			profile: func() *v1.AccessProfile {
				p := lifecycleProfile()
				p.Spec.Approval.PendingTimeout = &metav1.Duration{Duration: time.Hour}
				return p
			},
			check: func(t *testing.T, tr Transition) {
				wantPhase(t, tr, v1.RequestPhaseTimedOut)
				wantCondition(t, tr, "Decided", metav1.ConditionFalse, "TimedOut", "Request timed out awaiting approvals")
				if got := meta.FindStatusCondition(tr.Status.Conditions, "Decided").LastTransitionTime.Time; !got.Equal(lifecycleNow) {
					t.Fatalf("Decided changed at %v, want the moment it timed out %v", got, lifecycleNow)
				}
			},
		},
		{
			name: "requester cancellation cancels",
			req: func() *v1.AccessRequest {
				req := pendingRequest()
				req.Status.Approvals = []v1.ApprovalDecision{decision(aliceEmail, v1.ApprovalDecisionCancelled, lifecycleNow, "")}
				return req
			},
			profile: lifecycleProfile,
			check: func(t *testing.T, tr Transition) {
				wantPhase(t, tr, v1.RequestPhaseCancelled)
				wantCondition(t, tr, "Decided", metav1.ConditionFalse, "Cancelled", "Request cancelled by requester")
			},
		},
		{
			name: "rejection names the first rejector",
			req: func() *v1.AccessRequest {
				req := pendingRequest()
				req.Status.Approvals = []v1.ApprovalDecision{
					decision("carol@company.com", v1.ApprovalDecisionApproved, lifecycleNow, ""),
					decision("bob@company.com", v1.ApprovalDecisionRejected, lifecycleNow, "no"),
					decision("dave@company.com", v1.ApprovalDecisionRejected, lifecycleNow, "also no"),
				}
				return req
			},
			profile: lifecycleProfile,
			check: func(t *testing.T, tr Transition) {
				wantPhase(t, tr, v1.RequestPhaseRejected)
				wantCondition(t, tr, "Decided", metav1.ConditionFalse, "Rejected", "Rejected by bob@company.com: no")
			},
		},
		{
			name: "satisfied requirements approve",
			req: func() *v1.AccessRequest {
				req := pendingRequest()
				req.Status.Approvals = []v1.ApprovalDecision{decision("bob@company.com", v1.ApprovalDecisionApproved, lifecycleNow.Add(-time.Second), "")}
				return req
			},
			profile: lifecycleProfile,
			check: func(t *testing.T, tr Transition) {
				wantPhase(t, tr, v1.RequestPhaseApproved)
				wantCondition(t, tr, "Decided", metav1.ConditionTrue, "Approved", "All required approvals obtained")
				if got := meta.FindStatusCondition(tr.Status.Conditions, "Decided").LastTransitionTime.Time; !got.Equal(lifecycleNow) {
					t.Fatalf("Decided transition = %v, want %v", got, lifecycleNow)
				}
			},
		},
		{
			name:    "undecided pending polls every 30s",
			req:     pendingRequest,
			profile: lifecycleProfile,
			check: func(t *testing.T, tr Transition) {
				if tr.Status != nil || tr.Grant != nil || tr.RequeueAfter != 30*time.Second {
					t.Fatalf("expected requeue-only 30s transition, got %+v", tr)
				}
			},
		},
		{
			name: "undecided pending near timeout requeues just after the deadline",
			req: func() *v1.AccessRequest {
				req := pendingRequest()
				req.CreationTimestamp = metav1.NewTime(lifecycleNow.Add(-time.Hour + 10*time.Second))
				return req
			},
			profile: func() *v1.AccessProfile {
				p := lifecycleProfile()
				p.Spec.Approval.PendingTimeout = &metav1.Duration{Duration: time.Hour}
				return p
			},
			check: func(t *testing.T, tr Transition) {
				if tr.Status != nil || tr.Grant != nil || tr.RequeueAfter != 11*time.Second {
					t.Fatalf("expected requeue-only 11s transition, got %+v", tr)
				}
			},
		},
		{
			name: "approved without grant issues grant from the Decided transition",
			req: func() *v1.AccessRequest {
				req := approvedRequest()
				req.Status.Resolved.Requirements[0].RequiredApprovals = 2
				req.Status.Approvals = []v1.ApprovalDecision{
					decision("bob@company.com", v1.ApprovalDecisionApproved, lifecycleNow.Add(-50*time.Minute), ""),
					decision("carol@company.com", v1.ApprovalDecisionApproved, lifecycleNow.Add(-10*time.Minute), ""),
				}
				meta.SetStatusCondition(&req.Status.Conditions, metav1.Condition{Type: "Decided", Status: metav1.ConditionTrue, Reason: "Approved", LastTransitionTime: metav1.NewTime(lifecycleNow.Add(-10 * time.Minute))})
				return req
			},
			profile: lifecycleProfile,
			check: func(t *testing.T, tr Transition) {
				if tr.Status == nil || tr.Status.Phase != v1.RequestPhaseApproved || tr.Status.GrantRef != req1Grant {
					t.Fatalf("expected Approved status recording grant-req-1, got %+v", tr.Status)
				}
				want := &v1.AccessGrant{
					ObjectMeta: metav1.ObjectMeta{
						Name:      req1Grant,
						Namespace: defaultNs,
						Labels: map[string]string{
							"protem.dev/request-ref": reqOne,
							LabelProfileRef:          prodAdmin,
						},
					},
					Spec: v1.AccessGrantSpec{
						RequestRef: reqOne,
						ProfileRef: prodAdmin,
						Requester:  v1.RequesterInfo{Username: aliceUsername, Email: aliceEmail, Groups: []string{devsGroup}},
						Role:       v1.TargetRoleRef{Kind: roleKind, Name: adminName},
						Scope:      v1.ProfileScopeNamespaced,
						Namespaces: []string{productionNs},
						Clusters:   []string{hubClusterName},
						ExpiresAt:  metav1.NewTime(lifecycleNow.Add(-10*time.Minute + 2*time.Hour)),
					},
				}
				if !reflect.DeepEqual(tr.Grant, want) {
					t.Fatalf("grant = %+v, want %+v", tr.Grant, want)
				}
			},
		},
		{
			name: "profile edited after approval does not change the role or scope of the grant",
			req:  approvedRequest,
			profile: func() *v1.AccessProfile {
				p := lifecycleProfile()
				p.Spec.Role = v1.TargetRoleRef{Kind: roleKind, Name: "cluster-admin"}
				p.Spec.Scope = v1.ProfileScopeCluster
				return p
			},
			check: func(t *testing.T, tr Transition) {
				if tr.Grant == nil {
					t.Fatalf("expected a grant, got none (status %+v)", tr.Status)
				}
				if tr.Grant.Spec.Role.Name != adminName || tr.Grant.Spec.Scope != v1.ProfileScopeNamespaced {
					t.Fatalf("grant carries %s/%s, want the approved %s/%s", tr.Grant.Spec.Role.Name, tr.Grant.Spec.Scope, adminName, v1.ProfileScopeNamespaced)
				}
			},
		},
		{
			name: "approved request with no frozen role is rejected instead of reading the live profile",
			req: func() *v1.AccessRequest {
				req := approvedRequest()
				req.Status.Resolved.Role = nil
				return req
			},
			profile: lifecycleProfile,
			check: func(t *testing.T, tr Transition) {
				wantPhase(t, tr, v1.RequestPhaseRejected)
				wantCondition(t, tr, "Decided", metav1.ConditionFalse, "UnresolvedScope", "")
			},
		},
		{
			name:    "approved without a Decided condition expires the frozen duration from now",
			req:     approvedRequest,
			profile: lifecycleProfile,
			check: func(t *testing.T, tr Transition) {
				if tr.Grant == nil || !tr.Grant.Spec.ExpiresAt.Time.Equal(lifecycleNow.Add(2*time.Hour)) {
					t.Fatalf("expected grant expiring at %v, got %+v", lifecycleNow.Add(2*time.Hour), tr.Grant)
				}
			},
		},
		{
			name: "approved request with no frozen duration is rejected instead of using the profile's current default",
			req: func() *v1.AccessRequest {
				req := approvedRequest()
				req.Status.Resolved.Duration = metav1.Duration{}
				return req
			},
			profile: lifecycleProfile,
			check: func(t *testing.T, tr Transition) {
				wantPhase(t, tr, v1.RequestPhaseRejected)
				wantCondition(t, tr, "Decided", metav1.ConditionFalse, "UnresolvedScope", "")
			},
		},
		{
			name: "approved with grant recorded is settled",
			req: func() *v1.AccessRequest {
				req := approvedRequest()
				req.Status.GrantRef = req1Grant
				return req
			},
			profile: lifecycleProfile,
			check: func(t *testing.T, tr Transition) {
				if !reflect.DeepEqual(tr, Transition{}) {
					t.Fatalf("expected empty transition, got %+v", tr)
				}
			},
		},
		{
			name: "terminal phase is settled",
			req: func() *v1.AccessRequest {
				req := pendingRequest()
				req.Status.Phase = v1.RequestPhaseTimedOut
				return req
			},
			profile: lifecycleProfile,
			check: func(t *testing.T, tr Transition) {
				if !reflect.DeepEqual(tr, Transition{}) {
					t.Fatalf("expected empty transition, got %+v", tr)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, profile := tc.req(), tc.profile()
			reqBefore, profileBefore := req.DeepCopy(), profile.DeepCopy()

			tc.check(t, Advance(req, profile, tc.clusters, tc.others, lifecycleNow))

			if !reflect.DeepEqual(req, reqBefore) || !reflect.DeepEqual(profile, profileBefore) {
				t.Fatalf("Advance mutated its inputs")
			}
		})
	}
}
