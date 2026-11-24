package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
	"github.com/iLLeniumStudios/protem/internal/fleet"
)

func TestClusterAllowedRolesComeFromTheAgentLease(t *testing.T) {
	lease := func(cluster string, annotations map[string]string) *coordinationv1.Lease {
		return &coordinationv1.Lease{ObjectMeta: metav1.ObjectMeta{
			Namespace: fleet.ClusterNamespace(cluster), Name: fleet.AgentLeaseName, Annotations: annotations,
		}}
	}
	cluster := func(name string) *protemv1alpha1.TargetCluster {
		return &protemv1alpha1.TargetCluster{ObjectMeta: metav1.ObjectMeta{Name: name}}
	}
	c := fake.NewClientBuilder().WithScheme(setupTestScheme(t)).WithObjects(
		cluster("local"), cluster("prod"), cluster("silent"), cluster("partial"), cluster("orphan"),
		lease("prod", map[string]string{fleet.AllowedRolesAnnotation: "edit, ,view", fleet.AllowedRolesPartialAnnotation: "True"}),
		lease("silent", nil),
		lease("partial", map[string]string{fleet.AllowedRolesAnnotation: "view", fleet.AllowedRolesPartialAnnotation: "true"}),
		lease("orphan", map[string]string{fleet.AllowedRolesPartialAnnotation: "true"}),
	).Build()
	s, err := NewServer(c, ServerOptions{Addr: "127.0.0.1:0", DevAuth: true, HubClusterName: "local"})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	r := httptest.NewRequest(http.MethodGet, "/api/clusters", nil)
	r.Header.Set("X-Test-User", aliceEmail)
	w := httptest.NewRecorder()
	s.Mux.ServeHTTP(w, r)

	var views []map[string]json.RawMessage
	if err := json.NewDecoder(w.Body).Decode(&views); err != nil || len(views) != 5 {
		t.Fatalf("GET /api/clusters = %d, %d clusters, err %v", w.Code, len(views), err)
	}
	got := map[string][]string{}
	var partial []string
	for _, v := range views {
		var name string
		_ = json.Unmarshal(v["name"], &name)
		var isPartial bool
		if _ = json.Unmarshal(v["allowedRolesPartial"], &isPartial); isPartial {
			partial = append(partial, name)
		}
		raw, present := v["allowedRoles"]
		if !present {
			continue
		}
		var roles []string
		if err := json.Unmarshal(raw, &roles); err != nil {
			t.Fatal(err)
		}
		got[name] = roles
	}
	if !slices.Equal(got["prod"], []string{"edit", "view"}) {
		t.Errorf("prod allowedRoles = %q, want [edit view]", got["prod"])
	}
	for _, name := range []string{"local", "silent", "orphan"} {
		if roles, present := got[name]; present {
			t.Errorf("%s allowedRoles = %q, want the field absent", name, roles)
		}
	}
	if !slices.Equal(partial, []string{"partial"}) {
		t.Errorf("clusters with allowedRolesPartial = %q, want only partial", partial)
	}
}
