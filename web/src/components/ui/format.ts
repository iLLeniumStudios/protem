import { formatClock } from '../../app/format';

const MINUTE = 60_000;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;

const toDate = (v: Date | string) => (v instanceof Date ? v : new Date(v));

export function formatTimeLeft(expiresAt: Date | string, now: Date): string {
  const ms = toDate(expiresAt).getTime() - now.getTime();
  if (!(ms > 0)) return 'Expired';
  const minutes = Math.ceil(ms / MINUTE);
  const d = Math.floor(minutes / (DAY / MINUTE));
  const h = Math.floor((minutes % (DAY / MINUTE)) / 60);
  const m = minutes % 60;
  if (d > 0) return h > 0 ? `${d}d ${h}h left` : `${d}d left`;
  if (h > 0) return m > 0 ? `${h}h ${m}m left` : `${h}h left`;
  return `${m}m left`;
}

export function formatRelative(at: Date | string, now: Date): string {
  const date = toDate(at);
  const ms = now.getTime() - date.getTime();
  if (ms < MINUTE) return 'just now';
  if (ms < HOUR) return `${Math.floor(ms / MINUTE)} min ago`;
  if (ms < DAY) return `${Math.floor(ms / HOUR)} h ago`;
  if (ms < 30 * DAY) return `${Math.floor(ms / DAY)} d ago`;
  return formatTimestamp(date, now);
}

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];

/** An absolute local time, the one shape across the app: "09:01" today, "24 Nov, 09:01" another day, "24 Nov 2025, 09:01" another year. */
export function formatTimestamp(at: Date | string, now = new Date()): string {
  const d = toDate(at);
  const clock = formatClock(d);
  if (d.toDateString() === now.toDateString()) return clock;
  const day = `${d.getDate()} ${MONTHS[d.getMonth()]}`;
  return d.getFullYear() === now.getFullYear() ? `${day}, ${clock}` : `${day} ${d.getFullYear()}, ${clock}`;
}

export function formatElapsed(since: Date | string, now: Date): string {
  const ms = now.getTime() - toDate(since).getTime();
  if (ms < MINUTE) return 'under 1 min';
  if (ms < HOUR) return `${Math.floor(ms / MINUTE)} min`;
  if (ms < DAY) return `${Math.floor(ms / HOUR)} h`;
  return `${Math.floor(ms / DAY)} d`;
}

/** Elapsed share of a grant's lifetime, 0 before it starts and 1 once it is due or past due. */
export function timeUsedFraction(startedAt: string, expiresAt: string, now: Date): number {
  const start = Date.parse(startedAt);
  const end = Date.parse(expiresAt);
  if (!Number.isFinite(start) || !Number.isFinite(end) || end <= start) return 1;
  return Math.min(1, Math.max(0, (now.getTime() - start) / (end - start)));
}

const ENDING_SOON_MS = 30 * MINUTE;

export function endingSoon(expiresAt: Date | string, now: Date): boolean {
  const left = toDate(expiresAt).getTime() - now.getTime();
  return left > 0 && left < ENDING_SOON_MS;
}
