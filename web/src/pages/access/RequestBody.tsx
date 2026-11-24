import { useState, type ReactNode } from 'react';
import { CircleCheck, CircleDashed, ShieldAlert, Zap } from 'lucide-react';
import { ApiError, api } from '../../api';
import { useLoad } from '../../app/load';
import { byTime } from '../../app/order';
import { formatDuration, plural } from '../../app/format';
import type { DecisionView, PermissionRuleView, RequestView, RequirementView } from '../../generated/views';
import { Button, Identifier, RelativeTime, Skeleton, StatusPill } from '../../components/ui';
import { formatTimestamp } from '../../components/ui/format';
import { Permissions } from '../permissions/Permissions';
import { cellValues, flaggedFirst, otherRulesSummary } from '../permissions/rules';
import { ProfileLink } from '../profiles/ProfileLink';
import { approvalProgress, approvers } from './model';
import { IdList, LoadFailed } from './parts';
import { RoleScope, requesterName } from './RequestRows';

function Block({ id, title, aside, children }: { id: string; title: string; aside?: ReactNode; children: ReactNode }) {
  return (
    <section aria-labelledby={id} className="flex min-w-0 flex-col gap-2">
      <div className="flex min-w-0 flex-wrap items-baseline gap-x-2">
        <h3 id={id} className="text-title text-text">
          {title}
        </h3>
        {aside && <span className="text-label font-normal text-text-secondary">{aside}</span>}
      </div>
      {children}
    </section>
  );
}

const card = 'min-w-0 overflow-hidden rounded-lg border border-border bg-surface';

function Fact({ label, children, wide }: { label: string; children: ReactNode; wide?: boolean }) {
  return (
    <div className={wide ? 'col-span-full flex min-w-0 flex-col gap-0.5' : 'flex min-w-0 flex-col gap-0.5'}>
      <dt className="text-caption text-text-secondary">{label}</dt>
      <dd className="min-w-0 break-words text-body text-text">{children}</dd>
    </div>
  );
}

function Details({ request: r, profile }: { request: RequestView; profile: string }) {
  return (
    <dl className={`${card} grid grid-cols-[repeat(auto-fill,minmax(10rem,1fr))] gap-x-6 gap-y-3 px-4 py-3`}>
      <Fact label="Requester">
        <Identifier>{r.requester.email || r.requester.username}</Identifier>
      </Fact>
      <Fact label="Profile">
        <ProfileLink namespace={r.namespace} name={r.profile}>
          {profile}
        </ProfileLink>
      </Fact>
      <Fact label="Role and scope">
        {r.role ? <RoleScope role={r.role} namespaces={r.namespaces} /> : <span className="text-text-secondary">Not resolved yet</span>}
      </Fact>
      <Fact label="Clusters">
        <IdList items={r.clusters} empty="Not resolved yet" />
      </Fact>
      <Fact label="Duration">
        <span className="tabular-nums">{formatDuration(r.duration)}</span>
      </Fact>
      <Fact label="Ticket">{r.ticket ? <Identifier>{r.ticket}</Identifier> : <span className="text-text-secondary">None</span>}</Fact>
      <Fact label="Requested">
        <span title={formatTimestamp(r.createdAt)}>
          <RelativeTime at={r.createdAt} />
        </span>
      </Fact>
      <Fact label="Reason" wide>
        {r.reason ? (
          <span className="block max-w-prose whitespace-pre-line">{r.reason}</span>
        ) : (
          <span className="text-text-secondary">None given</span>
        )}
      </Fact>
    </dl>
  );
}

const tech = (r: PermissionRuleView) =>
  [cellValues(r.resources).join(', ') || cellValues(r.nonResourceURLs).join(', '), cellValues(r.verbs).join(', ')].filter(Boolean).join(' · ');

function Allows({ request }: { request: RequestView }) {
  const perms = useLoad(() => api.getProfilePermissions(request.namespace, request.profile), [request.namespace, request.profile]);
  const [all, setAll] = useState(false);
  const view = perms.data;
  const rules = flaggedFirst(view?.rules ?? []);
  const flagged = rules.filter((r) => r.risks?.length);
  const forbidden = perms.error instanceof ApiError && perms.error.status === 403;
  const roleName = request.role?.name ?? view?.role.name;
  return (
    <Block
      id={`allows-${request.name}`}
      title="What this access allows"
      aside={
        view?.found && (
          <>
            {plural(rules.length, 'rule')} in role {roleName && <Identifier>{roleName}</Identifier>}, {flagged.length} flagged
          </>
        )
      }
    >
      {perms.loading ? (
        <div className="flex flex-col gap-2" aria-busy="true">
          <span className="sr-only">Loading</span>
          <Skeleton className="w-2/3" />
          <Skeleton className="w-1/2" />
        </div>
      ) : forbidden ? (
        <p className="text-body text-text-secondary">You aren't allowed to see this profile's permissions.</p>
      ) : perms.error ? (
        <LoadFailed what="the permissions" error={perms.error} retry={perms.reload} />
      ) : !view?.found ? (
        <p className="break-words text-body text-text">{view?.message || 'The role could not be read.'}</p>
      ) : all ? (
        <div className={`${card} flex flex-col gap-3 p-4`}>
          <Permissions view={view} source="As defined on the hub" label={`Permission rules of ${roleName ?? 'the role'}`} />
          <Button size="sm" onClick={() => setAll(false)} className="self-start">
            Show flagged rules only
          </Button>
        </div>
      ) : (
        <ul className={`${card} divide-y divide-border`}>
          {flagged.map((r, i) => (
            <li key={i} className="flex gap-2.5 px-4 py-2.5">
              <ShieldAlert className="mt-0.5 h-4 w-4 shrink-0 text-danger-text" aria-hidden />
              <div className="flex min-w-0 flex-1 flex-col gap-0.5">
                <span className="sr-only">Flagged: </span>
                {(r.risks ?? []).map((risk) => (
                  <span key={risk} className="break-words text-body font-semibold text-text">
                    {risk}
                  </span>
                ))}
                <span className="break-words font-mono text-caption text-text-secondary">{tech(r)}</span>
              </div>
            </li>
          ))}
          <li className="flex flex-wrap items-start gap-x-2.5 gap-y-2 px-4 py-2.5">
            <CircleCheck className="mt-0.5 h-4 w-4 shrink-0 text-text-tertiary" aria-hidden />
            <span className="min-w-0 flex-1 basis-48 break-words text-label font-normal text-text-secondary">
              {rules.length === 0 ? 'This role grants no permissions.' : otherRulesSummary(rules) || 'Every rule is flagged.'}
            </span>
            {rules.length > 0 && (
              <Button size="sm" variant="quiet" onClick={() => setAll(true)}>
                Show all {plural(rules.length, 'rule')}
              </Button>
            )}
          </li>
        </ul>
      )}
    </Block>
  );
}

function needText(q: RequirementView) {
  if (q.mode !== 'Manual') return 'Granted at once';
  const who = approvers(q);
  return `Needs ${q.required}${who.length ? ` from ${who.join(', ')}` : ''}`;
}

function Approval({ request }: { request: RequestView }) {
  const requirements = request.requirements ?? [];
  const progress = approvalProgress(request);
  return (
    <Block
      id={`approval-${request.name}`}
      title="Approval"
      aside={progress ? `${progress.approved} of ${plural(progress.required, 'approval')}, each counts for every cluster its rule covers` : undefined}
    >
      {requirements.length === 0 ? (
        <p className="text-body text-text-secondary">
          The approvals needed appear once the request has been checked against the profile's approval rules.
        </p>
      ) : (
        <ul aria-label="Approval per cluster" className={`${card} divide-y divide-border`}>
          {requirements.map((q) => {
            const done = q.mode !== 'Manual' || (q.approvedBy?.length ?? 0) >= q.required;
            const Icon = q.mode !== 'Manual' ? Zap : done ? CircleCheck : CircleDashed;
            return (
              <li key={`${q.cluster}/${q.rule}`} className="flex flex-wrap items-center gap-x-4 gap-y-1 px-4 py-2.5">
                <Identifier className="min-w-0 basis-28 text-body font-medium text-text">{q.cluster}</Identifier>
                <span className="min-w-0 flex-1 break-words text-label font-normal text-text-secondary">
                  {needText(q)}
                  {q.approvedBy?.length ? <> · approved by {q.approvedBy.join(', ')}</> : null}
                </span>
                {q.mode === 'Manual' && (
                  <span className={done ? 'inline-flex items-center gap-1 text-label tabular-nums text-success-text' : 'inline-flex items-center gap-1 text-label tabular-nums text-text'}>
                    <Icon className="h-4 w-4 shrink-0" aria-hidden />
                    {Math.min(q.approvedBy?.length ?? 0, q.required)} of {q.required}
                  </span>
                )}
                {q.mode !== 'Manual' && <Icon className="h-4 w-4 shrink-0 text-text-tertiary" aria-hidden />}
              </li>
            );
          })}
        </ul>
      )}
    </Block>
  );
}

function Decisions({ request }: { request: RequestView }) {
  const decisions = [...(request.approvals ?? [])].sort(byTime<DecisionView>('asc', (d) => d.at, (d) => d.user, (d) => d.rule));
  return (
    <Block id={`decisions-${request.name}`} title="Decisions">
      <ol className={`${card} divide-y divide-border`}>
        <li className="flex flex-wrap items-center gap-x-2 gap-y-1 px-4 py-2.5 text-body">
          <Identifier className="min-w-0">{request.requester.email || requesterName(request)}</Identifier>
          <span className="text-text">requested access</span>
          <RelativeTime at={request.createdAt} className="text-label font-normal text-text-secondary" />
        </li>
        {decisions.map((d, i) => (
          <li key={`${d.user}/${d.at}/${i}`} className="flex flex-col gap-1 px-4 py-2.5">
            <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
              <Identifier className="min-w-0">{d.user}</Identifier>
              <StatusPill phase={d.decision} />
              <RelativeTime at={d.at} className="text-label font-normal text-text-secondary" />
            </div>
            {d.comment && <p className="max-w-prose break-words text-body text-text">{d.comment}</p>}
          </li>
        ))}
      </ol>
    </Block>
  );
}

/** A request in full: its facts, what it allows, its approval per cluster and its decisions. Shared by the pane and the request page. */
export function RequestBody({ request, profile }: { request: RequestView; profile: string }) {
  return (
    <div className="flex min-w-0 flex-col gap-6">
      <Details request={request} profile={profile} />
      <Allows request={request} />
      <Approval request={request} />
      <Decisions request={request} />
    </div>
  );
}
