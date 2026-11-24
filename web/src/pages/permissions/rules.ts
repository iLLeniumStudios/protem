import type { PermissionRuleView } from '../../generated/views';
import { plural } from '../../app/format';

const flagged = (r: PermissionRuleView) => Boolean(r.risks?.length);

export function flaggedFirst(rules: PermissionRuleView[]): PermissionRuleView[] {
  return [...rules.filter(flagged), ...rules.filter((r) => !flagged(r))];
}

export function rulesSummary(rules: PermissionRuleView[]): string {
  if (rules.length === 0) return 'This role grants no permissions.';
  const n = rules.filter(flagged).length;
  if (n === 0) return `${plural(rules.length, 'rule')}, none flagged for review.`;
  return `${n} of ${plural(rules.length, 'rule')} ${n === 1 ? 'is' : 'are'} flagged for review.`;
}

/** RBAC values as people read them: `*` is "all", and in API groups `""` is the core group. */
export function cellValues(values: string[] | null | undefined, kind?: 'group'): string[] {
  return (values ?? []).map((v) => (v === '*' ? 'all' : kind === 'group' && v === '' ? 'core' : v));
}

const NAMED = 4;

/** The rules not flagged, in one plain sentence: how many, and which resources they cover. Empty when every rule is flagged. */
export function otherRulesSummary(rules: PermissionRuleView[]): string {
  const others = rules.filter((r) => !flagged(r));
  if (others.length === 0) return '';
  const lead = `${plural(others.length, 'other rule')} ${others.length === 1 ? 'covers' : 'cover'}`;
  const resources = [...new Set(others.flatMap((r) => r.resources ?? []))].map((v) => (v === '*' ? 'all resources' : v));
  if (resources.length === 0) {
    const url = others.flatMap((r) => r.nonResourceURLs ?? [])[0];
    return url ? `${lead} no resources, only URLs such as ${url}.` : `${plural(others.length, 'other rule')}.`;
  }
  const named = resources.slice(0, NAMED);
  const rest = resources.length - named.length;
  if (rest > 0) return `${lead} ${named.join(', ')} and ${rest} more.`;
  return `${lead} ${named.length > 1 ? `${named.slice(0, -1).join(', ')} and ${named[named.length - 1]}` : named[0]}.`;
}
