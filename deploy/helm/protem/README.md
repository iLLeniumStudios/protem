# Protem

Just-in-time privileged access management for Kubernetes. People request a role for a few hours, an approver sees exactly what it grants, and the access is revoked automatically when it expires, across every cluster.

This chart installs the Protem **hub**: the operator, its web UI and API, the five custom resource definitions, and the RBAC the operator needs. Target clusters run an agent, which this chart does not install; see [How to connect a target cluster](https://protem.dev/docs/guides/how-to-connect-a-target-cluster/).

## Requirements

- Kubernetes 1.30 or later on the hub. The chart includes a `ValidatingAdmissionPolicy`.
- An OIDC identity provider for login.

## Install

```sh
helm install protem oci://ghcr.io/illeniumstudios/charts/protem \
  --version <version> \
  --namespace protem-system --create-namespace \
  --set oidc.issuerURL=https://idp.example.com/realms/example \
  --set oidc.clientID=protem
```

Nothing logs people in by default: the hub accepts only requests that carry an ID token from the issuer you set. Turn on the bundled oauth2-proxy with `authProxy.enabled=true`, or put your own identity-aware proxy in front. [How to set up login](https://protem.dev/docs/guides/how-to-set-up-login/) has the details.

## Values most people set

| Value | Default | Purpose |
|---|---|---|
| `oidc.issuerURL`, `oidc.clientID` | none | The identity provider whose ID tokens the hub accepts |
| `authProxy.enabled` | `false` | Run oauth2-proxy next to the operator as the only way into the UI |
| `authProxy.publicURL` | none | The URL people open |
| `authProxy.existingSecret` | none | Secret with the keys `client-secret` and `cookie-secret` |
| `authProxy.scope` | oauth2-proxy's `openid email profile` | OAuth scopes the proxy requests; Microsoft Entra ID needs `offline_access` added |
| `ingress.enabled`, `ingress.host`, `ingress.tls.secretName` | off | An Ingress for the UI |
| `hubCluster.register`, `hubCluster.labels` | `true`, `{}` | Register the hub as a `TargetCluster`, and the labels profiles select it by |
| `hubAPIURL` | none | The hub's API URL as agents reach it; needed to connect clusters from the UI |
| `hubAllowedRoles` | `[view, edit]` | The only roles the operator may bind on the hub itself |
| `subjectTemplate` | `{{.Email}}` | How a person is named in bindings |
| `uiManagement` | `readonly` | `enabled` lets the UI create and edit profiles and clusters |
| `historyRetention` | keep forever | Delete finished requests this long after their last event |

Every value is listed in the [chart values reference](https://protem.dev/docs/reference/chart-values/), and `helm show values` prints the defaults.

## More

- [Install, upgrade and uninstall with Helm](https://protem.dev/docs/guides/how-to-install-with-helm/)
- [Your first request](https://protem.dev/docs/getting-started/first-request/)
- [Security model](https://protem.dev/docs/architecture/security-model/): what Protem trusts and what it does not protect against
- [Source and issues](https://github.com/iLLeniumStudios/protem)
