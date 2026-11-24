# Protem

Just-in-time privileged access for Kubernetes fleets: people request time-boxed elevation, approvers decide, and the hub and its target clusters enforce it as RBAC bindings that expire.

## Product

**Protem**:
The product: time-boxed Kubernetes access across clusters. From "pro tem", holding an office for the time being.

## Access lifecycle

**Profile**:
A named, requestable elevation: which role, on which clusters and namespaces, for how long, and under which approval rules.
_Avoid_: role, template, entitlement

**Request**:
A person's ask for one profile's elevation, with a justification; its scope and requirements are frozen when it is resolved.
_Avoid_: ticket, elevation request

**Grant**:
The time-boxed elevation a request produces once approved; it owns the active, expired, and revoked lifecycle.
_Avoid_: session, binding, assignment

**Cluster grant**:
The slice of a grant one target cluster must enforce, carrying only what that cluster needs.
_Avoid_: ClusterAccessGrant, fan-out object

## Approval

**Requester**:
The person who submitted a request.
_Avoid_: owner, user

**Same person**:
Two identities are the same person when a non-empty username matches or a non-empty email matches; empty values never match.

**Approval rule**:
An entry in a profile's approval rules saying which clusters it covers, whether approval is automatic or manual, and who approves.
_Avoid_: policy, approval policy

**Requirement**:
What one approval rule demands for one resolved cluster of a request: the mode, the approvers, and how many distinct approvals are needed.
_Avoid_: condition, rule (when you mean the per-cluster instance)

**Approver**:
A person named, directly or through a group, by a requirement.
_Avoid_: reviewer

**Eligibility**:
Whether a person may decide on a requirement: they are its approver, it is manual, and they are not the requester unless the rule allows self-approval.

**Decision**:
One person's recorded Approved, Rejected, or Cancelled on a request.
_Avoid_: vote, review

**Rejection**:
An eligible approver refusing a pending request; it ends the request as Rejected.
_Avoid_: denial

**Cancellation**:
The requester withdrawing their own pending request; it ends the request as Cancelled.
_Avoid_: self-rejection, withdrawal

## Fleet

**Hub**:
The cluster that holds profiles, requests, and grants and fans grants out to target clusters.
_Avoid_: management cluster, control plane

**Target cluster**:
A cluster whose enforcer applies its cluster grants locally and removes them at expiry even when the hub is unreachable. Shortened to "cluster" in the UI.
_Avoid_: spoke, member cluster, managed cluster
