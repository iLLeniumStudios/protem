import { byTime } from '../../app/order';
import { toQuery } from '../../app/paging';
import type { ClusterView, GrantView } from '../../generated/views';
import { sortClusters } from '../fleet/table';

const HOUR = 3_600_000;
export const ZOOMS = { '6h': 6 * HOUR, '24h': 24 * HOUR, '7d': 7 * 24 * HOUR } as const;
export type Zoom = keyof typeof ZOOMS;
export const ZOOM_LABELS: Record<Zoom, string> = { '6h': '6 h', '24h': '24 h', '7d': '7 d' };
const TICK_HOURS: Record<Zoom, number> = { '6h': 1, '24h': 2, '7d': 24 };
export const ROW_LIMIT = 3;
const NO_ENV = 'No environment';
const ALL = 'All clusters';

type Timed = Pick<GrantView, 'startedAt' | 'expiresAt'>;
const clamp = (x: number) => Math.min(1, Math.max(0, x));

/** Where a grant's bar sits on an axis that starts at `now`, as fractions of the window; `cut` when it ends past the window. */
export function barGeometry(g: Timed, now: number, zoom: Zoom): { left: number; width: number; cut: boolean } {
  const span = ZOOMS[zoom];
  const end = Date.parse(g.expiresAt);
  const left = clamp((Math.max(Date.parse(g.startedAt), now) - now) / span);
  const right = clamp((end - now) / span);
  return { left, width: Math.max(0, right - left), cut: end > now + span };
}

export function labelPlacement(barPx: number, labelPx: number): 'inside' | 'beside' {
  return barPx >= labelPx ? 'inside' : 'beside';
}

export const byEnd = (grants: GrantView[]) => [...grants].sort(byTime<GrantView>('asc', (g) => g.expiresAt, (g) => g.namespace, (g) => g.name));

export function rowGrants(grants: GrantView[], expanded: boolean): { shown: GrantView[]; hidden: number } {
  if (expanded || grants.length <= ROW_LIMIT) return { shown: grants, hidden: 0 };
  return { shown: grants.slice(0, ROW_LIMIT), hidden: grants.length - ROW_LIMIT };
}

export interface TimelineRow {
  cluster: ClusterView;
  grants: GrantView[];
}

export interface TimelineGroup {
  name: string;
  /** Clusters with live grants or an agent or pause to look at. */
  rows: TimelineRow[];
  /** Clusters with nothing live and nothing wrong, shown as one summary line. */
  idle: ClusterView[];
  clusters: number;
  withLive: number;
  grants: number;
  lost: number;
  waiting: number;
  paused: number;
}

const has = (q: string, ...values: (string | undefined)[]) => values.some((v) => v?.toLocaleLowerCase('en').includes(q));

const quiet = (c: ClusterView) => !c.paused && (c.agent === 'connected' || c.agent === 'none');

/** Clusters grouped by their env label, each with the grants matching `query` (cluster, person or role). */
export function groupTimeline(clusters: ClusterView[], grants: GrantView[], query: string): TimelineGroup[] {
  const q = query.trim().toLocaleLowerCase('en');
  const anyEnv = clusters.some((c) => c.labels?.env);
  const sorted = byEnd(grants);
  const byGroup = new Map<string, TimelineRow[]>();
  for (const cluster of sortClusters(clusters)) {
    const mine = sorted.filter((g) => g.clusters?.includes(cluster.name));
    const nameMatches = !q || has(q, cluster.name, cluster.displayName);
    const shown = nameMatches ? mine : mine.filter((g) => has(q, g.requester.username, g.requester.email, g.role.name));
    if (!nameMatches && !shown.length) continue;
    const name = anyEnv ? (cluster.labels?.env ?? NO_ENV) : ALL;
    byGroup.set(name, [...(byGroup.get(name) ?? []), { cluster, grants: shown }]);
  }
  const names = [...byGroup.keys()].sort((a, b) => Number(a === NO_ENV) - Number(b === NO_ENV) || (a < b ? -1 : a > b ? 1 : 0));
  return names.map((name) => {
    const all = byGroup.get(name)!;
    const count = (agent: string) => all.filter((r) => r.cluster.agent === agent).length;
    return {
      name,
      rows: all.filter((r) => r.grants.length || !quiet(r.cluster)),
      idle: all.filter((r) => !r.grants.length && quiet(r.cluster)).map((r) => r.cluster),
      clusters: all.length,
      withLive: all.filter((r) => r.grants.length).length,
      grants: new Set(all.flatMap((r) => r.grants.map((g) => `${g.namespace}/${g.name}`))).size,
      lost: count('lost'),
      waiting: count('waiting'),
      paused: all.filter((r) => r.cluster.paused).length,
    };
  });
}

const timeFormat = new Intl.DateTimeFormat('en-GB', { hour: '2-digit', minute: '2-digit' });
const dayFormat = new Intl.DateTimeFormat('en-GB', { weekday: 'short' });
const dateFormat = new Intl.DateTimeFormat('en-GB', { weekday: 'short', day: 'numeric' });

export function clockTime(at: number | string): string {
  return timeFormat.format(new Date(at));
}

/** The share of the axis the "Now HH:mm" label covers at the narrowest width the axis is shown at. */
const NOW_LABEL_SHARE = 0.12;

/** Axis ticks on whole local hours (or days) after `now`, up to the window's end. */
export function ticks(now: number, zoom: Zoom): { at: number; fraction: number; label: string; labelled: boolean }[] {
  const step = TICK_HOURS[zoom];
  const d = new Date(now);
  if (zoom === '7d') d.setHours(24, 0, 0, 0);
  else d.setHours(d.getHours() + 1, 0, 0, 0);
  if (d.getHours() % step) d.setHours(d.getHours() + 1);
  const out = [];
  for (; d.getTime() <= now + ZOOMS[zoom]; d.setHours(d.getHours() + step)) {
    const label = zoom === '7d' ? dateFormat.format(d) : d.getHours() === 0 ? `${dayFormat.format(d)} ${timeFormat.format(d)}` : timeFormat.format(d);
    const fraction = (d.getTime() - now) / ZOOMS[zoom];
    out.push({ at: d.getTime(), fraction, label, labelled: fraction >= NOW_LABEL_SHARE });
  }
  return out;
}

export interface TimelineQuery {
  q: string;
  zoom: Zoom;
}

export function parseTimelineQuery(params: URLSearchParams): TimelineQuery {
  const zoom = params.get('zoom');
  return { q: params.get('q') ?? '', zoom: zoom && Object.prototype.hasOwnProperty.call(ZOOMS, zoom) ? (zoom as Zoom) : '6h' };
}

export function timelineQueryString(query: TimelineQuery): string {
  return toQuery({ q: query.q, zoom: query.zoom === '6h' ? undefined : query.zoom });
}
