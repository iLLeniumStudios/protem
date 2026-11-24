package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
)

var t0 = time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)

func at(minutes int) metav1.Time { return metav1.NewTime(t0.Add(time.Duration(minutes) * time.Minute)) }

func TestPaginate(t *testing.T) {
	items := make([]int, 30)
	for i := range items {
		items[i] = i
	}
	for _, tc := range []struct {
		query     string
		wantFirst int
		wantLen   int
		wantErr   bool
	}{
		{"", 0, 25, false},
		{"limit=100", 0, 30, false},
		{"limit=10&offset=25", 25, 5, false},
		{"offset=29", 29, 1, false},
		{"offset=30", 0, 0, false},
		{"offset=500", 0, 0, false},
		{"limit=1", 0, 1, false},
		{"limit=0", 0, 0, true},
		{"limit=101", 0, 0, true},
		{"limit=-1", 0, 0, true},
		{"limit=ten", 0, 0, true},
		{"limit=1.5", 0, 0, true},
		{"limit=", 0, 0, true},
		{"offset=-1", 0, 0, true},
		{"offset=x", 0, 0, true},
	} {
		q, _ := url.ParseQuery(tc.query)
		page, total, err := paginate(q, items)
		if tc.wantErr {
			if err == nil {
				t.Errorf("%q: expected an error, got %d items", tc.query, len(page))
			}
			continue
		}
		if err != nil || page == nil || total != 30 || len(page) != tc.wantLen || (tc.wantLen > 0 && page[0] != tc.wantFirst) {
			t.Errorf("%q: expected %d items from %d of 30, got %v total %d err %v", tc.query, tc.wantLen, tc.wantFirst, page, total, err)
		}
	}
}

func decodePage[T any](t *testing.T, s *Server, path, as string) ([]T, int) {
	t.Helper()
	w := call(s, http.MethodGet, path, as, "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s as %s = %d: %s", path, as, w.Code, w.Body.String())
	}
	var page struct {
		Items *[]T `json:"items"`
		Total int  `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || page.Items == nil {
		t.Fatalf("GET %s: expected a page with items, got %s (%v)", path, w.Body.String(), err)
	}
	return *page.Items, page.Total
}

func requestNames(t *testing.T, s *Server, path, as string) ([]string, int) {
	t.Helper()
	items, total := decodePage[RequestView](t, s, path, as)
	names := make([]string, 0, len(items))
	for _, v := range items {
		names = append(names, v.Name)
	}
	slices.Sort(names)
	return names, total
}

func historyRequest(ns, name, requester string, phase protemv1alpha1.RequestPhase, approverGroup string) *protemv1alpha1.AccessRequest {
	req := &protemv1alpha1.AccessRequest{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name, CreationTimestamp: at(0)},
		Spec: protemv1alpha1.AccessRequestSpec{
			ProfileRef:    "db-admin",
			Requester:     protemv1alpha1.RequesterInfo{Username: strings.Split(requester, "@")[0], Email: requester},
			Justification: protemv1alpha1.RequestJustification{Reason: "Fix INC-7", Ticket: "INC-7"},
		},
		Status: protemv1alpha1.AccessRequestStatus{Phase: phase},
	}
	if approverGroup != "" {
		req.Status.Resolved = &protemv1alpha1.ResolvedScope{
			Clusters:     []string{clusterA},
			Duration:     metav1.Duration{Duration: time.Hour},
			Requirements: []protemv1alpha1.ClusterApprovalRequirement{{ClusterName: clusterA, RuleName: "r1", Mode: protemv1alpha1.ApprovalModeManual, RequiredApprovals: 1, ApproverGroups: []string{approverGroup}}},
		}
	}
	return req
}

func TestRequestsAreFilteredAndPaged(t *testing.T) {
	pending, approved, rejected := protemv1alpha1.RequestPhasePending, protemv1alpha1.RequestPhaseApproved, protemv1alpha1.RequestPhaseRejected
	s, _ := newMgmtServer(t, denyAll, ServerOptions{},
		historyRequest(teamA, "open-mine", aliceEmail, pending, ""),
		historyRequest(teamA, "decided-mine", aliceEmail, approved, ""),
		historyRequest(teamA, "open-approver", bobEmail, pending, devsGroup),
		historyRequest(teamA, "decided-approver", bobEmail, rejected, devsGroup),
		historyRequest(teamA, "mine-and-approver", aliceEmail, pending, devsGroup),
		historyRequest(teamA, "unrelated", bobEmail, pending, sreLeadsGroup),
		historyRequest(teamB, "other-ns", aliceEmail, pending, ""),
	)
	for _, tc := range []struct {
		path string
		want []string
	}{
		{"/api/requests", []string{"decided-approver", "decided-mine", "mine-and-approver", "open-approver", "open-mine", "other-ns"}},
		{"/api/requests?state=open", []string{"mine-and-approver", "open-approver", "open-mine", "other-ns"}},
		{"/api/requests?state=decided", []string{"decided-approver", "decided-mine"}},
		{"/api/requests?mine=true", []string{"decided-mine", "mine-and-approver", "open-mine", "other-ns"}},
		{"/api/requests?mine=false", []string{"decided-approver", "decided-mine", "mine-and-approver", "open-approver", "open-mine", "other-ns"}},
		{"/api/requests?approver=true", []string{"decided-approver", "mine-and-approver", "open-approver"}},
		{"/api/requests?state=open&approver=true", []string{"mine-and-approver", "open-approver"}},
		{"/api/requests?mine=true&approver=true", []string{"mine-and-approver"}},
		{"/api/requests?state=decided&mine=true", []string{"decided-mine"}},
		{"/api/namespaces/team-a/requests?mine=true", []string{"decided-mine", "mine-and-approver", "open-mine"}},
	} {
		got, total := requestNames(t, s, tc.path, aliceEmail)
		if !slices.Equal(got, tc.want) || total != len(tc.want) {
			t.Errorf("%s: expected %v, got %v (total %d)", tc.path, tc.want, got, total)
		}
	}
	for _, path := range []string{"/api/requests?state=closed", "/api/requests?mine=yes", "/api/requests?approver=1", "/api/requests?limit=0", "/api/namespaces/team-a/requests?offset=-2"} {
		if w := call(s, http.MethodGet, path, aliceEmail, ""); w.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d: %s", path, w.Code, w.Body.String())
		}
	}

	first, total := decodePage[RequestView](t, s, "/api/requests?limit=4", aliceEmail)
	second, total2 := decodePage[RequestView](t, s, "/api/requests?limit=4&offset=4", aliceEmail)
	seen := make([]string, 0, len(first)+len(second))
	for _, v := range append(first, second...) {
		seen = append(seen, v.Namespace+"/"+v.Name)
	}
	unique := slices.Compact(slices.Sorted(slices.Values(seen)))
	if total != 6 || total2 != 6 || len(first) != 4 || len(second) != 2 || len(unique) != 6 {
		t.Errorf("expected two pages covering six requests once each, got %v (totals %d, %d)", seen, total, total2)
	}
}

func TestRequestByName(t *testing.T) {
	auditor := "auditor@company.com"
	s, _ := newMgmtServer(t, allowOnly(sarRule{auditor, "get", "accessrequests", teamA, ""}), ServerOptions{},
		historyRequest(teamA, "own", aliceEmail, protemv1alpha1.RequestPhasePending, ""),
		historyRequest(teamA, "to-review", bobEmail, protemv1alpha1.RequestPhasePending, devsGroup),
	)
	for _, tc := range []struct {
		path, as string
		want     int
	}{
		{"/api/namespaces/team-a/requests/own", aliceEmail, http.StatusOK},
		{"/api/namespaces/team-a/requests/to-review", aliceEmail, http.StatusOK},
		{"/api/namespaces/team-a/requests/own", auditor, http.StatusOK},
		{"/api/namespaces/team-a/requests/own", eveEmail, http.StatusForbidden},
		{"/api/namespaces/team-a/requests/nope", aliceEmail, http.StatusNotFound},
	} {
		w := call(s, http.MethodGet, tc.path, tc.as, "")
		if w.Code != tc.want {
			t.Errorf("GET %s as %s: expected %d, got %d: %s", tc.path, tc.as, tc.want, w.Code, w.Body.String())
			continue
		}
		var v RequestView
		if tc.want == http.StatusOK && (json.Unmarshal(w.Body.Bytes(), &v) != nil || "/api/namespaces/team-a/requests/"+v.Name != tc.path) {
			t.Errorf("GET %s: expected that request, got %s", tc.path, w.Body.String())
		}
	}
	if w := call(s, http.MethodPost, "/api/namespaces/team-a/requests/to-review/approve", aliceEmail, ""); w.Code != http.StatusOK {
		t.Errorf("approve route: expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGrantVisibility(t *testing.T) {
	grant := func(name, requestRef string, phase protemv1alpha1.GrantPhase) *protemv1alpha1.AccessGrant {
		return &protemv1alpha1.AccessGrant{
			ObjectMeta: metav1.ObjectMeta{Namespace: teamA, Name: name, CreationTimestamp: at(0)},
			Spec: protemv1alpha1.AccessGrantSpec{
				RequestRef: requestRef,
				Requester:  protemv1alpha1.RequesterInfo{Username: "bob", Email: bobEmail},
				Role:       protemv1alpha1.TargetRoleRef{Kind: "ClusterRole", Name: "edit"},
				Clusters:   []string{clusterA},
				ExpiresAt:  metav1.NewTime(time.Now().Add(time.Hour)),
			},
			Status: protemv1alpha1.AccessGrantStatus{Phase: phase},
		}
	}
	approvedByAlice := historyRequest(teamA, "req-1", bobEmail, protemv1alpha1.RequestPhaseApproved, sreLeadsGroup)
	approvedByAlice.Status.Resolved.Requirements[0].ApproverUsers = []string{aliceEmail}
	s, _ := newMgmtServer(t, denyAll, ServerOptions{},
		approvedByAlice,
		grant("g-1", "req-1", protemv1alpha1.GrantPhaseActive),
		grant("g-orphan", "gone", protemv1alpha1.GrantPhaseActive),
		grant("g-old", "gone-too", protemv1alpha1.GrantPhaseExpired),
		&rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "edit"}},
	)
	grantNames := func(path, as string) []string {
		t.Helper()
		items, total := decodePage[GrantView](t, s, path, as)
		names := make([]string, 0, len(items))
		for _, g := range items {
			names = append(names, g.Name)
		}
		if total != len(names) {
			t.Errorf("%s as %s: total %d for %d items", path, as, total, len(names))
		}
		return names
	}
	for _, tc := range []struct {
		path, as string
		want     []string
	}{
		{"/api/grants", bobEmail, []string{"g-orphan", "g-1", "g-old"}},
		{"/api/grants?state=active", bobEmail, []string{"g-orphan", "g-1"}},
		{"/api/grants?state=ended", bobEmail, []string{"g-old"}},
		{"/api/grants?mine=true", bobEmail, []string{"g-orphan", "g-1", "g-old"}},
		{"/api/grants", aliceEmail, []string{"g-1"}},
		{"/api/grants?mine=true", aliceEmail, []string{}},
		{"/api/grants", eveEmail, []string{}},
	} {
		if got := grantNames(tc.path, tc.as); !slices.Equal(got, tc.want) {
			t.Errorf("%s as %s: expected %v, got %v", tc.path, tc.as, tc.want, got)
		}
	}
	for _, path := range []string{"/api/grants?state=pending", "/api/grants?mine=maybe", "/api/grants?limit=101"} {
		if w := call(s, http.MethodGet, path, bobEmail, ""); w.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d", path, w.Code)
		}
	}

	for _, tc := range []struct {
		path, as string
		want     int
	}{
		{"/api/namespaces/team-a/grants/g-1", bobEmail, http.StatusOK},
		{"/api/namespaces/team-a/grants/g-1", aliceEmail, http.StatusOK},
		{"/api/namespaces/team-a/grants/g-1/permissions", aliceEmail, http.StatusOK},
		{"/api/namespaces/team-a/grants/g-1", eveEmail, http.StatusForbidden},
		{"/api/namespaces/team-a/grants/g-1/permissions", eveEmail, http.StatusForbidden},
		{"/api/namespaces/team-a/grants/g-orphan", aliceEmail, http.StatusForbidden},
		{"/api/namespaces/team-a/grants/g-orphan/permissions", aliceEmail, http.StatusForbidden},
		{"/api/namespaces/team-a/grants/nope", bobEmail, http.StatusNotFound},
	} {
		if w := call(s, http.MethodGet, tc.path, tc.as, ""); w.Code != tc.want {
			t.Errorf("GET %s as %s: expected %d, got %d: %s", tc.path, tc.as, tc.want, w.Code, w.Body.String())
		}
	}

	var asApprover GrantView
	_ = json.Unmarshal(call(s, http.MethodGet, "/api/namespaces/team-a/grants/g-1", aliceEmail, "").Body.Bytes(), &asApprover)
	if asApprover.Name != "g-1" || asApprover.Actions.Revoke {
		t.Errorf("expected the approver to see g-1 without a revoke action, got %+v", asApprover)
	}
	if w := call(s, http.MethodPost, "/api/namespaces/team-a/grants/g-1/revoke", aliceEmail, `{"reason":"x"}`); w.Code != http.StatusForbidden {
		t.Errorf("approver revoke: expected 403, got %d: %s", w.Code, w.Body.String())
	}
	var asRequester GrantView
	_ = json.Unmarshal(call(s, http.MethodGet, "/api/namespaces/team-a/grants/g-1", bobEmail, "").Body.Bytes(), &asRequester)
	if !asRequester.Actions.Revoke {
		t.Errorf("expected the requester to keep the revoke action, got %+v", asRequester)
	}
}

func TestFilterAudit(t *testing.T) {
	row := func(request string, seq, minute int, event, actor, comment string, clusters ...string) AuditItem {
		return AuditItem{
			ID: fmt.Sprintf("team-a/%s#%d", request, seq), Timestamp: at(minute).Time, EventType: event,
			Namespace: teamA, Request: request, Requester: aliceEmail, Actor: actor, Profile: "db-admin", Comment: comment, Clusters: clusters,
		}
	}
	rows := []AuditItem{
		row("req-b", 1, 0, "REQUEST_SUBMITTED", aliceEmail, ""),
		row("req-a", 1, 0, "REQUEST_SUBMITTED", aliceEmail, ""),
		row("req-a", 2, 3, "APPROVAL_GIVEN", bobEmail, "LGTM, ship it"),
		row("req-a", 3, 3, "REQUEST_APPROVED", "", ""),
		row("req-a", 4, 4, "ACCESS_GRANTED", "", "", clusterA),
		row("req-b", 2, 4, "ACCESS_GRANTED", "", "", clusterB),
	}
	ids := func(items []AuditItem) []string {
		out := make([]string, 0, len(items))
		for _, r := range items {
			out = append(out, strings.TrimPrefix(r.ID, "team-a/"))
		}
		return out
	}
	for _, tc := range []struct {
		query string
		want  []string
	}{
		{"", []string{"req-b#2", "req-a#4", "req-a#3", "req-a#2", "req-b#1", "req-a#1"}},
		{"order=desc", []string{"req-b#2", "req-a#4", "req-a#3", "req-a#2", "req-b#1", "req-a#1"}},
		{"order=asc", []string{"req-a#1", "req-b#1", "req-a#2", "req-a#3", "req-a#4", "req-b#2"}},
		{"event=ACCESS_GRANTED&order=asc", []string{"req-a#4", "req-b#2"}},
		{"cluster=cluster-b", []string{"req-b#2"}},
		{"q=lgtm", []string{"req-a#2"}},
		{"q=BOB@company", []string{"req-a#2"}},
		{"q=DB-ADMIN&event=REQUEST_SUBMITTED", []string{"req-b#1", "req-a#1"}},
		{"q=nothing-matches", []string{}},
	} {
		q, _ := url.ParseQuery(tc.query)
		got, err := filterAudit(slices.Clone(rows), q)
		if err != nil || !slices.Equal(ids(got), tc.want) {
			t.Errorf("%q: expected %v, got %v (err %v)", tc.query, tc.want, ids(got), err)
		}
	}
	if _, err := filterAudit(rows, url.Values{"order": {"newest"}}); err == nil {
		t.Error("expected an unknown order to be an error")
	}
}

func TestAuditRoute(t *testing.T) {
	objs := make([]client.Object, 0, 4)
	for _, name := range []string{"req-1", "req-2", "req-3"} {
		req := historyRequest(teamA, name, aliceEmail, protemv1alpha1.RequestPhaseCancelled, "")
		req.Status.Approvals = []protemv1alpha1.ApprovalDecision{{User: aliceEmail, Decision: protemv1alpha1.ApprovalDecisionCancelled, At: at(1)}}
		objs = append(objs, req)
	}
	objs = append(objs, historyRequest(teamA, "hidden", bobEmail, protemv1alpha1.RequestPhasePending, sreLeadsGroup))
	s, _ := newMgmtServer(t, denyAll, ServerOptions{}, objs...)

	first, total := decodePage[AuditItem](t, s, "/api/audit?limit=4", aliceEmail)
	second, _ := decodePage[AuditItem](t, s, "/api/audit?limit=4&offset=4", aliceEmail)
	all := append(first, second...)
	ids := make([]string, 0, len(all))
	for _, r := range all {
		ids = append(ids, r.ID)
	}
	if total != 6 || len(first) != 4 || len(second) != 2 || len(slices.Compact(slices.Sorted(slices.Values(ids)))) != 6 {
		t.Errorf("expected six rows over two pages, each once, got %v (total %d)", ids, total)
	}
	if all[0].EventType != "REQUEST_CANCELLED" || all[5].EventType != "REQUEST_SUBMITTED" {
		t.Errorf("expected newest first, got %v", ids)
	}
	cancelled, total := decodePage[AuditItem](t, s, "/api/audit?event=REQUEST_CANCELLED&order=asc", aliceEmail)
	if total != 3 || cancelled[0].Request != "req-1" || cancelled[0].Actor != aliceEmail {
		t.Errorf("expected three cancellations oldest first, got %+v", cancelled)
	}
	for _, path := range []string{"/api/audit?order=up", "/api/audit?limit=500"} {
		if w := call(s, http.MethodGet, path, aliceEmail, ""); w.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d", path, w.Code)
		}
	}
}
