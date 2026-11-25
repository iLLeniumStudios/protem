package api

import (
	"encoding/json"
	"net/http"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
)

// The agent binds a person by the cluster's subject template, else the hub's; the view names the caller the same way.
func TestClusterViewBoundAsIsTheCallersSubject(t *testing.T) {
	for _, tc := range []struct {
		name, clusterSubject, serverTemplate, caller, want string
	}{
		{"no templates anywhere means the email", "", "", aliceEmail, aliceEmail},
		{"the hub template applies when the cluster sets none", "", "{{.Username}}", aliceEmail, aliceUsername},
		{"the cluster template wins over the hub's", "oidc:{{.Email}}", "{{.Username}}", aliceEmail, "oidc:alice@company.com"},
		{"username", "{{.Username}}", "", aliceEmail, aliceUsername},
		{"named for the caller, not a fixed person", "aks:{{.Username}}", "", bobEmail, "aks:bob"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cluster := &protemv1alpha1.TargetCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "prod"},
				Spec:       protemv1alpha1.TargetClusterSpec{Subject: tc.clusterSubject},
			}
			s, _ := newMgmtServer(t, allowAll, ServerOptions{SubjectTemplate: tc.serverTemplate}, cluster)
			w := call(s, http.MethodGet, "/api/clusters", tc.caller, "")
			var views []ClusterView
			if err := json.NewDecoder(w.Body).Decode(&views); err != nil || len(views) != 1 {
				t.Fatalf("GET /api/clusters = %d, %d clusters, err %v", w.Code, len(views), err)
			}
			if got := views[0].BoundAs; got != tc.want {
				t.Errorf("boundAs = %q, want %q", got, tc.want)
			}
		})
	}
}

// On the hub itself the operator binds with its own template and ignores the cluster's.
func TestClusterViewBoundAsOnTheHubUsesTheOperatorsTemplate(t *testing.T) {
	hub := &protemv1alpha1.TargetCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "local"},
		Spec:       protemv1alpha1.TargetClusterSpec{Subject: "oidc:{{.Email}}"},
	}
	s, _ := newMgmtServer(t, allowAll, ServerOptions{SubjectTemplate: "{{.Username}}"}, hub)
	w := call(s, http.MethodGet, "/api/clusters", aliceEmail, "")
	var views []ClusterView
	if err := json.NewDecoder(w.Body).Decode(&views); err != nil || len(views) != 1 {
		t.Fatalf("GET /api/clusters = %d, %d clusters, err %v", w.Code, len(views), err)
	}
	if got := views[0].BoundAs; got != aliceUsername {
		t.Errorf("boundAs on the hub = %q, want %q", got, aliceUsername)
	}
}
