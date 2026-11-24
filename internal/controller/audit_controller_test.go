package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
)

var auditT0 = time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)

const auditGrant = "grant-1"

type auditLine struct {
	Kind      string `json:"kind"`
	ID        string `json:"id"`
	EventType string `json:"eventType"`
}

type auditHarness struct {
	r   *AuditReconciler
	c   client.Client
	out *bytes.Buffer
	now time.Time
}

func newAuditHarness(t *testing.T, funcs interceptor.Funcs, retention time.Duration, now time.Time, objs ...client.Object) *auditHarness {
	c := fake.NewClientBuilder().
		WithScheme(setupTestScheme(t)).
		WithObjects(objs...).
		WithStatusSubresource(&protemv1alpha1.AccessRequest{}).
		WithInterceptorFuncs(funcs).
		Build()
	h := &auditHarness{c: c, out: &bytes.Buffer{}, now: now}
	h.r = &AuditReconciler{Client: c, Out: h.out, Retention: retention, Now: func() time.Time { return h.now }}
	return h
}

func (h *auditHarness) reconcile() (ctrl.Result, error) {
	return h.r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Namespace: defaultNs, Name: reqOne}})
}

func (h *auditHarness) lines(t *testing.T) []auditLine {
	t.Helper()
	if h.out.Len() == 0 {
		return nil
	}
	raws := strings.Split(strings.TrimSuffix(h.out.String(), "\n"), "\n")
	lines := make([]auditLine, 0, len(raws))
	for _, raw := range raws {
		var l auditLine
		if err := json.Unmarshal([]byte(raw), &l); err != nil {
			t.Fatalf("line is not JSON: %q: %v", raw, err)
		}
		if l.Kind != "ProtemAudit" || !strings.HasPrefix(raw, `{"kind":"ProtemAudit",`) {
			t.Errorf("expected the line to start with kind ProtemAudit, got %q", raw)
		}
		lines = append(lines, l)
	}
	return lines
}

func (h *auditHarness) request(t *testing.T) (*protemv1alpha1.AccessRequest, bool) {
	t.Helper()
	var req protemv1alpha1.AccessRequest
	err := h.c.Get(context.Background(), client.ObjectKey{Namespace: defaultNs, Name: reqOne}, &req)
	if apierrors.IsNotFound(err) {
		return nil, false
	}
	if err != nil {
		t.Fatalf("failed to get request: %v", err)
	}
	return &req, true
}

func eventTypes(lines []auditLine) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, l.EventType)
	}
	return out
}

func auditRequest() *protemv1alpha1.AccessRequest {
	return &protemv1alpha1.AccessRequest{
		ObjectMeta: metav1.ObjectMeta{Namespace: defaultNs, Name: reqOne, CreationTimestamp: metav1.NewTime(auditT0)},
		Spec: protemv1alpha1.AccessRequestSpec{
			ProfileRef: profOne,
			Requester:  protemv1alpha1.RequesterInfo{Username: aliceUsername, Email: aliceEmail},
		},
	}
}

func cancelledRequest(at time.Time, audited int32) *protemv1alpha1.AccessRequest {
	req := auditRequest()
	req.Status.Phase = protemv1alpha1.RequestPhaseCancelled
	req.Status.Approvals = []protemv1alpha1.ApprovalDecision{{User: aliceEmail, Decision: protemv1alpha1.ApprovalDecisionCancelled, At: metav1.NewTime(at)}}
	req.Status.AuditedEvents = audited
	return req
}

func approve(req *protemv1alpha1.AccessRequest, at time.Time) {
	req.Status.Phase = protemv1alpha1.RequestPhaseApproved
	req.Status.Approvals = []protemv1alpha1.ApprovalDecision{{User: "bob@company.com", Decision: protemv1alpha1.ApprovalDecisionApproved, At: metav1.NewTime(at), Rule: manualRule}}
	req.Status.Conditions = []metav1.Condition{{Type: "Decided", Status: metav1.ConditionTrue, LastTransitionTime: metav1.NewTime(at), Reason: "Approved", Message: "approved"}}
	req.Status.GrantRef = auditGrant
}

func auditAccessGrant(created, expires time.Time) *protemv1alpha1.AccessGrant {
	return &protemv1alpha1.AccessGrant{
		ObjectMeta: metav1.ObjectMeta{Namespace: defaultNs, Name: auditGrant, CreationTimestamp: metav1.NewTime(created)},
		Spec:       protemv1alpha1.AccessGrantSpec{RequestRef: reqOne, ExpiresAt: metav1.NewTime(expires)},
	}
}

func TestAuditNewRequestIsLoggedOnce(t *testing.T) {
	h := newAuditHarness(t, interceptor.Funcs{}, 0, auditT0.Add(time.Minute), auditRequest())

	if _, err := h.reconcile(); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}
	if got := eventTypes(h.lines(t)); !slices.Equal(got, []string{"REQUEST_SUBMITTED"}) {
		t.Fatalf("expected one REQUEST_SUBMITTED line, got %v", got)
	}
	if req, _ := h.request(t); req.Status.AuditedEvents != 1 {
		t.Errorf("expected the cursor at 1, got %d", req.Status.AuditedEvents)
	}

	if _, err := h.reconcile(); err != nil {
		t.Fatalf("second reconcile failed: %v", err)
	}
	if n := len(h.lines(t)); n != 1 {
		t.Errorf("expected the second reconcile to write nothing, got %d lines", n)
	}
}

func TestAuditLogsOnlyNewEventsInOrder(t *testing.T) {
	ctx := context.Background()
	h := newAuditHarness(t, interceptor.Funcs{}, 0, auditT0.Add(10*time.Minute), auditRequest())
	if _, err := h.reconcile(); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	req, _ := h.request(t)
	approve(req, auditT0.Add(5*time.Minute))
	if err := h.c.Status().Update(ctx, req); err != nil {
		t.Fatalf("failed to approve: %v", err)
	}
	if err := h.c.Create(ctx, auditAccessGrant(auditT0.Add(6*time.Minute), auditT0.Add(time.Hour))); err != nil {
		t.Fatalf("failed to create grant: %v", err)
	}
	if _, err := h.reconcile(); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	lines := h.lines(t)
	if got, want := eventTypes(lines), []string{"REQUEST_SUBMITTED", "APPROVAL_GIVEN", "REQUEST_APPROVED", "ACCESS_GRANTED"}; !slices.Equal(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
	ids := make([]string, 0, len(lines))
	for _, l := range lines {
		ids = append(ids, l.ID)
	}
	if len(slices.Compact(slices.Sorted(slices.Values(ids)))) != len(lines) {
		t.Errorf("expected every line to have its own id, got %v", ids)
	}
	if req, _ := h.request(t); req.Status.AuditedEvents != 4 {
		t.Errorf("expected the cursor at 4, got %d", req.Status.AuditedEvents)
	}
}

func TestAuditFailedCursorUpdateWritesTheSameEventsAgain(t *testing.T) {
	patchErr := errors.New("conflict")
	failing := true
	funcs := interceptor.Funcs{SubResourcePatch: func(ctx context.Context, c client.Client, sub string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
		if failing {
			return patchErr
		}
		return c.SubResource(sub).Patch(ctx, obj, patch, opts...)
	}}
	h := newAuditHarness(t, funcs, 0, auditT0.Add(time.Minute), auditRequest())

	if _, err := h.reconcile(); !errors.Is(err, patchErr) {
		t.Fatalf("expected the patch error, got %v", err)
	}
	if req, _ := h.request(t); req.Status.AuditedEvents != 0 {
		t.Fatalf("expected the cursor to stay at 0, got %d", req.Status.AuditedEvents)
	}

	failing = false
	if _, err := h.reconcile(); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}
	lines := h.lines(t)
	if len(lines) != 2 || lines[0].ID != lines[1].ID {
		t.Errorf("expected the same event written twice, got %+v", lines)
	}
	if req, _ := h.request(t); req.Status.AuditedEvents != 1 {
		t.Errorf("expected the cursor at 1, got %d", req.Status.AuditedEvents)
	}
}

func TestAuditLogsExpiryWhenItHappens(t *testing.T) {
	req := auditRequest()
	approve(req, auditT0.Add(time.Minute))
	expires := auditT0.Add(time.Hour)
	h := newAuditHarness(t, interceptor.Funcs{}, 0, auditT0.Add(30*time.Minute), req, auditAccessGrant(auditT0.Add(2*time.Minute), expires))

	res, err := h.reconcile()
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}
	if want := 30*time.Minute + time.Second; res.RequeueAfter != want {
		t.Errorf("expected RequeueAfter %v, got %v", want, res.RequeueAfter)
	}

	h.now = expires.Add(time.Second)
	if _, err := h.reconcile(); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}
	lines := h.lines(t)
	if len(lines) == 0 || lines[len(lines)-1].EventType != "ACCESS_EXPIRED" {
		t.Errorf("expected ACCESS_EXPIRED last, got %v", eventTypes(lines))
	}
}

func TestAuditRetention(t *testing.T) {
	now := auditT0.Add(3 * time.Hour)
	revokedGrant := func(at time.Time) *protemv1alpha1.AccessGrant {
		g := auditAccessGrant(auditT0.Add(2*time.Minute), now.Add(time.Hour))
		g.Spec.Revocation = &protemv1alpha1.RevocationDetails{By: aliceEmail, At: metav1.NewTime(at)}
		return g
	}
	approved := func(audited int32) *protemv1alpha1.AccessRequest {
		req := auditRequest()
		approve(req, auditT0.Add(time.Minute))
		req.Status.AuditedEvents = audited
		return req
	}
	for _, tc := range []struct {
		name         string
		retention    time.Duration
		objs         []client.Object
		wantDeleted  bool
		wantRequeue  time.Duration
		wantNewLines int
	}{
		{"retention 0 keeps a finished request", 0, []client.Object{cancelledRequest(now.Add(-2*time.Hour), 2)}, false, 0, 0},
		{"finished 2h ago is deleted", time.Hour, []client.Object{cancelledRequest(now.Add(-2*time.Hour), 2)}, true, 0, 0},
		{"finished 30m ago is kept", time.Hour, []client.Object{cancelledRequest(now.Add(-30*time.Minute), 2)}, false, 30 * time.Minute, 0},
		{"revoked 2h ago is deleted", time.Hour, []client.Object{approved(5), revokedGrant(now.Add(-2 * time.Hour))}, true, 0, 0},
		{"approved with its grant gone is deleted", time.Hour, []client.Object{approved(5)}, true, 0, 0},
		{"a live grant is kept", time.Hour, []client.Object{approved(4), auditAccessGrant(auditT0.Add(2*time.Minute), now.Add(time.Hour))}, false, time.Hour + time.Second, 0},
		{"unlogged events are logged before deletion", time.Hour, []client.Object{cancelledRequest(now.Add(-2*time.Hour), 0)}, true, 0, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newAuditHarness(t, interceptor.Funcs{}, tc.retention, now, tc.objs...)
			res, err := h.reconcile()
			if err != nil {
				t.Fatalf("reconcile failed: %v", err)
			}
			if _, exists := h.request(t); exists == tc.wantDeleted {
				t.Errorf("expected deleted=%v, request exists=%v", tc.wantDeleted, exists)
			}
			if res.RequeueAfter != tc.wantRequeue {
				t.Errorf("expected RequeueAfter %v, got %v", tc.wantRequeue, res.RequeueAfter)
			}
			if n := len(h.lines(t)); n != tc.wantNewLines {
				t.Errorf("expected %d lines, got %d", tc.wantNewLines, n)
			}
		})
	}
}

// The reconciler reads through a cache that can lag its own cursor write by a moment.
func TestAuditStaleReadOfTheCursorWritesNothingTwice(t *testing.T) {
	stale := false
	funcs := interceptor.Funcs{Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
		err := c.Get(ctx, key, obj, opts...)
		if req, ok := obj.(*protemv1alpha1.AccessRequest); ok && stale {
			req.Status.AuditedEvents = 0
		}
		return err
	}}
	h := newAuditHarness(t, funcs, 0, auditT0.Add(time.Minute), auditRequest())
	if _, err := h.reconcile(); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	stale = true
	if _, err := h.reconcile(); err != nil {
		t.Fatalf("reconcile on a stale read failed: %v", err)
	}
	if got := eventTypes(h.lines(t)); !slices.Equal(got, []string{"REQUEST_SUBMITTED"}) {
		t.Errorf("expected the event once, got %v", got)
	}
}
