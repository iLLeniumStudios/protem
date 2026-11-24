import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { Check, Clock, Eye, Server, Ticket, Timer, TriangleAlert } from 'lucide-react';
import type { GrantView, ProfileView, RequestView } from '../../generated/views';
import { Badge, Button, Elapsed, Identifier, RelativeTime, Row, TimeUsed, cn } from '../../components/ui';
import { focusRing } from '../../components/ui/cn';
import { formatDuration, grantPath, plural, profileName } from '../../app/format';
import { isLongWait } from '../approvals/queue';
import { approvalProgress } from './model';
import { CancelRequest, DecideButtons, IdList, RevokeGrant } from './parts';
import type { RequestRef } from './model';

export const refKey = (r: RequestRef) => `${r.namespace}/${r.name}`;

export const linkClass = cn('rounded-sm font-medium text-accent-text underline-offset-4 hover:underline', focusRing);

export function requesterName(r: Pick<RequestView, 'requester'>) {
  return r.requester.username || r.requester.email;
}

function initials(name: string) {
  return (name.match(/[A-Za-z0-9]+/g) ?? ['?'])
    .slice(0, 2)
    .map((w) => w[0])
    .join('')
    .toLocaleUpperCase('en');
}

function Avatar({ name }: { name: string }) {
  return (
    <span
      aria-hidden
      className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-muted text-caption font-semibold text-text-secondary"
    >
      {initials(name)}
    </span>
  );
}

/** One fact of a row's second line, with an optional leading icon. */
function Item({ icon: Icon, children }: { icon?: typeof Server; children: ReactNode }) {
  return (
    <span className="inline-flex min-w-0 max-w-full items-center gap-1.5">
      {Icon && <Icon className="h-4 w-4 shrink-0 text-text-tertiary" aria-hidden />}
      {children}
    </span>
  );
}

/** Role in a monospace chip, then where it applies: the namespaces, or cluster-wide. */
export function RoleScope({ role, namespaces }: { role?: { name: string }; namespaces?: string[] | null }) {
  return (
    <Item>
      {role && <span className="rounded-sm border border-border bg-subtle px-1 font-mono text-caption text-text">{role.name}</span>}
      {namespaces?.length ? (
        <span className="flex min-w-0 gap-1">
          in <IdList items={namespaces} empty="" />
        </span>
      ) : (
        <span>cluster-wide</span>
      )}
    </Item>
  );
}

function Facts({ children }: { children: ReactNode }) {
  return <div className="flex min-w-0 flex-wrap items-center gap-x-3.5 gap-y-1 text-label font-normal text-text-secondary">{children}</div>;
}

function ViewButton({ request, label, onView }: { request: RequestRef; label: string; onView: () => void }) {
  return (
    <Button variant="quiet" size="sm" icon={Eye} onClick={onView} data-request-view={refKey(request)}>
      View request<span className="sr-only"> {label}</span>
    </Button>
  );
}

/** How many rules of the request's role are flagged for review; nothing when none are. */
export function FlaggedRules({ count }: { count?: number }) {
  if (!count || count < 1) return null;
  return (
    <span className="inline-flex shrink-0 items-center gap-1 text-label font-medium text-warning-text" title="Open the request to see what they allow">
      <TriangleAlert className="h-4 w-4 shrink-0" aria-hidden />
      {plural(count, 'flagged rule')}
    </span>
  );
}

/** Approvals so far as a row of steps, then in words. */
export function ApprovalSteps({ request }: { request: RequestView }) {
  const progress = approvalProgress(request);
  if (!progress) return <span className="text-label text-text-secondary">No approval needed</span>;
  const by = [...new Set((request.requirements ?? []).flatMap((q) => q.approvedBy ?? []))];
  return (
    <span className="flex min-w-0 flex-wrap items-center gap-2 text-label text-text-secondary">
      <span aria-hidden className="inline-flex gap-0.5">
        {Array.from({ length: progress.required }, (_, i) => (
          <span key={i} className={cn('h-1.5 w-4 rounded-full', i < progress.approved ? 'bg-success' : 'border border-border-strong bg-muted')} />
        ))}
      </span>
      <span className="tabular-nums">
        <span className="font-semibold text-text">
          {progress.approved} of {plural(progress.required, 'approval')}
        </span>
        {by.length > 0 && <> · approved by {by.join(', ')}</>}
      </span>
    </span>
  );
}

export interface RequestRowProps {
  request: RequestView;
  profiles: ProfileView[];
  selected: boolean;
  onView: () => void;
}

/** A request someone else made that this person may decide: who, what, where, how long, why, and the decision. */
export function DecisionRow({ request: r, profiles, selected, onView }: RequestRowProps) {
  const profile = profileName(profiles, r.namespace, r.profile);
  const who = requesterName(r);
  const progress = approvalProgress(r);
  return (
    <Row selected={selected} data-request={refKey(r)} className="flex gap-3">
      <Avatar name={who} />
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <div className="flex min-w-0 flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
          <p className="min-w-0 break-words text-body text-text-secondary">
            <span className="font-semibold text-text">{who}</span> requests <span className="font-semibold text-text">{profile}</span>
          </p>
          <span
            className={cn(
              'inline-flex shrink-0 items-center gap-1 text-label font-normal',
              isLongWait(r.createdAt, new Date()) ? 'text-warning-text' : 'text-text-secondary',
            )}
          >
            <Clock className="h-4 w-4 shrink-0 self-center" aria-hidden />
            waiting <Elapsed since={r.createdAt} />
          </span>
        </div>
        <Facts>
          <RoleScope role={r.role} namespaces={r.namespaces} />
          <Item icon={Server}>
            <IdList items={r.clusters} empty="No clusters yet" />
          </Item>
          <Item icon={Timer}>
            <span className="tabular-nums">{formatDuration(r.duration)}</span>
          </Item>
          {r.ticket ? (
            <Item icon={Ticket}>
              <Identifier>{r.ticket}</Identifier>
            </Item>
          ) : (
            <span>No ticket</span>
          )}
          {progress && (
            <span className="tabular-nums">
              {progress.approved} of {plural(progress.required, 'approval')}
            </span>
          )}
          <FlaggedRules count={r.flaggedRules} />
        </Facts>
        <div className="mt-1 flex min-w-0 flex-wrap items-center justify-between gap-x-3 gap-y-2">
          <p className="min-w-0 flex-1 basis-48 truncate text-label font-normal text-text" title={r.reason || undefined}>
            {r.reason ? <q>{r.reason}</q> : <span className="text-text-secondary">No reason given</span>}
          </p>
          <div className="flex flex-wrap items-center gap-2">
            <ViewButton request={r} label={`${profile} from ${who}`} onView={onView} />
            <DecideButtons request={r} profile={profile} size="sm" />
            {!r.actions.approve && (
              <Badge>
                <Check className="h-4 w-4 shrink-0" aria-hidden />
                You approved
              </Badge>
            )}
          </div>
        </div>
      </div>
    </Row>
  );
}

/** One of the person's own requests: what, where, how long, when, and how far its approval has come. */
export function MyRequestRow({ request: r, profiles, selected, onView, state }: RequestRowProps & { state?: ReactNode }) {
  const profile = profileName(profiles, r.namespace, r.profile);
  return (
    <Row selected={selected} data-request={refKey(r)} className="flex flex-wrap items-center justify-between gap-x-4 gap-y-3">
      <div className="flex min-w-0 flex-col gap-1">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <span className="break-words text-body font-semibold text-text">{profile}</span>
          {state}
        </div>
        <Facts>
          <Item>
            on <IdList items={r.clusters} empty="clusters not chosen yet" />
          </Item>
          <span className="tabular-nums">{formatDuration(r.duration)}</span>
          <span>
            sent <RelativeTime at={r.createdAt} />
          </span>
          <FlaggedRules count={r.flaggedRules} />
        </Facts>
        {r.phase === 'Pending' && <ApprovalSteps request={r} />}
      </div>
      <div className="flex shrink-0 flex-wrap items-center gap-2">
        <ViewButton request={r} label={profile} onView={onView} />
        {r.actions.cancel && <CancelRequest request={r} profile={profile} size="sm" />}
      </div>
    </Row>
  );
}

/** A live grant of the person's: what and where, the time used and left, and giving it back. */
export function GrantRow({ grant: g, profiles }: { grant: GrantView; profiles: ProfileView[] }) {
  const profile = profileName(profiles, g.namespace, g.profile);
  const clusters = g.clusters?.length ? g.clusters : (g.clusterStates ?? []).map((s) => s.name);
  return (
    <Row className="flex flex-wrap items-center gap-x-5 gap-y-3">
      <div className="flex min-w-0 flex-1 basis-60 flex-col gap-0.5">
        <Link to={grantPath(g)} className={cn(linkClass, 'self-start break-words font-semibold')}>
          {profile}
        </Link>
        <Facts>
          <RoleScope role={g.role} namespaces={g.namespaces} />
          <Item>
            on <IdList items={clusters} empty="no clusters" />
          </Item>
        </Facts>
      </div>
      <TimeUsed startedAt={g.startedAt} expiresAt={g.expiresAt} label={`Time used, ${profile}`} className="min-w-48 flex-1 basis-48 sm:max-w-60" />
      <div className="flex shrink-0">{g.actions.revoke && <RevokeGrant grant={g} profile={profile} size="sm" />}</div>
    </Row>
  );
}
