# How to match user names on EKS

Make the name Protem binds people as on an EKS cluster the same name that cluster gives them, so
an active grant gives access.

**Tested:** every result on this page was seen on 2026-11-25 on two EKS clusters (Kubernetes
1.37, eu-north-1), one as the hub and one as a target cluster with an agent, with Microsoft Entra
ID as the identity provider. It was run on chart 0.1.3 and again on 0.1.4; where the two
behave differently, the page says which is which. A role assumed by hand with
`--role-session-name` stood in for IAM Identity Center. Not tested: IAM Identity Center itself.

Kubernetes keeps no list of users. A binding for a name the API server never produces is created
without error and matches nobody, so the grant shows as active and gives no access. EKS names a
person in one of three ways. Find out which one your cluster uses, then follow that section:

```sh
kubectl --context <cluster> auth whoami
```

- `Username` starts with the prefix of an OIDC identity provider, for example
  `oidc:alice@example.com`: see [An OIDC identity provider](#an-oidc-identity-provider).
- `Username` is a name an administrator chose for an IAM role: see
  [An access entry with a user name per person](#an-access-entry-with-a-user-name-per-person).
- `Username` is `arn:aws:sts::<account>:assumed-role/<role>/<session name>`: see
  [A shared IAM role named by its session](#a-shared-iam-role-named-by-its-session).

## An OIDC identity provider

1. Associate the identity provider with the cluster only after `eksctl` has reported the cluster
   ready, not as soon as `aws eks describe-cluster` reports `ACTIVE`. Associating it at `ACTIVE`
   put both test clusters' CloudFormation stacks into `CREATE_FAILED` and then
   `ROLLBACK_FAILED` (`reached unexpected status UPDATING during stabilization`), and both
   clusters had to be created again.

   ```sh
   aws eks associate-identity-provider-config --cluster-name <cluster> \
     --oidc identityProviderConfigName=entra,issuerUrl=https://login.microsoftonline.com/<tenant id>/v2.0,clientId=<client id>,usernameClaim=preferred_username,usernamePrefix=oidc:,groupsClaim=groups,groupsPrefix=oidc:
   ```

   With these settings, `kubectl auth whoami` printed `oidc:alice@example.com` and the groups as
   `oidc:<group object id>`.

2. Put the username prefix in the subject of each target cluster:

   ```sh
   kubectl --context hub patch targetcluster <cluster> --type merge -p '{"spec":{"subject":"oidc:{{.Username}}"}}'
   ```

   With this subject, alice's grant of `edit` in `payments` let her list pods and create a
   ConfigMap there, and she was refused in `kube-system`. With the subject `{{.Email}}` instead,
   the grant was `Active` and the binding named `alice@example.com`, and she was refused in
   `payments` too.

3. If the hub runs on EKS with the same identity provider, set the chart's `subjectTemplate` to
   `oidc:{{.Username}}`. The hub's own cluster always binds with this template, whatever its
   `TargetCluster` says. Leave the Kubernetes RBAC that gives people Protem admin rights on the
   hub naming groups by their bare object id: the hub checks those rights with the groups it
   read from the login, which carry no prefix.

4. On such a hub, set the two prefixes of the requester-identity admission policy too (version
   0.1.4 and later):

   ```yaml
   admissionPolicy:
     usernamePrefix: 'oidc:'
     groupsPrefix: 'oidc:'
   ```

   Then a request created with `kubectl` names the person and their groups without the prefix,
   as a request created in the UI does, and profiles list groups by their bare object id in both
   `requesters.groups` and `approvers.groups`. See
   [How to enforce requester identity for kubectl requests](how-to-enforce-requester-identity-for-kubectl-requests.md#if-the-api-server-adds-a-prefix).

   What was seen on 0.1.3, which has no such settings: a `kubectl` request had to name the
   requester `oidc:alice@example.com` and list the group as `oidc:<group object id>`, so the
   profile had to list the prefixed group id. The binding for that request named
   `oidc:oidc:alice@example.com`, because `oidc:{{.Username}}` added the prefix a second time,
   and it gave no access.

   On 0.1.4 with the two settings: alice's `kubectl` request naming her bare user name and the
   bare group id, with or without `email`, was admitted, and its binding named
   `oidc:alice@example.com` once and gave her access. The prefixed user name, the prefixed
   group, a group she is not in, and naming someone else were refused. A request through
   `kubectl` and one through the UI for the same profile were refused as duplicates of each
   other.

## An access entry with a user name per person

Give each person their own IAM role, and give that role an access entry whose user name is
exactly the name Protem binds them as on this cluster. An IAM principal can be in only one access
entry ([Create access entries](https://docs.aws.amazon.com/eks/latest/userguide/creating-access-entries.html)).

1. Create the access entry:

   ```sh
   aws eks create-access-entry --cluster-name <cluster> \
     --principal-arn arn:aws:iam::<account>:role/<role> \
     --username 'oidc:alice@example.com'
   ```

   EKS refused user names starting with `system:`, `eks:`, `aws:`, `amazon:` or `iam:`
   (`The username must not begin with system:`). `oidc:` was accepted.

2. Leave the target cluster's subject rendering that same name, here `oidc:{{.Username}}`.

With alice's grant active, `kubectl auth whoami` as the role printed `oidc:alice@example.com`,
the role could list pods and create a ConfigMap in `payments`, and it was refused in
`kube-system`. After the grant was revoked it was refused in `payments` too. Protem's binding
did not change: it matches the name, whichever way the person signed in.

## A shared IAM role named by its session

This is the pattern IAM Identity Center uses: one role for many people, told apart by the
session name.

1. Create the role's access entry with a user name that keeps the session name as given
   ([Create access entries](https://docs.aws.amazon.com/eks/latest/userguide/creating-access-entries.html)):

   ```sh
   aws eks create-access-entry --cluster-name <cluster> \
     --principal-arn arn:aws:iam::<account>:role/<role> \
     --username 'arn:aws:sts::<account>:assumed-role/<role>/{{SessionNameRaw}}'
   ```

   With `{{SessionNameRaw}}`, a session named `alice@example.com` showed in
   `kubectl auth whoami` as `arn:aws:sts::<account>:assumed-role/<role>/alice@example.com`.

2. Set the target cluster's subject to render the same name:

   ```yaml
   spec:
     subject: 'arn:aws:sts::<account>:assumed-role/<role>/{{.Username}}'
   ```

   Use `{{.Username}}` or `{{.Email}}`, whichever the session name is.

With this, alice's grant of `view` was bound to
`arn:aws:sts::<account>:assumed-role/<role>/alice@example.com`. A session of the role named for
alice could list pods in `payments` and was refused in `kube-system`; a session of the same
role named for bob was refused in `payments`.

Do not rely on the default user name, which an access entry created without `--username`
gets: `arn:aws:sts::<account>:assumed-role/<role>/{{SessionName}}`. With it, EKS rewrote `@` to
`-` in the user name RBAC sees (`.../<role>/alice-example.com`, while `Extra: sessionName` kept
`alice@example.com`), so a subject built from an email address or an Entra ID UPN did not match.

Session names are limited by AWS STS. Each must match `[\w+=,.@-]*` and be at most 64
characters. Names with `'`, `#` or `~`, and a 72-character name, were refused with
`failed to satisfy constraint`, an error that does not mention Kubernetes or Protem.

## What else is different on EKS

- **The Clusters page shows a cluster's allowed roles followed by "or more"**, for example
  `edit view or more`. EKS's webhook authorizer cannot list the agent's own permissions
  (`webhook authorizer does not support user rule resolution`), so the agent reports the list
  of roles it may bind as partial. It still read `view` and `edit` from its `bind` rule. This is
  not EKS-specific: the same was seen on GKE, see
  [How to match user names on GKE](how-to-match-user-names-on-gke.md#what-else-is-different-on-gke).
- **Associating an OIDC identity provider too soon after creating the cluster fails the
  cluster's CloudFormation stack**, as described in step 1 of
  [An OIDC identity provider](#an-oidc-identity-provider).
