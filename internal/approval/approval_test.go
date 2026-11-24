package approval_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
	"github.com/iLLeniumStudios/protem/internal/approval"
)

const (
	aliceEmail    = "alice@company.com"
	bobEmail      = "bob@company.com"
	aliceUsername = "alice"
	bobUsername   = "bob"
	devsGroup     = "devs"
	sreRuleName   = "sre"
	erinUsername  = "erin"
)

var (
	alice   = approval.Person{Username: aliceUsername, Email: aliceEmail, Groups: []string{devsGroup}}
	bob     = approval.Person{Username: bobUsername, Email: bobEmail, Groups: []string{"sre-leads"}}
	mallory = approval.Person{Username: "mallory", Email: "mallory@company.com", Groups: []string{devsGroup}}
)

func manual(rule string, groups ...string) v1.ClusterApprovalRequirement {
	return v1.ClusterApprovalRequirement{ClusterName: "c-" + rule, RuleName: rule, Mode: v1.ApprovalModeManual, RequiredApprovals: 1, ApproverGroups: groups}
}

func request(phase v1.RequestPhase, reqs ...v1.ClusterApprovalRequirement) *v1.AccessRequest {
	r := &v1.AccessRequest{
		Spec:   v1.AccessRequestSpec{Requester: v1.RequesterInfo{Username: aliceUsername, Email: aliceEmail, Groups: []string{devsGroup}}},
		Status: v1.AccessRequestStatus{Phase: phase},
	}
	if reqs != nil {
		r.Status.Resolved = &v1.ResolvedScope{Requirements: reqs}
	}
	return r
}

func TestSamePerson(t *testing.T) {
	cases := []struct {
		name string
		a, b approval.Person
		want bool
	}{
		{"same username", approval.Person{Username: aliceUsername}, approval.Person{Username: aliceUsername, Email: "x@y"}, true},
		{"same email", approval.Person{Username: "a", Email: aliceEmail}, approval.Person{Username: "b", Email: aliceEmail}, true},
		{"empty emails do not match", approval.Person{Username: aliceUsername}, approval.Person{Username: bobUsername}, false},
		{"empty usernames do not match", approval.Person{Email: "a@x"}, approval.Person{Email: "b@x"}, false},
		{"all empty", approval.Person{}, approval.Person{}, false},
		{"different", alice, bob, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := approval.SamePerson(tc.a, tc.b); got != tc.want {
				t.Fatalf("SamePerson = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCanRequestAndIsApprover(t *testing.T) {
	profile := &v1.AccessProfile{Spec: v1.AccessProfileSpec{
		Requesters: v1.ProfileRequesters{Groups: []string{devsGroup}},
		Approval: v1.ProfileApprovalConfig{Rules: []v1.ApprovalRule{
			{Name: "auto", Mode: v1.ApprovalModeAuto},
			{Name: sreRuleName, Mode: v1.ApprovalModeManual, Approvers: &v1.ApprovalApprovers{Groups: []string{"sre-leads"}, Users: []string{"carol@company.com", "dave", ""}}},
		}},
	}}
	cases := []struct {
		name                 string
		p                    approval.Person
		canRequest, approver bool
	}{
		{"requester group", alice, true, false},
		{"approver group", bob, false, true},
		{"approver by email", approval.Person{Email: "carol@company.com"}, false, true},
		{"approver by username", approval.Person{Username: "dave"}, false, true},
		{"empty identity matches no user entry", approval.Person{}, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := approval.CanRequest(tc.p, profile); got != tc.canRequest {
				t.Fatalf("CanRequest = %v, want %v", got, tc.canRequest)
			}
			if got := approval.IsApprover(tc.p, profile); got != tc.approver {
				t.Fatalf("IsApprover = %v, want %v", got, tc.approver)
			}
		})
	}
}

func TestCanRequestByUser(t *testing.T) {
	usersOnly := &v1.AccessProfile{Spec: v1.AccessProfileSpec{Requesters: v1.ProfileRequesters{Users: []string{bobUsername, "carol@company.com", ""}}}}
	both := &v1.AccessProfile{Spec: v1.AccessProfileSpec{Requesters: v1.ProfileRequesters{Groups: []string{devsGroup}, Users: []string{bobEmail}}}}
	cases := []struct {
		name    string
		p       approval.Person
		profile *v1.AccessProfile
		want    bool
	}{
		{"user by username", bob, usersOnly, true},
		{"user by email", approval.Person{Username: "carol", Email: "carol@company.com"}, usersOnly, true},
		{"stranger", mallory, usersOnly, false},
		{"empty identity matches no empty user entry", approval.Person{}, usersOnly, false},
		{"user beside groups", bob, both, true},
		{"group beside users", alice, both, true},
		{"stranger beside groups", approval.Person{Username: erinUsername, Groups: []string{"ops"}}, both, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := approval.CanRequest(tc.p, tc.profile); got != tc.want {
				t.Fatalf("CanRequest = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCanView(t *testing.T) {
	auto := manual("auto", "sre-leads")
	auto.Mode = v1.ApprovalModeAuto
	cases := []struct {
		name string
		p    approval.Person
		req  *v1.AccessRequest
		want bool
	}{
		{"requester", alice, request(v1.RequestPhasePending), true},
		{"approver of auto requirement", bob, request(v1.RequestPhaseApproved, auto), true},
		{"stranger", mallory, request(v1.RequestPhasePending, manual(sreRuleName, "sre-leads")), false},
		{"unresolved stranger", bob, request(v1.RequestPhasePending), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := approval.CanView(tc.p, tc.req); got != tc.want {
				t.Fatalf("CanView = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDecide(t *testing.T) {
	auto := manual("auto", "sre-leads")
	auto.Mode = v1.ApprovalModeAuto
	selfOK := manual("self", devsGroup)
	selfOK.AllowSelfApproval = true
	byUsername := manual("by-user")
	byUsername.ApproverUsers = []string{erinUsername}
	twoNeeded := manual(sreRuleName, "sre-leads")
	twoNeeded.RequiredApprovals = 2
	approvedBy := func(user, rule string, req *v1.AccessRequest) *v1.AccessRequest {
		req.Status.Approvals = append(req.Status.Approvals, v1.ApprovalDecision{User: user, Decision: v1.ApprovalDecisionApproved, Rule: rule})
		return req
	}
	erin := approval.Person{Username: erinUsername}

	cases := []struct {
		name     string
		p        approval.Person
		req      *v1.AccessRequest
		kind     v1.ApprovalDecisionType
		wantErr  error
		wantRule []string
		wantUser string
	}{
		{"approve skips auto requirements", bob, request(v1.RequestPhasePending, auto, manual(sreRuleName, "sre-leads")), v1.ApprovalDecisionApproved, nil, []string{sreRuleName}, bobEmail},
		{"self-approval blocked", alice, request(v1.RequestPhasePending, manual(devsGroup, devsGroup)), v1.ApprovalDecisionApproved, approval.ErrNotEligible, nil, ""},
		{"self-approval allowed by rule", alice, request(v1.RequestPhasePending, manual(devsGroup, devsGroup), selfOK), v1.ApprovalDecisionApproved, nil, []string{"self"}, aliceEmail},
		{"approver of two rules gets two decisions, deduped per cluster", bob, request(v1.RequestPhasePending, manual("r1", "sre-leads"), manual("r1", "sre-leads"), manual("r2", "sre-leads")), v1.ApprovalDecisionApproved, nil, []string{"r1", "r2"}, bobEmail},
		{"approver who already approved may not approve again", bob, approvedBy(bobEmail, sreRuleName, request(v1.RequestPhasePending, twoNeeded)), v1.ApprovalDecisionApproved, approval.ErrAlreadyApproved, nil, ""},
		{"earlier approval recorded under username counts", bob, approvedBy(bobUsername, sreRuleName, request(v1.RequestPhasePending, twoNeeded)), v1.ApprovalDecisionApproved, approval.ErrAlreadyApproved, nil, ""},
		{"approver of two rules who approved one approves only the other", bob, approvedBy(bobEmail, "r1", request(v1.RequestPhasePending, manual("r1", "sre-leads"), manual("r2", "sre-leads"))), v1.ApprovalDecisionApproved, nil, []string{"r2"}, bobEmail},
		{"another approver's approval does not block", bob, approvedBy("carol@company.com", sreRuleName, request(v1.RequestPhasePending, twoNeeded)), v1.ApprovalDecisionApproved, nil, []string{sreRuleName}, bobEmail},
		{"approver who approved may still reject", bob, approvedBy(bobEmail, sreRuleName, request(v1.RequestPhasePending, twoNeeded)), v1.ApprovalDecisionRejected, nil, []string{sreRuleName}, bobEmail},
		{"user falls back to username", erin, request(v1.RequestPhasePending, byUsername), v1.ApprovalDecisionApproved, nil, []string{"by-user"}, erinUsername},
		{"earlier approval with empty user does not block", erin, approvedBy("", "by-user", request(v1.RequestPhasePending, byUsername)), v1.ApprovalDecisionApproved, nil, []string{"by-user"}, erinUsername},
		{"reject by approver uses first eligible rule", bob, request(v1.RequestPhasePending, manual("other", "ops"), manual("r1", "sre-leads"), manual("r2", "sre-leads")), v1.ApprovalDecisionRejected, nil, []string{"r1"}, bobEmail},
		{"reject by non-eligible", mallory, request(v1.RequestPhasePending, manual(sreRuleName, "sre-leads")), v1.ApprovalDecisionRejected, approval.ErrNotEligible, nil, ""},
		{"requester may not reject", alice, request(v1.RequestPhasePending, manual(sreRuleName, "sre-leads")), v1.ApprovalDecisionRejected, approval.ErrNotEligible, nil, ""},
		{"requester cancels", alice, request(v1.RequestPhasePending, manual(sreRuleName, "sre-leads")), v1.ApprovalDecisionCancelled, nil, []string{""}, aliceEmail},
		{"requester cancels unresolved request", alice, request(v1.RequestPhasePending), v1.ApprovalDecisionCancelled, nil, []string{""}, aliceEmail},
		{"approver may not cancel", bob, request(v1.RequestPhasePending, manual(sreRuleName, "sre-leads")), v1.ApprovalDecisionCancelled, approval.ErrNotEligible, nil, ""},
		{"unresolved", bob, request(v1.RequestPhasePending), v1.ApprovalDecisionApproved, approval.ErrUnresolved, nil, ""},
		{"resolved without requirements", bob, request(v1.RequestPhasePending, []v1.ClusterApprovalRequirement{}...), v1.ApprovalDecisionRejected, approval.ErrUnresolved, nil, ""},
		{"approve non-pending", bob, request(v1.RequestPhaseApproved, manual(sreRuleName, "sre-leads")), v1.ApprovalDecisionApproved, approval.ErrNotPending, nil, ""},
		{"cancel non-pending", alice, request(v1.RequestPhaseRejected, manual(sreRuleName, "sre-leads")), v1.ApprovalDecisionCancelled, approval.ErrNotPending, nil, ""},
	}
	at := metav1.NewTime(time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC))
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := approval.Decide(tc.p, tc.req, tc.kind, "because", at)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if len(got) != len(tc.wantRule) {
				t.Fatalf("got %d decisions %+v, want rules %v", len(got), got, tc.wantRule)
			}
			for i, d := range got {
				if d.Rule != tc.wantRule[i] || d.User != tc.wantUser || d.Decision != tc.kind || d.Comment != "because" || !d.At.Equal(&at) {
					t.Fatalf("decision %d = %+v, want rule %q user %q kind %s", i, d, tc.wantRule[i], tc.wantUser, tc.kind)
				}
			}
		})
	}
}

func TestOutcome(t *testing.T) {
	two := manual(sreRuleName, "sre-leads")
	two.RequiredApprovals = 2
	auto := manual("auto")
	auto.Mode = v1.ApprovalModeAuto
	unnamed := manual("", "sre-leads")
	d := func(user string, kind v1.ApprovalDecisionType, rule string) v1.ApprovalDecision {
		return v1.ApprovalDecision{User: user, Decision: kind, Rule: rule}
	}
	approved := v1.ApprovalDecisionApproved

	cases := []struct {
		name      string
		reqs      []v1.ClusterApprovalRequirement
		decisions []v1.ApprovalDecision
		want      v1.RequestPhase
	}{
		{"unresolved stays pending", nil, []v1.ApprovalDecision{d(bobUsername, approved, sreRuleName)}, v1.RequestPhasePending},
		{"1 of 2 stays pending", []v1.ClusterApprovalRequirement{two}, []v1.ApprovalDecision{d(bobUsername, approved, sreRuleName)}, v1.RequestPhasePending},
		{"2 of 2 distinct approves", []v1.ClusterApprovalRequirement{two}, []v1.ApprovalDecision{d(bobUsername, approved, sreRuleName), d("carol", approved, sreRuleName)}, v1.RequestPhaseApproved},
		{"duplicate user counted once", []v1.ClusterApprovalRequirement{two}, []v1.ApprovalDecision{d(bobUsername, approved, sreRuleName), d(bobUsername, approved, sreRuleName)}, v1.RequestPhasePending},
		{"approval for other rule does not count", []v1.ClusterApprovalRequirement{manual(sreRuleName)}, []v1.ApprovalDecision{d(bobUsername, approved, "other")}, v1.RequestPhasePending},
		{"empty rule counts nothing", []v1.ClusterApprovalRequirement{unnamed}, []v1.ApprovalDecision{d(bobUsername, approved, "")}, v1.RequestPhasePending},
		{"auto requirements need no approvals", []v1.ClusterApprovalRequirement{auto, manual(sreRuleName)}, []v1.ApprovalDecision{d(bobUsername, approved, sreRuleName)}, v1.RequestPhaseApproved},
		{"every manual requirement must be met", []v1.ClusterApprovalRequirement{manual(sreRuleName), manual("sec")}, []v1.ApprovalDecision{d(bobUsername, approved, sreRuleName)}, v1.RequestPhasePending},
		{"rejection rejects", []v1.ClusterApprovalRequirement{manual(sreRuleName)}, []v1.ApprovalDecision{d(bobUsername, approved, sreRuleName), d("carol", v1.ApprovalDecisionRejected, sreRuleName)}, v1.RequestPhaseRejected},
		{"cancellation beats rejection", []v1.ClusterApprovalRequirement{manual(sreRuleName)}, []v1.ApprovalDecision{d(bobUsername, v1.ApprovalDecisionRejected, sreRuleName), d(aliceUsername, v1.ApprovalDecisionCancelled, "")}, v1.RequestPhaseCancelled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := request(v1.RequestPhasePending, tc.reqs...)
			req.Status.Approvals = tc.decisions
			if got := approval.Outcome(req); got != tc.want {
				t.Fatalf("Outcome = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestProgress(t *testing.T) {
	auto := manual("auto")
	auto.Mode = v1.ApprovalModeAuto
	unnamed := manual("", "sre-leads")
	d := func(user string, kind v1.ApprovalDecisionType, rule string) v1.ApprovalDecision {
		return v1.ApprovalDecision{User: user, Decision: kind, Rule: rule}
	}
	approved := v1.ApprovalDecisionApproved

	cases := []struct {
		name      string
		reqs      []v1.ClusterApprovalRequirement
		decisions []v1.ApprovalDecision
		want      [][]string
	}{
		{"unresolved has no requirements", nil, []v1.ApprovalDecision{d(bobUsername, approved, sreRuleName)}, nil},
		{"distinct users in first-seen order", []v1.ClusterApprovalRequirement{manual(sreRuleName)},
			[]v1.ApprovalDecision{d("carol", approved, sreRuleName), d(bobUsername, approved, sreRuleName), d("carol", approved, sreRuleName)}, [][]string{{"carol", bobUsername}}},
		{"rejected and cancelled ignored", []v1.ClusterApprovalRequirement{manual(sreRuleName)},
			[]v1.ApprovalDecision{d(bobUsername, v1.ApprovalDecisionRejected, sreRuleName), d(aliceUsername, v1.ApprovalDecisionCancelled, sreRuleName)}, [][]string{nil}},
		{"other rules counted separately", []v1.ClusterApprovalRequirement{manual(sreRuleName), manual("sec")},
			[]v1.ApprovalDecision{d(bobUsername, approved, sreRuleName), d("carol", approved, "other")}, [][]string{{bobUsername}, nil}},
		{"empty rule name counts nothing", []v1.ClusterApprovalRequirement{unnamed},
			[]v1.ApprovalDecision{d(bobUsername, approved, "")}, [][]string{nil}},
		{"auto requirements listed with approvals", []v1.ClusterApprovalRequirement{auto, manual(sreRuleName)},
			[]v1.ApprovalDecision{d(bobUsername, approved, "auto"), d("carol", approved, sreRuleName)}, [][]string{{bobUsername}, {"carol"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := request(v1.RequestPhasePending, tc.reqs...)
			req.Status.Approvals = tc.decisions
			got := approval.Progress(req)
			if len(got) != len(tc.want) {
				t.Fatalf("Progress returned %d entries, want %d", len(got), len(tc.want))
			}
			for i, p := range got {
				if p.Requirement.RuleName != tc.reqs[i].RuleName || p.Requirement.Mode != tc.reqs[i].Mode {
					t.Fatalf("entry %d requirement = %+v, want %+v", i, p.Requirement, tc.reqs[i])
				}
				if p.ApprovedBy == nil || !slices.Equal(p.ApprovedBy, tc.want[i]) {
					t.Fatalf("entry %d ApprovedBy = %v, want %v", i, p.ApprovedBy, tc.want[i])
				}
			}
		})
	}
}
