package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

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

// The operator ignores the hub cluster's own subject, so accepting one would be a setting that
// silently does nothing.
func TestPatchClusterRefusesASubjectForTheHub(t *testing.T) {
	s, c := newMgmtServer(t, allowAll, ServerOptions{UIManagement: enabled, HubClusterName: "c1"}, testCluster(nil, nil))
	w := call(s, http.MethodPatch, "/api/clusters/c1", aliceEmail, `{"subject":"oidc:{{.Username}}"}`)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "subject template") {
		t.Fatalf("PATCH subject on the hub = %d: %s, want 400 naming the operator's subject template", w.Code, w.Body.String())
	}
	var got protemv1alpha1.TargetCluster
	if err := c.Get(context.Background(), client.ObjectKey{Name: "c1"}, &got); err != nil || got.Spec.Subject != "{{.Email}}" {
		t.Fatalf("hub subject = %q (err %v), want it unchanged", got.Spec.Subject, err)
	}
	if w := call(s, http.MethodPatch, "/api/clusters/c1", aliceEmail, `{"paused":true}`); w.Code != http.StatusOK {
		t.Fatalf("PATCH paused on the hub = %d: %s, want 200", w.Code, w.Body.String())
	}
}

// Nothing reports on the hub's own cluster through a ClusterGrant: the view comes from the grant,
// and one left behind by a version that wrote them must not be read.
func TestGrantPermissionsForTheHubComeFromTheGrant(t *testing.T) {
	grant := &protemv1alpha1.AccessGrant{
		ObjectMeta: metav1.ObjectMeta{Namespace: teamA, Name: "g1"},
		Spec: protemv1alpha1.AccessGrantSpec{
			Requester: protemv1alpha1.RequesterInfo{Username: aliceUsername, Email: aliceEmail},
			Role:      protemv1alpha1.TargetRoleRef{Kind: "ClusterRole", Name: "edit"},
			Clusters:  []string{"hub"},
		},
		Status: protemv1alpha1.AccessGrantStatus{Clusters: []protemv1alpha1.ClusterBindingState{{
			ClusterName: "hub",
			State:       protemv1alpha1.ClusterBindingStateActive,
			Message:     "Applied by the hub",
			Bindings:    []protemv1alpha1.ManagedBindingRef{{Kind: "RoleBinding", Namespace: "payments", Name: "protem-grant-team-a.g1"}},
		}}},
	}
	leftover := &protemv1alpha1.ClusterGrant{
		ObjectMeta: metav1.ObjectMeta{Namespace: "protem-cluster-hub", Name: "team-a.g1"},
		Spec:       protemv1alpha1.ClusterGrantSpec{Subject: "stale"},
	}
	s, _ := newMgmtServer(t, allowAll, ServerOptions{HubClusterName: "hub", SubjectTemplate: "oidc:{{.Username}}"}, grant, leftover)

	w := call(s, http.MethodGet, "/api/namespaces/team-a/grants/g1/permissions", aliceEmail, "")
	var got []ClusterPermissionsView
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || w.Code != http.StatusOK || len(got) != 1 {
		t.Fatalf("GET permissions = %d %v: %s", w.Code, err, w.Body.String())
	}
	hub := got[0]
	if hub.State != string(protemv1alpha1.ClusterBindingStateActive) || hub.Subject != "oidc:alice" || len(hub.Bindings) != 1 || hub.Bindings[0].Namespace != "payments" || hub.Permissions.Source != sourceHub {
		t.Errorf("hub view = %+v, want Active, bound as oidc:alice, one binding in payments, rules from the hub", hub)
	}
}
