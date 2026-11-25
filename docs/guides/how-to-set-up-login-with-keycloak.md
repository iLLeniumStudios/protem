# How to set up login with Keycloak

Sign people in to a hub installed with the Helm chart through Keycloak, and make the name
Protem binds on a target cluster match the name that cluster's API server gives the same person
when they use `kubectl` with Keycloak.

**Tested:** every command here was run on 2026-11-25. The setup was Keycloak 24.0.5 with a
certificate from a private CA, the chart with its bundled oauth2-proxy v7.12.0, and the hub's
own cluster as the target cluster: k3s v1.35.5 in k3d, with the API server's `--oidc-*` flags.
The guide was first run on chart 0.1.3; the private CA setting and setting B with requests
created by `kubectl` were run again on 0.1.4. A script played the browser with `curl` for the
hub sign-in and fetched the tokens for `kubectl`, and the UI was reached through a port-forward
at `http://localhost` rather than an Ingress. Not tested:

- a separate target cluster with an agent;
- Keycloak 25 or later;
- the API server's structured authentication configuration;
- using one Keycloak client for both the hub and `kubectl`.

## Create the client for the hub

These steps use Keycloak's admin CLI, `kcadm.sh`, from Keycloak's `bin` directory.

1. Log in to the admin CLI:

   ```sh
   kcadm.sh config credentials --server https://<keycloak host> --realm master --user <admin user> --password '<admin password>'
   ```

2. Create a confidential client with the standard flow. The redirect URI is the path oauth2-proxy
   redeems, `<authProxy.publicURL>/oauth2/callback`, and must match exactly:

   ```sh
   kcadm.sh create clients -r <realm> -s clientId=protem -s protocol=openid-connect -s publicClient=false -s standardFlowEnabled=true -s directAccessGrantsEnabled=false -s 'redirectUris=["https://<your UI host>/oauth2/callback"]'
   ```

3. Put the person's groups in the ID token's `groups` claim with a Group Membership mapper:

   ```sh
   CID=$(kcadm.sh get clients -r <realm> -q clientId=protem --fields id | jq -r '.[0].id')
   kcadm.sh create clients/$CID/protocol-mappers/models -r <realm> -s name=groups -s protocol=openid-connect -s protocolMapper=oidc-group-membership-mapper -s 'config."claim.name"=groups' -s 'config."full.path"=false' -s 'config."id.token.claim"=true' -s 'config."access.token.claim"=true' -s 'config."userinfo.token.claim"=true'
   ```

   With `full.path` set to `false`, the claim carries group names, such as `devs`. With
   `true` (the **Full group path** switch in the admin console), it carries paths, such as
   `/devs`. Profiles and RBAC must use the form the token carries.

4. Record the client secret:

   ```sh
   kcadm.sh get clients/$CID/client-secret -r <realm>
   ```

No audience mapper is needed. Keycloak's ID token for this client already has `aud` set to
`protem`, which is what the hub checks against `oidc.clientID`.

The hub takes the email from the `email` claim and the username from `preferred_username` (see
[Claims](../reference/authentication.md#claims)). Keycloak's default `email` and `profile` client
scopes put both in the ID token. It puts no `realm_access` claim there, so `GET /api/me` shows
`"roles":null`; roles grant nothing in Protem anyway.

## Install or upgrade Protem

Create the namespace and the Secret with the client secret and a cookie secret:

```sh
kubectl create namespace protem-system
kubectl -n protem-system create secret generic oauth2-proxy \
  --from-literal=client-secret='<client secret>' \
  --from-literal=cookie-secret="$(openssl rand -base64 32 | tr -- '+/' '-_')"
```

Write `values.yaml`. Set `subjectTemplate` from the table in
[Line names up with the target clusters](#line-names-up-with-the-target-clusters):

```yaml
oidc:
  issuerURL: https://<keycloak host>/realms/<realm>
  clientID: protem
subjectTemplate: '{{.Email}}'
hubCluster:
  labels:
    env: dev
authProxy:
  enabled: true
  publicURL: https://<your UI host>
  existingSecret: oauth2-proxy
uiManagement: enabled
```

Install or upgrade:

```sh
helm upgrade --install protem oci://ghcr.io/illeniumstudios/charts/protem \
  --namespace protem-system -f values.yaml
```

To serve the UI at its own host over HTTPS, add the `ingress` values from
[How to install the hub with Helm](how-to-install-with-helm.md#example-login-with-the-bundled-auth-proxy).

Leave `authProxy.scope` unset. Keycloak returns a refresh token with the default scopes, so the
proxy renews the session. A person added to a group in Keycloak had that group in the hub about
4 minutes 40 seconds later, without signing in again. Adding `offline_access` to the scope works
only while the person and the client hold Keycloak's `offline_access` role. New realms give it
to everyone through the realm's default roles. Without it, sign-in ends on an error page from
the proxy, and Keycloak logs `reason="Offline tokens not allowed for the user or client"`.

### If Keycloak's certificate is from a private CA

With a certificate the hub does not trust, the operator and oauth2-proxy both fail at startup
with `x509: certificate signed by unknown authority`, and the pod stays in `CrashLoopBackOff`.
From version 0.1.4 the chart can add your CA to both:

1. Put the CA certificate, in PEM, in a ConfigMap in the release namespace:

   ```sh
   kubectl -n protem-system create configmap keycloak-ca --from-file=ca.crt=<CA file>
   ```

2. Name it in `values.yaml` and upgrade:

   ```yaml
   oidc:
     caConfigMap: keycloak-ca
   ```

The certificates are added to the public CAs the images already trust, not used instead of them.

On 0.1.4 against Keycloak: without the setting both containers crashed with the error above;
with it both started, and a person signed in through the proxy. Chart 0.1.3 has no such setting.

## Use Keycloak groups in profiles

`requesters.groups` and `approvers.groups` take the group names exactly as the `groups` claim
carries them, without the API server's groups prefix:

```yaml
apiVersion: protem.dev/v1alpha1
kind: AccessProfile
metadata:
  name: team-a-edit
  namespace: default
spec:
  displayName: Team A edit
  role:
    kind: ClusterRole
    name: edit
  scope: Namespaced
  namespaces:
  - team-a
  clusters:
    selector:
      matchLabels:
        env: dev
  duration:
    default: 2m
    max: 1h
  requesters:
    groups:
    - devs
  justification:
    reason: Required
  approval:
    pendingTimeout: 24h
    rules:
    - name: approvers-approve
      mode: Manual
      approvers:
        groups:
        - approvers
```

## Give a Keycloak group hub admin rights

The hub decides admin rights with a SubjectAccessReview. It names the person with the
`subjectTemplate` and lists the groups from the token as they are, without the API server's
groups prefix. Bind the group by its name in the token:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: protem-hub-admin
rules:
- apiGroups: ["protem.dev"]
  resources: ["accessgrants", "accessrequests"]
  verbs: ["get", "list", "update"]
- apiGroups: ["protem.dev"]
  resources: ["accessprofiles", "targetclusters"]
  verbs: ["get", "list", "create", "update", "delete"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: protem-hub-admin
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: protem-hub-admin
subjects:
- apiGroup: rbac.authorization.k8s.io
  kind: Group
  name: protem-admins
```

A member of `protem-admins` sees every request and the fleet, and can revoke grants. With
`uiManagement: enabled`, they can also manage profiles and clusters. The same binding worked
with and without `--oidc-groups-prefix=oidc:` on the API server.

## Set up kubectl sign-in on the target clusters

### Create a client for kubectl

Create a public client for `kubectl`. Its redirect URIs are the local ports the
[`kubectl oidc-login`](https://github.com/int128/kubelogin) plugin listens on. Give it the same
groups mapper as the hub's client:

```sh
kcadm.sh create clients -r <realm> -s clientId=kubernetes -s protocol=openid-connect -s publicClient=true -s standardFlowEnabled=true -s directAccessGrantsEnabled=false -s 'redirectUris=["http://localhost:8000","http://localhost:18000"]'
KID=$(kcadm.sh get clients -r <realm> -q clientId=kubernetes --fields id | jq -r '.[0].id')
kcadm.sh create clients/$KID/protocol-mappers/models -r <realm> -s name=groups -s protocol=openid-connect -s protocolMapper=oidc-group-membership-mapper -s 'config."claim.name"=groups' -s 'config."full.path"=false' -s 'config."id.token.claim"=true' -s 'config."access.token.claim"=true' -s 'config."userinfo.token.claim"=true'
```

The ID token from this client has `aud` set to `kubernetes`, which the API server checks against
`--oidc-client-id`, so it needs no audience mapper either. A public client keeps the hub's client
secret off people's machines.

### Configure the API server

The API server flags decide how the cluster names a person:

| Flag | Effect |
|---|---|
| `--oidc-issuer-url` | Keycloak's issuer, `https://<keycloak host>/realms/<realm>`; HTTPS only |
| `--oidc-client-id` | The audience the token must carry: `kubernetes` |
| `--oidc-username-claim` | The claim the username comes from, for example `email` or `preferred_username` |
| `--oidc-username-prefix` | Text put before the username. Without this flag, any claim other than `email` gets the issuer URL and `#` as a prefix |
| `--oidc-groups-claim` | `groups` |
| `--oidc-groups-prefix` | Text put before every group name |
| `--oidc-ca-file` | Only for a private CA: the CA file |

Two settings were run end to end:

- **A. Name people by email**

  ```text
  --oidc-issuer-url=https://<keycloak host>/realms/<realm>
  --oidc-client-id=kubernetes
  --oidc-username-claim=email
  --oidc-groups-claim=groups
  --oidc-ca-file=<CA file>
  ```

- **B. Name people by username, with a prefix**

  ```text
  --oidc-issuer-url=https://<keycloak host>/realms/<realm>
  --oidc-client-id=kubernetes
  --oidc-username-claim=preferred_username
  --oidc-username-prefix=oidc:
  --oidc-groups-claim=groups
  --oidc-groups-prefix=oidc:
  --oidc-ca-file=<CA file>
  ```

If you set these flags in a YAML file rather than on the command line (k3s and RKE2 do), quote any
value that ends in a colon, such as `- 'oidc-username-prefix=oidc:'`. Unquoted, YAML reads it
as a mapping and the API server does not start.

### Sign in with kubectl

Each person installs the `kubectl oidc-login` plugin and adds a user to their kubeconfig. Leave out
`--certificate-authority` when Keycloak's certificate is from a public CA:

```sh
kubectl config set-credentials keycloak \
  --exec-api-version=client.authentication.k8s.io/v1 \
  --exec-interactive-mode=IfAvailable \
  --exec-command=kubectl \
  --exec-arg=oidc-login \
  --exec-arg=get-token \
  --exec-arg=--oidc-issuer-url=https://<keycloak host>/realms/<realm> \
  --exec-arg=--oidc-client-id=kubernetes \
  --exec-arg=--certificate-authority=<CA file>
kubectl config set-context --current --user=keycloak
kubectl auth whoami
```

The plugin opens a browser for the Keycloak sign-in. With setting A, `kubectl auth whoami` printed:

```text
ATTRIBUTE                                           VALUE
Username                                            alice@example.com
Groups                                              [devs system:authenticated]
```

and with setting B:

```text
ATTRIBUTE                                           VALUE
Username                                            oidc:alice
Groups                                              [oidc:devs system:authenticated]
```

## Line names up with the target clusters

Kubernetes keeps no list of users. A binding for a name the API server never produces is created
without error and matches nobody. Set the subject so Protem renders exactly the `Username` that
`kubectl auth whoami` prints:

| API server | `kubectl auth whoami` | Subject |
|---|---|---|
| A: `--oidc-username-claim=email` | `alice@example.com` | `'{{.Email}}'` |
| B: `--oidc-username-claim=preferred_username --oidc-username-prefix=oidc:` | `oidc:alice` | `'oidc:{{.Username}}'` |
| `--oidc-username-claim=preferred_username` with no prefix flag | `https://<keycloak host>/realms/<realm>#alice` | `'https://<keycloak host>/realms/<realm>#{{.Username}}'` |

On the hub's own cluster, the subject is always the chart's `subjectTemplate`; the hub ignores
that cluster's own `spec.subject`. On any other target cluster, it is `TargetCluster.spec.subject`,
or the same `subjectTemplate` when that field is left empty (see
[How to connect a target cluster](how-to-connect-a-target-cluster.md)). Only the hub's own
cluster was tested here. A target cluster with an agent binds the same rendered name, but that
was not run.

With setting A and `subjectTemplate: '{{.Username}}'`, the grant showed `Active` and the binding
named `alice`. The API server calls the same person `alice@example.com`, so access was refused:

```text
NAME                                                 ROLE               AGE   USERS   GROUPS   SERVICEACCOUNTS
protem-grant-default.grant-req-1795610932096-bt7qk   ClusterRole/edit   6s    alice
Error from server (Forbidden): pods is forbidden: User "alice@example.com" cannot list resource "pods" in API group "" in the namespace "team-a"
```

## Requests created with kubectl

The chart's admission policy, `protem-requester-identity`, accepts a request only when:

- `spec.requester.username` is the caller's username as the API server sees it;
- `spec.requester.email` is empty or equal to that username;
- every entry of `spec.requester.groups` is one of the caller's groups as the API server sees them.

See [How to enforce requester identity for kubectl requests](how-to-enforce-requester-identity-for-kubectl-requests.md).

- **Setting A works.** Alice could create a request with `username` and `email` both
  `alice@example.com` and `groups: [devs]`. Bob approved it in the UI, and the binding named
  `alice@example.com`. A request with `username: alice` was refused with
  `spec.requester.username must be your own username, alice@example.com`.
- **Setting B cannot work.** With a username or groups prefix, a person cannot create their own
  working request with `kubectl`:

  | `spec.requester` | Result |
  |---|---|
  | `username: alice`, `email: alice@example.com` | Refused: `spec.requester.username must be your own username, oidc:alice` |
  | `username: oidc:alice`, `email: alice@example.com` | Refused: `spec.requester.email must be empty or oidc:alice` |
  | `username: oidc:alice`, `email: ''`, `groups: [devs]` | Refused: `spec.requester.groups may only list groups you belong to` |
  | `username: oidc:alice`, `email: ''`, `groups: [oidc:devs]` | Admitted, then rejected by Protem: `"oidc:alice" may not request this profile. It can be requested by the groups [devs].` |
  With `oidc:devs` also added to the profile's `requesters.groups`, the last request was
  approved. It was bound as `oidc:oidc:alice`, because `oidc:{{.Username}}` adds the prefix a
  second time, so the grant gave no access. Under setting B on 0.1.3, people request access in
  the UI. Only automation that holds the `delegate` verb can create requests for them with
  `kubectl`.

  From 0.1.4, setting B works for `kubectl` requests too: set the admission policy's
  `usernamePrefix` and `groupsPrefix` both to `oidc:` (chart values `admissionPolicy.usernamePrefix`
  / `admissionPolicy.groupsPrefix`). The policy then compares the caller's name and groups with
  the prefix removed, so a request can name `username: alice`, `groups: [devs]` as setting A
  does. See
  [How to enforce requester identity for kubectl requests](how-to-enforce-requester-identity-for-kubectl-requests.md#if-the-api-server-adds-a-prefix).
  On 0.1.4 against Keycloak, alice's `kubectl` request naming `alice` and `groups: [devs]` was
  admitted, approved and bound as `oidc:alice`, and gave her access; the forms with `oidc:alice`
  or `oidc:devs`, a group she is not in, and naming bob were refused. A request through
  `kubectl` and one through the UI for the same profile were refused as duplicates of each other.

## Check it works

1. Open `https://<your UI host>` and sign in with Keycloak. The UI loads.
2. `GET /api/me` shows what the hub read from the token:

   ```json
   {"username":"bob","email":"bob@example.com","groups":["approvers","devs"],"roles":null,"devAuth":false,"signOutURL":"/oauth2/sign_out"}
   ```

3. Have someone in `devs` request the profile. Have someone in `approvers` approve it. The
   requester's own approval is refused with `is not eligible to record a Approved decision on
   this request`.
4. As the requester, run `kubectl auth whoami`. Then compare the `Username` with the binding:

   ```sh
   kubectl get rolebinding -n team-a -o wide
   kubectl auth can-i create deployments -n team-a
   kubectl auth can-i create deployments -n team-b
   ```

   The `USERS` column matches the `Username`, and the answers are `yes` and `no`. When the grant
   expires, the binding is gone, and the same person is refused in `team-a` again.

## Troubleshooting

- **Keycloak's login form ends in an error, and Keycloak logs `error="cookie_not_found"`.**
  The browser reached Keycloak over plain HTTP at a host other than `localhost`. Keycloak 24
  marks its login cookies `Secure`, so they are not sent back over HTTP. Serve Keycloak over HTTPS.
- **The operator or oauth2-proxy restarts with `x509: certificate signed by unknown authority`.**
  Keycloak's certificate is from a CA the hub does not trust. See
  [If Keycloak's certificate is from a private CA](#if-keycloaks-certificate-is-from-a-private-ca).
- **oauth2-proxy logs `id token issued by a different provider`.** Keycloak sets `iss` from the
  URL the browser signed in at. Use the same `https://<keycloak host>` for people, the hub, the
  proxy and every API server.
- **Sign-in fails, and Keycloak logs `Offline tokens not allowed for the user or client`.**
  `authProxy.scope` asks for `offline_access`, which the person lacks. Remove `offline_access`
  from the scope.
- **The grant is `Active`, but `kubectl` is `Forbidden`.** The subject does not match the name the
  API server uses. See [How to fix a grant that gives no access](../operations/runbooks/how-to-fix-a-grant-that-gives-no-access.md).
- **Groups appear as `/devs` in `GET /api/me`.** The mapper has `full.path` set to `true`. Turn it
  off, or use `/devs` in profiles and RBAC.
