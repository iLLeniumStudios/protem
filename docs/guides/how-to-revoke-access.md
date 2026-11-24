# How to revoke access

End a grant before it would expire on its own, from the UI. There is no manifest form for this:
revoking is a hub API action only.

1. Open the grant, from **For you** (your own active grants) or from a request's or grant's
   detail page.
2. Choose **Revoke access**. A reason is optional, but it is kept on the grant and shown in the
   audit log.
3. Confirm. This can't be undone: getting access back takes a new request. Access on every
   cluster the grant covers ends as soon as each cluster's agent next reads the hub (ordinarily
   within its sync interval).

You can always revoke your own access early. Revoking someone else's grant needs `update` on
`accessgrants` in the grant's namespace — ask a hub admin for that permission if you don't have
it.

A grant already revoked or past its expiry refuses a second revoke.
