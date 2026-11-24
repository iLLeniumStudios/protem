import { useSearchParams } from 'react-router-dom';
import {
  CircleCheck,
  CirclePause,
  CircleX,
  Clock,
  Server,
  SearchX,
  TriangleAlert,
  type LucideIcon,
} from 'lucide-react';
import {
  Button,
  ConsoleCard,
  ConsoleToolbar,
  DenseTable,
  EmptyState,
  FigureButton,
  FilterChip,
  Identifier,
  PageHeader,
  RelativeTime,
  SearchField,
  TBody,
  TD,
  TH,
  THead,
  TR,
  TablePager,
  cn,
} from '../components/ui';
import { useData } from '../app/data';
import { clampPage, slicePage } from '../app/paging';
import { plural } from '../app/format';
import type { ClusterView } from '../generated/views';
import { AddCluster } from './fleet/AddCluster';
import { ClusterActions } from './fleet/ClusterActions';
import { clusterActions } from './fleet/enrol';
import { clusterStatus } from './fleet/fleet';
import {
  AGENT_STATES,
  allowedRolesText,
  chooseFigure,
  clusterFigures,
  clusterQueryString,
  envOptions,
  figureOf,
  filterClusters,
  parseClusterQuery,
  sortClusters,
  type ClusterQuery,
  type Figure,
} from './fleet/table';
import { canEditCluster } from './profiles/manage';
import { ManagedBadge } from './profiles/ManagedBadge';

const PAGE = 25;
const ROLES_SHOWN = 3;
const PARTIAL_ROLES = "This cluster's authorizer cannot list every rule, so more roles may be allowed";

const FIGURES: Record<Figure, { label: string; icon: LucideIcon; tone: 'success' | 'warning' | 'neutral' }> = {
  ready: { label: 'ready', icon: CircleCheck, tone: 'success' },
  lost: { label: 'agent not reporting', icon: TriangleAlert, tone: 'warning' },
  paused: { label: 'paused', icon: CirclePause, tone: 'neutral' },
  waiting: { label: 'waiting for agent', icon: Clock, tone: 'warning' },
};

const AGENT_LABELS: Record<(typeof AGENT_STATES)[number], string> = {
  connected: 'Connected',
  lost: 'Not reporting',
  waiting: 'Waiting for agent',
  none: 'Hub, no agent',
};

function agentCell(c: ClusterView): { label: string; icon: LucideIcon; className: string; title?: string } {
  if (c.paused) return { label: 'Paused', icon: CirclePause, className: 'text-text-secondary' };
  const { state, detail } = clusterStatus(c);
  switch (c.agent) {
    case 'connected':
      return state === 'NotReady'
        ? { label: 'Not ready', icon: CircleX, className: 'text-danger-text', title: detail }
        : { label: 'Connected', icon: CircleCheck, className: 'text-success-text' };
    case 'lost':
      return { label: 'Not reporting', icon: TriangleAlert, className: 'text-warning-text' };
    case 'waiting':
      return { label: 'Waiting for agent', icon: Clock, className: 'text-warning-text' };
    case 'none':
      return { label: 'Hub, no agent', icon: Server, className: 'text-text-secondary' };
    default:
      return { label: 'Unknown', icon: CircleX, className: 'text-text-secondary' };
  }
}

function Agent({ cluster }: { cluster: ClusterView }) {
  const { label, icon: Icon, className, title } = agentCell(cluster);
  return (
    <span title={title} className={cn('flex items-center gap-1.5 whitespace-nowrap font-medium', className)}>
      <Icon className="h-4 w-4 shrink-0" aria-hidden />
      {label}
    </span>
  );
}

function Heartbeat({ cluster }: { cluster: ClusterView }) {
  if (cluster.agent === 'none') return <span className="text-text-secondary">No agent</span>;
  if (!cluster.lastHeartbeat) return <span className="text-text-secondary">Never</span>;
  return <RelativeTime at={cluster.lastHeartbeat} className={cn(cluster.agent === 'lost' && 'font-semibold text-warning-text')} />;
}

function Dash({ title }: { title: string }) {
  return (
    <span title={title} className="text-text-secondary">
      <span aria-hidden>—</span>
      <span className="sr-only">{title}</span>
    </span>
  );
}

function Env({ cluster }: { cluster: ClusterView }) {
  const env = cluster.labels?.env;
  if (!env) return <Dash title="No env label" />;
  return <span className="inline-flex h-5 items-center rounded-sm border border-border bg-subtle px-1.5 font-mono text-caption text-text-secondary">{env}</span>;
}

function AllowedRoles({ cluster }: { cluster: ClusterView }) {
  const allowed = allowedRolesText(cluster);
  if (allowed.kind === 'operator') return <span className="text-text-secondary">Operator's limit</span>;
  if (allowed.kind === 'unreported') return <Dash title="Not reported by the agent" />;
  if (allowed.kind === 'any') return <span className="text-text">Any role</span>;
  if (!allowed.roles.length) {
    return allowed.partial ? (
      <span className="text-text-secondary" title={PARTIAL_ROLES}>
        Not listed
      </span>
    ) : (
      <span className="text-text-secondary">None</span>
    );
  }
  const more = allowed.roles.length - ROLES_SHOWN;
  return (
    <ul className="flex items-center gap-1" title={allowed.roles.join(', ')} aria-label="Allowed roles">
      {allowed.roles.slice(0, ROLES_SHOWN).map((r) => (
        <li key={r} className="rounded-sm border border-border bg-subtle px-1.5 font-mono text-caption text-text">
          {r}
        </li>
      ))}
      {more > 0 && (
        <li className="text-caption text-text-secondary">
          +{more}
          <span className="sr-only"> more: {allowed.roles.slice(ROLES_SHOWN).join(', ')}</span>
        </li>
      )}
      {allowed.partial && (
        <li className="whitespace-nowrap text-caption text-text-secondary" title={PARTIAL_ROLES}>
          or more
        </li>
      )}
    </ul>
  );
}

function Name({ cluster }: { cluster: ClusterView }) {
  const display = cluster.displayName && cluster.displayName !== cluster.name ? cluster.displayName : undefined;
  return (
    <span className="flex min-w-0 items-baseline gap-2">
      {display && <span className="truncate font-semibold text-text">{display}</span>}
      <Identifier className={cn('text-caption', display ? 'text-text-secondary' : 'font-semibold text-text')}>{cluster.name}</Identifier>
      <ManagedBadge by={cluster.managedBy} />
    </span>
  );
}

export default function Fleet() {
  const { clusters, capabilities } = useData();
  const [params, setParams] = useSearchParams();
  const query = parseClusterQuery(params);
  const setQuery = (next: ClusterQuery) => setParams(new URLSearchParams(clusterQueryString(next)), { replace: true });
  const patch = (change: Partial<ClusterQuery>) => setQuery({ ...query, page: 0, ...change });

  const editable = (c: ClusterView) => canEditCluster(capabilities, c);
  const hasActions = (c: ClusterView) => editable(c) || clusterActions(capabilities, c).length > 0;
  const anyActions = clusters.some(hasActions);
  const envs = envOptions(clusters);
  const agentsPresent = AGENT_STATES.filter((a) => a === query.agent || clusters.some((c) => c.agent === a));
  const found = sortClusters(filterClusters(clusters, query));
  const page = clampPage(query.page, found.length, PAGE);
  const rows = slicePage(found, page, PAGE);
  const chosen = figureOf(query);
  const filtered = Boolean(query.q.trim() || query.status || query.agent || query.env);
  const hubName = clusters.find((c) => c.agent === 'none')?.name;
  const onlyHub = clusters.length > 0 && clusters.every((c) => c.agent === 'none');
  const add = capabilities?.addClusters ? <AddCluster hubName={hubName} /> : undefined;
  const clear = (
    <Button size="sm" onClick={() => setQuery({ q: '', page: 0 })}>
      Clear filters
    </Button>
  );

  return (
    <>
      <PageHeader
        title={clusters.length ? plural(clusters.length, 'cluster') : 'Clusters'}
        description={onlyHub ? `The hub is the only cluster so far.${add ? ' Add a cluster to grant access beyond the hub.' : ''}` : undefined}
        actions={add}
      />
      {clusters.length > 0 && (
        <div className="flex flex-col gap-3">
          <div role="group" aria-label="Filter by state" className="flex flex-wrap gap-2">
            {clusterFigures(clusters).map(({ figure, count }) => (
              <FigureButton
                key={figure}
                count={count}
                label={FIGURES[figure].label}
                icon={FIGURES[figure].icon}
                tone={FIGURES[figure].tone}
                pressed={chosen === figure}
                onClick={() => setQuery(chooseFigure(query, figure))}
              />
            ))}
          </div>
          <ConsoleToolbar label="Filter clusters">
            <SearchField
              label="Search clusters"
              placeholder="Search by name or label"
              value={query.q}
              onChange={(q) => patch({ q })}
            />
            {envs.length > 0 && (
              <FilterChip
                label="Environment"
                value={query.env ?? ''}
                options={envs.map((e) => ({ value: e, label: e }))}
                onValueChange={(env) => patch({ env: env || undefined })}
              />
            )}
            <FilterChip
              label="Agent state"
              value={query.agent ?? ''}
              options={agentsPresent.map((a) => ({ value: a, label: AGENT_LABELS[a] }))}
              onValueChange={(agent) => patch({ agent: (agent || undefined) as ClusterQuery['agent'] })}
            />
            {filtered && clear}
            <span className="ml-auto text-caption text-text-secondary">Needs attention first, then name</span>
          </ConsoleToolbar>
        </div>
      )}

      <ConsoleCard label="Clusters">
        {clusters.length === 0 ? (
          <EmptyState
            icon={Server}
            title="No clusters registered"
            body={add ? 'Add a cluster to start granting access on it.' : 'A cluster appears here once an admin registers it with the hub.'}
          />
        ) : found.length === 0 ? (
          <EmptyState icon={SearchX} title="No clusters match these filters" body="Try a shorter search or another filter." action={clear} />
        ) : (
          <>
            <div className="hidden lg:block">
              <DenseTable label="Clusters">
                <THead>
                  <TR>
                    <TH className="w-1/4">Cluster</TH>
                    <TH>Environment</TH>
                    <TH>Agent</TH>
                    <TH>Last heartbeat</TH>
                    <TH className="text-right">Live grants</TH>
                    <TH>Allowed roles</TH>
                    {anyActions && (
                      <TH className="w-12">
                        <span className="sr-only">Actions</span>
                      </TH>
                    )}
                  </TR>
                </THead>
                <TBody>
                  {rows.map((c) => (
                    <TR key={c.name}>
                      <TD className="max-w-96">
                        <Name cluster={c} />
                      </TD>
                      <TD>
                        <Env cluster={c} />
                      </TD>
                      <TD>
                        <Agent cluster={c} />
                      </TD>
                      <TD className="whitespace-nowrap">
                        <Heartbeat cluster={c} />
                      </TD>
                      <TD className="text-right">{c.activeGrantsCount}</TD>
                      <TD>
                        <AllowedRoles cluster={c} />
                      </TD>
                      {anyActions && (
                        <TD className="text-right">{hasActions(c) && <ClusterActions cluster={c} editable={editable(c)} />}</TD>
                      )}
                    </TR>
                  ))}
                </TBody>
              </DenseTable>
            </div>
            <ul aria-label="Clusters" className="flex flex-col divide-y divide-border lg:hidden">
              {rows.map((c) => (
                <li key={c.name} className="flex flex-col gap-2 px-4 py-3">
                  <div className="flex min-w-0 items-start justify-between gap-2">
                    <span className="flex min-w-0 flex-col">
                      <span className="truncate font-semibold text-text">{c.displayName || c.name}</span>
                      <Identifier className="text-caption text-text-secondary">{c.name}</Identifier>
                    </span>
                    {hasActions(c) && <ClusterActions cluster={c} editable={editable(c)} />}
                  </div>
                  <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-label font-normal">
                    <Agent cluster={c} />
                    <span className="text-text-secondary">
                      Heartbeat <Heartbeat cluster={c} />
                    </span>
                    <span className="text-text-secondary">
                      <span className="tabular-nums text-text">{c.activeGrantsCount}</span> live
                    </span>
                    {c.labels?.env && <Env cluster={c} />}
                  </div>
                  <div className="flex min-w-0 items-center gap-2 text-label font-normal">
                    <span className="shrink-0 text-text-secondary">Allowed roles</span>
                    <AllowedRoles cluster={c} />
                  </div>
                </li>
              ))}
            </ul>
            <TablePager
              label="clusters"
              page={page}
              pageSize={PAGE}
              total={found.length}
              onPageChange={(p) => setQuery({ ...query, page: p })}
              className="border-t border-border px-4 py-2"
            />
          </>
        )}
      </ConsoleCard>
    </>
  );
}
