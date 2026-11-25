# How to set up login with Microsoft Entra ID

Put oauth2-proxy in front of a hub installed with the Helm chart in `deploy/helm/protem`, reached at a hostname over HTTPS, and sign people in with Microsoft Entra ID.

**Tested:** this guide was followed end to end on 2026-11-25 against a real Entra ID tenant on the Free tier, with the hub and a separate target cluster on AKS (Kubernetes 1.36, Microsoft Entra integration with Kubernetes RBAC), again with the hub and a separate target cluster on two EKS clusters (Kubernetes 1.37, eu-north-1), and again on GKE (Kubernetes 1.36, a Standard hub and an Autopilot target), all on 0.1.5 for the GKE run. Not tested: group-based assignment with P1 or P2, guest accounts, people in more than 200 groups, and Conditional Access or MFA policies.

## Register the application

1. In the [Microsoft Entra admin center](https://entra.microsoft.com), go to **Entra ID** > **App registrations** > **New registration** ([quickstart](https://learn.microsoft.com/en-us/entra/identity-platform/quickstart-register-app)).
2. Under **Supported account types**, choose **Single tenant only** unless you also need personal Microsoft accounts or other tenants.
3. Under **Redirect URI**, choose **Web** and enter `https://<your UI host>/oauth2/callback`: this is the path oauth2-proxy redeems in both the Helm chart (`--redirect-url=<authProxy.publicURL>/oauth2/callback`) and `config/auth-proxy/manager_patch.yaml`, and it must match exactly ([add a redirect URI](https://learn.microsoft.com/en-us/entra/identity-platform/how-to-add-redirect-uri)).
4. Record the **Application (client) ID** and the **Directory (tenant) ID** from the app's **Overview** page.
5. Under **Certificates & secrets** > **Client secrets**, create a new client secret and record its **Value** immediately; it is not shown again ([add credentials](https://learn.microsoft.com/en-us/entra/identity-platform/how-to-add-credentials)).
6. Under **API permissions**, check that **Microsoft Graph** > **User.Read** (delegated) is listed, then select **Grant admin consent** for your tenant. An app registered in the admin center has that permission already; one created with `az ad app create` has none, and sign-in then fails with `AADSTS65001` until you add it (`az ad app permission add`) and consent. Without admin consent, people who sign in see Entra ID's "Need admin approval" screen instead of the UI, even though the app asks for nothing beyond sign-in.

The hub verifies the ID token's issuer against `--oidc-issuer-url`, so use the v2.0 endpoint for both the app and the hub: `https://login.microsoftonline.com/<tenant id>/v2.0`. Entra ID issues v1.0 or v2.0 ID tokens depending on the endpoint used to request them ([ID token claims](https://learn.microsoft.com/en-us/entra/identity-platform/id-token-claims-reference)); the v2.0 endpoint's discovery document reports `issuer` as `https://login.microsoftonline.com/<tenant id>/v2.0`, which is what oauth2-proxy's `--oidc-issuer-url` and the hub's `--oidc-issuer-url` must both be set to.

## Get groups into the ID token

1. On the app registration, under **Manage** > **Token configuration**, select **Add groups claim** ([configure groups claims](https://learn.microsoft.com/en-us/entra/identity/hybrid/connect/how-to-connect-fed-group-claims)).
2. Group-based assignment to an enterprise application — and so the **Groups assigned to the application** option — needs Microsoft Entra ID P1 or P2 ([manage users and groups assignment](https://learn.microsoft.com/en-us/entra/identity/enterprise-apps/assign-user-or-group-access-portal#group-based-assignment)); individual user assignment works on Free. With P1 or P2, choose **Groups assigned to the application**: Entra ID emits at most 200 groups in a JWT, and exceeding that limit drops the `groups` claim entirely rather than truncating it ([group overages](https://learn.microsoft.com/en-us/security/zero-trust/develop/configure-tokens-group-claims-app-roles#group-overages)), and this option counts only the groups assigned to this app registration toward that limit. On the Free tier, choose **Security groups** instead and assign users to the application individually; the 200-group limit then applies to each user's total security-group membership, so a person in many groups can still lose the `groups` claim entirely, with no group-assignment option available to narrow it.
3. Values in the `groups` claim are the groups' object IDs (GUIDs), unless you configure the on-premises `sAMAccountName` or cloud-only display name attribute instead.

The hub reads the `groups` claim only (`internal/api/auth.go`); it does not read `wids` (directory roles) or `roles` (app roles). Entra's [app roles](https://learn.microsoft.com/en-us/entra/identity-platform/howto-add-app-roles-in-apps) are an alternative to groups for authorization, but they arrive in the ID token's `roles` claim, which the hub does not read: app roles do not work for group matching in Protem today. Use `requesters.groups` and `approvers.groups` with the groups claim instead.

## Make sure the token carries an email or username

The hub resolves email as `email`, then `preferred_username`, then `sub`; and username as `preferred_username`, then `sub`, then `email` (see [Claims](../reference/authentication.md#claims)). `email` is included by default for guest accounts, but for members of the tenant it must be requested as an optional claim: under **Token configuration**, select **Add optional claim**, choose **ID**, and add `email` ([configure optional claims](https://learn.microsoft.com/en-us/entra/identity-platform/optional-claims)). Without it, the hub falls back to `preferred_username`, Entra's mutable sign-in name (often the user's UPN), which is still usable as the login identity but not guaranteed to be an email address.

Add the optional claim anyway, even though oauth2-proxy's `oidc` provider already requests the `email` scope by default ([`oidc.go`](https://github.com/oauth2-proxy/oauth2-proxy/blob/v7.12.0/providers/oidc.go)): a member with no mailbox attribute still gets no `email` claim either way, and when oauth2-proxy's session ends up with no email at all, it refuses the login outright with "neither the id_token nor the profileURL set an email" — the hub never sees that person's token, independent of what the hub itself would have accepted. Add `--oidc-email-claim=preferred_username` to `authProxy.extraArgs` as a fallback so that login still succeeds for such members, falling back to `preferred_username` as the proxy's email.

Even with the optional claim added, Entra ID omits `email` for a user whose `mail` attribute is
in a domain the tenant has not verified: a Google Workspace address is a common example, since a
tenant's free default domain does not cover it. The hub then falls back to
`preferred_username`, so `{{.Email}}` renders the UPN instead. A tenant whose own domain is
verified should emit the claim for a `mail` in that domain with no further change (not tested
here). To emit it anyway, set `authenticationBehaviors.removeUnverifiedEmailClaim` to `false` on
the application ([Manage application authenticationBehaviors](https://learn.microsoft.com/en-us/graph/applications-authenticationbehaviors), Microsoft Graph beta):

```sh
az rest --method PATCH --uri "https://graph.microsoft.com/beta/applications/<application object id>" \
  --body '{"authenticationBehaviors":{"removeUnverifiedEmailClaim":false}}'
```

**Caution:** with this set, whoever can change a user's `mail` attribute (an admin) chooses which
address that person is bound as everywhere `{{.Email}}` is used, including on other clusters. A
verified domain is the right fix for production; use this override only where that is not
possible.

## Restrict who can sign in

Under **Enterprise applications**, find the app, open **Properties**, and set **Assignment required?** to **Yes** ([restrict your app to a set of users](https://learn.microsoft.com/en-us/entra/identity-platform/howto-restrict-your-app-to-a-set-of-users)); this works on the Free tier. A freshly registered app does not require assignment by default, so skipping this step leaves every tenant member able to sign in regardless of who is listed below. From the CLI: `az ad sp update --id <service principal id> --set appRoleAssignmentRequired=true`. Then, under **Users and groups**, choose **Add user/group** and assign the people who should reach the hub (and groups, if you have P1 or P2). Conditional access policies and MFA requirements that apply to other applications in the tenant apply here too.

## Install or upgrade Protem

The chart's `authProxy` uses oauth2-proxy's generic `oidc` provider by default, which is enough for Entra ID as a standard OIDC issuer; oauth2-proxy's dedicated `entra-id` provider adds group-overage handling (fetching groups from Microsoft Graph above 200) and multi-tenant support, neither needed here since groups are already restricted to those assigned to the application. Stay with the chart's default `oidc` provider.

Create the namespace and the Secret with the client secret and a cookie secret:

```sh
kubectl create namespace protem-system
kubectl -n protem-system create secret generic oauth2-proxy \
  --from-literal=client-secret='<client secret value>' \
  --from-literal=cookie-secret="$(openssl rand -base64 32 | tr -- '+/' '-_')"
```

The chart reads those keys, `client-secret` and `cookie-secret`, from `authProxy.existingSecret`.

Testing only: if `authProxy.publicURL` is a plain `http://localhost` address (for example behind
`kubectl port-forward`), add `--cookie-secure=false` to `authProxy.extraArgs`. oauth2-proxy marks
its session cookie `Secure` by default, which a browser refuses to send back over plain HTTP.
Entra ID accepts an `http://localhost` redirect URI for this case.

Write `values.yaml`:

```yaml
oidc:
  issuerURL: https://login.microsoftonline.com/<tenant id>/v2.0
  clientID: <application (client) id>
subjectTemplate: '{{.Username}}'             # the UPN, which is how AKS names an Entra ID member
authProxy:
  enabled: true
  publicURL: https://protem.example.com
  existingSecret: oauth2-proxy
  scope: openid email profile offline_access     # Entra ID returns a refresh token only with offline_access
  extraArgs:
  - --oidc-email-claim=preferred_username        # fallback for members with no mail attribute
ingress:
  enabled: true
  className: nginx
  host: protem.example.com
  tls:
    secretName: protem-ui-tls
networkPolicy:
  enabled: true
```

Install or upgrade:

```sh
helm upgrade --install protem oci://ghcr.io/illeniumstudios/charts/protem \
  --namespace protem-system --create-namespace -f values.yaml
```

Do not leave `offline_access` out of `authProxy.scope`. The chart's default scopes do not include it, and without it Entra ID gives the proxy no refresh token, so the proxy cannot renew a session: someone removed from a group keeps that group in the hub until their next full sign-in, about an hour later. With it, the change reached the hub about four minutes after the removal, at the proxy's next refresh.

## Use Entra groups in profiles

`AccessProfile.spec.requesters.groups` and `approval.rules[].approvers.groups` take the groups' object IDs, since that is what the `groups` claim carries by default:

```yaml
apiVersion: protem.dev/v1alpha1
kind: AccessProfile
metadata:
  name: dev-view
  namespace: default
spec:
  displayName: Dev read-only
  role:
    kind: ClusterRole
    name: view
  scope: Namespaced
  namespaces:
  - default
  clusters:
    selector:
      matchLabels:
        env: dev
  duration:
    default: 1h
    max: 4h
  requesters:
    groups:
    - ac51800c-2679-4ecb-8130-636380a3b491
  justification:
    reason: Required
  approval:
    pendingTimeout: 24h
    rules:
    - name: dev-self-service
      mode: Auto
```

## Line names up with the target clusters

On an AKS cluster with Microsoft Entra ID integration, a Kubernetes `RoleBinding` or `ClusterRoleBinding` names a person by their UPN (when they are in the same tenant as the cluster; otherwise by their object ID) and names a group by its object ID, given as `subjects[].name` with `kind: Group` or `kind: User` ([Entra ID and Kubernetes RBAC](https://learn.microsoft.com/en-us/azure/aks/kubernetes-rbac-entra-id)).

Set `TargetCluster.spec.subject` for that cluster to match:

```yaml
spec:
  subject: '{{.Username}}'
```

Use `{{.Username}}`: the hub reads it from `preferred_username`, which for a member of the tenant is the UPN. `{{.Email}}` gives the same string only while the person has no `mail` attribute. Once they have one that differs from the UPN, `{{.Email}}` produces a binding for a name AKS never uses: the grant shows as active and gives no access, with no error anywhere. Set the chart's `subjectTemplate` the same way, as above; it names people in bindings on the hub itself and in the hub's own permission checks. For a cluster that authenticates some other way (its own OIDC issuer, a service account token, or local RBAC), match its `subject` to however it names the same email or username instead. On EKS, names and prefixes work differently again: see [How to match user names on EKS](how-to-match-user-names-on-eks.md).

On GKE, the subject stays `{{.Email}}` (GKE's own default), and the Entra user's `mail` must be
their Google identity's email, so the two systems agree on the same string: see
[How to match user names on GKE](how-to-match-user-names-on-gke.md).

## Check it works

Open `https://<your UI host>` and sign in. On success, the UI loads instead of showing a sign-in redirect loop. To see the claims the hub resolved for the signed-in person, call `GET /api/me` (handled in `internal/api/server.go`), which returns the username, email, groups and roles the hub read from the token.

To grant access on the hub's own cluster, register it first: see [Register the hub cluster](how-to-install-with-helm.md#register-the-hub-cluster). Then confirm a binding works for a real person: have them request a profile, and once it is active run `kubectl auth whoami` as them on the target cluster and compare the user name with the subject of the `RoleBinding` Protem created.

If the UI shows "You're signed out" right after a successful login, see [How to fix "You're signed out" after login](../operations/runbooks/how-to-fix-signed-out-after-login.md).
