// Run: cd web && bun test src/pages/profiles
import { expect, test } from 'bun:test';
import { activationOf, filterProfiles, parseProfileQuery, profileFacets, profileQueryString, whoCanRequest } from './list';

const p = (namespace, name, over = {}) => ({
  namespace,
  name,
  displayName: name,
  role: { kind: 'ClusterRole', name: 'view' },
  scope: 'Namespaced',
  clusters: [],
  defaultDuration: '1h0m0s',
  maxDuration: '8h0m0s',
  reasonRequired: true,
  activation: 'at-once',
  canRequest: true,
  canApprove: false,
  ...over,
});

const all = [
  p('team-a', 'a-view'),
  p('team-a', 'a-admin', { role: { kind: 'ClusterRole', name: 'admin' }, activation: 'approval' }),
  p('team-b', 'b-edit', { displayName: 'Billing editors', role: { kind: 'Role', name: 'edit' }, activation: 'approval' }),
  p('team-b', 'b-mixed', { displayName: 'Mixed', activation: 'mixed' }),
];
const names = (list) => list.map((x) => x.name);
const q = (over = {}) => ({ q: '', page: 0, ...over });

test('filters by namespace, role and how it is granted, on top of the search, in display-name order', () => {
  expect(names(filterProfiles(all, q()))).toEqual(['a-admin', 'a-view', 'b-edit', 'b-mixed']);
  expect(names(filterProfiles(all, q({ ns: 'team-a' })))).toEqual(['a-admin', 'a-view']);
  expect(names(filterProfiles(all, q({ role: 'edit' })))).toEqual(['b-edit']);
  expect(names(filterProfiles(all, q({ grant: 'at-once' })))).toEqual(['a-view']);
  expect(names(filterProfiles(all, q({ grant: 'mixed' })))).toEqual(['b-mixed']);
  expect(names(filterProfiles(all, q({ grant: 'approval', ns: 'team-a' })))).toEqual(['a-admin']);
  expect(names(filterProfiles(all, q({ q: 'billing' })))).toEqual(['b-edit']);
});

test('how a profile is granted: an unknown or empty activation counts as with approval', () => {
  expect(activationOf(p('n', 'x'))).toBe('at-once');
  expect(activationOf(p('n', 'x', { activation: 'mixed' }))).toBe('mixed');
  expect(activationOf(p('n', 'x', { activation: 'approval' }))).toBe('approval');
  expect(activationOf(p('n', 'x', { activation: '' }))).toBe('approval');
  expect(activationOf(p('n', 'x', { activation: 'sometimes' }))).toBe('approval');
  expect(names(filterProfiles([p('n', 'odd', { activation: 'sometimes' })], q({ grant: 'approval' })))).toEqual(['odd']);
});

test('the filter options are the namespaces and roles that exist, sorted', () => {
  expect(profileFacets(all)).toEqual({ namespaces: ['team-a', 'team-b'], roles: ['admin', 'edit', 'view'] });
});

test('the query string round-trips and drops what it does not know', () => {
  const full = { q: 'pay', ns: 'team-a', role: 'view', grant: 'approval', page: 1 };
  expect(profileQueryString(full)).toBe('?q=pay&ns=team-a&role=view&grant=approval&page=2');
  expect(parseProfileQuery(new URLSearchParams(profileQueryString(full)))).toEqual(full);
  expect(profileQueryString(q())).toBe('');
  expect(parseProfileQuery(new URLSearchParams('grant=sometimes&page=0'))).toEqual(q());
});

test('old links with grant=auto or grant=manual still filter', () => {
  expect(parseProfileQuery(new URLSearchParams('grant=auto'))).toEqual(q({ grant: 'at-once' }));
  expect(parseProfileQuery(new URLSearchParams('grant=manual'))).toEqual(q({ grant: 'approval' }));
  expect(parseProfileQuery(new URLSearchParams('grant=mixed'))).toEqual(q({ grant: 'mixed' }));
  expect(profileQueryString(q({ grant: 'at-once' }))).toBe('?grant=at-once');
});

test('who can request: groups first, then users, two shown and the rest counted', () => {
  expect(whoCanRequest(p('n', 'x', { requesterGroups: ['sre', 'dev'], requesterUsers: ['ann@x.io'] }))).toEqual({
    shown: ['sre', 'dev'],
    more: 1,
    all: ['sre', 'dev', 'ann@x.io'],
  });
  expect(whoCanRequest(p('n', 'x', { requesterUsers: ['ann@x.io'] }))).toEqual({ shown: ['ann@x.io'], more: 0, all: ['ann@x.io'] });
  expect(whoCanRequest(p('n', 'x'))).toEqual({ shown: [], more: 0, all: [] });
});
