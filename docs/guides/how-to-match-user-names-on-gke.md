# How to match user names on GKE

Make the name Protem binds people as on a GKE cluster the same name that cluster gives them, so
an active grant gives access.

**Tested:** two runs. On 2026-11-25, chart 0.1.4, every result below not otherwise marked was seen
on two GKE Standard zonal clusters (Kubernetes 1.36, europe-north1), one as the hub and one as a
target cluster with an agent. Google service accounts stood in for people, signing in to the
hub's API with Google-signed ID tokens (issuer `https://accounts.google.com`). A second run, on
chart 0.1.5, used a Standard hub and an **Autopilot** target, with Microsoft Entra ID sign-in
through the web UI instead of direct Google tokens. Not tested in either run: the web UI with
Google sign-in (it needs an OAuth client), human Google accounts other than the project owner,
Google Groups for RBAC, and workforce identity federation.

## Find out which identity GKE gives a caller

```sh
kubectl --context <cluster> auth whoami
```

This always prints a Google principal's email as `Username`, with no prefix — a person's own
email, or an impersonated service account's email. `Groups` is always
`[system:authenticated]`; GKE's Kubernetes RBAC authorizer never sees IAM roles or Google Group
membership as Kubernetes groups.

## Keep the subject at the email

There is nothing to configure on the cluster: leave `TargetCluster.spec.subject` empty (it then
falls back to the chart's `subjectTemplate`, `{{.Email}}` by default), or set it to `{{.Email}}`
explicitly. With `{{.Username}}` instead, the binding named the numeric id the hub's HTTP API
reads from the token (see below) and gave no access: the grant showed `Active` and
`kubectl` was refused.

## What the hub reads from a token

With Google sign-in, the hub's HTTP API (the UI and `/api/*`, behind the OIDC ID token) resolves
`username` as the token's numeric `sub`, `email` as the email, and no groups: Google issues no
`groups` claim. Name people in `AccessProfile.spec.requesters.users` /
`approval.rules[].approvers.users` by email, never by the numeric id — nothing on GKE ever
produces that id as a Kubernetes username, so a profile that lists it matches no one. Give hub
admin rights with Kubernetes RBAC on `User <email>`.

With an identity provider that carries groups in front of the UI instead (Microsoft Entra ID, as
in the second run), `username` is the UPN, `email` is the person's `mail`, and groups work for
the UI and API too: the hub sends the token's group object ids in its SubjectAccessReviews, so
`requesters.groups` / `approvers.groups` can list them, and hub admin rights can instead be
`Group <group object id>`. `User <email>` is still the other option, same as with Google sign-in.

## IAM already grants access; Protem only adds to it

The project owner, and anyone holding a broad IAM role such as `roles/container.admin`, has
access through IAM regardless of what Protem binds: Protem's bindings add RBAC, they do not take
IAM access away. Give each person only an IAM role that lets them fetch cluster credentials
(`roles/container.clusterViewer` was used here, and gave no access inside either cluster before
any grant existed) and let Protem grant the rest.

## Requests created with kubectl

Name yourself by your email in `spec.requester.username`.

On 0.1.4, a request that set only `username` and left `email` out was admitted and approved,
but the UI did not treat it as the same person's as her requests made through the UI: she could
not cancel it there until the manifest also set `email` to her address. From 0.1.5 a missing
email stands for the username, so both forms are the same person; this was confirmed live on
GKE, not only by the test suite: a request naming only `username` showed up on the requester's
**My requests** page and she could cancel it there, and a second such request was correctly
refused as a duplicate of a UI-created grant for the same person, in both directions.

On GKE, `kubectl` never carries identity-provider groups: every caller's token groups are empty
beyond `system:authenticated`, whichever identity provider fronts the UI. A manifest that lists
`spec.requester.groups` is refused by the admission policy ("spec.requester.groups may only list
groups you belong to"), and a profile that names only groups in `requesters.groups` rejects a
kubectl-created request with `UnauthorizedRequester`, since it can never prove group membership
this way. A kubectl-created request works only for a profile that also lists the person by email
in `requesters.users`.

## What else is different on GKE

- **The Clusters page shows a cluster's allowed roles followed by "or more"**, as on EKS: GKE
  Standard clusters also layer a webhook authorizer in front of RBAC, so the agent's own
  `SelfSubjectRulesReview` comes back `incomplete: true` and it reports its allowed roles as
  partial. See
  [How to match user names on EKS](how-to-match-user-names-on-eks.md#what-else-is-different-on-eks).
- **NetworkPolicy is enforced by default** (Dataplane V2/eBPF on a new GKE Standard cluster), not
  merely accepted. The agent's own deny-ingress `NetworkPolicy` is therefore live from the first
  apply; it caused no problem here, since it only denies ingress and the agent only needs
  outbound access to the hub.

### Autopilot

- **Autopilot raises the agent's resource requests to its own minimums**, with a warning on
  `kubectl apply`: cpu `10m` to `50m`, memory `32Mi` to `52Mi`, and adds an ephemeral-storage
  request and limit of `1Gi` (the memory limit was unchanged). The chart's security context
  (`runAsNonRoot`, seccomp `RuntimeDefault`, no privilege escalation, dropped capabilities, a
  read-only root filesystem) and its `NetworkPolicy` were accepted unchanged, with no admission
  denial.
- **A fresh Autopilot cluster has zero nodes**, so the agent's pod stays `Pending` until a
  scale-up finishes; it waited 3m20s here, inside the 15-minute join token's lifetime but longer
  than "within a minute". See
  [How to connect a target cluster](how-to-connect-a-target-cluster.md#4-what-gets-installed-on-the-target-cluster).
- **The allowed-roles list is "or more" here too**: Autopilot's webhook authorizer behaves the
  same as Standard's for `SelfSubjectRulesReview`.

## Troubleshooting: `kubectl` keeps acting as the wrong identity

If `kubectl auth whoami` prints the real signed-in Google account instead of the identity you
meant to act as, check that first, before suspecting Protem's subject template.
`CLOUDSDK_AUTH_IMPERSONATE_SERVICE_ACCOUNT`, even placed in a generated kubeconfig's own
`exec.env`, did not change which identity `gke-gcloud-auth-plugin` authenticated as in this run.
A kubeconfig built from a static bearer token
(`gcloud auth print-access-token --impersonate-service-account=<service account>`) authenticated
reliably as the intended identity; it expires in about an hour and must be regenerated.

## What else is different on GKE

- **The Clusters page shows a cluster's allowed roles followed by "or more"**, as on EKS: GKE
  Standard clusters also layer a webhook authorizer in front of RBAC, so the agent's own
  `SelfSubjectRulesReview` comes back `incomplete: true` and it reports its allowed roles as
  partial. See
  [How to match user names on EKS](how-to-match-user-names-on-eks.md#what-else-is-different-on-eks).
- **NetworkPolicy is enforced by default** (Dataplane V2/eBPF on a new GKE Standard cluster), not
  merely accepted. The agent's own deny-ingress `NetworkPolicy` is therefore live from the first
  apply; it caused no problem here, since it only denies ingress and the agent only needs
  outbound access to the hub.
