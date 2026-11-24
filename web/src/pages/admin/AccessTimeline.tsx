import { useEffect, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { ChartGantt, ChevronDown, ChevronRight, ChevronsRight, CirclePause, Clock, SearchX, TriangleAlert } from 'lucide-react';
import { api } from '../../api';
import { useData } from '../../app/data';
import { grantPath, plural } from '../../app/format';
import { usePagedList } from '../../app/load';
import {
  Badge,
  ConsoleCard,
  ConsoleToolbar,
  EmptyState,
  Identifier,
  PageHeader,
  SearchField,
  SegmentedControl,
  Skeleton,
  TablePager,
  TextButton,
  TimeLeft,
  cn,
} from '../../components/ui';
import { colorTransition, focusRing } from '../../components/ui/cn';
import type { ClusterView, GrantView } from '../../generated/views';
import { LoadFailed } from '../access/parts';
import {
  ZOOMS,
  ZOOM_LABELS,
  barGeometry,
  byEnd,
  clockTime,
  groupTimeline,
  labelPlacement,
  parseTimelineQuery,
  rowGrants,
  ticks,
  timelineQueryString,
  type TimelineGroup,
  type TimelineQuery,
  type Zoom,
} from './timeline';

const PAGE = 100;
const TICK_MS = 30_000;
/** Rough width of one character of the bar label at its size, and the bar's own padding, in px. */
const CHAR_PX = 6.6;
const LABEL_PAD_PX = 20;
const MIN_BAR_PX = 4;

const dayFormat = new Intl.DateTimeFormat('en-GB', { weekday: 'short' });
const sameDay = (a: Date, b: Date) => a.toDateString() === b.toDateString();
const until = (at: string, now: number) => {
  const d = new Date(at);
  return sameDay(d, new Date(now)) ? clockTime(at) : `${dayFormat.format(d)} ${clockTime(at)}`;
};

const person = (g: GrantView) => g.requester.email || g.requester.username;
const where = (g: GrantView) => (g.scope === 'Cluster' ? 'cluster-wide' : `in ${(g.namespaces ?? []).join(', ') || 'its namespaces'}`);

function useNow() {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), TICK_MS);
    return () => clearInterval(id);
  }, []);
  return now;
}

/** The element's width, measured whenever it changes; pass the setter as the element's ref. */
function useWidth() {
  const [el, setEl] = useState<HTMLElement | null>(null);
  const [width, setWidth] = useState(0);
  useEffect(() => {
    if (!el) return;
    const observer = new ResizeObserver(([entry]) => setWidth(entry.contentRect.width));
    observer.observe(el);
    return () => observer.disconnect();
  }, [el]);
  return [setEl, width] as const;
}

function toggle(set: Set<string>, key: string): Set<string> {
  const next = new Set(set);
  if (!next.delete(key)) next.add(key);
  return next;
}

function Bar({ grant, cluster, now, zoom, axisPx }: { grant: GrantView; cluster: ClusterView; now: number; zoom: Zoom; axisPx: number }) {
  const { left, width, cut } = barGeometry(grant, now, zoom);
  const barPx = Math.max(MIN_BAR_PX, width * axisPx);
  const end = until(grant.expiresAt, now);
  const text = `${person(grant)} · ${grant.role.name} ${where(grant)} · until ${end}`;
  const inside = labelPlacement(barPx, text.length * CHAR_PX + LABEL_PAD_PX) === 'inside';
  const name = `${text}, on ${cluster.name}${cut ? ', continues past the end of the view' : ''}`;
  const head = (
    <>
      <span className="font-semibold">{person(grant)}</span> · <span className="font-mono">{grant.role.name}</span> {where(grant)}
    </>
  );
  return (
    <li className="relative h-5">
      <Link
        to={grantPath(grant)}
        aria-label={name}
        title={name}
        style={{ left: left * axisPx }}
        className={cn('group absolute inset-y-0 flex max-w-full items-center gap-1.5 rounded-sm text-caption', focusRing)}
      >
        <span
          style={{ width: barPx }}
          className={cn(
            'flex h-4 shrink-0 items-center overflow-hidden whitespace-nowrap bg-accent px-2 text-accent-ink group-hover:bg-accent-hover',
            cut ? 'rounded-l-sm' : 'rounded-sm',
            colorTransition,
          )}
        >
          {inside && (
            <>
              <span className="min-w-0 truncate">{head}</span>
              <span className="ml-auto pl-2 font-semibold">until {end}</span>
            </>
          )}
          {cut && <ChevronsRight className={cn('h-3.5 w-3.5 shrink-0', !inside && 'ml-auto')} aria-hidden />}
        </span>
        {!inside && (
          <span className="truncate whitespace-nowrap bg-surface pr-1 text-text">
            {head} · until {end}
          </span>
        )}
      </Link>
    </li>
  );
}

function ClusterState({ cluster }: { cluster: ClusterView }) {
  if (cluster.paused) {
    return (
      <Badge>
        <CirclePause className="h-3.5 w-3.5 shrink-0" aria-hidden />
        Paused, no new grants
      </Badge>
    );
  }
  if (cluster.agent === 'lost' || cluster.agent === 'waiting') {
    return (
      <Badge tone="warning">
        {cluster.agent === 'lost' ? <TriangleAlert className="h-3.5 w-3.5 shrink-0" aria-hidden /> : <Clock className="h-3.5 w-3.5 shrink-0" aria-hidden />}
        {cluster.agent === 'lost' ? 'Agent not reporting' : 'Waiting for agent'}
      </Badge>
    );
  }
  return null;
}

interface RowProps {
  cluster: ClusterView;
  grants: GrantView[];
  now: number;
  zoom: Zoom;
  axisPx: number;
  expanded: boolean;
  onToggle: () => void;
}

function ClusterRow({ cluster, grants, now, zoom, axisPx, expanded, onToggle }: RowProps) {
  const { shown, hidden } = rowGrants(grants, expanded);
  const display = cluster.displayName && cluster.displayName !== cluster.name ? cluster.displayName : undefined;
  return (
    <li className="flex border-b border-border">
      <div className="relative z-10 flex w-64 shrink-0 flex-col items-start gap-0.5 border-r border-border bg-surface px-4 py-1.5">
        <span className="flex w-full min-w-0 items-baseline gap-2">
          <Identifier className="text-label font-medium text-text">{cluster.name}</Identifier>
          {display && <span className="truncate text-caption text-text-secondary">{display}</span>}
        </span>
        <ClusterState cluster={cluster} />
        {grants.length > 1 && (
          <span className="flex items-center gap-2 text-caption text-text-secondary">
            {plural(grants.length, 'grant')}
            {(hidden > 0 || expanded) && (
              <TextButton onClick={onToggle}>
                {expanded ? 'Show fewer' : `Show ${hidden} more`}
                <span className="sr-only"> on {cluster.name}</span>
              </TextButton>
            )}
          </span>
        )}
      </div>
      <div className="min-w-0 flex-1 py-1.5">
        {grants.length === 0 ? (
          <p className="px-3 text-caption leading-5 text-text-secondary">No live grants</p>
        ) : (
          <ul aria-label={`Live grants on ${cluster.name}`} className="flex flex-col gap-0.5">
            {shown.map((g) => (
              <Bar key={`${g.namespace}/${g.name}`} grant={g} cluster={cluster} now={now} zoom={zoom} axisPx={axisPx} />
            ))}
          </ul>
        )}
      </div>
    </li>
  );
}

function GroupMeta({ group }: { group: TimelineGroup }) {
  const warn = [
    group.lost && `${plural(group.lost, 'agent')} not reporting`,
    group.waiting && `${group.waiting} waiting for agent`,
    group.paused && `${group.paused} paused`,
  ].filter(Boolean);
  return (
    <span className="min-w-0 truncate text-caption text-text-secondary">
      {plural(group.clusters, 'cluster')} · {group.withLive} with live access · {plural(group.grants, 'grant')}
      {warn.map((w) => (
        <span key={String(w)} className="font-semibold text-warning-text">
          {' '}
          · {w}
        </span>
      ))}
    </span>
  );
}

export default function AccessTimeline() {
  const { clusters } = useData();
  const [params, setParams] = useSearchParams();
  const query = parseTimelineQuery(params);
  const setQuery = (next: TimelineQuery) => setParams(new URLSearchParams(timelineQueryString(next)), { replace: true });
  const now = useNow();
  const [axisRef, axisPx] = useWidth();
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const [openIdle, setOpenIdle] = useState<Set<string>>(new Set());
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const paged = usePagedList((offset, limit) => api.getGrants({ state: 'active', offset, limit }), '', PAGE);

  const groups = groupTimeline(clusters, paged.items, query.q);
  const rows = groups.flatMap((g) => g.rows);
  const liveRows = rows.filter((r) => r.grants.length);
  const visible = byEnd([...new Map(rows.flatMap((r) => r.grants).map((g) => [`${g.namespace}/${g.name}`, g])).values()]);
  const axisTicks = ticks(now, query.zoom);
  const searching = query.q.trim() !== '';

  return (
    <>
      <PageHeader
        title="Access timeline"
        description={
          paged.loading
            ? 'Live grants by cluster, from now until they end.'
            : `${plural(visible.length, 'grant')} live on ${plural(liveRows.length, 'cluster')} of ${clusters.length}.`
        }
      />
      <ConsoleToolbar label="Filter the timeline">
        <SearchField
          label="Search clusters, people or roles"
          placeholder="Search clusters, people or roles"
          value={query.q}
          onChange={(q) => setQuery({ ...query, q })}
        />
        <div className="ml-auto hidden items-center gap-2 lg:flex">
          <span className="text-label text-text-secondary" aria-hidden>
            Zoom
          </span>
          <SegmentedControl
            label="Zoom"
            value={query.zoom}
            onValueChange={(zoom) => setQuery({ ...query, zoom })}
            options={(Object.keys(ZOOMS) as Zoom[]).map((z) => ({ value: z, label: ZOOM_LABELS[z] }))}
          />
        </div>
      </ConsoleToolbar>

      {paged.total > PAGE && (
        <div className="flex flex-col gap-2 rounded-md border border-border bg-subtle px-4 py-2 text-label font-normal text-text-secondary sm:flex-row sm:items-center">
          <p>
            More grants are live than are loaded at once: this view shows {paged.items.length} of {paged.total}, in pages of {PAGE}.
          </p>
          <TablePager label="live grants" page={paged.page} pageSize={PAGE} total={paged.total} onPageChange={paged.setPage} className="sm:ml-auto" />
        </div>
      )}

      {paged.error && !paged.items.length ? (
        <LoadFailed what="live grants" error={paged.error} retry={paged.reload} />
      ) : paged.loading ? (
        <div className="flex flex-col gap-3" aria-busy="true">
          <span className="sr-only">Loading</span>
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-40 w-full rounded-lg" />
        </div>
      ) : groups.length === 0 ? (
        <ConsoleCard label="Live grants">
          {searching ? (
            <EmptyState icon={SearchX} title="Nothing matches this search" body="Search by cluster, person or role." />
          ) : (
            <EmptyState icon={ChartGantt} title="No clusters yet" body="Clusters and their live grants appear here once they are registered." />
          )}
        </ConsoleCard>
      ) : (
        <>
          <ConsoleCard label={`Live grants by cluster, from now to ${ZOOM_LABELS[query.zoom]} ahead`} className="hidden lg:block">
            <div className="sticky top-0 z-20 flex h-9 border-b border-border-strong bg-surface" aria-hidden>
              <div className="flex w-64 shrink-0 items-center border-r border-border px-4 text-caption font-semibold text-text-secondary">Cluster</div>
              <div ref={axisRef} className="relative min-w-0 flex-1 overflow-hidden">
                {axisTicks.filter((t) => t.labelled).map((t) => (
                  <span
                    key={t.at}
                    style={{ left: `${t.fraction * 100}%` }}
                    className={cn('absolute top-2 -translate-x-1/2 whitespace-nowrap text-caption text-text-secondary', t.fraction > 0.97 && '-translate-x-full')}
                  >
                    {t.label}
                  </span>
                ))}
                <span className="absolute left-0 top-1.5 rounded-r-sm bg-text px-1.5 text-caption font-semibold text-surface">Now {clockTime(now)}</span>
              </div>
            </div>
            <div className="relative">
              <div className="pointer-events-none absolute inset-y-0 left-64 right-0" aria-hidden>
                {axisTicks.map((t) => (
                  <span key={t.at} style={{ left: `${t.fraction * 100}%` }} className="absolute inset-y-0 w-px bg-border" />
                ))}
                <span className="absolute inset-y-0 left-0 z-10 w-0.5 bg-text" />
              </div>
              {groups.map((group) => {
                const open = !collapsed.has(group.name);
                const idleOpen = openIdle.has(group.name);
                return (
                  <section key={group.name} aria-labelledby={`group-${group.name}`} className="relative">
                    <div className="relative z-10 flex h-8 items-center gap-2 border-b border-border bg-subtle px-2">
                      <h3 id={`group-${group.name}`} className="text-label font-bold">
                        <TextButton tone="plain" aria-expanded={open} onClick={() => setCollapsed((s) => toggle(s, group.name))}>
                          {open ? <ChevronDown className="h-4 w-4 shrink-0" aria-hidden /> : <ChevronRight className="h-4 w-4 shrink-0" aria-hidden />}
                          {group.name}
                        </TextButton>
                      </h3>
                      <GroupMeta group={group} />
                    </div>
                    {open && (
                      <ul aria-label={`Clusters in ${group.name}`}>
                        {group.rows.map((r) => (
                          <ClusterRow
                            key={r.cluster.name}
                            cluster={r.cluster}
                            grants={r.grants}
                            now={now}
                            zoom={query.zoom}
                            axisPx={axisPx}
                            expanded={expanded.has(r.cluster.name)}
                            onToggle={() => setExpanded((s) => toggle(s, r.cluster.name))}
                          />
                        ))}
                        {idleOpen &&
                          group.idle.map((c) => (
                            <ClusterRow key={c.name} cluster={c} grants={[]} now={now} zoom={query.zoom} axisPx={axisPx} expanded={false} onToggle={() => undefined} />
                          ))}
                        {group.idle.length > 0 && (
                          <li className="relative z-10 flex h-8 items-center gap-3 border-b border-border bg-surface pl-10 pr-4 text-caption">
                            <span className="shrink-0 text-text-secondary">{plural(group.idle.length, 'cluster')} with nothing live</span>
                            <span className="min-w-0 flex-1 truncate font-mono text-text-secondary">
                              {idleOpen ? '' : group.idle.map((c) => c.name).join(', ')}
                            </span>
                            <TextButton aria-expanded={idleOpen} onClick={() => setOpenIdle((s) => toggle(s, group.name))}>
                              {idleOpen ? 'Hide' : `Show ${group.idle.length}`}
                              <span className="sr-only"> clusters with nothing live in {group.name}</span>
                              <ChevronDown className={cn('h-4 w-4 shrink-0', idleOpen && 'rotate-180')} aria-hidden />
                            </TextButton>
                          </li>
                        )}
                      </ul>
                    )}
                  </section>
                );
              })}
            </div>
          </ConsoleCard>

          <ConsoleCard label="Live grants, ending soonest first" className="lg:hidden">
            {visible.length === 0 ? (
              <p className="px-4 py-3 text-body text-text-secondary">No one holds access right now.</p>
            ) : (
              <ul className="flex flex-col divide-y divide-border">
                {visible.map((g) => (
                  <li key={`${g.namespace}/${g.name}`} className="flex flex-col gap-0.5 px-4 py-3 text-label font-normal">
                    <span className="flex items-baseline justify-between gap-3">
                      <Link to={grantPath(g)} className={cn('min-w-0 truncate rounded-sm font-semibold text-accent-text hover:underline', focusRing)}>
                        {person(g)}
                      </Link>
                      <TimeLeft expiresAt={g.expiresAt} className="shrink-0 text-text" />
                    </span>
                    <span className="text-text-secondary">
                      <span className="font-mono text-text">{g.role.name}</span> {where(g)} on {(g.clusters ?? []).join(', ')}
                    </span>
                    <span className="text-caption text-text-secondary">until {until(g.expiresAt, now)}</span>
                  </li>
                ))}
              </ul>
            )}
          </ConsoleCard>
        </>
      )}
    </>
  );
}
