# How to fix a grant that gives no access

Find out why an active grant does not let its holder do anything on a target cluster, and fix
the name Protem binds people as there.

**Notice:** a grant's detail page shows a cluster as `Active`, but `kubectl` on that cluster
refuses the person with `Forbidden` for exactly what the grant should allow.

**Cause.** The agent bound a Kubernetes `User` name that is not the name the cluster's API server
gives that person. Kubernetes keeps no list of users, so the RoleBinding is created without error
and simply never matches anyone. The name comes from the TargetCluster's `spec.subject`, or from
the operator's `--subject-template` (default `{{.Email}}`) when the cluster sets none.

**Confirm.** As the person, with `kubectl` pointed at the target cluster, ask what the cluster
calls them:

```sh
kubectl --context <cluster> auth whoami
```

Then look at the subject the grant's binding names (the grant's detail page shows it as **Bound
subject**):

```sh
kubectl --context <cluster> get rolebinding -n <namespace> -o wide
```

For a cluster-wide grant, use `get clusterrolebinding -o wide`. If the `USERS` column differs from
the `Username` that `auth whoami` printed, this is the cause.

**Act.** Set the cluster's `spec.subject` so it renders the name `auth whoami` printed, using
`{{.Email}}` or `{{.Username}}` and any literal text around them:

- Microsoft Entra ID on AKS: `{{.Username}}`.
- A cluster whose OIDC setup adds a prefix (`--oidc-username-prefix`) needs that prefix as
  literal text, for example `oidc:{{.Email}}`.
- A subject that already has the prefix built in, used against a caller whose name the API
  server already prefixed (for example an admission-policy `usernamePrefix` or a
  `--oidc-username-prefix`), binds the prefix twice: `oidc:oidc:alice` matches nobody. The
  prefix belongs in the subject template, or in the identity, never both.
- On EKS, a shared IAM role named by its session can have its user name rewritten: EKS replaces
  `@` with `-` in the name RBAC sees, so a subject built from an email address does not match
  the session. See [How to match user names on EKS](../../guides/how-to-match-user-names-on-eks.md).

```sh
kubectl --context hub patch targetcluster <cluster> --type merge -p '{"spec":{"subject":"{{.Username}}"}}'
```

From the UI (with `--ui-management=enabled`): on the Clusters page, open the cluster's actions
menu, choose **Edit settings**, and change **Subject**. The dialog shows the name you would be
bound as while you type.

A grant already issued keeps the name it was bound with. The person requests access again, and
the new grant is bound with the corrected name.

The hub's own cluster is the exception: the operator binds people there with its own
`--subject-template` (the chart's `subjectTemplate`) and ignores that cluster's `spec.subject`.
Change the operator's template instead.
