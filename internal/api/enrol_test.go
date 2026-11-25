package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
	"github.com/iLLeniumStudios/protem/internal/fleet"
)

const (
	hubURL      = "https://hub.example.com:6443"
	edgeImage   = "registry.example.com:5000/protem-agent:v1.2.3"
	edgeSources = "https://example.com/protem//config/agent/spoke?ref=v1.2.3"
)

var hubCA = []byte("-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n")

func enrolOptions() ServerOptions {
	return ServerOptions{
		UIManagement: enabled, HubClusterName: hubClusterName, HubAPIURL: hubURL, HubCAData: hubCA,
		AgentImage: edgeImage, AgentManifests: edgeSources,
	}
}

func hubCluster() *protemv1alpha1.TargetCluster {
	return &protemv1alpha1.TargetCluster{ObjectMeta: metav1.ObjectMeta{Name: hubClusterName}, Spec: protemv1alpha1.TargetClusterSpec{DisplayName: "Hub"}}
}

func agentAccount(cluster string) client.ObjectKey {
	return client.ObjectKey{Namespace: fleet.ClusterNamespace(cluster), Name: fleet.AgentName}
}

func activeGrant(cluster string) *protemv1alpha1.AccessGrant {
	return &protemv1alpha1.AccessGrant{
		ObjectMeta: metav1.ObjectMeta{Namespace: teamA, Name: "g-active"},
		Spec:       protemv1alpha1.AccessGrantSpec{Clusters: []string{cluster}, ExpiresAt: metav1.NewTime(time.Now().Add(time.Hour))},
		Status:     protemv1alpha1.AccessGrantStatus{Phase: protemv1alpha1.GrantPhaseActive},
	}
}

func enrolCases(as string) []writeCase {
	cluster := func() client.Object { return &protemv1alpha1.TargetCluster{} }
	return []writeCase{
		{"add", http.MethodPost, "/api/clusters", `{"name":"edge","displayName":"Edge"}`,
			client.ObjectKey{Name: "edge"}, cluster, sarRule{as, "create", "targetclusters", "", ""}, http.StatusCreated},
		{"join", http.MethodPost, "/api/clusters/c1/join", "",
			client.ObjectKey{Name: "c1"}, cluster, sarRule{as, "update", "targetclusters", "", "c1"}, http.StatusOK},
		{"reissue", http.MethodPost, "/api/clusters/c1/reissue", "",
			client.ObjectKey{Name: "c1"}, cluster, sarRule{as, "update", "targetclusters", "", "c1"}, http.StatusOK},
		{"delete", http.MethodDelete, "/api/clusters/c1", "",
			client.ObjectKey{Name: "c1"}, cluster, sarRule{as, "delete", "targetclusters", "", "c1"}, http.StatusNoContent},
	}
}

func TestEnrol_RoutesOnlyWithManagementAndPermission(t *testing.T) {
	for _, mode := range []string{"", "readonly", enabled} {
		for _, tc := range enrolCases(aliceEmail) {
			for _, permitted := range []bool{false, true} {
				opt := enrolOptions()
				opt.UIManagement = mode
				policy := denyAll
				if permitted {
					policy = allowOnly(tc.sar)
				}
				s, c := newMgmtServer(t, policy, opt, testCluster(nil, nil))
				before := stateOf(t, c, tc.key, tc.obj())
				w := call(s, tc.method, tc.path, aliceEmail, tc.body)
				want := tc.success
				switch {
				case mode != enabled:
					want = http.StatusNotFound
				case !permitted:
					want = http.StatusForbidden
				}
				if w.Code != want {
					t.Errorf("mode %q %s permitted=%v: expected %d, got %d: %s", mode, tc.name, permitted, want, w.Code, w.Body.String())
				}
				changed := stateOf(t, c, tc.key, tc.obj()) != before
				account := stateOf(t, c, agentAccount("c1"), &corev1.ServiceAccount{}).exists
				if want != tc.success && (changed || account) {
					t.Errorf("mode %q %s permitted=%v: refused request wrote: cluster changed %v, agent account %v", mode, tc.name, permitted, changed, account)
				}
				if want == tc.success && !changed {
					t.Errorf("enabled %s: expected the TargetCluster to change", tc.name)
				}
			}
		}
	}
}

func TestEnrol_AddCluster(t *testing.T) {
	s, c := newMgmtServer(t, allowAll, enrolOptions(), testCluster(nil, nil), hubCluster())
	for _, tc := range []struct {
		body string
		want int
	}{
		{`{"name":"Edge_1","displayName":"Edge"}`, http.StatusBadRequest},
		{`{"name":"","displayName":"Edge"}`, http.StatusBadRequest},
		{`{"name":"` + strings.Repeat("a", 41) + `","displayName":"Edge"}`, http.StatusBadRequest},
		{`{"name":"` + hubClusterName + `","displayName":"Hub again"}`, http.StatusBadRequest},
		{`{"name":"edge","displayName":"  "}`, http.StatusBadRequest},
		{`{"name":"edge","displayName":"Edge","labels":{"protem.dev/team":"x"}}`, http.StatusBadRequest},
		{`{"name":"edge","displayName":"Edge","enforcement":"Authorizer"}`, http.StatusBadRequest},
		{`{"name":"c1","displayName":"Again"}`, http.StatusConflict},
	} {
		w := call(s, http.MethodPost, "/api/clusters", aliceEmail, tc.body)
		if w.Code != tc.want {
			t.Errorf("%s: expected %d, got %d: %s", tc.body, tc.want, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"error"`) {
			t.Errorf("%s: expected an error message, got %s", tc.body, w.Body.String())
		}
	}
	if stateOf(t, c, client.ObjectKey{Name: "edge"}, &protemv1alpha1.TargetCluster{}).exists {
		t.Fatal("a refused add created the TargetCluster")
	}

	name := strings.Repeat("a", 40)
	w := call(s, http.MethodPost, "/api/clusters", aliceEmail,
		`{"name":"`+name+`","displayName":"Edge","description":"Edge site","labels":{"env":"edge"}}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var view ClusterView
	_ = json.Unmarshal(w.Body.Bytes(), &view)
	if view.Name != name || view.DisplayName != "Edge" || view.Agent != "waiting" || view.Labels["env"] != "edge" {
		t.Errorf("unexpected view %+v", view)
	}
	var stored protemv1alpha1.TargetCluster
	if err := c.Get(context.Background(), client.ObjectKey{Name: name}, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Spec.DisplayName != "Edge" || stored.Spec.Description != "Edge site" || stored.Spec.Subject != "" || stored.Spec.Enforcement != "" {
		t.Errorf("unexpected spec %+v", stored.Spec)
	}
}

type kustomization struct {
	Resources []string `json:"resources"`
	Images    []struct {
		Name    string `json:"name"`
		NewName string `json:"newName"`
		NewTag  string `json:"newTag"`
	} `json:"images"`
	ConfigMapGenerator []struct {
		Name      string   `json:"name"`
		Namespace string   `json:"namespace"`
		Literals  []string `json:"literals"`
	} `json:"configMapGenerator"`
	SecretGenerator []struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
		Options   struct {
			DisableNameSuffixHash bool `json:"disableNameSuffixHash"`
		} `json:"options"`
		Files []string `json:"files"`
	} `json:"secretGenerator"`
}

func decodeJoin(t *testing.T, w interface{ Bytes() []byte }) (JoinView, kustomization) {
	t.Helper()
	var view JoinView
	if err := json.Unmarshal(w.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	var k kustomization
	if err := yaml.UnmarshalStrict([]byte(view.Kustomization), &k); err != nil {
		t.Fatalf("kustomization does not parse: %v\n%s", err, view.Kustomization)
	}
	return view, k
}

func TestEnrol_JoinReturnsKubeconfigAndKustomization(t *testing.T) {
	s, c := newMgmtServer(t, allowAll, enrolOptions(), testCluster(nil, nil))
	w := call(s, http.MethodPost, "/api/clusters/c1/join", aliceEmail, "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	view, k := decodeJoin(t, w.Body)
	if view.Cluster != "c1" || view.Command != "kubectl apply -k ." || view.ExpiresAt.IsZero() {
		t.Errorf("unexpected view %+v", view)
	}

	cfg, err := clientcmd.Load([]byte(view.Kubeconfig))
	if err != nil {
		t.Fatalf("kubeconfig does not parse: %v", err)
	}
	current, ok := cfg.Contexts[cfg.CurrentContext]
	if !ok {
		t.Fatalf("current context %q missing", cfg.CurrentContext)
	}
	cluster, user := cfg.Clusters[current.Cluster], cfg.AuthInfos[current.AuthInfo]
	if current.Cluster != "hub" || cluster == nil || cluster.Server != hubURL || string(cluster.CertificateAuthorityData) != string(hubCA) {
		t.Errorf("unexpected cluster %q %+v", current.Cluster, cluster)
	}
	if current.AuthInfo != fleet.AgentName || user == nil || user.Token == "" {
		t.Errorf("unexpected user %q %+v", current.AuthInfo, user)
	}

	want := `resources:
- ` + edgeSources + `
images:
- name: protem-agent
  newName: registry.example.com:5000/protem-agent
  newTag: v1.2.3
configMapGenerator:
- name: protem-agent
  namespace: protem-system
  literals:
  - cluster-name=c1
secretGenerator:
- name: protem-agent-hub
  namespace: protem-system
  options:
    disableNameSuffixHash: true
  files:
  - kubeconfig=hub.kubeconfig
`
	if view.Kustomization != want {
		t.Errorf("kustomization:\n%s\nwant:\n%s", view.Kustomization, want)
	}
	if len(k.Images) != 1 || k.Images[0].NewName != "registry.example.com:5000/protem-agent" || k.Images[0].NewTag != "v1.2.3" ||
		len(k.ConfigMapGenerator) != 1 || k.ConfigMapGenerator[0].Literals[0] != "cluster-name=c1" ||
		len(k.Resources) != 1 || k.Resources[0] != edgeSources {
		t.Errorf("unexpected kustomization %+v", k)
	}

	for _, obj := range []client.Object{&corev1.ServiceAccount{}} {
		if err := c.Get(context.Background(), agentAccount("c1"), obj); err != nil {
			t.Errorf("join did not provision the agent: %v", err)
		}
	}
}

func TestEnrol_ImageSplitAndMissingCA(t *testing.T) {
	for _, tc := range []struct{ image, name, tag string }{
		{"protem-agent:latest", "protem-agent", "latest"},
		{"registry.example.com:5000/team/protem-agent", "registry.example.com:5000/team/protem-agent", "latest"},
		{"ghcr.io/x/protem-agent:1.10", "ghcr.io/x/protem-agent", "1.10"},
	} {
		opt := enrolOptions()
		opt.AgentImage, opt.HubCAData = tc.image, nil
		s, _ := newMgmtServer(t, allowAll, opt, testCluster(nil, nil))
		w := call(s, http.MethodPost, "/api/clusters/c1/join", aliceEmail, "")
		if w.Code != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d: %s", tc.image, w.Code, w.Body.String())
		}
		view, k := decodeJoin(t, w.Body)
		if len(k.Images) != 1 || k.Images[0].NewName != tc.name || k.Images[0].NewTag != tc.tag {
			t.Errorf("%s: images %+v, want %s and %s", tc.image, k.Images, tc.name, tc.tag)
		}
		cfg, err := clientcmd.Load([]byte(view.Kubeconfig))
		if err != nil {
			t.Fatal(err)
		}
		if ca := cfg.Clusters["hub"].CertificateAuthorityData; len(ca) != 0 {
			t.Errorf("%s: expected no CA, got %q", tc.image, ca)
		}
	}
}

func TestEnrol_JoinAndReissueRefusals(t *testing.T) {
	argo := map[string]string{"argocd.argoproj.io/instance": "protem"}
	for _, route := range []string{"join", "reissue"} {
		noURL := enrolOptions()
		noURL.HubAPIURL = ""
		for _, tc := range []struct {
			name, cluster string
			opt           ServerOptions
			want          int
			says          string
		}{
			{"unknown", "nope", enrolOptions(), http.StatusNotFound, "nope"},
			{"hub", hubClusterName, enrolOptions(), http.StatusConflict, "hub"},
			{"no hub url", "c1", noURL, http.StatusConflict, "--hub-api-url"},
			{"managed by Argo CD", "c1", enrolOptions(), http.StatusOK, ""},
		} {
			s, c := newMgmtServer(t, allowAll, tc.opt, testCluster(argo, nil), hubCluster())
			w := call(s, http.MethodPost, "/api/clusters/"+tc.cluster+"/"+route, aliceEmail, "")
			if w.Code != tc.want || !strings.Contains(w.Body.String(), tc.says) {
				t.Errorf("%s %s: expected %d naming %q, got %d: %s", route, tc.name, tc.want, tc.says, w.Code, w.Body.String())
			}
			if tc.want != http.StatusOK && stateOf(t, c, agentAccount(tc.cluster), &corev1.ServiceAccount{}).exists {
				t.Errorf("%s %s: refused request created the agent ServiceAccount", route, tc.name)
			}
		}
	}
}

func TestEnrol_ReissueReplacesServiceAccount(t *testing.T) {
	old := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Namespace: fleet.ClusterNamespace("c1"), Name: fleet.AgentName, UID: "old-uid"}}
	s, c := newMgmtServer(t, allowAll, enrolOptions(), testCluster(nil, nil), old)
	w := call(s, http.MethodPost, "/api/clusters/c1/reissue", aliceEmail, "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	decodeJoin(t, w.Body)
	var sa corev1.ServiceAccount
	if err := c.Get(context.Background(), agentAccount("c1"), &sa); err != nil {
		t.Fatal(err)
	}
	if sa.UID == old.UID {
		t.Errorf("ServiceAccount still has UID %s; reissue must replace it", sa.UID)
	}
}

func TestEnrol_DeleteCluster(t *testing.T) {
	argo := map[string]string{"argocd.argoproj.io/instance": "protem"}
	for _, tc := range []struct {
		name    string
		cluster *protemv1alpha1.TargetCluster
		objs    []client.Object
		path    string
		want    int
		says    string
	}{
		{"unknown", testCluster(nil, nil), nil, "/api/clusters/nope", http.StatusNotFound, "nope"},
		{"active grant", testCluster(nil, nil), []client.Object{activeGrant("c1")}, "/api/clusters/c1", http.StatusConflict, "revoke"},
		{"managed", testCluster(argo, nil), nil, "/api/clusters/c1", http.StatusConflict, "Argo CD"},
		{"plain", testCluster(nil, nil), nil, "/api/clusters/c1", http.StatusNoContent, ""},
	} {
		s, c := newMgmtServer(t, allowAll, enrolOptions(), append(tc.objs, tc.cluster)...)
		w := call(s, http.MethodDelete, tc.path, aliceEmail, "")
		if w.Code != tc.want || !strings.Contains(w.Body.String(), tc.says) {
			t.Errorf("%s: expected %d naming %q, got %d: %s", tc.name, tc.want, tc.says, w.Code, w.Body.String())
		}
		if exists := stateOf(t, c, client.ObjectKey{Name: "c1"}, &protemv1alpha1.TargetCluster{}).exists; exists != (tc.want != http.StatusNoContent) {
			t.Errorf("%s: TargetCluster exists = %v after %d", tc.name, exists, w.Code)
		}
	}
}

func TestEnrol_WritesStampAndRecordEvents(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	opt := enrolOptions()
	opt.Recorder = recorder
	s, c := newMgmtServer(t, allowAll, opt, testCluster(nil, map[string]string{lastModKey: "mallory@company.com"}))
	for _, tc := range []struct {
		method, path, body, cluster, event string
		want                               int
	}{
		{http.MethodPost, "/api/clusters", `{"name":"edge","displayName":"Edge"}`, "edge", "ClusterAdded", http.StatusCreated},
		{http.MethodPost, "/api/clusters/c1/join", "", "c1", "JoinTokenIssued", http.StatusOK},
		{http.MethodPost, "/api/clusters/edge/reissue", "", "edge", "CredentialsReissued", http.StatusOK},
		{http.MethodDelete, "/api/clusters/c1", "", "", "ClusterRemoved", http.StatusNoContent},
	} {
		if w := call(s, tc.method, tc.path, bobEmail, tc.body); w.Code != tc.want {
			t.Fatalf("%s %s: expected %d, got %d: %s", tc.method, tc.path, tc.want, w.Code, w.Body.String())
		}
		if tc.cluster != "" {
			var stored protemv1alpha1.TargetCluster
			if err := c.Get(context.Background(), client.ObjectKey{Name: tc.cluster}, &stored); err != nil {
				t.Fatal(err)
			}
			if got := stored.Annotations[lastModKey]; got != bobEmail {
				t.Errorf("%s: expected last-modified-by %s, got %q", tc.event, bobEmail, got)
			}
		}
		select {
		case e := <-recorder.Events:
			if !strings.HasPrefix(e, "Normal "+tc.event+" ") || !strings.Contains(e, bobEmail) {
				t.Errorf("expected a Normal %s event naming bob, got %q", tc.event, e)
			}
		default:
			t.Errorf("expected a %s event", tc.event)
		}
	}
}

func TestEnrol_AgentState(t *testing.T) {
	ready := func(name string, status metav1.ConditionStatus, heartbeat bool) *protemv1alpha1.TargetCluster {
		c := &protemv1alpha1.TargetCluster{ObjectMeta: metav1.ObjectMeta{Name: name}}
		meta.SetStatusCondition(&c.Status.Conditions, metav1.Condition{Type: "Ready", Status: status, Reason: "Test"})
		if heartbeat {
			c.Status.LastHeartbeat = &metav1.Time{Time: time.Now()}
		}
		return c
	}
	hub := ready(hubClusterName, metav1.ConditionTrue, false)
	paused := ready("d-paused", metav1.ConditionTrue, true)
	paused.Spec.Paused = true
	s, _ := newMgmtServer(t, allowAll, enrolOptions(), hub, paused,
		ready("a-waiting", metav1.ConditionFalse, false),
		ready("b-connected", metav1.ConditionTrue, true),
		ready("c-lost", metav1.ConditionFalse, true))
	w := call(s, http.MethodGet, "/api/clusters", aliceEmail, "")
	var views []ClusterView
	_ = json.Unmarshal(w.Body.Bytes(), &views)
	got := map[string]string{}
	for _, v := range views {
		got[v.Name] = v.Agent
	}
	want := map[string]string{hubClusterName: "none", "a-waiting": "waiting", "b-connected": "connected", "c-lost": "lost", "d-paused": "connected"}
	if !mapsEqual(got, want) {
		t.Errorf("agent states = %v, want %v", got, want)
	}
}

func TestEnrol_CapabilitiesAndOptions(t *testing.T) {
	for _, tc := range []struct {
		mode   string
		policy sarPolicy
		want   bool
	}{
		{enabled, allowOnly(sarRule{aliceEmail, "create", "targetclusters", "", ""}), true},
		{enabled, allowOnly(sarRule{aliceEmail, "update", "targetclusters", "", ""}), false},
		{"readonly", allowAll, false},
	} {
		opt := enrolOptions()
		opt.UIManagement = tc.mode
		s, _ := newMgmtServer(t, tc.policy, opt)
		var caps CapabilitiesView
		_ = json.Unmarshal(call(s, http.MethodGet, "/api/capabilities", aliceEmail, "").Body.Bytes(), &caps)
		if caps.AddClusters != tc.want {
			t.Errorf("%s: addClusters = %v, want %v", tc.mode, caps.AddClusters, tc.want)
		}
	}

	c := fake.NewClientBuilder().WithScheme(setupTestScheme(t)).Build()
	for _, ttl := range []time.Duration{9 * time.Minute, 61 * time.Minute} {
		if _, err := NewServer(c, ServerOptions{JoinTokenTTL: ttl}); err == nil || !strings.Contains(err.Error(), "join-token-ttl") {
			t.Errorf("--join-token-ttl=%v: expected a startup error naming the flag, got %v", ttl, err)
		}
	}
	for _, ttl := range []time.Duration{0, 10 * time.Minute, time.Hour} {
		if _, err := NewServer(c, ServerOptions{JoinTokenTTL: ttl}); err != nil {
			t.Errorf("--join-token-ttl=%v: %v", ttl, err)
		}
	}
}
