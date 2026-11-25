# How to enforce requester identity for kubectl requests

Make sure a request created outside the UI (`kubectl apply`, a pipeline) carries a requester
identity the API server trusts, instead of whatever `spec.requester` the manifest happens to say.

A request sent from the UI gets its requester from the login. A request created directly
carries whatever `spec.requester` the manifest names, and approval rules, the self-approval
check and the binding all trust it. `config/default` therefore installs an admission policy,
`protem-requester-identity`. To install it on its own:

```sh
kubectl apply -k config/admission
```

It is a `ValidatingAdmissionPolicy` (Kubernetes 1.30 or later, no webhook). The API server then
accepts a request only when `spec.requester.username` is the caller's own username, the email is
empty or equal to that username, and every listed group is one the caller belongs to.
`spec.requester.email` may be left out of the manifest entirely; the CRD no longer requires it.

## Let trusted automation request on someone else's behalf

A caller who may use the verb `delegate` on `accessrequests` in the request's namespace skips
these checks and may name anyone. The policy asks RBAC, not a list of names: the operator's
ClusterRole grants it `delegate`, because it creates requests for the person logged in to the
UI. To let trusted automation create requests for others, give it a Role with that verb next to
`create`:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: request-delegate
  namespace: <namespace of the requests>
rules:
- apiGroups: ["protem.dev"]
  resources: ["accessrequests"]
  verbs: ["create", "delegate"]
```

**Warning:** whoever holds `delegate` can request access in anyone's name, and approval rules
and the self-approval check then treat the request as that person's. Grant it as carefully as
the access the profiles give.

## If the API server adds a prefix

An API server that reads users from an OIDC token often puts a prefix on the names and groups it
reads (an EKS cluster's `usernamePrefix`/`groupsPrefix`, or `--oidc-username-prefix` /
`--oidc-groups-prefix` on a plain Kubernetes API server). The login to the UI carries no prefix,
so without more, a `kubectl`-created request must name the prefixed caller, and the resulting
binding then gets that prefix a second time from the subject template, matching nobody.

Set the matching prefix, for a Helm install:

```yaml
admissionPolicy:
  usernamePrefix: 'oidc:'
  groupsPrefix: 'oidc:'
```

or, for a kustomize install, the two `variables` at the top of
`config/admission/requester_identity.yaml`:

```yaml
variables:
- name: usernamePrefix
  expression: "'oidc:'"
- name: groupsPrefix
  expression: "'oidc:'"
```

With a prefix set, the policy strips it from the caller's name and groups before comparing, so
`spec.requester` is written exactly as a request from the UI would be: the bare name and group
ids. A caller whose name does not carry the prefix cannot create a request in their own name at
all, and neither can one whose name starts with `system:` once the prefix is removed; groups
like that are ignored. This was checked with an API server in the test suite enforcing the
policy with these two variables set (`internal/controller/admission_test.go`), not by a rerun
on EKS or Keycloak.

## Keep usernames consistent

Usernames must mean the same thing in both places. If people reach the UI as
`alice@company.com` but `kubectl` as `oidc:alice`, set the prefix as in
[If the API server adds a prefix](#if-the-api-server-adds-a-prefix) rather than asking people to
name themselves differently with `kubectl`: the policy then treats the two logins as the same
person instead of two.
