# How to approve or reject a request

Decide a request that is waiting for your approval, from the UI. There is no manifest form for
this: approving and rejecting are hub API actions only.

1. Open **Approvals**. **Waiting for you** lists requests you are eligible to decide, oldest
   first; **Recent decisions** shows what has already been decided.
2. Choose **View request** to open its pane: who is requesting, the profile, the clusters, the
   role it grants, and the approval progress per cluster.
3. Add a comment if you want (required to reject), then choose **Approve request** or **Reject
   request**. Rejecting asks you to confirm; the requester sees your reason, the request ends as
   `Rejected`, and no access is granted.

You can approve a request that needs more than one approval without it being decided yet; it
stays `Pending` until every resolved cluster's requirement is met. You cannot approve the same
rule twice, and a request already decided or no longer `Pending` refuses further decisions.

Who may decide a request is the profile's approval rules for the clusters it resolved to:
[How to require approval on some clusters](how-to-require-approval-on-some-clusters.md) covers
setting that up.
