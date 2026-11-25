# How to match user names on GKE

Make the name Protem binds people as on a GKE cluster the same name that cluster gives them, so
an active grant gives access.

**Tested:** every result on this page was seen on 2026-11-25 on two GKE Standard zonal clusters
(Kubernetes 1.36, europe-north1), one as the hub and one as a target cluster with an agent, chart
0.1.4. Google service accounts stood in for people, signing in to the hub's API with
Google-signed ID tokens (issuer `https://accounts.google.com`). Not tested: the web UI with
Google sign-in (it needs an OAuth client), human Google accounts other than the project owner,
Google Groups for RBAC, Autopilot, and workforce identity federation.

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

## What the hub reads from a Google token

The hub's HTTP API (the UI and `/api/*`, behind the OIDC ID token) resolves `username` as the
token's numeric `sub`, `email` as the email, and no groups: Google issues no `groups` claim.
Name people in `AccessProfile.spec.requesters.users` / `approval.rules[].approvers.users` by
email, never by the numeric id — nothing on GKE ever produces that id as a Kubernetes username,
so a profile that lists it matches no one. Give hub admin rights with Kubernetes RBAC on `User
<email>`.

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
email stands for the username, so both forms are the same person. The 0.1.5 behaviour is covered
by the test suite, not by a rerun on GKE.

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
