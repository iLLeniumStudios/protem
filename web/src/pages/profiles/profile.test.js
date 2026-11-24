// Run: cd web && bun test src/pages/profiles
import { expect, test } from 'bun:test';
import { slicePage } from '../../app/paging';
import { scopeText, searchProfiles, selectorTerms } from './profile';

const p = (namespace, name, displayName, description, role) => ({ namespace, name, displayName, description, role: { kind: 'ClusterRole', name: role } });

test('profile search matches display name, object name, description and role, case-insensitively, in name order', () => {
  const list = [
    p('b', 'db', 'Database admin', 'Break-glass for Postgres', 'admin'),
    p('a', 'view', 'Read only', undefined, 'view'),
    p('a', 'debug', 'Prod debug', 'Exec into pods', 'pod-exec'),
    p('c', 'db', 'Database admin', undefined, 'admin'),
  ];
  const names = (q) => searchProfiles(list, q).map((x) => `${x.namespace}/${x.name}`);
  expect(names('')).toEqual(['b/db', 'c/db', 'a/debug', 'a/view']);
  expect(names('  POSTGRES ')).toEqual(['b/db']);
  expect(names('pod-exec')).toEqual(['a/debug']);
  expect(names('view')).toEqual(['a/view']);
  expect(names('admin')).toEqual(['b/db', 'c/db']);
  expect(names('nothing')).toEqual([]);
});

test('searched profiles page at 20', () => {
  const many = Array.from({ length: 45 }, (_, i) => p('ns', `p${String(i).padStart(2, '0')}`, `Profile ${String(i).padStart(2, '0')}`, '', 'view'));
  const found = searchProfiles(many, 'profile');
  expect(found).toHaveLength(45);
  expect(slicePage(found, 2, 20).map((x) => x.name)).toEqual(['p40', 'p41', 'p42', 'p43', 'p44']);
});

test('a label selector reads as its terms, the way kubectl prints it', () => {
  expect(
    selectorTerms({
      matchLabels: { env: 'prod', region: 'eu' },
      matchExpressions: [
        { key: 'tier', operator: 'In', values: ['a', 'b'] },
        { key: 'zone', operator: 'NotIn', values: ['x'] },
        { key: 'pci', operator: 'Exists' },
        { key: 'legacy', operator: 'DoesNotExist' },
      ],
    }),
  ).toEqual(['env=prod', 'region=eu', 'tier in (a, b)', 'zone notin (x)', 'pci', '!legacy']);
  expect(selectorTerms({})).toEqual([]);
  expect(selectorTerms(undefined)).toEqual([]);
});

test('cluster scope says it covers every namespace', () => {
  expect(scopeText('Cluster')).toBe('Cluster: every namespace, cluster-wide');
  expect(scopeText('Namespaced')).toBe('Namespaced');
});
