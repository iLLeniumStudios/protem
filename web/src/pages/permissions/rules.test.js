// Run: cd web && bun test src/pages/permissions
import { expect, test } from 'bun:test';
import { cellValues, flaggedFirst, otherRulesSummary, rulesSummary } from './rules';

const rule = (resources, risks) => ({ apiGroups: [''], resources, verbs: ['get'], risks });

test('flagged rules come first, each group keeping the role order', () => {
  const rules = [rule(['pods']), rule(['secrets'], ['Reads secrets']), rule(['services']), rule(['*'], ['Every resource']), rule(['nodes'], [])];
  expect(flaggedFirst(rules).map((r) => r.resources[0])).toEqual(['secrets', '*', 'pods', 'services', 'nodes']);
  expect(rules[0].resources[0]).toBe('pods');
});

test('the summary counts flagged rules', () => {
  expect(rulesSummary([rule(['pods']), rule(['secrets'], ['Reads secrets']), rule(['*'], ['Every resource'])])).toBe(
    '2 of 3 rules are flagged for review.',
  );
  expect(rulesSummary([rule(['secrets'], ['Reads secrets'])])).toBe('1 of 1 rule is flagged for review.');
  expect(rulesSummary([rule(['pods']), rule(['services'], [])])).toBe('2 rules, none flagged for review.');
  expect(rulesSummary([])).toBe('This role grants no permissions.');
});

test('the core API group and wildcards read as words', () => {
  expect(cellValues(['', 'apps', '*'], 'group')).toEqual(['core', 'apps', 'all']);
  expect(cellValues(['*'])).toEqual(['all']);
  expect(cellValues(['get', 'list'])).toEqual(['get', 'list']);
  expect(cellValues([''])).toEqual(['']);
  expect(cellValues(undefined)).toEqual([]);
  expect(cellValues(null, 'group')).toEqual([]);
});

test('the rules not flagged read as a count and the resources they cover, a few named and the rest counted', () => {
  const rule = (resources, risks) => ({ resources, verbs: ['get'], risks });
  const rules = [
    rule(['secrets'], ['Can read Secrets.']),
    rule(['pods', 'services']),
    rule(['pods', 'configmaps', '*']),
    rule(['deployments', 'jobs', 'cronjobs']),
    rule(undefined),
  ];
  expect(otherRulesSummary(rules)).toBe('4 other rules cover pods, services, configmaps, all resources and 3 more.');
  expect(otherRulesSummary([rule(['pods'])])).toBe('1 other rule covers pods.');
  expect(otherRulesSummary([rule(['secrets'], ['x'])])).toBe('');
  expect(otherRulesSummary([{ nonResourceURLs: ['/healthz'], verbs: ['get'] }])).toBe('1 other rule covers no resources, only URLs such as /healthz.');
});
