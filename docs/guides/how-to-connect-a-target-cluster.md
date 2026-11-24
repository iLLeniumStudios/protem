# How to connect a target cluster

Register a cluster on the hub and run the agent on it, so profiles can grant access there. This page also covers pausing a cluster and getting a disconnected agent back.

The agent (`cmd/enforcer`) runs on the target cluster with its own ServiceAccount, and reaches the hub with a kubeconfig whose token it renews itself. Why agents pull from the hub, and what happens while the hub is unreachable, is in [Hub and agents](../architecture/hub-and-agents.md).

## By hand or with GitOps

The steps below use `prod-eu` as the cluster name. Replace it everywhere.

### 1. Register the cluster on the hub

```sh
kubectl --context hub apply -f - <<EOF
apiVersion: protem.dev/v1alpha1
kind: TargetCluster
metadata:
  name: prod-eu
  labels:
    env: prod
spec:
  displayName: Production EU
EOF
```

Profiles select clusters by these labels or by name. The other fields are in
[Custom resources](../reference/custom-resources.md#targetcluster).

### 2. Check the agent's identity on the hub

Once the TargetCluster exists, the operator creates the namespace `protem-cluster-prod-eu` and,
in it, a ServiceAccount, Role and RoleBinding named `protem-agent`, and keeps them as they should
be. The Role lets the agent list ClusterGrants, update their status, create and renew its Lease,
and request new tokens for its own ServiceAccount, in this one namespace and nowhere else.
`config/agent/hub` holds the same three objects for reference.

```sh
kubectl --context hub -n protem-cluster-prod-eu get serviceaccount,role,rolebinding protem-agent
```

### 3. Mint the first token

The agent needs this token only to start; it then renews its own token, so a short one is
enough. Deploy the agent before it expires.

```sh
TOKEN=$(kubectl --context hub -n protem-cluster-prod-eu create token protem-agent --duration 24h)
```

Put it in a kubeconfig for the hub:

```sh
SERVER=$(kubectl --context hub config view --minify -o jsonpath='{.clusters[0].cluster.server}')
kubectl --context hub config view --minify --raw \
  -o jsonpath='{.clusters[0].cluster.certificate-authority-data}' | base64 -d > hub-ca.crt

kubectl config --kubeconfig hub.kubeconfig set-cluster hub --server "$SERVER" \
  --certificate-authority hub-ca.crt --embed-certs
kubectl config --kubeconfig hub.kubeconfig set-credentials protem-agent --token "$TOKEN"
kubectl config --kubeconfig hub.kubeconfig set-context hub --cluster hub --user protem-agent
kubectl config --kubeconfig hub.kubeconfig use-context hub
```

### 4. What gets installed on the target cluster

`config/agent/spoke` creates the namespace `protem-system`, a ServiceAccount `protem-agent`, a
ClusterRole that lets the agent manage RoleBindings and ClusterRoleBindings, read Roles and
ClusterRoles, and bind only the roles named in `resourceNames`, a Role that lets it read and
update the one Secret `protem-agent-hub` (where it keeps its renewed hub token), and the
agent's Deployment. You apply it in step 6, through an overlay that adds the cluster name, the
image and the hub kubeconfig.

### 5. Set the ceiling

The `bind` rule's `resourceNames` in `config/agent/spoke/cluster_role.yaml` lists the only roles
the agent can bind on this cluster. It ships as `view` and `edit`. Add each role an
AccessProfile for this cluster names, before anyone requests it:

```yaml
  resourceNames:
  - view
  - edit
  - db-operator
```

[How to limit the roles a cluster allows](how-to-limit-the-roles-a-cluster-allows.md) covers
changing the list on a running cluster. Why the list lives on the target cluster is in
[Hub and agents](../architecture/hub-and-agents.md#why-each-cluster-has-a-role-ceiling).

### 6. Deploy the agent

Build and push the image, then apply an overlay from the repository root. The hub kubeconfig
from step 3 has to sit inside the overlay directory, because kustomize reads files only from
there. Keep it out of git. The Secret keeps its exact name `protem-agent-hub`, without
kustomize's hash suffix, because the agent writes its renewed token there by name.

```sh
make docker-build-agent docker-push-agent AGENT_IMG=registry.example.com/protem-agent:v0.1.0

mkdir -p clusters/prod-eu-agent
mv hub.kubeconfig clusters/prod-eu-agent/
cat > clusters/prod-eu-agent/kustomization.yaml <<EOF
resources:
- ../../config/agent/spoke
images:
- name: protem-agent
  newName: registry.example.com/protem-agent
  newTag: v0.1.0
configMapGenerator:
- name: protem-agent
  namespace: protem-system
  literals:
  - cluster-name=prod-eu
secretGenerator:
- name: protem-agent-hub
  namespace: protem-system
  files:
  - kubeconfig=hub.kubeconfig
  options:
    disableNameSuffixHash: true
EOF
kubectl --context prod-eu apply -k clusters/prod-eu-agent
```

The hub must be reachable from the target cluster at the `server` address in that kubeconfig.
Within a minute the cluster shows as Ready on the hub's Clusters page, and its Lease is fresh:

```sh
kubectl --context hub get targetcluster prod-eu
kubectl --context hub -n protem-cluster-prod-eu get lease agent
```

The Deployment runs one replica and replaces it before starting the next, so two agents never
write the same grant's status. To pass more flags, patch the container's `args`; every flag is
in [Agent flags](../reference/agent-flags.md).

#### Running it outside the cluster instead

```sh
go build -o bin/protem-agent ./cmd/enforcer
bin/protem-agent \
  --cluster-name prod-eu \
  --hub-kubeconfig hub.kubeconfig \
  --target-kubeconfig target.kubeconfig
```

Mint `target.kubeconfig` for `protem-system/protem-agent` on the target cluster the same way as in
step 3. With your own kubeconfig instead, the agent acts with your permissions, not the
ClusterRole's. One of `--hub-kubeconfig` or `--hub-context` is required. Without
`--hub-token-secret` the agent keeps its renewed hub token in memory only, so after a restart it
starts again from the token in `hub.kubeconfig`.

## From the UI

This needs the operator started with `--ui-management=enabled` (chart value
`uiManagement: enabled`, or `config/rbac/ui_management_role.yaml` applied with kustomize) and with
`--hub-api-url` (chart value `hubAPIURL`) set to the hub's Kubernetes API address as the target
clusters reach it, for example `https://hub.example.com:6443`. Without it the UI can add clusters
but refuses to issue join tokens. `--agent-image` and `--agent-manifests` set what the generated
files install; see [Operator flags](../reference/operator-flags.md).

1. On the Clusters page, choose **Add cluster**. The name is a DNS label of at most 40 characters
   and cannot be changed later. This needs `create` on `targetclusters` on the hub.
2. Save the two files the UI shows, `hub.kubeconfig` and `kustomization.yaml`, in one empty
   directory. Keep `hub.kubeconfig` out of git: it holds the join token.
3. In that directory, run `kubectl apply -k .` against the target cluster.

The join token works for `--join-token-ttl` (default 15 minutes, allowed 10m to 1h), so run the
command soon. The agent replaces the join token with its own renewing token as soon as it starts.
If the token runs out first, open the cluster's actions menu, choose **Connect agent**, and apply
the new files.

**Reissue credentials**, in the same menu, deletes and recreates the agent's ServiceAccount on the
hub. That cuts off every token issued for it at once, the join token and the running agent's own,
so the agent stops reaching the hub until you apply the new files on the target cluster. Use it
when a token may have leaked. Issuing and reissuing need `update` on that TargetCluster.

**Remove cluster** deletes the TargetCluster; it is refused while an active grant targets the
cluster (revoke those first) or while the TargetCluster is managed by a GitOps tool. The operator
then deletes the agent's ServiceAccount, which cuts the agent off, its RoleBinding, its Lease and
any ClusterGrants left in the namespace; the namespace and its empty Role remain; delete the
namespace to clear them. The agent on the target cluster is not uninstalled: run
`kubectl delete -k .` there.

The ceiling in [Set the ceiling](#5-set-the-ceiling) applies to clusters added from the UI too.
To ship a different ceiling, point `--agent-manifests` at your own copy of `config/agent/spoke`.

## Pause a cluster

A paused cluster takes no new requests; access already live there keeps running until it ends.

```sh
kubectl --context hub patch targetcluster prod-eu --type merge -p '{"spec":{"paused":true}}'
```

Set it back to `false` to accept requests again. In Git, set `spec.paused: true` on the
TargetCluster.

From the UI (with `--ui-management=enabled`): on the Clusters page, open the cluster's actions
menu, choose **Edit settings**, tick **Paused** and choose **Save settings**. Clusters managed by
a GitOps tool can only be paused in Git.

## Restore an agent whose hub token expired

If the hub was unreachable for longer than the agent's token lifetime (`--hub-token-ttl`, default
`24h`), the agent cannot renew its token and cannot reach the hub on its own.

- **Added by hand:** mint a new first token (step 3), then replace the `kubeconfig` key of the
  Secret `protem-system/protem-agent-hub` and delete its `token` key:

  ```sh
  kubectl --context prod-eu -n protem-system create secret generic protem-agent-hub \
    --from-file=kubeconfig=hub.kubeconfig --dry-run=client -o yaml | kubectl --context prod-eu apply -f -
  kubectl --context prod-eu -n protem-system rollout restart deployment protem-agent
  ```

  Applying the Secret this way replaces its data, so the old `token` key is gone too.
- **Added from the UI:** choose **Connect agent** (or **Reissue credentials** if the token may
  have leaked) and apply the new files on the target cluster.
