# Security policy

Protem controls who gets privileged access to Kubernetes clusters, so reports of security problems are taken seriously and handled first.

## Reporting a vulnerability

Report it privately through GitHub: on the repository's **Security** tab choose **Report a vulnerability**. Please do not open a public issue for a suspected vulnerability.

Include what you can of: the version or commit, how it is deployed (hub only, or hub and agents), what an attacker needs to start from (no account, a requester, an approver, a stolen agent token), and the steps to reproduce.

You will get an acknowledgement, then a fix or an explanation of why it is not treated as a vulnerability. Credit is given in the release notes unless you prefer otherwise.

## Scope

In scope: gaining or keeping access that a profile, an approval rule or a cluster's ceiling should have prevented; acting as another person; reading requests or grants one should not see; anything that lets an agent or its token do more than [docs/architecture/security-model.md](docs/architecture/security-model.md) says it can.

Known limits, listed in [docs/architecture/security-model.md](docs/architecture/security-model.md) under "What Protem does not protect against", are not vulnerabilities, though ideas for closing them are welcome as ordinary issues.

## Supported versions

Protem has no stable release yet. Fixes land on the main branch.
