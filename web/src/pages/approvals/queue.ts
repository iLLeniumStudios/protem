import type { RequestView } from '../../generated/views';

const LONG_WAIT_MS = 30 * 60_000;

const time = (at: string) => new Date(at).getTime();

export function decidedAt(r: RequestView): string {
  return (r.approvals ?? []).reduce((latest, d) => (time(d.at) > time(latest) ? d.at : latest), r.createdAt);
}

export function isLongWait(createdAt: string, now: Date): boolean {
  return now.getTime() - time(createdAt) > LONG_WAIT_MS;
}
