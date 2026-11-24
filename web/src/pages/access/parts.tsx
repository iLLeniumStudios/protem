import { useState, type ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { Check, CircleAlert, FileQuestion, Lock, X } from 'lucide-react';
import { ApiError } from '../../api';
import { useData } from '../../app/data';
import { formatDuration } from '../../app/format';
import { isMe } from '../../app/roles';
import type { GrantView, RequestView } from '../../generated/views';
import {
  Button,
  ConfirmDialog,
  EmptyState,
  Identifier,
  Panel,
  buttonClass,
  cn,
  useToast,
  type ButtonSize,
} from '../../components/ui';

export function Facts({ children }: { children: ReactNode }) {
  return <dl className="grid gap-x-6 gap-y-4 sm:grid-cols-2">{children}</dl>;
}

export function Fact({ label, children, wide }: { label: string; children: ReactNode; wide?: boolean }) {
  return (
    <div className={cn('flex min-w-0 flex-col gap-1', wide && 'sm:col-span-2')}>
      <dt className="text-label text-text-secondary">{label}</dt>
      <dd className="min-w-0 break-words text-body text-text">{children}</dd>
    </div>
  );
}

/** A list of identifiers that wraps between items, never inside one; an item wider than its box truncates with a title. */
export function IdList({ items, empty }: { items?: string[] | null; empty: string }) {
  if (!items?.length) return <span className="text-text-secondary">{empty}</span>;
  return (
    <span className="inline-flex max-w-full flex-wrap gap-x-1">
      {items.map((item, i) => (
        <span key={item} className="inline-flex min-w-0 max-w-full">
          <Identifier className="min-w-0">{item}</Identifier>
          {i < items.length - 1 && ','}
        </span>
      ))}
    </span>
  );
}

/** One line saying the poll holds fewer items than exist, instead of truncating silently. */
export function SnapshotLimit({
  shown,
  total,
  what,
  next = 'Use Audit to find the rest.',
}: {
  shown: number;
  total: number;
  what: string;
  next?: string;
}) {
  if (total <= shown) return null;
  return (
    <p className="text-body text-text-secondary">
      Showing {shown} of {total} {what}. {next}
    </p>
  );
}

const WHO_CAN_SEE = {
  request: 'Only its requester, its approvers and admins can see it.',
  grant: 'Only its requester, the approvers of its request and admins can see it.',
};

/** The state of a detail page whose object could not be shown: not allowed, not found, or failed to load. */
export function Unavailable({
  kind,
  ns,
  name,
  error,
  retry,
}: {
  kind: 'request' | 'grant';
  ns: string;
  name: string;
  error: Error | null;
  retry: () => void;
}) {
  const status = error instanceof ApiError ? error.status : 0;
  const back = (
    <Link to="/requests" className={buttonClass('primary')}>
      Go to My requests
    </Link>
  );
  if (error && status !== 403 && status !== 404) {
    return (
      <Panel>
        <LoadFailed what={`this ${kind}`} error={error} retry={retry} />
      </Panel>
    );
  }
  return (
    <Panel>
      <EmptyState
        icon={status === 403 ? Lock : FileQuestion}
        title={status === 403 ? `You can't view this ${kind}` : `${kind === 'request' ? 'Request' : 'Grant'} not found`}
        body={
          status === 403 ? (
            WHO_CAN_SEE[kind]
          ) : (
            <>
              There is no {kind} <Identifier>{name}</Identifier> in <Identifier>{ns}</Identifier>.
            </>
          )
        }
        action={back}
        className="py-6"
      />
    </Panel>
  );
}

export function LoadFailed({ what, error, retry }: { what: string; error: Error; retry: () => void }) {
  return (
    <div role="alert" className="flex flex-col items-start gap-3">
      <p className="flex items-start gap-2 text-body text-danger-text">
        <CircleAlert className="mt-0.5 h-4 w-4 shrink-0" aria-hidden />
        <span className="flex min-w-0 flex-col gap-1">
          <span className="font-medium">Couldn't load {what}.</span>
          <span className="break-words">{error.message}</span>
        </span>
      </p>
      <Button size="sm" onClick={retry}>
        Try again
      </Button>
    </div>
  );
}

export function RevokeGrant({ grant, profile, size }: { grant: GrantView; profile: string; size?: ButtonSize }) {
  const { me, actions } = useData();
  const { toast } = useToast();
  const [open, setOpen] = useState(false);
  const clusterCount = grant.clusters?.length ?? grant.clusterStates?.length ?? 0;
  const who = isMe(me, grant.requester) ? 'You lose' : `${grant.requester.email} loses`;
  return (
    <>
      <Button variant="reject" size={size} onClick={() => setOpen(true)}>
        Revoke access<span className="sr-only"> to {profile}</span>
      </Button>
      <ConfirmDialog
        open={open}
        onOpenChange={setOpen}
        title={`Revoke access to ${profile}?`}
        body={`${who} this access on ${clusterCount === 1 ? 'its cluster' : `all ${clusterCount} clusters`} now. This can't be undone; getting it back takes a new request.`}
        confirmLabel="Revoke access"
        tone="danger"
        reason="optional"
        onConfirm={async (reason) => {
          await actions.revoke(grant, reason || undefined);
          toast({ title: `Access to ${profile} revoked`, tone: 'success' });
        }}
      />
    </>
  );
}

export function CancelRequest({ request, profile, size }: { request: RequestView; profile: string; size?: ButtonSize }) {
  const { actions } = useData();
  const { toast } = useToast();
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button variant="secondary" size={size} onClick={() => setOpen(true)}>
        Cancel request<span className="sr-only"> for {profile}</span>
      </Button>
      <ConfirmDialog
        open={open}
        onOpenChange={setOpen}
        title={`Cancel your request for ${profile}?`}
        body="It ends as Cancelled and approvers can no longer act on it. To get access later, send a new request."
        confirmLabel="Cancel request"
        cancelLabel="Keep request"
        tone="danger"
        onConfirm={async () => {
          await actions.cancel(request);
          toast({ title: 'Request cancelled', tone: 'success' });
        }}
      />
    </>
  );
}

/** Reject and approve buttons for a request, each confirmed in a dialog; only the ones the server allows. */
export function DecideButtons({ request, profile, size }: { request: RequestView; profile: string; size?: ButtonSize }) {
  const { actions } = useData();
  const { toast } = useToast();
  const [open, setOpen] = useState<'approve' | 'reject' | null>(null);
  const setOpenTo = (which: 'approve' | 'reject') => (next: boolean) => setOpen(next ? which : null);
  const who = request.requester.email || request.requester.username;
  return (
    <>
      {request.actions.reject && (
        <Button variant="reject" size={size} icon={X} onClick={() => setOpen('reject')}>
          Reject request<span className="sr-only"> for {profile} from {who}</span>
        </Button>
      )}
      {request.actions.approve && (
        <Button variant="approve" size={size} icon={Check} onClick={() => setOpen('approve')}>
          Approve request<span className="sr-only"> for {profile} from {who}</span>
        </Button>
      )}
      <ConfirmDialog
        open={open === 'approve'}
        onOpenChange={setOpenTo('approve')}
        title={`Approve ${profile} for ${who}?`}
        body={`${who} gets ${profile} for ${formatDuration(request.duration)} once every required approval is in. Your comment is saved with the approval.`}
        confirmLabel="Approve request"
        tone="primary"
        reason="optional"
        reasonLabel="Comment"
        onConfirm={async (comment) => {
          await actions.approve(request, comment || undefined);
          toast({ title: `Approved ${profile} for ${who}`, tone: 'success' });
        }}
      />
      <ConfirmDialog
        open={open === 'reject'}
        onOpenChange={setOpenTo('reject')}
        title={`Reject ${profile} for ${who}?`}
        body="The request ends as Rejected and no access is granted. The requester sees your reason."
        confirmLabel="Reject request"
        tone="danger"
        reason="required"
        onConfirm={async (reason) => {
          await actions.reject(request, reason);
          toast({ title: `Rejected ${profile} for ${who}`, tone: 'info' });
        }}
      />
    </>
  );
}
