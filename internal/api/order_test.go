package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
)

// The informer cache returns lists in no particular order, and creation timestamps have one-second
// resolution, so every list route must impose a total order of its own.
func TestListRoutesReturnTheSameOrderHoweverTheStoreOrdersItems(t *testing.T) {
	created := metav1.NewTime(time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC))
	objs := make([]client.Object, 0, 12)
	for _, name := range []string{"a", "b", "c"} {
		objs = append(objs,
			&protemv1alpha1.AccessRequest{
				ObjectMeta: metav1.ObjectMeta{Namespace: "team", Name: "req-" + name, CreationTimestamp: created},
				Spec:       protemv1alpha1.AccessRequestSpec{ProfileRef: "p-a", Requester: protemv1alpha1.RequesterInfo{Email: aliceEmail}},
				Status:     protemv1alpha1.AccessRequestStatus{Phase: protemv1alpha1.RequestPhasePending},
			},
			&protemv1alpha1.AccessGrant{
				ObjectMeta: metav1.ObjectMeta{Namespace: "team", Name: "grant-" + name, CreationTimestamp: created},
				Spec:       protemv1alpha1.AccessGrantSpec{Requester: protemv1alpha1.RequesterInfo{Email: aliceEmail}, ExpiresAt: metav1.NewTime(created.Add(time.Hour))},
				Status:     protemv1alpha1.AccessGrantStatus{Phase: protemv1alpha1.GrantPhaseActive},
			},
			&protemv1alpha1.AccessProfile{ObjectMeta: metav1.ObjectMeta{Namespace: "team", Name: "p-" + name, CreationTimestamp: created}},
			&protemv1alpha1.TargetCluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster-" + name, CreationTimestamp: created}},
		)
	}

	reversed := false
	c := fake.NewClientBuilder().WithScheme(setupTestScheme(t)).WithObjects(objs...).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if sar, ok := obj.(*authorizationv1.SubjectAccessReview); ok {
					sar.Status.Allowed = true
					return nil
				}
				return c.Create(ctx, obj, opts...)
			},
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				if err := c.List(ctx, list, opts...); err != nil || !reversed {
					return err
				}
				items, err := meta.ExtractList(list)
				if err != nil {
					return err
				}
				slices.Reverse(items)
				return meta.SetList(list, items)
			},
		}).Build()
	s, err := NewServer(c, ServerOptions{Addr: "127.0.0.1:0", DevAuth: true})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	get := func(path string) string {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.Header.Set("X-Test-User", aliceEmail)
		w := httptest.NewRecorder()
		s.Mux.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s = %d: %s", path, w.Code, w.Body.String())
		}
		return w.Body.String()
	}

	for _, path := range []string{"/api/requests", "/api/grants", "/api/profiles", "/api/clusters", "/api/audit"} {
		reversed = false
		forward := get(path)
		reversed = true
		if backward := get(path); backward != forward {
			t.Errorf("GET %s depends on the store's order:\n forward:  %s\n reversed: %s", path, forward, backward)
		}
	}
}

func TestRequestsCreatedInTheSameMillisecondAllSucceed(t *testing.T) {
	profile := &protemv1alpha1.AccessProfile{ObjectMeta: metav1.ObjectMeta{Namespace: "team", Name: "p"}}
	c := fake.NewClientBuilder().WithScheme(setupTestScheme(t)).WithObjects(profile).Build()
	s, err := NewServer(c, ServerOptions{Addr: "127.0.0.1:0", DevAuth: true})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	const n = 50
	for i := range n {
		r := httptest.NewRequest(http.MethodPost, "/api/namespaces/team/requests", strings.NewReader(`{"profileRef":"p","reason":"burst"}`))
		r.Header.Set("X-Test-User", fmt.Sprintf("user%d@company.com", i))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != http.StatusCreated {
			t.Fatalf("request %d of %d = %d: %s", i+1, n, w.Code, w.Body.String())
		}
	}
	var list protemv1alpha1.AccessRequestList
	if err := c.List(context.Background(), &list); err != nil || len(list.Items) != n {
		t.Fatalf("stored %d requests (err %v), want %d", len(list.Items), err, n)
	}
}

func TestRequestingAProfileAlreadyWaitedForIsRefused(t *testing.T) {
	profile := &protemv1alpha1.AccessProfile{ObjectMeta: metav1.ObjectMeta{Namespace: "team", Name: "p"}}
	c := fake.NewClientBuilder().WithScheme(setupTestScheme(t)).WithObjects(profile).Build()
	s, err := NewServer(c, ServerOptions{Addr: "127.0.0.1:0", DevAuth: true})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	send := func(user, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/namespaces/team/requests", strings.NewReader(body))
		r.Header.Set("X-Test-User", user)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w
	}
	if w := send(aliceEmail, `{"profileRef":"p","reason":"first","targetClusters":["prod"]}`); w.Code != http.StatusCreated {
		t.Fatalf("first request = %d: %s", w.Code, w.Body.String())
	}
	if w := send(aliceEmail, `{"profileRef":"p","reason":"again","targetClusters":["prod"]}`); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "on prod is already waiting") {
		t.Errorf("same profile and cluster again = %d: %s, want 409 saying a request is already waiting", w.Code, w.Body.String())
	}
	if w := send(aliceEmail, `{"profileRef":"p","reason":"elsewhere","targetClusters":["dev"]}`); w.Code != http.StatusCreated {
		t.Errorf("same profile on another cluster = %d: %s, want 201", w.Code, w.Body.String())
	}
	if w := send(bobEmail, `{"profileRef":"p","reason":"mine","targetClusters":["prod"]}`); w.Code != http.StatusCreated {
		t.Errorf("someone else's request = %d: %s, want 201", w.Code, w.Body.String())
	}
}
