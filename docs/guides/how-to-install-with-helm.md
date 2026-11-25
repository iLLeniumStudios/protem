# How to install the hub with Helm

Install the Protem operator, UI and API on the hub cluster with the published Helm chart, and
upgrade or uninstall it later.

It needs Kubernetes 1.30 or later: the chart includes a `ValidatingAdmissionPolicy`.

Target clusters run an agent, which this chart does not install. See
[How to connect a target cluster](how-to-connect-a-target-cluster.md). To install with OLM
instead, see [How to install with OLM](how-to-install-with-olm.md).

## Install

The chart is published to `oci://ghcr.io/illeniumstudios/charts/protem` for each release.
Without `--version`, Helm installs the latest release. To pin one, add `--version` with the
release's version without the leading `v` (release `v1.2.3` is chart version `1.2.3`):

```sh
helm install protem oci://ghcr.io/illeniumstudios/charts/protem \
  --namespace protem-system --create-namespace \
  --set oidc.issuerURL=https://idp.example.com/realms/example \
  --set oidc.clientID=protem
```

Replace the issuer and client ID with your identity provider's. The operator reads the issuer's
discovery document when it starts, and does not start if it cannot reach it.

By default the operator image is `ghcr.io/illeniumstudios/protem` tagged with the release, and
nothing logs people in: the UI is reachable only through the `protem-ui` Service, and the hub
accepts only requests that carry an ID token. Either turn on the bundled auth proxy (below) or
put your own identity-aware proxy in front. [How to set up login](how-to-set-up-login.md)
explains what the hub expects from the token and how to set up the identity provider.

To see every value with its default:

```sh
helm show values oci://ghcr.io/illeniumstudios/charts/protem
```

## Values most people set

| Value | Default | Purpose |
|---|---|---|
| `oidc.issuerURL`, `oidc.clientID` | none | The identity provider whose ID tokens the hub accepts |
| `authProxy.enabled` | `false` | Run oauth2-proxy next to the operator as the only way into the UI |
| `authProxy.publicURL` | none | The URL people open; the identity provider redirects to `<publicURL>/oauth2/callback` |
| `authProxy.existingSecret` | none | Secret with the keys `client-secret` and `cookie-secret` |
| `authProxy.scope` | oauth2-proxy's `openid email profile` | OAuth scopes the proxy requests; Microsoft Entra ID needs `offline_access` added |
| `ingress.enabled`, `ingress.host`, `ingress.tls.secretName` | off | An Ingress for the UI |
| `hubClusterName` | `local` | The `TargetCluster` name that means the hub itself |
| `hubCluster.register`, `hubCluster.labels` | `true`, `{}` | Register the hub as a `TargetCluster`, and the labels profiles select it by |
| `hubAPIURL` | none | The hub's API URL as agents reach it; needed to connect clusters from the UI |
| `hubAllowedRoles` | `[view, edit]` | The only roles the operator may bind on the hub itself; `[]` grants nothing there |
| `subjectTemplate` | `{{.Email}}` | How a person is named in bindings; must match how each cluster's API server names users |
| `uiManagement` | `readonly` | `enabled` lets the UI create and edit profiles and clusters, and installs the role for it |
| `historyRetention` | keep forever | Delete finished requests this long after their last event, e.g. `720h` |
| `networkPolicy.enabled` | `false` | Admit traffic to the operator only on the UI port and, from namespaces labelled `metrics: enabled`, the metrics port |

Any other operator flag goes in `extraArgs`, and any other oauth2-proxy flag in
`authProxy.extraArgs`.

## Register the hub cluster

The chart registers the hub as a target cluster: it creates a `TargetCluster` named
`hubClusterName` (default `local`), which the operator marks Ready without an agent. Profiles
select clusters by label, so give it the labels your profiles use:

```yaml
hubCluster:
  displayName: Hub cluster
  labels:
    env: prod
```

To register it yourself instead, set `hubCluster.register=false`. If a `TargetCluster` with that
name already exists and Helm did not create it, the chart leaves it as it is, labels included.

Without it, a profile that targets the hub shows "None of this profile's clusters is ready
right now". Other clusters are added with an agent: see
[How to connect a target cluster](how-to-connect-a-target-cluster.md).

The hub always binds people on its own cluster with `subjectTemplate`, ignoring that
`TargetCluster`'s own `spec.subject`: the API refuses to set it there, and the UI shows the field
read-only for this cluster. On every other cluster, an empty `spec.subject` falls back to that
same `subjectTemplate`.

## Example: login with the bundled auth proxy

Register an OIDC client with your identity provider with the redirect URI
`https://protem.example.com/oauth2/callback` ([How to set up login](how-to-set-up-login.md) has
the details for Keycloak, Microsoft Entra ID and Dex). Then create the Secret the proxy reads:

```sh
kubectl create namespace protem-system
kubectl -n protem-system create secret generic oauth2-proxy \
  --from-literal=client-secret='<client secret>' \
  --from-literal=cookie-secret="$(openssl rand -base64 32 | tr -- '+/' '-_')"
```

Write `values.yaml`:

```yaml
oidc:
  issuerURL: https://idp.example.com/realms/example
  clientID: protem
authProxy:
  enabled: true
  publicURL: https://protem.example.com
  existingSecret: oauth2-proxy
ingress:
  enabled: true
  className: nginx
  host: protem.example.com
  tls:
    secretName: protem-ui-tls
networkPolicy:
  enabled: true
```

And install:

```sh
helm install protem oci://ghcr.io/illeniumstudios/charts/protem \
  --namespace protem-system -f values.yaml
```

With the proxy on, the operator listens on `127.0.0.1` only, the `protem-ui` Service points
at the proxy, and the UI shows a Sign out link (`/oauth2/sign_out`; set `signOutURL` to
change it). The certificate goes in the Secret `protem-ui-tls`, or add a
`cert-manager.io/cluster-issuer` annotation under `ingress.annotations`.

## Upgrade

```sh
helm upgrade protem oci://ghcr.io/illeniumstudios/charts/protem --version <new version> \
  --namespace protem-system -f values.yaml
```

Helm installs the CRDs on the first install and never changes them afterwards. Before
upgrading the chart, apply the CRDs of the new version yourself, from the chart:

```sh
helm pull oci://ghcr.io/illeniumstudios/charts/protem --version <new version> --untar
kubectl apply --server-side -f protem/crds/
```

or from the repository at the release tag:

```sh
git checkout v<new version>
kubectl apply --server-side -f config/crd/bases/
```

## Uninstall

`helm uninstall` removes the operator, its RBAC and the admission policy. It leaves behind:

- the CRDs, and with them every AccessProfile, AccessRequest, AccessGrant, ClusterGrant and
  TargetCluster, including the hub's `TargetCluster` the chart created: target clusters are
  removed while the operator still runs, as below, so `helm uninstall` keeps it;
- the bindings the operator created for grants that are still active;
- the namespaces `protem-cluster-<name>` the operator created for each target cluster;
- the `protem-system` namespace, and any Secret you created in it.

To remove everything, end all access and remove the target clusters first, while the operator
still runs: grants and target clusters carry a finalizer that only the operator removes.

```sh
kubectl delete accessrequests,accessgrants --all --all-namespaces
kubectl delete targetclusters --all
helm uninstall protem --namespace protem-system
kubectl delete crd accessgrants.protem.dev accessprofiles.protem.dev accessrequests.protem.dev \
  clustergrants.protem.dev targetclusters.protem.dev
kubectl get namespaces -o name | grep '^namespace/protem-cluster-' | xargs -r kubectl delete
kubectl delete namespace protem-system
```

## Kustomize

`make deploy IMG=<image>` builds `config/default` with kustomize and applies it; `config/sso`
adds the auth proxy the same way. These stay available for development and for anyone who
prefers kustomize. The chart's CRDs and the operator's ClusterRole are copied from `config/`
with `make helm-sync`, so both installs grant the operator the same permissions.
