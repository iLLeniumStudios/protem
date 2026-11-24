package audit

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
)

const (
	teamA      = "team-a"
	aliceEmail = "alice@company.com"
	bobEmail   = "bob@company.com"
	clusterA   = "cluster-a"
)

var t0 = time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)

func at(minutes int) metav1.Time { return metav1.NewTime(t0.Add(time.Duration(minutes) * time.Minute)) }

func request(phase protemv1alpha1.RequestPhase) *protemv1alpha1.AccessRequest {
	return &protemv1alpha1.AccessRequest{
		ObjectMeta: metav1.ObjectMeta{Namespace: teamA, Name: "req-1", CreationTimestamp: at(0)},
		Spec: protemv1alpha1.AccessRequestSpec{
			ProfileRef:    "db-admin",
			Requester:     protemv1alpha1.RequesterInfo{Username: "alice", Email: aliceEmail},
			Justification: protemv1alpha1.RequestJustification{Reason: "Fix INC-7", Ticket: "INC-7"},
		},
		Status: protemv1alpha1.AccessRequestStatus{Phase: phase, Resolved: &protemv1alpha1.ResolvedScope{
			Clusters: []string{clusterA},
			Duration: metav1.Duration{Duration: time.Hour},
		}},
	}
}

type row struct {
	event   string
	at      metav1.Time
	actor   string
	comment string
}

func decided(status metav1.ConditionStatus, when metav1.Time) metav1.Condition {
	return metav1.Condition{Type: "Decided", Status: status, LastTransitionTime: when, Reason: "x", Message: "x"}
}

func TestEvents(t *testing.T) {
	now := t0.Add(24 * time.Hour)
	approvedBy := func(user string, when metav1.Time, comment, rule string) protemv1alpha1.ApprovalDecision {
		return protemv1alpha1.ApprovalDecision{User: user, Decision: protemv1alpha1.ApprovalDecisionApproved, At: when, Comment: comment, Rule: rule}
	}
	withDecisions := func(phase protemv1alpha1.RequestPhase, decisions []protemv1alpha1.ApprovalDecision, conditions ...metav1.Condition) *protemv1alpha1.AccessRequest {
		req := request(phase)
		req.Status.Approvals, req.Status.Conditions = decisions, conditions
		return req
	}
	autoApproved := func() *protemv1alpha1.AccessRequest {
		return withDecisions(protemv1alpha1.RequestPhaseApproved,
			[]protemv1alpha1.ApprovalDecision{approvedBy("system:auto-approval", at(1), "", "auto")}, decided(metav1.ConditionTrue, at(1)))
	}
	grant := func(expires metav1.Time, revocation *protemv1alpha1.RevocationDetails) *protemv1alpha1.AccessGrant {
		return &protemv1alpha1.AccessGrant{
			ObjectMeta: metav1.ObjectMeta{Namespace: teamA, Name: "grant-1", CreationTimestamp: at(2)},
			Spec:       protemv1alpha1.AccessGrantSpec{RequestRef: "req-1", Clusters: []string{clusterA}, ExpiresAt: expires, Revocation: revocation},
		}
	}
	submitted := row{"REQUEST_SUBMITTED", at(0), aliceEmail, ""}
	autoRow := row{"REQUEST_APPROVED", at(1), "", "No approval needed"}

	for _, tc := range []struct {
		name  string
		req   *protemv1alpha1.AccessRequest
		grant *protemv1alpha1.AccessGrant
		want  []row
	}{
		{"auto approved", autoApproved(), nil, []row{submitted, autoRow}},
		{"two approvals then approved", withDecisions(protemv1alpha1.RequestPhaseApproved, []protemv1alpha1.ApprovalDecision{
			approvedBy(bobEmail, at(3), "lgtm", "r1"), approvedBy(bobEmail, at(3), "lgtm", "r2"), approvedBy("carol@company.com", at(5), "", "r1"),
		}, decided(metav1.ConditionTrue, at(5))), nil, []row{
			submitted, {"APPROVAL_GIVEN", at(3), bobEmail, "lgtm"}, {"APPROVAL_GIVEN", at(5), "carol@company.com", ""}, {"REQUEST_APPROVED", at(5), "", ""},
		}},
		{"rejected by an approver", withDecisions(protemv1alpha1.RequestPhaseRejected, []protemv1alpha1.ApprovalDecision{
			{User: bobEmail, Decision: protemv1alpha1.ApprovalDecisionRejected, At: at(4), Comment: "too broad", Rule: "r1"},
		}, decided(metav1.ConditionFalse, at(1))), nil, []row{submitted, {"REQUEST_REJECTED", at(4), bobEmail, "too broad"}}},
		{"rejected by validation", withDecisions(protemv1alpha1.RequestPhaseRejected, nil,
			metav1.Condition{Type: "Validated", Status: metav1.ConditionFalse, LastTransitionTime: at(1), Reason: "InvalidTicket", Message: "Ticket does not match"},
		), nil, []row{submitted, {"REQUEST_REJECTED", at(1), "", "Ticket does not match"}}},
		{"cancelled", withDecisions(protemv1alpha1.RequestPhaseCancelled, []protemv1alpha1.ApprovalDecision{
			{User: aliceEmail, Decision: protemv1alpha1.ApprovalDecisionCancelled, At: at(6), Comment: "not needed"},
		}, decided(metav1.ConditionFalse, at(1))), nil, []row{submitted, {"REQUEST_CANCELLED", at(6), aliceEmail, "not needed"}}},
		{"timed out", withDecisions(protemv1alpha1.RequestPhaseTimedOut, nil, decided(metav1.ConditionFalse, at(60))), nil,
			[]row{submitted, {"REQUEST_TIMED_OUT", at(60), "", ""}}},
		{"granted then revoked", autoApproved(), grant(at(62), &protemv1alpha1.RevocationDetails{By: bobEmail, At: at(30), Reason: "incident over"}), []row{
			submitted, autoRow, {"ACCESS_GRANTED", at(2), "", ""}, {"ACCESS_REVOKED", at(30), bobEmail, "incident over"},
		}},
		{"granted then expired", autoApproved(), grant(at(62), nil), []row{submitted, autoRow, {"ACCESS_GRANTED", at(2), "", ""}, {"ACCESS_EXPIRED", at(62), "", ""}}},
		{"granted, still running", autoApproved(), grant(metav1.NewTime(now.Add(time.Minute)), nil), []row{submitted, autoRow, {"ACCESS_GRANTED", at(2), "", ""}}},
	} {
		events := Events(tc.req, tc.grant, now)
		got := make([]row, 0, len(events))
		for i, e := range events {
			got = append(got, row{e.EventType, metav1.NewTime(e.Timestamp), e.Actor, e.Comment})
			if want := fmt.Sprintf("team-a/req-1#%d", i+1); e.ID != want {
				t.Errorf("%s event %d: expected id %s, got %s", tc.name, i, want, e.ID)
			}
			if e.Namespace != teamA || e.Request != "req-1" || e.Requester != aliceEmail || e.Profile != "db-admin" || e.Reason != "Fix INC-7" || e.Ticket != "INC-7" {
				t.Errorf("%s event %d: expected the request's facts on every event, got %+v", tc.name, i, e)
			}
			if !slices.Equal(e.Clusters, []string{clusterA}) {
				t.Errorf("%s event %d: expected every event to carry the request's clusters, got %+v", tc.name, i, e)
			}
			if strings.HasPrefix(e.EventType, "ACCESS_") && e.Grant != "grant-1" {
				t.Errorf("%s event %d: expected grant events to name the grant, got %+v", tc.name, i, e)
			}
			if e.EventType == "ACCESS_GRANTED" && e.Duration != "1h0m0s" {
				t.Errorf("%s: expected the granted event to carry the duration, got %q", tc.name, e.Duration)
			}
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s:\n expected %v\n got      %v", tc.name, tc.want, got)
		}
	}
}

// The API server stamps creation times and the hub stamps decisions. When their clocks differ, a grant can
// carry a creation time earlier than the approval that caused it.
func TestEventsOfOneRequestNeverGoBackInTime(t *testing.T) {
	req := request(protemv1alpha1.RequestPhaseApproved)
	req.Status.Approvals = []protemv1alpha1.ApprovalDecision{{User: bobEmail, Decision: protemv1alpha1.ApprovalDecisionApproved, At: at(40), Rule: "r1"}}
	req.Status.Conditions = []metav1.Condition{decided(metav1.ConditionTrue, at(40))}
	grant := &protemv1alpha1.AccessGrant{
		ObjectMeta: metav1.ObjectMeta{Namespace: teamA, Name: "grant-1", CreationTimestamp: at(2)},
		Spec:       protemv1alpha1.AccessGrantSpec{RequestRef: "req-1", ExpiresAt: at(100)},
	}
	events := Events(req, grant, t0.Add(time.Hour))
	for i := 1; i < len(events); i++ {
		if events[i].Timestamp.Before(events[i-1].Timestamp) {
			t.Errorf("%s at %v comes before %s at %v", events[i].EventType, events[i].Timestamp, events[i-1].EventType, events[i-1].Timestamp)
		}
	}
}

func TestEventsAreHistoryNotSnapshot(t *testing.T) {
	now := t0.Add(24 * time.Hour)
	req := request(protemv1alpha1.RequestPhasePending)
	req.Status.Approvals = []protemv1alpha1.ApprovalDecision{{User: bobEmail, Decision: protemv1alpha1.ApprovalDecisionApproved, At: at(3), Rule: "r1"}}
	req.Status.Conditions = []metav1.Condition{decided(metav1.ConditionFalse, at(1))}
	pending := Events(req, nil, now)

	req = req.DeepCopy()
	req.Status.Phase = protemv1alpha1.RequestPhaseApproved
	req.Status.Approvals = append(req.Status.Approvals, protemv1alpha1.ApprovalDecision{User: "carol@company.com", Decision: protemv1alpha1.ApprovalDecisionApproved, At: at(5), Rule: "r1"})
	req.Status.Conditions = []metav1.Condition{decided(metav1.ConditionTrue, at(5))}
	req.Status.GrantRef = "grant-1"
	g := &protemv1alpha1.AccessGrant{
		ObjectMeta: metav1.ObjectMeta{Namespace: teamA, Name: "grant-1", CreationTimestamp: at(6)},
		Spec:       protemv1alpha1.AccessGrantSpec{RequestRef: "req-1", ExpiresAt: metav1.NewTime(now.Add(time.Hour))},
	}
	granted := Events(req, g, now)

	g = g.DeepCopy()
	g.Spec.Revocation = &protemv1alpha1.RevocationDetails{By: bobEmail, At: at(20)}
	revoked := Events(req, g, now)

	if len(pending) != 2 || !reflect.DeepEqual(granted[:len(pending)], pending) {
		t.Errorf("events from the pending state changed once it was approved:\n before %+v\n after  %+v", pending, granted)
	}
	if len(granted) != 5 || !reflect.DeepEqual(revoked[:len(granted)], granted) {
		t.Errorf("events from the granted state changed once it was revoked:\n before %+v\n after  %+v", granted, revoked)
	}
}

// A request created with kubectl has no email (the admission policy allows only an empty one).
func TestSubmittedNamesTheRequesterWhenThereIsNoEmail(t *testing.T) {
	req := &protemv1alpha1.AccessRequest{
		ObjectMeta: metav1.ObjectMeta{Namespace: teamA, Name: "req-1"},
		Spec:       protemv1alpha1.AccessRequestSpec{ProfileRef: "db-admin", Requester: protemv1alpha1.RequesterInfo{Username: "alice"}},
	}
	if got := Events(req, nil, time.Now())[0]; got.Actor != "alice" || got.Requester != "alice" {
		t.Errorf("actor = %q, requester = %q, want alice for both", got.Actor, got.Requester)
	}
}
