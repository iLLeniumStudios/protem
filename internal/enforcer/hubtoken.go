package enforcer

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MinHubTokenTTL is the shortest lifetime the TokenRequest API accepts.
const MinHubTokenTTL = 10 * time.Minute

const (
	renewBackoffStart = 30 * time.Second
	renewBackoffMax   = 10 * time.Minute
	hubTokenKey       = "token"
)

// HubTokenRenewer keeps the agent's hub service-account token fresh through the TokenRequest API
// and hands the current token to every hub request.
type HubTokenRenewer struct {
	target         client.Client
	secret         client.ObjectKey
	ttl            time.Duration
	serviceAccount client.ObjectKey
	token          atomic.Pointer[string]
	issued, expiry time.Time
}

type tokenClaims struct {
	Subject  string `json:"sub"`
	IssuedAt int64  `json:"iat"`
	Expiry   int64  `json:"exp"`
}

func (c tokenClaims) times() (issued, expiry time.Time) {
	if c.IssuedAt != 0 {
		issued = time.Unix(c.IssuedAt, 0)
	}
	if c.Expiry != 0 {
		expiry = time.Unix(c.Expiry, 0)
	}
	return issued, expiry
}

// parseClaims reads a JWT's claims without verifying it, which is safe only for a token we hold.
func parseClaims(token string) (tokenClaims, error) {
	var claims tokenClaims
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return claims, errors.New("token is not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return claims, fmt.Errorf("decode token payload: %w", err)
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return claims, fmt.Errorf("decode token claims: %w", err)
	}
	return claims, nil
}

func serviceAccountFromClaims(claims tokenClaims) (client.ObjectKey, error) {
	qualified, ok := strings.CutPrefix(claims.Subject, "system:serviceaccount:")
	namespace, name, found := strings.Cut(qualified, ":")
	if !ok || !found || namespace == "" || name == "" {
		return client.ObjectKey{}, fmt.Errorf("token subject %q is not a service account", claims.Subject)
	}
	return client.ObjectKey{Namespace: namespace, Name: name}, nil
}

func serviceAccountOf(token string) (client.ObjectKey, error) {
	claims, err := parseClaims(token)
	if err != nil {
		return client.ObjectKey{}, err
	}
	return serviceAccountFromClaims(claims)
}

// renewDelay is how long to wait before renewing a token: until a third of its lifetime has
// passed, or not at all when its lifetime is unknown.
func renewDelay(issued, expiry, now time.Time) time.Duration {
	if issued.IsZero() || expiry.IsZero() {
		return 0
	}
	return max(issued.Add(expiry.Sub(issued)/3).Sub(now), 0)
}

func nextBackoff(prev time.Duration) time.Duration {
	if prev == 0 {
		return renewBackoffStart
	}
	return min(2*prev, renewBackoffMax)
}

// NewHubTokenRenewer switches hubCfg from its static service-account token to the renewer's
// current token. It returns nil, and leaves hubCfg alone, when hubCfg authenticates any other way.
// A non-zero secret on the target cluster supplies a stored token at start, if one is still valid
// and not older than hubCfg's, and receives every renewed token.
func NewHubTokenRenewer(ctx context.Context, hubCfg *rest.Config, target client.Client, secret client.ObjectKey, ttl time.Duration) (*HubTokenRenewer, error) {
	if hubCfg.BearerToken == "" || hubCfg.BearerTokenFile != "" || len(hubCfg.CertData) > 0 || hubCfg.CertFile != "" ||
		hubCfg.ExecProvider != nil || hubCfg.AuthProvider != nil || hubCfg.Username != "" {
		enforcerLog.Info("Hub token renewal is off: the hub kubeconfig does not authenticate with a token alone")
		return nil, nil
	}
	token := hubCfg.BearerToken
	if secret.Name != "" {
		stored, err := storedToken(ctx, target, secret)
		if err != nil {
			return nil, err
		}
		token = startToken(token, stored)
	}
	claims, err := parseClaims(token)
	if err != nil {
		enforcerLog.Info("Hub token renewal is off: the hub token is not a service-account token", "reason", err.Error())
		return nil, nil
	}
	serviceAccount, err := serviceAccountFromClaims(claims)
	if err != nil {
		enforcerLog.Info("Hub token renewal is off: the hub token is not a service-account token", "reason", err.Error())
		return nil, nil
	}

	r := &HubTokenRenewer{target: target, secret: secret, ttl: ttl, serviceAccount: serviceAccount}
	r.token.Store(&token)
	r.issued, r.expiry = claims.times()
	hubCfg.BearerToken = ""
	hubCfg.Wrap(r.WrapTransport)
	return r, nil
}

// startToken picks the token to start with: the stored one, unless the kubeconfig's was issued
// after it. Reissuing a cluster's credentials revokes the stored token and hands out a newer
// kubeconfig, while the Secret keeps the revoked token.
func startToken(fromKubeconfig, stored string) string {
	if stored == "" {
		return fromKubeconfig
	}
	configured, err := parseClaims(fromKubeconfig)
	if err != nil {
		return stored
	}
	held, _ := parseClaims(stored)
	if configured.IssuedAt > held.IssuedAt {
		return fromKubeconfig
	}
	return stored
}

// storedToken returns the Secret's token if it is a token that has not expired, else "".
func storedToken(ctx context.Context, target client.Client, key client.ObjectKey) (string, error) {
	var secret corev1.Secret
	if err := target.Get(ctx, key, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			return "", nil
		}
		return "", fmt.Errorf("read the stored hub token from Secret %s: %w", key, err)
	}
	token := string(secret.Data[hubTokenKey])
	if token == "" {
		return "", nil
	}
	claims, err := parseClaims(token)
	if err != nil {
		enforcerLog.Info("Ignoring the stored hub token", "secret", key.String(), "reason", err.Error())
		return "", nil
	}
	if _, expiry := claims.times(); !expiry.IsZero() && !time.Now().Before(expiry) {
		enforcerLog.Info("Ignoring the stored hub token: it has expired", "secret", key.String(), "expiredAt", expiry)
		return "", nil
	}
	return token, nil
}

// Token returns the token hub requests currently carry.
func (r *HubTokenRenewer) Token() string {
	return *r.token.Load()
}

// WrapTransport sets the current token on every request; use it as the hub rest.Config's WrapTransport.
func (r *HubTokenRenewer) WrapTransport(rt http.RoundTripper) http.RoundTripper {
	return roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req = req.Clone(req.Context())
		req.Header.Set("Authorization", "Bearer "+r.Token())
		return rt.RoundTrip(req)
	})
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// Run renews the token whenever a third of its lifetime has passed, retrying failures with backoff
// while the current token stays in use. hub must be built from the config NewHubTokenRenewer wrapped.
func (r *HubTokenRenewer) Run(ctx context.Context, hub client.Client) {
	var backoff time.Duration
	for {
		delay := renewDelay(r.issued, r.expiry, time.Now())
		if backoff > 0 {
			delay = backoff
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if err := r.renew(ctx, hub); err != nil {
			backoff = nextBackoff(backoff)
			enforcerLog.Error(err, "Failed to renew the hub token; keeping the current one", "retryIn", backoff.String())
			continue
		}
		backoff = 0
	}
}

func (r *HubTokenRenewer) renew(ctx context.Context, hub client.Client) error {
	seconds := int64(r.ttl / time.Second)
	req := &authenticationv1.TokenRequest{Spec: authenticationv1.TokenRequestSpec{ExpirationSeconds: &seconds}}
	sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Namespace: r.serviceAccount.Namespace, Name: r.serviceAccount.Name}}
	issued := time.Now()
	if err := hub.SubResource("token").Create(ctx, sa, req); err != nil {
		return fmt.Errorf("request a token for ServiceAccount %s: %w", r.serviceAccount, err)
	}
	token := req.Status.Token
	r.token.Store(&token)
	r.issued, r.expiry = issued, req.Status.ExpirationTimestamp.Time
	enforcerLog.Info("Renewed the hub token", "expiresAt", r.expiry)
	if r.secret.Name != "" {
		if err := r.store(ctx, token); err != nil {
			enforcerLog.Error(err, "Failed to store the renewed hub token; it is kept in memory", "secret", r.secret.String())
		}
	}
	return nil
}

func (r *HubTokenRenewer) store(ctx context.Context, token string) error {
	patch, err := json.Marshal(map[string]any{"data": map[string][]byte{hubTokenKey: []byte(token)}})
	if err != nil {
		return err
	}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: r.secret.Namespace, Name: r.secret.Name}}
	return r.target.Patch(ctx, secret, client.RawPatch(types.MergePatchType, patch))
}
