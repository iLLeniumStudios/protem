# How to fix "You're signed out" after login

Diagnose why the hub UI shows "You're signed out" straight after a successful login, and fix it.

**Notice:** right after signing in at the identity provider, the UI shows the signed-out screen
instead of the app (`web/src/app/Shell.tsx`'s `SignedOut` component), or signs the person out
again soon after.

**Confirm and act**, by symptom:

| Symptom | Cause | Fix |
|---|---|---|
| "You're signed out", and Sign in returns to the same screen with "token is expired" | `--cookie-refresh` is unset or longer than the ID token lifetime, or the client returns no refresh token | Lower `--cookie-refresh` below the provider's ID token lifetime; confirm the client returns a refresh token |
| Signed out with "expected audience" in the message | `OIDC_CLIENT_ID` differs between the proxy and the hub | Make the proxy's and the hub's `OIDC_CLIENT_ID`/`--oidc-client-id` match |
| Logged in, but no profiles can be requested | the token has no `groups` claim, or the group names differ from the profiles' | Check the token's claims (browser devtools, or decode the `Authorization` header the proxy sends); fix the app registration or the profile's `requesters.groups` |
| Logged in, grants are Active, but `kubectl` is still forbidden | the target cluster's API server names users differently | Set `TargetCluster.spec.subject` for that cluster |
| The proxy shows "Internal Server Error" after login and logs "token is expired" for a token issued seconds ago | the clocks of the identity provider and the proxy or hub differ by more than the token lifetime | Fix clock sync (NTP) on the proxy/hub nodes |

Confirm which case applies with the browser's devtools network tab (look at the failing request's
status and body) or the proxy's own logs (`kubectl logs` the manager pod's oauth2-proxy
container; look for "token is expired" or "expected audience").

For the full login setup these settings belong to, see
[How to set up login](../../guides/how-to-set-up-login.md).
