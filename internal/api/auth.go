package api

import (
	"cmp"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

const (
	aliceEmail        = "alice@company.com"
	bobEmail          = "bob@company.com"
	aliceUsername     = "alice"
	adminName         = "admin"
	devsGroup         = "devs"
	sreLeadsGroup     = "sre-leads"
	securityTeamGroup = "security-team"
	// devAdminsGroup is what hack/dev-personas-rbac.yaml binds; the "protem-dev:" prefix keeps it from matching a real IdP group.
	devAdminsGroup = "protem-dev:admins"
)

var (
	// ErrUnauthorized is returned when a request lacks valid authentication credentials.
	ErrUnauthorized = errors.New("unauthorized: missing or invalid credentials")
)

// Predefined personas for local development and testing
var DevPersonas = map[string]UserInfo{
	aliceEmail: {
		Username: aliceUsername,
		Email:    aliceEmail,
		Groups:   []string{devsGroup},
		Roles:    []string{"protem-user"},
	},
	bobEmail: {
		Username: "bob",
		Email:    bobEmail,
		Groups:   []string{sreLeadsGroup},
		Roles:    []string{"protem-approver"},
	},
	"admin@company.com": {
		Username: adminName,
		Email:    "admin@company.com",
		Groups:   []string{securityTeamGroup, sreLeadsGroup, devsGroup, devAdminsGroup},
		Roles:    []string{"protem-admin"},
	},
}

// TokenVerifier defines the interface for verifying raw JWT tokens.
type TokenVerifier interface {
	Verify(ctx context.Context, rawIDToken string) (*oidc.IDToken, error)
}

// Authenticator encapsulates token verification and optional dev auth.
type Authenticator struct {
	devAuth  bool
	verifier TokenVerifier
}

// NewAuthenticator creates an Authenticator.
func NewAuthenticator(devAuth bool, verifier TokenVerifier) *Authenticator {
	return &Authenticator{
		devAuth:  devAuth,
		verifier: verifier,
	}
}

// Authenticate extracts UserInfo from the request.
// If devAuth is true, permits test persona overrides via X-Test-User or ?as=.
// If devAuth is false, test headers are strictly ignored and a valid verified token is required.
func (a *Authenticator) Authenticate(r *http.Request) (UserInfo, error) {
	// 1. Dev auth check (only when explicitly enabled on loopback)
	if a.devAuth {
		testUser := r.Header.Get("X-Test-User")
		if testUser == "" {
			testUser = r.URL.Query().Get("as")
		}
		if testUser != "" {
			if persona, exists := DevPersonas[testUser]; exists {
				return persona, nil
			}
			return UserInfo{
				Username: strings.Split(testUser, "@")[0],
				Email:    testUser,
				Groups:   []string{devsGroup},
				Roles:    []string{"protem-user"},
			}, nil
		}
	}

	// 2. Bearer token check
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

		// If real cryptographic OIDC verifier is configured, verify signature via JWKS
		if a.verifier != nil {
			idToken, err := a.verifier.Verify(r.Context(), tokenStr)
			if err != nil {
				return UserInfo{}, fmt.Errorf("%w: %v", ErrUnauthorized, err)
			}

			var claims tokenClaims
			if err := idToken.Claims(&claims); err != nil {
				return UserInfo{}, ErrUnauthorized
			}
			return claims.user()
		}

		// If in devAuth mode without a remote OIDC provider configured, parse JWT payload with expiration check
		if a.devAuth {
			parts := strings.Split(tokenStr, ".")
			if len(parts) == 3 {
				payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
				if err == nil {
					var claims tokenClaims
					if err := json.Unmarshal(payloadBytes, &claims); err == nil {
						if claims.Exp > 0 && time.Now().Unix() > claims.Exp {
							return UserInfo{}, ErrUnauthorized
						}
						return claims.user()
					}
				}
			}
		}
	}

	return UserInfo{}, ErrUnauthorized
}

type tokenClaims struct {
	PreferredUsername string   `json:"preferred_username"`
	Email             string   `json:"email"`
	Sub               string   `json:"sub"`
	Exp               int64    `json:"exp"`
	Groups            []string `json:"groups"`
	RealmAccess       struct {
		Roles []string `json:"roles"`
	} `json:"realm_access"`
}

// systemPrefix marks identities Kubernetes reserves for itself (system:masters, system:serviceaccount:...).
// A token from an identity provider must never carry them: this user's name and groups go into
// SubjectAccessReviews and RBAC bindings.
const systemPrefix = "system:"

func (c tokenClaims) user() (UserInfo, error) {
	email := cmp.Or(c.Email, c.PreferredUsername, c.Sub)
	username := cmp.Or(c.PreferredUsername, c.Sub, c.Email)
	if email == "" || strings.HasPrefix(email, systemPrefix) || strings.HasPrefix(username, systemPrefix) {
		return UserInfo{}, ErrUnauthorized
	}
	groups := slices.DeleteFunc(slices.Clone(c.Groups), func(g string) bool { return strings.HasPrefix(g, systemPrefix) })
	return UserInfo{Username: username, Email: email, Groups: groups, Roles: c.RealmAccess.Roles}, nil
}
