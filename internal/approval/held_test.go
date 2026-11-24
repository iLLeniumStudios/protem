package approval

import (
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
)

func TestHeld(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	alice := v1.RequesterInfo{Username: "alice", Email: "alice@company.com"}
	carol := v1.RequesterInfo{Username: "carol", Email: "carol@company.com"}

	request := func(name string, who v1.RequesterInfo, profile string, phase v1.RequestPhase, age time.Duration, resolved ...string) v1.AccessRequest {
		r := v1.AccessRequest{
			ObjectMeta: metav1.ObjectMeta{Namespace: "team", Name: name, CreationTimestamp: metav1.NewTime(now.Add(-age))},
			Spec:       v1.AccessRequestSpec{ProfileRef: profile, Requester: who},
			Status:     v1.AccessRequestStatus{Phase: phase},
		}
		if resolved != nil {
			r.Status.Resolved = &v1.ResolvedScope{Clusters: resolved}
		}
		return r
	}
	grant := func(who v1.RequesterInfo, profile string, expiresIn time.Duration, clusters ...string) v1.AccessGrant {
		return v1.AccessGrant{
			ObjectMeta: metav1.ObjectMeta{Namespace: "team", Name: "grant-old"},
			Spec:       v1.AccessGrantSpec{RequestRef: "old", ProfileRef: profile, Requester: who, Clusters: clusters, ExpiresAt: metav1.NewTime(now.Add(expiresIn))},
			Status:     v1.AccessGrantStatus{Phase: v1.GrantPhaseActive},
		}
	}
	revoked := grant(alice, "view", time.Hour, "prod")
	revoked.Spec.Revocation = &v1.RevocationDetails{At: metav1.NewTime(now)}
	withGrant := request("old", alice, "view", v1.RequestPhaseApproved, time.Hour, "prod")
	withGrant.Status.GrantRef = "grant-old"

	judged := request("new", alice, "view", "", time.Minute)

	for _, tc := range []struct {
		name     string
		clusters []string
		requests []v1.AccessRequest
		grants   []v1.AccessGrant
		want     string // substring of the explanation; empty means not held
	}{
		{"nothing else", []string{"prod"}, nil, nil, ""},
		{"itself", []string{"prod"}, []v1.AccessRequest{judged}, nil, ""},
		{"pending request on the same cluster", []string{"prod"}, []v1.AccessRequest{request("old", alice, "view", v1.RequestPhasePending, time.Hour, "prod")}, nil, "waiting for a decision"},
		{"pending request on another cluster", []string{"prod"}, []v1.AccessRequest{request("old", alice, "view", v1.RequestPhasePending, time.Hour, "dev")}, nil, ""},
		{"pending request for another profile", []string{"prod"}, []v1.AccessRequest{request("old", alice, "edit", v1.RequestPhasePending, time.Hour, "prod")}, nil, ""},
		{"someone else's pending request", []string{"prod"}, []v1.AccessRequest{request("old", carol, "view", v1.RequestPhasePending, time.Hour, "prod")}, nil, ""},
		{"decided requests do not count", []string{"prod"}, []v1.AccessRequest{
			request("a", alice, "view", v1.RequestPhaseRejected, time.Hour, "prod"),
			request("b", alice, "view", v1.RequestPhaseCancelled, time.Hour, "prod"),
			request("c", alice, "view", v1.RequestPhaseTimedOut, time.Hour, "prod"),
		}, nil, ""},
		{"older request not yet validated", []string{"prod"}, []v1.AccessRequest{request("old", alice, "view", "", time.Hour)}, nil, "waiting for a decision"},
		{"newer request not yet validated", []string{"prod"}, []v1.AccessRequest{request("newer", alice, "view", "", time.Second)}, nil, ""},
		{"same instant: the earlier name wins", []string{"prod"}, []v1.AccessRequest{request("a-first", alice, "view", "", time.Minute)}, nil, "waiting for a decision"},
		{"approved, grant not issued yet", []string{"prod"}, []v1.AccessRequest{request("old", alice, "view", v1.RequestPhaseApproved, time.Hour, "prod")}, nil, "waiting for a decision"},
		{"approved with a grant is judged by the grant", []string{"prod"}, []v1.AccessRequest{withGrant}, nil, ""},
		{"live grant on the same cluster", []string{"prod"}, nil, []v1.AccessGrant{grant(alice, "view", time.Hour, "prod", "dev")}, "already granted on prod until 2026-11-24 13:00 UTC"},
		{"live grant on another cluster", []string{"prod"}, nil, []v1.AccessGrant{grant(alice, "view", time.Hour, "dev")}, ""},
		{"expired grant", []string{"prod"}, nil, []v1.AccessGrant{grant(alice, "view", -time.Minute, "prod")}, ""},
		{"revoked grant", []string{"prod"}, nil, []v1.AccessGrant{revoked}, ""},
		{"someone else's grant", []string{"prod"}, nil, []v1.AccessGrant{grant(carol, "view", time.Hour, "prod")}, ""},
		{"no clusters named means all of them", nil, nil, []v1.AccessGrant{grant(alice, "view", time.Hour, "dev")}, "already granted on dev"},
	} {
		got := Held(&judged, tc.clusters, tc.requests, tc.grants, now)
		if (tc.want == "") != (got == "") || !strings.Contains(got, tc.want) {
			t.Errorf("%s: got %q, want it to contain %q", tc.name, got, tc.want)
		}
	}

	unsaved := v1.AccessRequest{Spec: judged.Spec}
	if got := Held(&unsaved, nil, []v1.AccessRequest{request("any", alice, "view", "", time.Second)}, nil, now); got == "" {
		t.Error("a request not created yet must count every earlier unvalidated request")
	}
}
