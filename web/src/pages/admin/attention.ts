import { byTime, oldestFirst } from '../../app/order';
import type { ClusterView, RequestView } from '../../generated/views';

export function attentionClusters(clusters: ClusterView[]): ClusterView[] {
  return clusters
    .filter((c) => c.agent === 'lost' || c.agent === 'waiting')
    .sort(byTime<ClusterView>('asc', (c) => c.lastHeartbeat, (c) => c.name));
}

export function waitingLongest(requests: RequestView[], n: number): RequestView[] {
  return requests
    .filter((r) => r.phase === 'Pending')
    .sort(oldestFirst((r) => r.createdAt))
    .slice(0, n);
}
