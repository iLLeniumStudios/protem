import { oldestFirst } from '../../app/order';
import type { ClusterView, GrantView } from '../../generated/views';

export type ClusterState = 'Ready' | 'NotReady' | 'Paused';

/** One status per cluster: paused is deliberate, so it is never reported as not ready. */
export function clusterStatus(c: Pick<ClusterView, 'ready' | 'paused' | 'phase'>): { state: ClusterState; detail?: string } {
  if (c.paused) return { state: 'Paused', detail: 'New requests cannot target it' };
  if (c.ready) return { state: 'Ready' };
  return c.phase && c.phase !== 'Ready' ? { state: 'NotReady', detail: c.phase } : { state: 'NotReady' };
}

export function clusterHealth(clusters: ClusterView[]) {
  const states = clusters.map((c) => clusterStatus(c).state);
  const unhealthy = clusters.filter((_, i) => states[i] === 'NotReady').sort((a, b) => a.name.localeCompare(b.name));
  return {
    ready: states.filter((s) => s === 'Ready').length,
    paused: states.filter((s) => s === 'Paused').length,
    total: clusters.length,
    unhealthy,
  };
}

export function liveGrants(grants: GrantView[]): GrantView[] {
  return grants
    .filter((g) => g.phase === 'Active')
    .sort(oldestFirst((g) => g.expiresAt));
}
