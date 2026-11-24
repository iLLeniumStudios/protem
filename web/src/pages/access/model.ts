import type { ProfileView, RequestView, RequirementView } from '../../generated/views';

const PRESETS = ['15m', '30m', '1h', '2h', '4h', '8h'];

/** Minutes in a Go duration ("1h30m0s") or a preset ("15m"); NaN when unparseable. */
function minutes(d: string): number {
  const m = /^(?:(\d+)h)?(?:(\d+)m)?(?:(\d+)s)?$/.exec(d);
  if (!d || !m) return NaN;
  return Number(m[1] ?? 0) * 60 + Number(m[2] ?? 0) + Number(m[3] ?? 0) / 60;
}

/** Duration in the form Go's time.ParseDuration accepts, e.g. "1h30m". */
function compact(total: number): string {
  const rounded = Math.round(total);
  const h = Math.floor(rounded / 60);
  const m = rounded % 60;
  return `${h ? `${h}h` : ''}${m || !h ? `${m}m` : ''}`;
}

export function defaultDuration(profile: Pick<ProfileView, 'defaultDuration'>): string {
  return compact(minutes(profile.defaultDuration));
}

export function durationOptions(profile: Pick<ProfileView, 'defaultDuration' | 'maxDuration'>): string[] {
  const max = minutes(profile.maxDuration);
  const options = PRESETS.filter((p) => !(minutes(p) > max));
  const fallback = defaultDuration(profile);
  if (!options.includes(fallback)) options.push(fallback);
  return options.sort((a, b) => minutes(a) - minutes(b));
}

/**
 * Manual approvals so far over approvals needed, counted per rule: the server records one
 * approval per rule, covering every cluster that rule applies to.
 */
export function approvalProgress(req: Pick<RequestView, 'requirements'>): { approved: number; required: number } | null {
  const rules = new Map<string, { required: number; approved: number }>();
  for (const q of req.requirements ?? []) {
    if (q.mode !== 'Manual') continue;
    const key = q.rule || q.cluster;
    const prev = rules.get(key);
    rules.set(key, {
      required: Math.max(prev?.required ?? 0, q.required),
      approved: Math.max(prev?.approved ?? 0, q.approvedBy?.length ?? 0),
    });
  }
  if (rules.size === 0) return null;
  let approved = 0;
  let required = 0;
  for (const rule of rules.values()) {
    required += rule.required;
    approved += Math.min(rule.approved, rule.required);
  }
  return { approved, required };
}

/** The groups and named users who can satisfy a manual requirement. */
export function approvers(r: Pick<RequirementView, 'approverGroups' | 'approverUsers'>): string[] {
  return [...(r.approverGroups ?? []), ...(r.approverUsers ?? [])];
}

/** Clusters a request may target: the server lists the profile's allowed clusters that are ready. */
export function defaultClusters(profile: Pick<ProfileView, 'clusters'>): string[] {
  return [...(profile.clusters ?? [])];
}

export function ticketError(pattern: string | undefined, ticket: string): string | undefined {
  if (!pattern) return undefined;
  let re: RegExp;
  try {
    re = new RegExp(pattern, 'u');
  } catch {
    // Go's RE2 accepts patterns JavaScript rejects; the server validates the ticket instead.
    return undefined;
  }
  const value = ticket.trim();
  if (re.test(value)) return undefined;
  return value ? `Use the format ${pattern}.` : 'Enter a ticket ID.';
}

export type RequestRef = Pick<RequestView, 'namespace' | 'name'>;

/** The `request` URL parameter, "<namespace>/<name>"; null unless it names exactly one request. */
export function parseRequestRef(value: string | null): RequestRef | null {
  const parts = (value ?? '').split('/');
  if (parts.length !== 2 || !parts[0] || !parts[1]) return null;
  return { namespace: parts[0], name: parts[1] };
}

/** `search` with the `request` parameter set to `ref`, or removed for null; the slash stays readable. */
export function withRequestRef(search: string, ref: RequestRef | null): string {
  const params = new URLSearchParams(search);
  params.delete('request');
  const rest = params.toString();
  const own = ref && `request=${encodeURIComponent(ref.namespace)}/${encodeURIComponent(ref.name)}`;
  const query = [rest, own].filter(Boolean).join('&');
  return query ? `?${query}` : '';
}
