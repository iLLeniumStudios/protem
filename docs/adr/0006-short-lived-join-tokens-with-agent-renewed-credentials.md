# Short-lived join tokens, agent-renewed credentials

Connecting a target cluster uses a join token that expires in minutes. Once connected, the agent renews its own longer-lived hub token itself; the hub never has to push a new one to it.

**Status:** Accepted, 2026-11-24.

## Context

Enrolling a new agent needs a credential handed to it once, out of band (copied into a kustomization applied to the target cluster). If that credential were long-lived, it would sit in a file and in shell history for as long as the join takes, with no expiry if the join is abandoned partway. The credential the agent uses afterwards, for its day-to-day connection to the hub, has the opposite requirement: it should outlive a single join without the hub needing a side channel to push a replacement to it.

## Decision

Two different token lifetimes meet these needs:

- **Join token.** `issueCredentials` in `internal/api/enrol.go` requests a `TokenRequest` for the agent's ServiceAccount with `--join-token-ttl` as its `ExpirationSeconds` (default `api.DefaultJoinTokenTTL`, 15 minutes; the flag accepts 10 minutes to 1 hour). It is embedded in the kubeconfig the UI hands out and is good for exactly long enough to apply the join files.
- **Hub token.** `cmd/enforcer` requests its own longer-lived token (`--hub-token-ttl`, default 24 hours, minimum 10 minutes) and `internal/enforcer/hubtoken.go`'s `HubTokenRenewer` renews it once a third of its lifetime has passed (`renewDelay`), retrying with backoff on failure. No hub-initiated push is involved; the agent always asks for its own next token and, if `--hub-token-secret` is set, stores the renewed token in a Secret on the target cluster so a restart does not fall back to an older, possibly-revoked one.

"Reissue credentials" in the UI, or deleting the agent's ServiceAccount, replaces the ServiceAccount outright (`fleet.ReplaceAgentAccount`), invalidating every token issued for the old one at once, including any the agent is still renewing.

## Consequences

- A join token found later than 15 minutes (default) after issue is already useless; it cannot be used to enrol a new cluster and reveals nothing about the cluster's ongoing access.
- Normal operation needs no hub-to-agent channel for credential delivery: the agent always initiates the renewal request.
- An agent that cannot reach the hub for longer than its hub token's lifetime is locked out until it is given new credentials (same limit noted in the security model's "When the hub is unreachable" section).
- Reissuing credentials is immediate and total: every earlier token for that cluster, including one an attacker captured, stops working at once, at the cost of needing the new join files reapplied to reconnect the legitimate agent.
