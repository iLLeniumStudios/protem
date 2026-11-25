# Security model

What Protem trusts, what each party can and cannot do, and what it does not protect against. Setup steps are in [how to set up login](../guides/how-to-set-up-login.md) and [how to connect a target cluster](../guides/how-to-connect-a-target-cluster.md); the audit log is described in [the audit log reference](../operations/audit-log.md).

## The parts and the lines between them

- **The hub** is one Kubernetes cluster. It holds every AccessProfile, AccessRequest and AccessGrant as custom resources, and runs the operator, which serves the UI and its API.
- **An agent** runs on each other cluster (a target cluster). It reads its own cluster's grants from the hub and creates ordinary RoleBindings on its own cluster.
- **People** reach the hub's UI through a login proxy and an OIDC identity provider.

Three choices shape everything below:

1. **Agents pull; the hub never connects to a target cluster.** The hub holds no credentials for any other cluster.
2. **Each cluster's owner sets a ceiling.** The agent may bind only the roles its ClusterRole lists by name, and the cluster's own API server enforces that. The hub has the same kind of list for itself (`config/rbac/hub_bindable_roles.yaml`).
3. **Access is plain RBAC with an expiry stamped on it.** The agent removes expired bindings on its own timer, without asking the hub.

## How each connection is protected

| Connection | Protection |
|---|---|
| Browser to UI | TLS at your ingress; login at your identity provider through oauth2-proxy; the hub verifies the ID token's signature, issuer, audience and expiry itself |
| UI actions | Every read and write is checked against the caller's own permissions on the hub (SubjectAccessReview) and the profile's approval rules. Changing requests must be JSON, which a form on another site cannot send |
| Agent to hub | TLS with the hub's certificate authority pinned; a ServiceAccount token that is valid for one namespace on the hub and that the agent renews itself (24 hours by default) |
| Hub to target cluster | None exists |
| Agent to its own cluster | Its in-cluster ServiceAccount; RoleBindings and the roles on the ceiling list, nothing else |
| `kubectl` to hub | Hub RBAC, plus an admission policy that makes a request name its caller as the requester |

## What each party can and cannot do

**A requester** can ask for the profiles their groups allow, for clusters and namespaces inside the profile, for up to the profile's longest duration. They cannot name someone else as the requester, approve their own request unless the profile's rule says so, hold the same access twice, or change a request after sending it. The role, scope and duration are fixed when the request is created, so editing a profile afterwards does not change what an approved request grants.

**An approver** can approve or reject the requests whose rules name them, and see the resulting grant. They cannot revoke it, and cannot change what was asked for.

**Whoever can write AccessProfiles on the hub** decides what can be requested and who approves it. That is the most sensitive permission in the system; it is ordinary hub RBAC. The UI can edit profiles only when the operator runs with `--ui-management=enabled`, only for people the hub's RBAC allows, and never for objects a GitOps tool manages.

**Someone who steals an agent's hub token** can read that one cluster's grants (who holds which role until when; not the reasons or tickets, which never leave the request), report false status and heartbeats for it, and renew the token until the cluster's credentials are reissued. They cannot read other clusters' grants, any request or profile, create or change a grant, or reach any cluster's API through it. "Reissue credentials" on the Fleet page, or deleting the agent's ServiceAccount on the hub, cuts the token off at once.

**Someone who takes over an agent, or is an administrator of a target cluster,** can create bindings on that cluster for the roles on its ceiling list, and can keep expired bindings alive there. They gain nothing on the hub beyond the stolen-token case, and nothing on any other cluster.

**Someone who takes over the hub's operator, or is an administrator of the hub,** can issue grants for any person on any cluster, read every request, and mint agent tokens. On each target cluster they are still limited to the roles on that cluster's ceiling list; on the hub itself the operator is limited to the roles in `hub_bindable_roles.yaml`. They cannot connect to a target cluster. This is why the ceiling exists: a compromised hub can hand out `view` and `edit`, or whatever each owner listed, and no more.

**Someone on the network** sees TLS. A page on another website cannot make a logged-in person's browser change anything, because changing requests must be JSON and the UI's API allows no other origins outside development mode.

## When the hub is unreachable

No new access can be requested or approved. Access already on a cluster still ends on time. A revocation waits until the agent can read the hub again, so a revoked binding stays until then or until it expires, whichever is first. An agent that cannot reach the hub for longer than its token's lifetime must be given new credentials.

## What Protem does not protect against

- **Open sessions.** Removing a binding stops new requests. A `kubectl exec`, port-forward or watch that is already open keeps running until it closes; Kubernetes does not end established streams when RBAC changes.
- **What a role contains.** The ceiling lists role names. A cluster owner who lists `edit` trusts whatever `edit` means on that cluster, including anything aggregated into it.
- **A tampered audit trail.** Audit events are lines in the operator's log. Whoever controls the hub or the log pipeline can alter or drop them. Ship them to a store you control.
- **A join token in the wrong hands.** The token issued by "Add cluster" works for 15 minutes by default and cannot be revoked on its own; reissuing the cluster's credentials revokes it together with everything else.
- **Clocks that disagree.** Expiry is judged by the target cluster's clock. Keep clocks synchronised.
- **Supply chain.** From v0.1.2 the operator and agent images and the Helm chart are signed with cosign by the release workflow; earlier versions are not. No bill of materials is published yet. Nothing checks the signatures for you: verify them yourself, or enforce them with an admission policy.

To verify the chart of a release:

```sh
cosign verify ghcr.io/illeniumstudios/charts/protem:<version> \
  --certificate-identity https://github.com/iLLeniumStudios/protem/.github/workflows/release.yml@refs/tags/v<version> \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

The images `ghcr.io/illeniumstudios/protem:v<version>` and `protem-agent:v<version>` verify the same way.

Protem has not had an independent security review. It has been run on kind, k3d, AKS, EKS and GKE, with sign-in tested through Microsoft Entra ID, Keycloak and Google-issued tokens.

## Reporting a vulnerability

See [SECURITY.md](../../SECURITY.md).
