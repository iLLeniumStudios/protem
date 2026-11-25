# Authentication

What the hub accepts as proof of who a person is, the claims it reads from it, the settings that control it, and the identity providers it has been used with. Steps are in [How to set up login](../guides/how-to-set-up-login.md).

## Token

| Property | Value |
|---|---|
| Accepted credential | An OIDC ID token in the request header `Authorization: Bearer <token>` |
| Who sends it | An identity-aware proxy in front of the hub; the chart's and `config/auth-proxy`'s oauth2-proxy sends it with `--pass-authorization-header=true` |
| Verification | Signature against the keys in the issuer's discovery document, issuer equal to `--oidc-issuer-url`, audience containing `--oidc-client-id`, not expired |
| Discovery | Fetched from `--oidc-issuer-url` when the operator starts; the operator exits if it cannot be fetched or its `issuer` differs from the flag |
| Without `--oidc-issuer-url` and `--oidc-client-id` | No token is accepted; every API request gets 401, unless `--dev-auth` is on |
| Other request headers | Not read. Headers such as `X-Forwarded-User` or `X-Forwarded-Email` have no effect; `X-Test-User` and `?as=` are read only with `--dev-auth` |

Verification is done by [go-oidc](https://github.com/coreos/go-oidc) (`internal/api/server.go`); the claims below are read in `internal/api/auth.go`.

## Claims

| Claim | Read as | Order of preference | Notes |
|---|---|---|---|
| `email` | email, username | 1st for email, 2nd for username | Before 0.1.6 it came after `sub` for the username |
| `preferred_username` | username, email | 1st for username, 2nd for email | |
| `sub` | username, email | 3rd for both | |
| `groups` | groups | only source of groups | A JSON array of strings. Entries starting with `system:` are dropped |
| `realm_access.roles` | roles | only source of roles | Keycloak's realm roles. Shown in `GET /api/me`; no effect on what a person may request, approve or administer |
| `exp` | expiry | | Checked during verification |

Every other claim is ignored, including a top-level `roles` claim (for example Microsoft Entra ID app roles), `upn`, `oid`, `name`, and the group overage claims `_claim_names` and `hasgroups`.

The token is rejected with 401 when:

- verification fails (signature, issuer, audience or expiry);
- `email`, `preferred_username` and `sub` are all empty;
- the resolved email or username starts with `system:`.

## How the identity is used

| Use | Input | Match |
|---|---|---|
| Who may request a profile | groups, email, username | Any group equal to an entry of `AccessProfile.spec.requesters.groups`, or email or username equal to an entry of `requesters.users` |
| Who may approve | groups, email, username | The same match against a rule's `approvers.groups` and `approvers.users` |
| Hub admin rights | subject, groups | A SubjectAccessReview on the hub with the user from `--subject-template` and the token's groups |
| Binding on a target cluster | email, username | The RBAC `User` subject from the cluster's subject template |

Group matching is exact string comparison: a profile that lists an object ID matches only a token that carries that object ID.

## Settings

### Operator flags

Each flag can also be set with its environment variable; a flag on the command line wins. All flags are in [Operator flags](operator-flags.md).

| Flag | Environment variable | Default | Effect |
|---|---|---|---|
| `--oidc-issuer-url` | `PROTEM_OIDC_ISSUER_URL` | empty | Issuer whose ID tokens are accepted |
| `--oidc-client-id` | `PROTEM_OIDC_CLIENT_ID` | empty | Audience the ID token must carry |
| `--subject-template` | `PROTEM_SUBJECT_TEMPLATE` | `{{.Email}}` | Subject for bindings on the hub cluster and for the hub's SubjectAccessReviews |
| `--sign-out-url` | `PROTEM_SIGN_OUT_URL` | empty | URL of the UI's Sign out link; empty hides the link |
| `--dev-auth` | `PROTEM_DEV_AUTH` | `false` | Built-in personas instead of login; refused unless `--api-bind-address` is a loopback address |
| `--api-bind-address` | `PROTEM_API_BIND_ADDRESS` | `:8080` | Address of the UI and API server |

### Helm values

All values are in [Chart values](chart-values.md).

| Value | Default | Effect |
|---|---|---|
| `oidc.issuerURL` | `""` | Sets `--oidc-issuer-url` |
| `oidc.clientID` | `""` | Sets `--oidc-client-id` |
| `subjectTemplate` | `""` | Sets `--subject-template`; empty keeps the operator's default |
| `signOutURL` | `""` | Sets `--sign-out-url`; empty uses `/oauth2/sign_out` when `authProxy.enabled` |
| `authProxy.enabled` | `false` | Adds an oauth2-proxy container on port 4180, binds the operator to `127.0.0.1:8080`, and points the `-ui` Service at the proxy |
| `authProxy.image` | `quay.io/oauth2-proxy/oauth2-proxy:v7.12.0` | oauth2-proxy image |
| `authProxy.issuerURL` | `""` | Proxy's `--oidc-issuer-url`; defaults to `oidc.issuerURL` |
| `authProxy.clientID` | `""` | Proxy's `--client-id`; defaults to `oidc.clientID` |
| `authProxy.publicURL` | `""` | Proxy's `--redirect-url` is `<publicURL>/oauth2/callback` |
| `authProxy.existingSecret` | `""` | Secret with the keys `client-secret` and `cookie-secret`, passed as `OAUTH2_PROXY_CLIENT_SECRET` and `OAUTH2_PROXY_COOKIE_SECRET` |
| `authProxy.extraArgs` | `[]` | Further oauth2-proxy arguments, added after the fixed ones |

The fixed oauth2-proxy arguments the chart sets: `--provider=oidc`, `--http-address=0.0.0.0:4180`, `--upstream=http://127.0.0.1:8080`, `--pass-authorization-header=true`, `--api-route=^/api/`, `--skip-provider-button=true`, `--code-challenge-method=S256`, `--cookie-refresh=4m`, `--cookie-secure=true`, `--cookie-samesite=lax`, `--email-domain=*`. `config/auth-proxy/manager_patch.yaml` sets the same arguments for a kustomize install.

### Subject template

| Token | Replaced with |
|---|---|
| `{{.Email}}` | the resolved email |
| `{{.Username}}` | the resolved username |

The two tokens are replaced as literal text, not evaluated as a Go template; other text is kept, so `oidc:{{.Email}}` gives `oidc:alice@example.com`. A template containing neither token, or one that resolves to an empty string, gives the email, or the username when the email is empty.

| Where | Applies to | Default |
|---|---|---|
| `--subject-template` on the operator | the hub cluster's own bindings and SubjectAccessReviews | `{{.Email}}` |
| `TargetCluster.spec.subject` | bindings on that cluster | `{{.Email}}`; when empty, the operator's `--subject-template` |
| `--subject-template` on the agent | bindings on its cluster, only for a ClusterGrant that carries no subject | `{{.Email}}` |

## Not provided

- A login page. Sign-in happens at the identity provider, through the proxy.
- SAML, LDAP, or OAuth2 without OIDC ID tokens. These reach the hub only through an OIDC issuer in between, such as [Dex](https://dexidp.io/docs/connectors/).
- Local users or passwords. The only built-in identities are the development personas of `--dev-auth`, which is refused on a non-loopback address.
- API tokens of its own. Scripts and pipelines create requests with `kubectl`; see [How to enforce requester identity for kubectl requests](../guides/how-to-enforce-requester-identity-for-kubectl-requests.md).
- A lookup of groups outside the token. A person whose ID token has no `groups` claim has no groups in Protem.

## Identity providers

| Provider | How groups reach the ID token | What to watch for | oauth2-proxy provider | Status |
|---|---|---|---|---|
| Generic OIDC issuer | `groups` claim, as the issuer sends it | `aud` must contain the client ID | `oidc` (chart default) | Run end to end |
| Microsoft Entra ID | Groups claim under **Token configuration**; [object IDs](https://learn.microsoft.com/en-us/entra/identity/hybrid/connect/how-to-connect-fed-group-claims) | Over [200 groups](https://learn.microsoft.com/en-us/security/zero-trust/develop/configure-tokens-group-claims-app-roles#group-overages) gives no `groups` claim; [app roles](https://learn.microsoft.com/en-us/entra/identity-platform/howto-add-app-roles-in-apps) arrive in `roles`, which is not read | `oidc`; [`entra-id`](https://oauth2-proxy.github.io/oauth2-proxy/configuration/providers/ms_entra_id) also exists | From the provider's documentation, not run |
| Keycloak | [Group Membership mapper](https://oauth2-proxy.github.io/oauth2-proxy/configuration/providers/keycloak_oidc) with claim name `groups`; names, prefixed with `/` when "Full group path" is on | A client's ID token already carries `aud` equal to its own client ID, so no audience mapper is needed; realm roles do not grant anything; `offline_access` needs the role of that name, which new realms give everyone by default | `oidc` or `keycloak-oidc` | Run end to end, see [How to set up login with Keycloak](../guides/how-to-set-up-login-with-keycloak.md) |
| Okta | [Groups claim filter](https://developer.okta.com/docs/guides/customize-tokens-groups-claim/main/) on the app or authorization server; names | Okta's examples request the `groups` scope | [`oidc`](https://oauth2-proxy.github.io/oauth2-proxy/configuration/providers/openid_connect) | From the provider's documentation, not run |
| Google | None: the [ID token](https://developers.google.com/identity/openid-connect/openid-connect#an-id-tokens-payload) has no `groups` claim | Profiles must name people in `users` by email | [`google`](https://oauth2-proxy.github.io/oauth2-proxy/configuration/providers/google); `--google-group` only restricts login | From the provider's documentation, not run |
| GitHub, LDAP or SAML through Dex | Dex `groups` claim: GitHub [`org:team`](https://dexidp.io/docs/connectors/github/), LDAP [`groupSearch`](https://dexidp.io/docs/connectors/ldap/), SAML [`groupsAttr`](https://dexidp.io/docs/connectors/saml/) | Request the `groups` scope; Dex marks its SAML connector unmaintained | [`oidc`](https://oauth2-proxy.github.io/oauth2-proxy/configuration/providers/openid_connect) | From the provider's documentation, not run |

The chart's oauth2-proxy requests the scopes `openid email profile` unless `--scope` is set in `authProxy.extraArgs` ([`providers/oidc.go`](https://github.com/oauth2-proxy/oauth2-proxy/blob/v7.12.0/providers/oidc.go)). For Microsoft Entra ID, see [How to set up login with Microsoft Entra ID](../guides/how-to-set-up-login-with-entra-id.md).
