import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { Clock, TriangleAlert, type LucideIcon } from 'lucide-react';
import { useData } from '../../app/data';
import { grantPath, plural, profileName, requestPath } from '../../app/format';
import {
  Elapsed,
  Identifier,
  PageHeader,
  QuietLine,
  RelativeTime,
  Row,
  RowList,
  Section,
  TimeLeft,
  cn,
} from '../../components/ui';
import { colorTransition, focusRing } from '../../components/ui/cn';
import type { ClusterView } from '../../generated/views';
import { clusterStatus } from '../fleet/fleet';
import { attentionClusters, waitingLongest } from './attention';
import { byEnd } from './timeline';

const LISTED = 5;
const LIVE_LISTED = 8;
const linkClass = cn('rounded-sm font-medium text-accent-text underline-offset-4 hover:underline', focusRing);

function Figure({ to, value, label }: { to: string; value: string; label: string }) {
  return (
    <Link
      to={to}
      className={cn(
        'flex min-w-0 flex-col gap-1 rounded-lg border border-border bg-surface px-4 py-3 hover:border-border-strong hover:bg-subtle',
        colorTransition,
        focusRing,
      )}
    >
      <span className="text-page tabular-nums text-text">{value}</span>
      <span className="text-label text-text-secondary">{label}</span>
    </Link>
  );
}

function Item({ to, title, detail, end }: { to: string; title: string; detail: ReactNode; end: ReactNode }) {
  return (
    <Row className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 py-2.5">
      <span className="flex min-w-0 flex-col">
        <Link to={to} className={cn(linkClass, 'truncate')}>
          {title}
        </Link>
        <span className="truncate text-caption text-text-secondary">{detail}</span>
      </span>
      {end}
    </Row>
  );
}

function ClusterRow({ cluster }: { cluster: ClusterView }) {
  const lost = cluster.agent === 'lost';
  const Icon: LucideIcon = lost ? TriangleAlert : Clock;
  return (
    <Item
      to={`/clusters?q=${encodeURIComponent(cluster.name)}`}
      title={cluster.displayName || cluster.name}
      detail={<Identifier>{cluster.name}</Identifier>}
      end={
        <span className="flex items-center gap-1.5 text-label text-warning-text">
          <Icon className="h-4 w-4 shrink-0" aria-hidden />
          {lost ? 'Agent not reporting' : 'Waiting for agent'}
          {lost && cluster.lastHeartbeat && (
            <span className="font-normal text-text-secondary">
              , last heard <RelativeTime at={cluster.lastHeartbeat} />
            </span>
          )}
        </span>
      }
    />
  );
}

export default function Overview() {
  const { clusters, openRequests, activeGrants, profiles } = useData();
  const ready = clusters.filter((c) => clusterStatus(c).state === 'Ready').length;
  const lost = clusters.filter((c) => c.agent === 'lost').length;
  const silent = attentionClusters(clusters);
  const waiting = waitingLongest(openRequests.items, LISTED);
  const live = byEnd(activeGrants.items).slice(0, LIVE_LISTED);

  return (
    <>
      <PageHeader title="Overview" description="What needs you across the hub and its clusters." />
      <nav aria-label="Summary" className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <Figure to="/approvals" value={String(openRequests.total)} label="Requests waiting for a decision" />
        <Figure to="/timeline" value={String(activeGrants.total)} label="Live grants" />
        <Figure to="/clusters?status=ready" value={`${ready} of ${clusters.length}`} label="Clusters ready" />
        <Figure to="/clusters?agent=lost" value={String(lost)} label="Clusters with an agent not reporting" />
      </nav>

      <div className="grid grid-cols-1 gap-8 xl:grid-cols-2">
        <Section id="attention" title="Needs attention">
          {silent.length === 0 && waiting.length === 0 ? (
            <QuietLine>Nothing needs attention: every agent is reporting and no request is waiting.</QuietLine>
          ) : (
            <div className="flex flex-col gap-4">
              {silent.length > 0 && (
                <div className="flex flex-col gap-2">
                  <h3 className="text-label text-text-secondary">Clusters without a reporting agent</h3>
                  <RowList label="Clusters without a reporting agent">
                    {silent.slice(0, LISTED).map((c) => (
                      <ClusterRow key={c.name} cluster={c} />
                    ))}
                    {silent.length > LISTED && (
                      <Row className="py-2.5 text-label">
                        <Link to="/clusters" className={linkClass}>
                          Show all {plural(silent.length, 'cluster')}
                        </Link>
                      </Row>
                    )}
                  </RowList>
                </div>
              )}
              {waiting.length > 0 && (
                <div className="flex flex-col gap-2">
                  <h3 className="text-label text-text-secondary">Requests waiting longest</h3>
                  <RowList label="Requests waiting longest">
                    {waiting.map((r) => (
                      <Item
                        key={`${r.namespace}/${r.name}`}
                        to={requestPath(r)}
                        title={profileName(profiles, r.namespace, r.profile)}
                        detail={<Identifier>{r.requester.email || r.requester.username}</Identifier>}
                        end={
                          <span className="text-label text-text-secondary">
                            Waiting <Elapsed since={r.createdAt} className="text-text" />
                          </span>
                        }
                      />
                    ))}
                    {openRequests.total > waiting.length && (
                      <Row className="py-2.5 text-label">
                        <Link to="/approvals" className={linkClass}>
                          Show all {plural(openRequests.total, 'waiting request')}
                        </Link>
                      </Row>
                    )}
                  </RowList>
                </div>
              )}
            </div>
          )}
        </Section>

        <Section
          id="live"
          title="Live access"
          count={activeGrants.total ? `${activeGrants.total} live` : undefined}
          action={
            <Link to="/timeline" className={linkClass}>
              Open access timeline
            </Link>
          }
        >
          {live.length === 0 ? (
            <QuietLine>No one holds access right now.</QuietLine>
          ) : (
            <RowList label="Live grants ending soonest">
              {live.map((g) => (
                <Item
                  key={`${g.namespace}/${g.name}`}
                  to={grantPath(g)}
                  title={g.requester.email || g.requester.username}
                  detail={
                    <>
                      <Identifier className="text-text">{g.role.name}</Identifier> on {(g.clusters ?? []).join(', ') || 'no cluster'}
                    </>
                  }
                  end={<TimeLeft expiresAt={g.expiresAt} className="text-label text-text" />}
                />
              ))}
            </RowList>
          )}
        </Section>
      </div>
    </>
  );
}
