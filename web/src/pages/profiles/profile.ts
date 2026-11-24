import type { LabelSelector } from '../../generated/crd';
import type { ProfileView } from '../../generated/views';

const OPERATORS: Record<string, string> = { In: 'in', NotIn: 'notin' };

/** A label selector as kubectl prints it, one term per entry. */
export function selectorTerms(selector: LabelSelector | null | undefined): string[] {
  const labels = Object.entries(selector?.matchLabels ?? {}).map(([k, v]) => `${k}=${v}`);
  const expressions = (selector?.matchExpressions ?? []).map(({ key, operator, values }) => {
    if (operator === 'Exists') return key;
    if (operator === 'DoesNotExist') return `!${key}`;
    return `${key} ${OPERATORS[operator] ?? operator} (${(values ?? []).join(', ')})`;
  });
  return [...labels, ...expressions];
}

export function scopeText(scope: string): string {
  return scope === 'Cluster' ? 'Cluster: every namespace, cluster-wide' : scope;
}

type Searchable = Pick<ProfileView, 'namespace' | 'name' | 'displayName' | 'description' | 'role'>;

const byKey = (a: string, b: string) => (a < b ? -1 : a > b ? 1 : 0);

/** Profiles whose display name, name, description or role contains `query` (any case), by display name, then namespace and name. */
export function searchProfiles<T extends Searchable>(profiles: T[], query: string): T[] {
  const q = query.trim().toLocaleLowerCase('en');
  return profiles
    .filter((p) => !q || [p.displayName, p.name, p.description, p.role?.name].some((v) => v?.toLocaleLowerCase('en').includes(q)))
    .sort(
      (a, b) =>
        (a.displayName || a.name).localeCompare(b.displayName || b.name, 'en') ||
        byKey(a.namespace, b.namespace) ||
        byKey(a.name, b.name),
    );
}
