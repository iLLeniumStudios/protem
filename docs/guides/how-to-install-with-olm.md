# How to install with OLM

Operator Lifecycle Manager (OLM) installs and upgrades operators from a catalog image. Each
Protem release publishes one, `ghcr.io/illeniumstudios/protem-catalog:v<version>`, holding the
package `protem` with the single channel `alpha`. You need Kubernetes 1.30 or later and OLM v0
(built into OpenShift; elsewhere `operator-sdk olm install`). OLM installs the operator on the
hub only: target clusters run an agent, see
[How to connect a target cluster](how-to-connect-a-target-cluster.md). To install with Helm
instead, see [How to install with Helm](how-to-install-with-helm.md).

## Install

1. Write `protem-olm.yaml`, with the release you want in the catalog image tag:

   ```yaml
   apiVersion: v1
   kind: Namespace
   metadata:
     name: protem-system
   ---
   apiVersion: operators.coreos.com/v1alpha1
   kind: CatalogSource
   metadata:
     name: protem
     namespace: protem-system
   spec:
     sourceType: grpc
     image: ghcr.io/illeniumstudios/protem-catalog:latest
     displayName: Protem
     publisher: iLLeniumStudios
   ---
   # No targetNamespaces: the operator watches all namespaces.
   apiVersion: operators.coreos.com/v1
   kind: OperatorGroup
   metadata:
     name: protem
     namespace: protem-system
   ---
   apiVersion: operators.coreos.com/v1alpha1
   kind: Subscription
   metadata:
     name: protem
     namespace: protem-system
   spec:
     channel: alpha
     name: protem
     source: protem
     sourceNamespace: protem-system
     config:
       env:
       - name: PROTEM_OIDC_ISSUER_URL
         value: https://idp.example.com/realms/example
       - name: PROTEM_OIDC_CLIENT_ID
         value: protem
   ```

   Use a namespace of its own: a namespace may hold only one OperatorGroup, and OLM's
   `operators` namespace already has one. Replace the issuer and client ID with your identity
   provider's: the operator reads the issuer's discovery document when it starts, and does not
   start if it cannot reach it.

2. Apply it and wait for the phase `Succeeded`:

   ```sh
   kubectl apply -f protem-olm.yaml
   kubectl -n protem-system get csv -w
   ```

## Set options

Every operator flag can be set as an environment variable: `--some-name` reads
`PROTEM_SOME_NAME`, and a flag given on the command line wins over the variable. OLM accepts
environment variables, not arguments, so options go in the Subscription's `spec.config.env`.
`kubectl -n protem-system exec deploy/protem-controller-manager -- /manager --help` lists every
flag, and so does the [operator flags reference](../reference/operator-flags.md). A value that does not parse
stops the operator at startup with the variable's name in its log.

Edit the Subscription to change an option; OLM restarts the operator with the new value. If
the operator keeps crashing after you correct a bad value, delete its Deployment
(`kubectl -n protem-system delete deployment protem-controller-manager`): OLM may leave the
failing one in place, and recreates it with the Subscription's current values within a few
minutes.

## Add the admission policy

The bundle installs no `ValidatingAdmissionPolicy`, because OLM bundles cannot carry one.
Without it, a request created with `kubectl` is not checked against the caller's identity: its
`spec.requester` is whatever the manifest says, and approval rules and the binding trust it.
Requests sent from the UI are not affected. To add the policy, apply it from the release you
installed:

```sh
kubectl apply -k 'https://github.com/iLLeniumStudios/protem//config/admission?ref=v<version>'
```

Replace `<version>` with the installed version, which `kubectl -n protem-system get csv` shows.

What it checks is in
[How to enforce requester identity for kubectl requests](how-to-enforce-requester-identity-for-kubectl-requests.md).
Delete it with `kubectl delete -k` and the same URL.

## After installing

1. **Login.** OLM installs no proxy and no Service for the UI. The operator serves the UI and
   API on port 8080 and accepts only requests carrying an ID token from the configured issuer.
   Run an OIDC-aware proxy, such as oauth2-proxy, as its own Deployment with the settings in
   [How to set up login](how-to-set-up-login.md), and point it at this Service:

   ```yaml
   apiVersion: v1
   kind: Service
   metadata:
     name: protem-ui
     namespace: protem-system
   spec:
     selector:
       control-plane: controller-manager
       app.kubernetes.io/name: protem
     ports:
     - name: http
       port: 8080
       targetPort: 8080
   ```

2. **The hub as a cluster.** To grant access on the hub itself, create a `TargetCluster` named
   like `PROTEM_HUB_CLUSTER_NAME` (default `local`). The operator marks it Ready without an
   agent, and may bind only the `view` and `edit` roles there.
3. **Target clusters.** Connect each one with an agent:
   [How to connect a target cluster](how-to-connect-a-target-cluster.md). Connecting from the
   UI needs `PROTEM_UI_MANAGEMENT=enabled`, `PROTEM_HUB_API_URL`, and the ClusterRole in
   `config/rbac/ui_management_role.yaml` bound to the ServiceAccount
   `protem-controller-manager` in `protem-system`, which the bundle does not install.

## Upgrade

Point the CatalogSource at the new release's catalog:

```sh
kubectl -n protem-system patch catalogsource protem --type merge \
  -p '{"spec":{"image":"ghcr.io/illeniumstudios/protem-catalog:v<new version>"}}'
```

OLM then replaces the operator and updates the CRDs. Each catalog may replace any earlier
version directly. To review upgrades before they happen, set `installPlanApproval: Manual` in
the Subscription and approve each InstallPlan.

## Remove

Removing the operator leaves the CRDs, and with them every profile, request, grant and cluster,
the bindings of grants that are still active, and the bundle's `protem-*` ClusterRoles. To
remove everything, end all access and remove the clusters first, while the operator still
runs: grants and target clusters carry a finalizer that only the operator removes, together
with the bindings and agent credentials it created.

```sh
kubectl delete accessrequests,accessgrants --all --all-namespaces
kubectl delete targetclusters --all
kubectl -n protem-system delete subscription protem
kubectl -n protem-system delete csv -l operators.coreos.com/protem.protem-system
kubectl delete crd accessgrants.protem.dev accessprofiles.protem.dev accessrequests.protem.dev \
  clustergrants.protem.dev targetclusters.protem.dev
kubectl get clusterroles -o name | grep '^clusterrole.rbac.authorization.k8s.io/protem-' | xargs -r kubectl delete
kubectl get namespaces -o name | grep '^namespace/protem-cluster-' | xargs -r kubectl delete
kubectl delete namespace protem-system
```
