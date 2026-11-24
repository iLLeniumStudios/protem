package api

import (
	"context"
	"net/http"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
)

// The controllers write a request's status between an approver loading it and deciding on it
// (a phase change, the audit cursor). conflictingServer makes the first n status writes lose that race.
func conflictingServer(t *testing.T, n int, objs ...client.Object) (*Server, client.Client) {
	t.Helper()
	c := fake.NewClientBuilder().WithScheme(setupTestScheme(t)).WithObjects(objs...).WithStatusSubresource(&protemv1alpha1.AccessRequest{}).
		WithInterceptorFuncs(interceptor.Funcs{SubResourceUpdate: func(ctx context.Context, c client.Client, sub string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
			if n > 0 {
				n--
				return apierrors.NewConflict(schema.GroupResource{Group: protemv1alpha1.GroupVersion.Group, Resource: "accessrequests"}, obj.GetName(), nil)
			}
			return c.SubResource(sub).Update(ctx, obj, opts...)
		}}).Build()
	s, err := NewServer(c, ServerOptions{Addr: testAddr, DevAuth: true})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return s, c
}

func TestDecisionSurvivesAStatusWriteByTheController(t *testing.T) {
	const path = "/api/namespaces/team-a/requests/req-1/approve"
	pending := func() *protemv1alpha1.AccessRequest {
		return historyRequest(teamA, "req-1", aliceEmail, protemv1alpha1.RequestPhasePending, sreLeadsGroup)
	}

	s, c := conflictingServer(t, 2, pending())
	if w := call(s, http.MethodPost, path, bobEmail, `{"comment":"ok"}`); w.Code != http.StatusOK {
		t.Fatalf("approve after two lost races = %d: %s, want 200", w.Code, w.Body.String())
	}
	var req protemv1alpha1.AccessRequest
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: teamA, Name: "req-1"}, &req); err != nil {
		t.Fatal(err)
	}
	if len(req.Status.Approvals) != 1 || req.Status.Approvals[0].User != bobEmail {
		t.Errorf("approvals = %+v, want bob's decision exactly once", req.Status.Approvals)
	}

	s, _ = conflictingServer(t, 100, pending())
	if w := call(s, http.MethodPost, path, bobEmail, `{"comment":"ok"}`); w.Code != http.StatusConflict {
		t.Errorf("approve that keeps losing the race = %d: %s, want 409 asking to try again", w.Code, w.Body.String())
	}
}
