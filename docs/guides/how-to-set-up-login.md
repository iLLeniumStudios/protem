# How to set up login

Put an identity-aware proxy in front of the hub UI so people sign in with your identity
provider, and the hub reads who they are from the OIDC token the proxy attaches.

The hub has no login page of its own. It accepts one thing: an OIDC ID token in
`Authorization: Bearer <token>`, which it verifies itself (issuer, audience, signature, expiry).
It trusts no header the proxy adds, only the signed token; someone who reaches the hub without
going through the proxy still needs a valid ID token for the same client. From the token it
reads the person's email and username (for the RBAC subject on target clusters) and their
`groups` (which profiles they may request or approve); `aud` must equal `--oidc-client-id`.
Logging in grants nothing by itself — what a person can request or approve comes from their
groups matched against each profile, and hub admin rights come from Kubernetes RBAC for that
user or those groups. For exactly which claims the hub reads and in what order, see
[Authentication](../reference/authentication.md).

## Deploy with oauth2-proxy

`config/sso` is `config/default` plus the `config/auth-proxy` component, which adds an
oauth2-proxy container to the manager pod and a Service named `protem-ui`.

1. Register an OIDC client with your identity provider. Redirect URI:
   `https://<your UI host>/oauth2/callback`. The client must return a refresh token. Microsoft
   Entra ID returns one only when the proxy requests the `offline_access` scope: add
   `--scope=openid email profile offline_access` to the proxy in
   `config/auth-proxy/manager_patch.yaml`, or set `authProxy.scope` with the Helm chart.
   Keycloak returns one without it, and can reject the scope with "Offline tokens not allowed"
   when the user or client lacks the `offline_access` role.
2. Set the three values in `config/auth-proxy/auth-proxy.env`:

   | Key | Value |
   |---|---|
   | `OIDC_ISSUER_URL` | the issuer, exactly as it appears in the token's `iss` |
   | `OIDC_CLIENT_ID` | the client ID; the hub checks the token's audience against it |
   | `PUBLIC_URL` | the URL people open, without a trailing slash |

3. Create the Secret the proxy reads:

   ```sh
   kubectl -n protem-system create secret generic oauth2-proxy \
     --from-literal=client-secret='<client secret>' \
     --from-literal=cookie-secret="$(openssl rand -base64 32 | tr -- '+/' '-_')"
   ```

4. Deploy, and expose the Service over HTTPS (see [HTTPS](#https)):

   ```sh
   kubectl apply -k config/sso
   ```

The settings in `config/auth-proxy/manager_patch.yaml` that matter:

| Flag | Why |
|---|---|
| `--pass-authorization-header=true` | sends the ID token to the hub |
| `--api-route=^/api/` | API calls get 401 instead of a redirect, so the UI shows "You're signed out" |
| `--cookie-refresh=4m` | refreshes the ID token before it expires; set it below your provider's ID token lifetime |
| `--cookie-secure=true` | the session cookie is sent over HTTPS only |

The hub flag `--sign-out-url=/oauth2/sign_out` adds a Sign out link to the UI. That URL ends
only the proxy session: while the person is still logged in at the identity provider, the next
page load signs them straight back in. To end both, point the flag at the proxy's sign-out with
a redirect to the provider's logout endpoint, and allow that host in the proxy:

```
--sign-out-url=/oauth2/sign_out?rd=https%3A%2F%2Fidp.example.com%2Frealms%2Fexample%2Fprotocol%2Fopenid-connect%2Flogout
--whitelist-domain=idp.example.com        # oauth2-proxy
```

The UI polls several API routes at once, so when the cookie is due for a refresh the proxy
can send several refresh requests in parallel. If your provider rotates refresh tokens and
rejects a reused one, use oauth2-proxy's Redis session store, which serialises refreshes.

### HTTPS

The `config/ingress` component adds an Ingress for the `protem-ui` Service with TLS. Add it in
your overlay, set your host in place of `protem.example.com`, and provide the certificate:

```yaml
resources:
- <path to>/config/sso
components:
- <path to>/config/ingress
patches:
- target: {kind: Ingress, name: protem-ui}
  patch: |-
    - {op: replace, path: /spec/rules/0/host, value: protem.company.com}
    - {op: replace, path: /spec/tls/0/hosts/0, value: protem.company.com}
```

The certificate goes in the Secret `protem-ui-tls` in `protem-system`
(`kubectl -n protem-system create secret tls protem-ui-tls --cert=... --key=...`), or let
cert-manager issue it by uncommenting the annotation in `config/ingress/ingress.yaml`. `PUBLIC_URL`
must be `https://` and that host.

`config/sso` also installs NetworkPolicies that let traffic reach the manager pod only on the
proxy port, plus metrics from namespaces labelled `metrics: enabled`.

## Who can log in

- **In the identity provider.** Restrict the client to the people or groups who should reach
  the UI at all. Conditional access and MFA apply as for any other application.
- **In oauth2-proxy.** Optionally add `--allowed-group=<group>` or replace `--email-domain=*`
  with your domain.

## Microsoft Entra ID

See [How to set up login with Microsoft Entra ID](how-to-set-up-login-with-entra-id.md).

## Other sources: Dex

GitHub, LDAP and SAML do not issue OIDC ID tokens. Put [Dex](https://dexidp.io) between them
and oauth2-proxy: Dex is the issuer both the proxy and the hub point at.

## Requests created directly, not from the UI

A request created with `kubectl` or a pipeline, rather than through the UI, needs its own
requester identity enforced at the API server. See
[How to enforce requester identity for kubectl requests](how-to-enforce-requester-identity-for-kubectl-requests.md).

## Local development

`--dev-auth` replaces login with built-in personas chosen in the UI. The server refuses to start
with it unless `--api-bind-address` is a loopback address. `hack/dev-personas-rbac.yaml` gives
the admin persona hub admin rights.

## If login doesn't work

See [How to fix "You're signed out" after login](../operations/runbooks/how-to-fix-signed-out-after-login.md).
