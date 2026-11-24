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

## Keep usernames consistent

Usernames must mean the same thing in both places. If people reach the UI as
`alice@company.com` but `kubectl` as `oidc:alice`, their kubectl requests are a different
person to the approval rules, and their bindings use the kubectl name.
