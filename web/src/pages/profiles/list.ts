import { toQuery } from '../../app/paging';
import type { ProfileView } from '../../generated/views';
import { pageParam } from '../fleet/table';
import { searchProfiles } from './profile';

const GRANT_MODES = ['at-once', 'approval', 'mixed'] as const;
export type GrantMode = (typeof GRANT_MODES)[number];
const OLD_GRANT_MODES = new Map<string, GrantMode>([
  ['auto', 'at-once'],
  ['manual', 'approval'],
]);

export interface ProfileQuery {
  q: string;
  ns?: string;
  role?: string;
  grant?: GrantMode;
  /** 0-based; the URL carries it 1-based. */
  page: number;
}

export function activationOf(p: Pick<ProfileView, 'activation'>): GrantMode {
  return GRANT_MODES.find((m) => m === p.activation) ?? 'approval';
}

export function filterProfiles(profiles: ProfileView[], query: ProfileQuery): ProfileView[] {
  return searchProfiles(profiles, query.q).filter(
    (p) =>
      (!query.ns || p.namespace === query.ns) &&
      (!query.role || p.role.name === query.role) &&
      (!query.grant || activationOf(p) === query.grant),
  );
}

const sortedUnique = (values: string[]) => [...new Set(values)].sort((a, b) => (a < b ? -1 : a > b ? 1 : 0));

export function profileFacets(profiles: ProfileView[]): { namespaces: string[]; roles: string[] } {
  return { namespaces: sortedUnique(profiles.map((p) => p.namespace)), roles: sortedUnique(profiles.map((p) => p.role.name)) };
}

export function parseProfileQuery(params: URLSearchParams): ProfileQuery {
  const out: ProfileQuery = { q: params.get('q') ?? '', page: pageParam(params.get('page')) };
  const ns = params.get('ns');
  const role = params.get('role');
  const grant = params.get('grant') ?? '';
  if (ns) out.ns = ns;
  if (role) out.role = role;
  const mode = GRANT_MODES.find((m) => m === grant) ?? OLD_GRANT_MODES.get(grant);
  if (mode) out.grant = mode;
  return out;
}

export function profileQueryString(query: ProfileQuery): string {
  return toQuery({
    q: query.q,
    ns: query.ns,
    role: query.role,
    grant: query.grant,
    page: query.page > 0 ? query.page + 1 : undefined,
  });
}

const SHOWN = 2;

export interface Few {
  shown: string[];
  more: number;
  all: string[];
}

export function firstFew(all: string[]): Few {
  return { shown: all.slice(0, SHOWN), more: Math.max(0, all.length - SHOWN), all };
}

export function whoCanRequest(p: Pick<ProfileView, 'requesterGroups' | 'requesterUsers'>): Few {
  return firstFew([...(p.requesterGroups ?? []), ...(p.requesterUsers ?? [])]);
}
