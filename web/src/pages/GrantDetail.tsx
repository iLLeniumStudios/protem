import { Link, useParams } from 'react-router-dom';
import { ApiError, api } from '../api';
import { useData } from '../app/data';
import { useLive } from '../app/load';
import type { BindingView, GrantView } from '../generated/views';
import {
  EmptyState,
  Identifier,
  PageHeader,
  Panel,
  RelativeTime,
  Skeleton,
  StatusPill,
  TBody,
  TD,
  TH,
  THead,
  TR,
  Table,
  TimeLeft,
  buttonClass,
} from '../components/ui';
import { formatTimestamp } from '../components/ui/format';
import { profileName, requestPath } from '../app/format';
import { Fact, Facts, IdList, LoadFailed, RevokeGrant, Unavailable } from './access/parts';
import { Permissions } from './permissions/Permissions';
import { ProfileLink } from './profiles/ProfileLink';

const bindingText = (b: BindingView) => `${b.kind} ${b.namespace ? `${b.namespace}/` : ''}${b.name}`;

function ClusterPermissions({ grant }: { grant: GrantView }) {
  const perms = useLive(() => api.getGrantPermissions(grant.namespace, grant.name), [grant.namespace, grant.name]);
  const forbidden = perms.error instanceof ApiError && perms.error.status === 403;
  return (
    <Panel title="Permissions on each cluster" description="What this grant holds on each cluster, rule by rule.">
      {perms.loading ? (
        <div className="flex flex-col gap-3" aria-busy="true">
          <span className="sr-only">Loading</span>
          <Skeleton className="w-2/3" />
          <Skeleton className="w-1/2" />
        </div>
      ) : forbidden ? (
        <p className="text-body text-text-secondary">You aren't allowed to see the permissions this grant holds.</p>
      ) : perms.error ? (
        <LoadFailed what="the permissions" error={perms.error} retry={perms.reload} />
      ) : !perms.data?.length ? (
        <p className="text-body text-text-secondary">No cluster has been chosen for this grant yet.</p>
      ) : (
        <ul className="flex flex-col divide-y divide-border">
          {perms.data.map((c) => (
            <li key={c.cluster} className="flex flex-col gap-4 py-6 first:pt-0 last:pb-0">
              <div className="flex flex-wrap items-center gap-2">
                <h3 className="min-w-0 text-body font-medium text-text">
                  <Identifier>{c.cluster}</Identifier>
                </h3>
                <StatusPill phase={c.state} />
              </div>
              {c.state !== 'Active' && c.message && <p className="break-words text-body text-text">{c.message}</p>}
              <Facts>
                <Fact label="Bound subject">
                  {c.subject ? <Identifier>{c.subject}</Identifier> : <span className="text-text-secondary">Not reported</span>}
                </Fact>
                <Fact label="Bindings">
                  <IdList items={(c.bindings ?? []).map(bindingText)} empty="None" />
                </Fact>
              </Facts>
              <Permissions
                view={c.permissions}
                label={`Permission rules on ${c.cluster}`}
                source={
                  c.permissions.source === 'cluster'
                    ? 'Reported by this cluster'
                    : 'As defined on the hub cluster; this cluster has not reported yet'
                }
              />
            </li>
          ))}
        </ul>
      )}
    </Panel>
  );
}

export default function GrantDetail() {
  const { ns = '', name = '' } = useParams();
  return <GrantPage key={`${ns}/${name}`} ns={ns} name={name} />;
}

function GrantPage({ ns, name }: { ns: string; name: string }) {
  const { profiles } = useData();
  const loaded = useLive(() => api.getGrant(ns, name), []);
  const grant = loaded.data;

  if (loaded.loading) {
    return (
      <>
        <PageHeader title="Grant" />
        <Panel>
          <div className="flex flex-col gap-3">
            <Skeleton className="h-5 w-1/3" />
            <Skeleton className="w-2/3" />
            <Skeleton className="w-1/2" />
          </div>
        </Panel>
      </>
    );
  }

  if (!grant) {
    return (
      <>
        <PageHeader title="Grant" />
        <Unavailable kind="grant" ns={ns} name={name} error={loaded.error} retry={loaded.reload} />
      </>
    );
  }

  const profile = profileName(profiles, grant.namespace, grant.profile);
  const request = grant.request ? { namespace: grant.namespace, name: grant.request } : null;
  const states = grant.clusterStates ?? [];
  const active = grant.phase === 'Active';

  return (
    <>
      <PageHeader
        title={profile}
        description={
          <span className="flex flex-wrap items-center gap-2">
            <StatusPill phase={grant.phase} />
            <Identifier className="min-w-0">{grant.name}</Identifier>
          </span>
        }
        actions={
          (request || grant.actions.revoke) && (
            <>
              {request && (
                <Link to={requestPath(request)} className={buttonClass('secondary')}>
                  View request
                </Link>
              )}
              {grant.actions.revoke && <RevokeGrant grant={grant} profile={profile} />}
            </>
          )
        }
      />
      <Panel title="Details">
        <Facts>
          <Fact label="Requester">
            <Identifier>{grant.requester.email || grant.requester.username}</Identifier>
          </Fact>
          <Fact label={active ? 'Time left' : 'Ended'}>
            {active ? (
              <>
                <TimeLeft expiresAt={grant.expiresAt} className="font-medium" />, until {formatTimestamp(grant.expiresAt)}
              </>
            ) : (
              formatTimestamp(grant.revocation?.at ?? grant.expiresAt)
            )}
          </Fact>
          <Fact label="Profile">
            <ProfileLink namespace={grant.namespace} name={grant.profile}>
              {profile}
            </ProfileLink>{' '}
            <Identifier className="text-text-secondary">({grant.profile})</Identifier>
          </Fact>
          <Fact label="Role">
            <Identifier>{grant.role.name}</Identifier>{' '}
            <span className="text-text-secondary">({grant.role.kind})</span>
          </Fact>
          <Fact label="Clusters">
            <IdList items={grant.clusters} empty="None" />
          </Fact>
          <Fact label="Namespaces">
            <IdList items={grant.namespaces} empty="All namespaces" />
          </Fact>
        </Facts>
      </Panel>
      {grant.revocation && (
        <Panel title="Revocation">
          <Facts>
            <Fact label="Revoked by">
              <Identifier>{grant.revocation.by || 'System'}</Identifier>
            </Fact>
            <Fact label="Revoked">
              {formatTimestamp(grant.revocation.at)} (<RelativeTime at={grant.revocation.at} />)
            </Fact>
            <Fact label="Reason" wide>
              {grant.revocation.reason ? (
                <span className="whitespace-pre-line">{grant.revocation.reason}</span>
              ) : (
                <span className="text-text-secondary">None given</span>
              )}
            </Fact>
          </Facts>
        </Panel>
      )}
      <Panel title="Status on each cluster" description="What each cluster reports about this grant.">
        {states.length === 0 ? (
          <p className="text-body text-text-secondary">No cluster has reported on this grant yet.</p>
        ) : (
          <Table label="Status on each cluster">
            <THead>
              <TR>
                <TH>Cluster</TH>
                <TH>State</TH>
                <TH>Message</TH>
              </TR>
            </THead>
            <TBody>
              {states.map((s) => (
                <TR key={s.name}>
                  <TD className="max-w-xs">
                    <Identifier>{s.name}</Identifier>
                  </TD>
                  <TD>
                    <StatusPill phase={s.state} />
                  </TD>
                  <TD className="min-w-48 break-words">
                    {s.message || <span className="text-text-secondary">No message</span>}
                  </TD>
                </TR>
              ))}
            </TBody>
          </Table>
        )}
      </Panel>
      <ClusterPermissions grant={grant} />
    </>
  );
}
