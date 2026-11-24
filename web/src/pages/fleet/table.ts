import { toQuery } from '../../app/paging';
import type { ClusterView } from '../../generated/views';
import { clusterStatus, type ClusterState } from './fleet';

export const AGENT_STATES = ['connected', 'lost', 'waiting', 'none'] as const;
export type AgentFilter = (typeof AGENT_STATES)[number];
const STATUSES = ['ready', 'paused'] as const;
type StatusFilter = (typeof STATUSES)[number];

export type Figure = 'ready' | 'lost' | 'paused' | 'waiting';

export interface ClusterQuery {
  q: string;
  status?: StatusFilter;
  agent?: AgentFilter;
  env?: string;
  /** 0-based; the URL carries it 1-based. */
  page: number;
}

const FIGURES: Record<Figure, Pick<ClusterQuery, 'status' | 'agent'>> = {
  ready: { status: 'ready' },
  lost: { agent: 'lost' },
  paused: { status: 'paused' },
  waiting: { agent: 'waiting' },
};

function matches(c: ClusterView, query: Omit<ClusterQuery, 'page'>): boolean {
  if (query.status === 'ready' && clusterStatus(c).state !== 'Ready') return false;
  if (query.status === 'paused' && !c.paused) return false;
  if (query.agent && c.agent !== query.agent) return false;
  if (query.env && c.labels?.env !== query.env) return false;
  const q = query.q.trim().toLocaleLowerCase('en');
  if (!q) return true;
  const labels = Object.entries(c.labels ?? {}).map(([k, v]) => `${k}=${v}`);
  return [c.name, c.displayName, ...labels].some((v) => v?.toLocaleLowerCase('en').includes(q));
}

export function filterClusters(clusters: ClusterView[], query: ClusterQuery): ClusterView[] {
  return clusters.filter((c) => matches(c, query));
}

/** The summary figures, each counting what choosing it shows; "waiting for agent" only when a cluster waits. */
export function clusterFigures(clusters: ClusterView[]): { figure: Figure; count: number }[] {
  return (Object.keys(FIGURES) as Figure[])
    .map((figure) => ({ figure, count: filterClusters(clusters, { q: '', page: 0, ...FIGURES[figure] }).length }))
    .filter((f) => f.figure !== 'waiting' || f.count > 0);
}

/** The figure the query shows exactly, if any. */
export function figureOf(query: ClusterQuery): Figure | undefined {
  return (Object.keys(FIGURES) as Figure[]).find((f) => query.status === FIGURES[f].status && query.agent === FIGURES[f].agent);
}

export function chooseFigure(query: ClusterQuery, figure: Figure): ClusterQuery {
  const { status: _s, agent: _a, ...rest } = query;
  return { ...rest, page: 0, ...(figureOf(query) === figure ? {} : FIGURES[figure]) };
}

const ATTENTION_FIRST: Record<ClusterState, number> = { NotReady: 0, Paused: 1, Ready: 2 };
const byKey = (a: string, b: string) => (a < b ? -1 : a > b ? 1 : 0);

export function sortClusters(clusters: ClusterView[]): ClusterView[] {
  const rank = (c: ClusterView) => ATTENTION_FIRST[clusterStatus(c).state];
  return [...clusters].sort(
    (a, b) =>
      rank(a) - rank(b) ||
      (a.displayName || a.name).localeCompare(b.displayName || b.name, 'en') ||
      byKey(a.name, b.name),
  );
}

export function envOptions(clusters: ClusterView[]): string[] {
  return [...new Set(clusters.map((c) => c.labels?.env).filter((e): e is string => Boolean(e)))].sort(byKey);
}

const oneOf = <T extends string>(values: readonly T[], v: string | null): T | undefined =>
  values.find((x) => x === v);

export function pageParam(v: string | null): number {
  const n = Number(v);
  return Number.isInteger(n) && n > 1 ? n - 1 : 0;
}

export function parseClusterQuery(params: URLSearchParams): ClusterQuery {
  const out: ClusterQuery = { q: params.get('q') ?? '', page: pageParam(params.get('page')) };
  const status = oneOf(STATUSES, params.get('status'));
  const agent = oneOf(AGENT_STATES, params.get('agent'));
  const env = params.get('env');
  if (status) out.status = status;
  if (agent) out.agent = agent;
  if (env) out.env = env;
  return out;
}

export function clusterQueryString(query: ClusterQuery): string {
  return toQuery({
    q: query.q,
    status: query.status,
    agent: query.agent,
    env: query.env,
    page: query.page > 0 ? query.page + 1 : undefined,
  });
}

export type AllowedRoles = { kind: 'operator' } | { kind: 'any' } | { kind: 'unreported' } | { kind: 'roles'; roles: string[]; partial: boolean };

export function allowedRolesText(c: Pick<ClusterView, 'agent' | 'allowedRoles' | 'allowedRolesPartial'>): AllowedRoles {
  if (c.agent === 'none') return { kind: 'operator' };
  if (!c.allowedRoles) return { kind: 'unreported' };
  if (c.allowedRoles.includes('*')) return { kind: 'any' };
  return { kind: 'roles', roles: c.allowedRoles, partial: Boolean(c.allowedRolesPartial) };
}
