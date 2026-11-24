import type { GrantView, ProfileView, RequestView } from '../generated/views';

/** Go's time.Duration string ("1h30m0s") as "1h 30m"; anything unparseable is returned as is. */
export function formatDuration(d: string): string {
  const parts = d.match(/\d+(\.\d+)?[a-zµ]+/g);
  if (!parts) return d;
  const kept = parts.filter((p) => parseFloat(p) !== 0);
  return (kept.length ? kept : parts).join(' ');
}

export const plural = (n: number, word: string) => `${n} ${word}${n === 1 ? '' : 's'}`;

const two = (n: number) => String(n).padStart(2, '0');

/** A local clock time as "HH:mm", 24-hour, the one clock format across the app. */
export function formatClock(at: Date | string | number): string {
  const d = new Date(at);
  return `${two(d.getHours())}:${two(d.getMinutes())}`;
}

const path = (base: string, namespace: string, name: string) =>
  `/${base}/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`;

export const requestPath = (r: Pick<RequestView, 'namespace' | 'name'>) => path('requests', r.namespace, r.name);

export const grantPath = (g: Pick<GrantView, 'namespace' | 'name'>) => path('grants', g.namespace, g.name);

export const requestFormPath = (p: Pick<ProfileView, 'namespace' | 'name'>) => path('request', p.namespace, p.name);

export const profilePath = (p: Pick<ProfileView, 'namespace' | 'name'>) => path('profiles', p.namespace, p.name);

/** The profile's display name, falling back to its object name. */
export function profileName(profiles: ProfileView[], namespace: string, name: string): string {
  return profiles.find((p) => p.namespace === namespace && p.name === name)?.displayName || name;
}
