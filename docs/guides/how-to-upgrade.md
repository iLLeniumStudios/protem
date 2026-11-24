# How to upgrade

Upgrade the hub and the agent on each target cluster to a new Protem release.

## The hub

Helm and OLM each cover their own upgrade already:

- Helm: see [How to install the hub with Helm](how-to-install-with-helm.md#upgrade) — apply the
  new release's CRDs yourself before `helm upgrade`, since Helm never changes CRDs after the
  first install.
- OLM: see [How to install with OLM](how-to-install-with-olm.md).

## The agent

There is no separate agent Helm chart (`config/agent/spoke` is a kustomize base, not packaged in
the hub chart), so upgrading the agent on a target cluster means rebuilding its image and
redeploying the same overlay you used in
[How to connect a target cluster](how-to-connect-a-target-cluster.md):

```sh
make docker-build-agent docker-push-agent AGENT_IMG=registry.example.com/protem-agent:v<new version>
```

Then either bump the image tag in the overlay you already applied and re-apply it:

```sh
kubectl --context prod-eu apply -k clusters/prod-eu-agent
```

or, if you added the cluster from the UI and want the agent image and manifests the hub is
currently configured with (`--agent-image`, `--agent-manifests`), re-run its join flow: open the
cluster's actions menu and choose **Connect agent**, save the new files, and apply them. Either
way the Deployment replaces its one replica before starting the next, so access is never enforced
by two agent versions at once.
