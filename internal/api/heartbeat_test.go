package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
)

// The controller writes status.lastHeartbeat only when readiness changes, so the live value is the agent's Lease.
func TestClusterHeartbeatIsTheAgentLeaseRenewal(t *testing.T) {
	stale := metav1.NewTime(time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC))
	renewed := metav1.NewMicroTime(time.Date(2026, 11, 24, 10, 7, 30, 0, time.UTC))
	cluster := &protemv1alpha1.TargetCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "prod"},
		Status:     protemv1alpha1.TargetClusterStatus{LastHeartbeat: &stale},
	}
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Namespace: "protem-cluster-prod", Name: "agent"},
		Spec:       coordinationv1.LeaseSpec{RenewTime: &renewed},
	}
	silent := &protemv1alpha1.TargetCluster{ObjectMeta: metav1.ObjectMeta{Name: "silent"}}
	c := fake.NewClientBuilder().WithScheme(setupTestScheme(t)).WithObjects(cluster, lease, silent).Build()
	s, err := NewServer(c, ServerOptions{Addr: "127.0.0.1:0", DevAuth: true})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	r := httptest.NewRequest(http.MethodGet, "/api/clusters", nil)
	r.Header.Set("X-Test-User", aliceEmail)
	w := httptest.NewRecorder()
	s.Mux.ServeHTTP(w, r)

	var views []ClusterView
	if err := json.NewDecoder(w.Body).Decode(&views); err != nil || len(views) != 2 {
		t.Fatalf("GET /api/clusters = %d, %d clusters, err %v", w.Code, len(views), err)
	}
	if got := views[0].LastHeartbeat; got == nil || !got.Equal(renewed.Time) {
		t.Errorf("prod lastHeartbeat = %v, want the lease renewal %v", got, renewed.Time)
	}
	if got := views[1].LastHeartbeat; got != nil {
		t.Errorf("silent lastHeartbeat = %v, want none", got)
	}
}
