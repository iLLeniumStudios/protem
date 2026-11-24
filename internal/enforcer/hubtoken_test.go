package enforcer

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
)

func unsignedToken(claims map[string]any) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256"}`))
	payload, _ := json.Marshal(claims)
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}

func TestRenewDelay(t *testing.T) {
	issued := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	expiry := issued.Add(24 * time.Hour)
	for _, tc := range []struct {
		name           string
		issued, expiry time.Time
		now            time.Time
		want           time.Duration
	}{
		{"before one third of the lifetime", issued, expiry, issued.Add(time.Hour), 7 * time.Hour},
		{"at one third", issued, expiry, issued.Add(8 * time.Hour), 0},
		{"after one third", issued, expiry, issued.Add(20 * time.Hour), 0},
		{"already expired", issued, expiry, issued.Add(25 * time.Hour), 0},
		{"no exp", issued, time.Time{}, issued.Add(time.Minute), 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := renewDelay(tc.issued, tc.expiry, tc.now); got != tc.want {
				t.Errorf("renewDelay = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestServiceAccountOfReadsTheSubject(t *testing.T) {
	got, err := serviceAccountOf(unsignedToken(map[string]any{"sub": "system:serviceaccount:protem-cluster-a:protem-agent"}))
	if err != nil {
		t.Fatal(err)
	}
	if want := (client.ObjectKey{Namespace: "protem-cluster-a", Name: "protem-agent"}); got != want {
		t.Errorf("serviceAccountOf = %v, want %v", got, want)
	}
}

func TestServiceAccountOfRejectsMalformedTokens(t *testing.T) {
	notJSON := "e30." + base64.RawURLEncoding.EncodeToString([]byte("not json")) + ".sig"
	for _, token := range []string{
		"",
		"opaque-static-token",
		"a.b",
		"a.!!!.c",
		notJSON,
		unsignedToken(map[string]any{"sub": "alice@example.com"}),
		unsignedToken(map[string]any{"sub": "system:serviceaccount:only-namespace"}),
		unsignedToken(map[string]any{"sub": "system:serviceaccount::name"}),
	} {
		if got, err := serviceAccountOf(token); err == nil {
			t.Errorf("serviceAccountOf(%q) = %v, want an error", token, got)
		}
	}
}

func TestWrapTransportSendsTheSwappedToken(t *testing.T) {
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("Authorization"))
	}))
	defer server.Close()
	r := &HubTokenRenewer{}
	r.token.Store(new("first"))
	httpClient := &http.Client{Transport: r.WrapTransport(http.DefaultTransport)}

	for _, next := range []string{"second", ""} {
		resp, err := httpClient.Get(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if next != "" {
			r.token.Store(&next)
		}
	}

	if len(seen) != 2 || seen[0] != "Bearer first" || seen[1] != "Bearer second" {
		t.Errorf("Authorization headers = %q, want [Bearer first, Bearer second]", seen)
	}
}

func TestNextBackoffGrowsAndCaps(t *testing.T) {
	got := make([]time.Duration, 0, 7)
	backoff := time.Duration(0)
	for range 7 {
		backoff = nextBackoff(backoff)
		got = append(got, backoff)
	}
	want := []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 10 * time.Minute, 10 * time.Minute}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("backoff sequence = %v, want %v", got, want)
		}
	}
}

func TestNewHubTokenRenewerIsOffWithoutAStaticToken(t *testing.T) {
	token := unsignedToken(map[string]any{"sub": "system:serviceaccount:ns:protem-agent"})
	for name, cfg := range map[string]*rest.Config{
		"client certificate":          {Host: "https://hub", BearerToken: token, TLSClientConfig: rest.TLSClientConfig{CertData: []byte("cert"), KeyData: []byte("key")}},
		"exec plugin":                 {Host: "https://hub", BearerToken: token, ExecProvider: &clientcmdapi.ExecConfig{Command: "get-token"}},
		"token file":                  {Host: "https://hub", BearerToken: token, BearerTokenFile: "/var/run/token"},
		"no credentials":              {Host: "https://hub"},
		"not a service account token": {Host: "https://hub", BearerToken: "opaque-static-token"},
	} {
		t.Run(name, func(t *testing.T) {
			before := *cfg
			r, err := NewHubTokenRenewer(t.Context(), cfg, nil, client.ObjectKey{}, time.Hour)
			if err != nil || r != nil {
				t.Fatalf("NewHubTokenRenewer = %v, %v, want nil, nil", r, err)
			}
			if cfg.BearerToken != before.BearerToken || cfg.WrapTransport != nil {
				t.Error("the hub config was changed although renewal is off")
			}
		})
	}
}

const hubTokenSecret = "protem-agent-hub"

func (e *manifestEnv) mintAgentToken(t *testing.T, ttl time.Duration) string {
	t.Helper()
	seconds := int64(ttl / time.Second)
	req := &authenticationv1.TokenRequest{Spec: authenticationv1.TokenRequestSpec{ExpirationSeconds: &seconds}}
	sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Namespace: e.hubNS, Name: "protem-agent"}}
	if err := e.admin.SubResource("token").Create(t.Context(), sa, req); err != nil {
		t.Fatalf("mint token for %s/protem-agent: %v", e.hubNS, err)
	}
	return req.Status.Token
}

func (e *manifestEnv) tokenConfig(token string) *rest.Config {
	return &rest.Config{Host: e.cfg.Host, TLSClientConfig: rest.TLSClientConfig{CAData: e.cfg.CAData}, BearerToken: token}
}

func (e *manifestEnv) tokenClient(t *testing.T, cfg *rest.Config) client.Client {
	t.Helper()
	c, err := client.New(cfg, client.Options{Scheme: e.scheme})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (e *manifestEnv) setHubSecret(t *testing.T, data map[string][]byte) {
	t.Helper()
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: agentSpokeNS, Name: hubTokenSecret}}
	err := e.admin.Get(t.Context(), client.ObjectKeyFromObject(secret), secret)
	switch {
	case apierrors.IsNotFound(err):
		secret.Data = data
		err = e.admin.Create(t.Context(), secret)
	case err == nil:
		secret.Data = data
		err = e.admin.Update(t.Context(), secret)
	}
	if err != nil {
		t.Fatal(err)
	}
}

func (e *manifestEnv) hubSecretKey() client.ObjectKey {
	return client.ObjectKey{Namespace: agentSpokeNS, Name: hubTokenSecret}
}

func (e *manifestEnv) assertHubTokenRenewed(t *testing.T) {
	ctx := t.Context()
	kubeconfig := []byte("apiVersion: v1\nkind: Config\n")
	e.setHubSecret(t, map[string][]byte{"kubeconfig": kubeconfig})
	first := e.mintAgentToken(t, 10*time.Minute)
	hubCfg := e.tokenConfig(first)

	r, err := NewHubTokenRenewer(ctx, hubCfg, e.target, e.hubSecretKey(), time.Hour)
	if err != nil || r == nil {
		t.Fatalf("NewHubTokenRenewer = %v, %v, want a renewer", r, err)
	}
	if hubCfg.BearerToken != "" {
		t.Error("the hub config still carries a static bearer token")
	}
	hub := e.tokenClient(t, hubCfg)
	if err := r.renew(ctx, hub); err != nil {
		t.Fatalf("renew: %v", err)
	}

	renewed := r.Token()
	if renewed == first || renewed == "" {
		t.Fatal("the token was not replaced")
	}
	if sa, err := serviceAccountOf(renewed); err != nil || sa != (client.ObjectKey{Namespace: e.hubNS, Name: "protem-agent"}) {
		t.Errorf("renewed token is for %v (%v), want %s/protem-agent", sa, err, e.hubNS)
	}
	if lifetime := time.Until(r.expiry); lifetime < 50*time.Minute || lifetime > time.Hour {
		t.Errorf("renewed token expires in %v, want about the requested hour", lifetime)
	}
	onlyRenewed := e.tokenClient(t, e.tokenConfig(renewed))
	if err := onlyRenewed.List(ctx, &protemv1alpha1.ClusterGrantList{}, client.InNamespace(e.hubNS)); err != nil {
		t.Errorf("listing ClusterGrants with the renewed token: %v", err)
	}
	if err := hub.List(ctx, &protemv1alpha1.ClusterGrantList{}, client.InNamespace(e.hubNS)); err != nil {
		t.Errorf("listing ClusterGrants through the renewing hub client: %v", err)
	}

	var secret corev1.Secret
	if err := e.admin.Get(ctx, e.hubSecretKey(), &secret); err != nil {
		t.Fatal(err)
	}
	if string(secret.Data["token"]) != renewed {
		t.Error("the renewed token was not written to the Secret's token key")
	}
	if string(secret.Data["kubeconfig"]) != string(kubeconfig) {
		t.Errorf("the Secret's kubeconfig key changed to %q", secret.Data["kubeconfig"])
	}
}

func (e *manifestEnv) assertHubTokenOnlyForItsOwnServiceAccount(t *testing.T) {
	ctx := t.Context()
	other := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Namespace: e.hubNS, Name: "someone-else"}}
	if err := e.admin.Create(ctx, other); err != nil {
		t.Fatal(err)
	}
	hub := e.tokenClient(t, e.tokenConfig(e.mintAgentToken(t, 10*time.Minute)))
	seconds := int64(600)
	req := &authenticationv1.TokenRequest{Spec: authenticationv1.TokenRequestSpec{ExpirationSeconds: &seconds}}
	if err := hub.SubResource("token").Create(ctx, other, req); !apierrors.IsForbidden(err) {
		t.Errorf("hub identity requesting a token for %s: err = %v, want forbidden", other.Name, err)
	}
}

func (e *manifestEnv) assertTargetIdentityConfinedToHubSecret(t *testing.T) {
	ctx := t.Context()
	other := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: agentSpokeNS, Name: "unrelated"}, Data: map[string][]byte{"k": []byte("v")}}
	if err := e.admin.Create(ctx, other); err != nil {
		t.Fatal(err)
	}
	if err := e.target.Get(ctx, client.ObjectKeyFromObject(other), &corev1.Secret{}); !apierrors.IsForbidden(err) {
		t.Errorf("target identity reading Secret %s: err = %v, want forbidden", other.Name, err)
	}
	if err := e.target.List(ctx, &corev1.SecretList{}, client.InNamespace(agentSpokeNS)); !apierrors.IsForbidden(err) {
		t.Errorf("target identity listing Secrets: err = %v, want forbidden", err)
	}
	patch := client.RawPatch(types.MergePatchType, []byte(`{"data":{"k":"eA=="}}`))
	if err := e.target.Patch(ctx, other, patch); !apierrors.IsForbidden(err) {
		t.Errorf("target identity patching Secret %s: err = %v, want forbidden", other.Name, err)
	}
}

func (e *manifestEnv) assertStoredHubTokenPreferredUnlessExpired(t *testing.T) {
	ctx := t.Context()
	fromKubeconfig := e.mintAgentToken(t, 10*time.Minute)
	stored := e.mintAgentToken(t, 20*time.Minute)
	expired := unsignedToken(map[string]any{
		"sub": "system:serviceaccount:" + e.hubNS + ":protem-agent",
		"iat": time.Now().Add(-2 * time.Hour).Unix(),
		"exp": time.Now().Add(-time.Hour).Unix(),
	})
	for _, tc := range []struct {
		name, stored, want string
	}{
		{"valid stored token is used", stored, stored},
		{"expired stored token is ignored", expired, fromKubeconfig},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e.setHubSecret(t, map[string][]byte{"kubeconfig": []byte("x"), "token": []byte(tc.stored)})
			r, err := NewHubTokenRenewer(ctx, e.tokenConfig(fromKubeconfig), e.target, e.hubSecretKey(), time.Hour)
			if err != nil || r == nil {
				t.Fatalf("NewHubTokenRenewer = %v, %v, want a renewer", r, err)
			}
			if r.Token() != tc.want {
				t.Error("the renewer started with the wrong token")
			}
		})
	}
}

// Reissuing a cluster's credentials revokes the stored token and hands out a kubeconfig with a
// newer one. The Secret still holds the old token, because kubectl apply keeps keys it never set.
func TestStartTokenPrefersAKubeconfigIssuedAfterTheStoredToken(t *testing.T) {
	at := func(minutes int) string {
		issued := time.Date(2026, 1, 1, 0, minutes, 0, 0, time.UTC)
		return unsignedToken(map[string]any{"iat": issued.Unix(), "exp": issued.Add(time.Hour).Unix(), "n": minutes})
	}
	for _, tc := range []struct {
		name, fromKubeconfig, stored, want string
	}{
		{"nothing stored", at(0), "", at(0)},
		{"stored token renewed after the kubeconfig was issued", at(0), at(5), at(5)},
		{"issued in the same second", at(5), at(5), at(5)},
		{"kubeconfig reissued after the stored token", at(9), at(5), at(9)},
		{"kubeconfig token of unknown age", "opaque", at(5), at(5)},
	} {
		if got := startToken(tc.fromKubeconfig, tc.stored); got != tc.want {
			t.Errorf("%s: started with the wrong token", tc.name)
		}
	}
}
