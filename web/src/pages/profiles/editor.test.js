// Run: cd web && bun test src/pages/profiles
import { expect, test } from 'bun:test';
import {
  formToSpec,
  newProfileForm,
  parseDuration,
  shortDuration,
  specToForm,
  validateForm,
  validateNew,
  validateSpec,
} from './editor';

const valid = () => ({
  displayName: 'Prod debug',
  role: { kind: 'ClusterRole', name: 'edit' },
  scope: 'Namespaced',
  namespaces: ['payments'],
  clusters: { names: ['prod-eu'] },
  duration: { default: '1h0m0s', max: '8h0m0s' },
  requesters: { groups: ['sre'] },
  justification: { reason: 'Required' },
  approval: { rules: [{ name: 'default', mode: 'Manual', approvers: { groups: ['leads'] }, requiredApprovals: 1 }] },
});

const errorKeys = (spec) => Object.keys(validateSpec(spec)).sort();

/** Durations compared by value: "1h0m0s" and "1h" are the same duration. */
function byValue(spec) {
  const s = structuredClone(spec);
  const seconds = (d) => (d === undefined ? undefined : parseDuration(d));
  s.duration = { default: seconds(s.duration.default), max: seconds(s.duration.max) };
  if (s.approval.pendingTimeout !== undefined) s.approval.pendingTimeout = seconds(s.approval.pendingTimeout);
  for (const r of s.approval.rules) if (r.maxDuration !== undefined) r.maxDuration = seconds(r.maxDuration);
  return s;
}

test('a spec the form cannot fully edit comes back unchanged when nothing is edited', () => {
  const specs = [
    {
      displayName: 'Prod admin',
      description: 'Break-glass access to production',
      role: { kind: 'ClusterRole', name: 'admin' },
      scope: 'Namespaced',
      namespaces: ['payments', 'ledger'],
      clusters: {
        selector: {
          matchLabels: { env: 'prod', region: 'eu' },
          matchExpressions: [
            { key: 'tier', operator: 'In', values: ['a', 'b'] },
            { key: 'legacy', operator: 'DoesNotExist' },
          ],
        },
      },
      duration: { default: '30m0s', max: '4h0m0s' },
      requesters: { groups: ['sre', 'payments-oncall'], users: ['alice', 'bob@company.com'] },
      justification: { reason: 'Optional', ticketPattern: '^(INC|OPS)-[0-9]+$' },
      approval: {
        pendingTimeout: '24h0m0s',
        rules: [
          {
            name: 'pci',
            when: {
              clusterSelector: { matchExpressions: [{ key: 'pci', operator: 'Exists' }] },
              requesterGroups: ['contractors'],
              requesterUsers: ['carol@company.com'],
            },
            mode: 'Manual',
            maxDuration: '1h0m0s',
            approvers: { groups: ['security'], users: ['ciso@company.com'] },
            requiredApprovals: 2,
            allowSelfApproval: true,
          },
          {
            name: 'eu-oncall',
            when: { clusterSelector: { matchLabels: { region: 'eu' } } },
            mode: 'Manual',
            approvers: { users: ['lead@company.com'] },
            requiredApprovals: 1,
          },
          { name: 'rest', mode: 'Auto', requiredApprovals: 1 },
          { name: 'empty-when', when: {}, mode: 'Auto', approvers: {} },
          { name: 'only-users', when: { requesterUsers: ['dave'] }, mode: 'Auto' },
        ],
      },
    },
    {
      displayName: 'Cluster reader',
      role: { kind: 'ClusterRole', name: 'view' },
      scope: 'Cluster',
      namespaces: ['ignored-but-kept'],
      clusters: { names: ['a', 'b'] },
      duration: { default: '1h0m0s', max: '1h0m0s' },
      requesters: { groups: ['everyone'] },
      justification: { reason: 'Required' },
      approval: { rules: [{ name: 'all', mode: 'Auto' }] },
    },
    { ...valid(), clusters: { selector: {} } },
    { ...valid(), requesters: { users: ['alice@company.com'] } },
  ];
  for (const spec of specs) {
    const before = structuredClone(spec);
    expect(byValue(formToSpec(specToForm(spec)))).toStrictEqual(byValue(before));
  }
});

test('loaded durations show in the short form', () => {
  const form = specToForm({
    ...valid(),
    duration: { default: '1h30m0s', max: '24h0m0s' },
    approval: { pendingTimeout: '30m0s', rules: [{ name: 'r', mode: 'Auto', maxDuration: '1h0m0s' }] },
  });
  expect([form.defaultDuration, form.maxDuration, form.pendingTimeout, form.rules[0].maxDuration]).toEqual([
    '1h30m',
    '24h',
    '30m',
    '1h',
  ]);
  expect(shortDuration('0s')).toBe('0s');
  expect(shortDuration('1m30s')).toBe('1m30s');
  expect(shortDuration('1.5s')).toBe('1.5s');
  expect(shortDuration('500ms')).toBe('500ms');
  expect(shortDuration('soon')).toBe('soon');
});

test('an edit keeps the selector expressions the form only shows', () => {
  const spec = {
    ...valid(),
    clusters: { selector: { matchExpressions: [{ key: 'pci', operator: 'Exists' }] } },
    approval: {
      rules: [{ name: 'r', when: { clusterSelector: { matchExpressions: [{ key: 'z', operator: 'In', values: ['1'] }] } }, mode: 'Auto' }],
    },
  };
  const form = specToForm(spec);
  form.displayName = 'Renamed';
  form.clusterLabels = [{ key: 'env', value: 'prod' }];
  form.rules[0].clusterLabels = [{ key: 'region', value: 'eu' }];
  const out = formToSpec(form);
  expect(out.displayName).toBe('Renamed');
  expect(out.clusters).toStrictEqual({
    selector: { matchLabels: { env: 'prod' }, matchExpressions: [{ key: 'pci', operator: 'Exists' }] },
  });
  expect(out.approval.rules[0].when).toStrictEqual({
    clusterSelector: { matchLabels: { region: 'eu' }, matchExpressions: [{ key: 'z', operator: 'In', values: ['1'] }] },
  });
});

test('choosing clusters by name drops the selector, and by label drops the names', () => {
  const form = specToForm({ ...valid(), clusters: { selector: { matchLabels: { env: 'prod' } } } });
  form.clusterMode = 'names';
  form.clusterNames = 'a\n b \n\n';
  expect(formToSpec(form).clusters).toStrictEqual({ names: ['a', 'b'] });
  form.clusterMode = 'selector';
  expect(formToSpec(form).clusters).toStrictEqual({ selector: { matchLabels: { env: 'prod' } } });
});

test('a new profile starts with one manual rule and is invalid until filled in', () => {
  const form = newProfileForm();
  expect(form.rules.map((r) => [r.name, r.mode])).toEqual([['default', 'Manual']]);
  expect(Object.keys(validateForm(form)).sort()).toEqual([
    'clusters',
    'displayName',
    'namespaces',
    'requesters',
    'role.name',
    'rules.0.approvers',
  ]);
});

test('a complete spec has no errors', () => {
  expect(validateSpec(valid())).toEqual({});
  expect(validateSpec({ ...valid(), clusters: { selector: { matchLabels: { env: 'prod' } } } })).toEqual({});
  expect(validateSpec({ ...valid(), clusters: { selector: { matchExpressions: [{ key: 'pci', operator: 'Exists' }] } } })).toEqual({});
});

test('choosing clusters by label needs at least one label; an empty selector never means the whole fleet', () => {
  const errors = validateSpec({ ...valid(), clusters: { selector: {} } });
  expect(errors).toEqual({ clusterLabels: 'Add at least one label. A profile cannot target every cluster by leaving this empty.' });
  expect(errorKeys({ ...valid(), clusters: { selector: { matchLabels: {}, matchExpressions: [] } } })).toEqual(['clusterLabels']);
  const rules = [{ name: 'r', when: { clusterSelector: {} }, mode: 'Auto' }];
  expect(errorKeys({ ...valid(), approval: { rules } })).toEqual([]);
});

test('the default duration may not exceed the maximum', () => {
  expect(errorKeys({ ...valid(), duration: { default: '8h', max: '1h' } })).toEqual(['duration.default']);
  expect(errorKeys({ ...valid(), duration: { default: '90m', max: '1h30m0s' } })).toEqual([]);
  expect(errorKeys({ ...valid(), duration: { default: 'soon', max: '' } })).toEqual(['duration.default', 'duration.max']);
});

test('rule names must be unique; the later duplicate is flagged', () => {
  const spec = valid();
  spec.approval.rules = [spec.approval.rules[0], { name: 'default', mode: 'Auto' }, { name: 'other', mode: 'Auto' }];
  expect(errorKeys(spec)).toEqual(['rules.1.name']);
});

test('a manual rule needs approver groups or users and at least one approval', () => {
  const rule = (over) => ({ ...valid(), approval: { rules: [{ name: 'r', mode: 'Manual', ...over }] } });
  expect(errorKeys(rule({}))).toEqual(['rules.0.approvers']);
  expect(errorKeys(rule({ approvers: { groups: [], users: [] } }))).toEqual(['rules.0.approvers']);
  expect(errorKeys(rule({ approvers: { users: ['bob'] } }))).toEqual([]);
  expect(errorKeys(rule({ approvers: { groups: ['g'] }, requiredApprovals: 0 }))).toEqual(['rules.0.requiredApprovals']);
  expect(errorKeys({ ...valid(), approval: { rules: [{ name: 'r', mode: 'Auto' }] } })).toEqual([]);
  expect(errorKeys({ ...valid(), approval: { rules: [] } })).toEqual(['rules']);
});

test('requesters are at least one group or user', () => {
  expect(errorKeys({ ...valid(), requesters: { users: ['alice'] } })).toEqual([]);
  expect(errorKeys({ ...valid(), requesters: { groups: ['sre'], users: ['alice'] } })).toEqual([]);
  expect(validateSpec({ ...valid(), requesters: { groups: [], users: [] } })).toEqual({
    requesters: 'Add at least one group or user. Only they can request this profile.',
  });
  expect(errorKeys({ ...valid(), requesters: {} })).toEqual(['requesters']);
});

test('requester users are edited one per line, on the profile and on each rule', () => {
  const form = specToForm(valid());
  form.requesterGroups = '';
  form.requesterUsers = ' alice \n\nbob@company.com\n';
  form.rules[0].requesterUsers = 'carol';
  const out = formToSpec(form);
  expect(out.requesters).toStrictEqual({ users: ['alice', 'bob@company.com'] });
  expect(out.approval.rules[0].when).toStrictEqual({ requesterUsers: ['carol'] });
  expect(validateForm(form)).toEqual({});
});

test('cluster scope needs a ClusterRole', () => {
  expect(errorKeys({ ...valid(), scope: 'Cluster', role: { kind: 'Role', name: 'edit' } })).toEqual(['scope']);
  expect(errorKeys({ ...valid(), scope: 'Cluster', namespaces: undefined })).toEqual([]);
  expect(errorKeys({ ...valid(), namespaces: [] })).toEqual(['namespaces']);
});

test('a ticket pattern must compile; Go flag groups are accepted', () => {
  const pattern = (ticketPattern) => ({ ...valid(), justification: { reason: 'Required', ticketPattern } });
  expect(errorKeys(pattern('^(INC-[0-9]+$'))).toEqual(['ticketPattern']);
  expect(errorKeys(pattern('^(INC|OPS)-[0-9]+$'))).toEqual([]);
  expect(errorKeys(pattern('(?i)^inc-\\d+$'))).toEqual([]);
});

test('clusters are chosen by exactly one of names or selector', () => {
  expect(errorKeys({ ...valid(), clusters: {} })).toEqual(['clusters']);
  expect(errorKeys({ ...valid(), clusters: { names: [] } })).toEqual(['clusters']);
  expect(errorKeys({ ...valid(), clusters: { names: ['a'], selector: {} } })).toEqual(['clusters']);
});

test('a label row with a value but no key is an error, not silently dropped', () => {
  const form = specToForm({ ...valid(), clusters: { selector: {} } });
  form.clusterLabels = [{ key: '', value: 'prod' }];
  expect(Object.keys(validateForm(form))).toEqual(['clusterLabels']);
  form.rules[0].clusterLabels = [{ key: ' ', value: 'eu' }];
  expect(Object.keys(validateForm(form)).sort()).toEqual(['clusterLabels', 'rules.0.clusterLabels']);
});

test('a new profile needs a namespace and a Kubernetes object name', () => {
  expect(validateNew('team-a', 'prod-debug')).toEqual({});
  expect(Object.keys(validateNew('', '')).sort()).toEqual(['name', 'namespace']);
  expect(Object.keys(validateNew('Team A', 'Prod_Debug')).sort()).toEqual(['name', 'namespace']);
});

test('Go durations parse to seconds; anything else is null', () => {
  expect(parseDuration('1h30m')).toBe(5400);
  expect(parseDuration('1h0m0s')).toBe(3600);
  expect(parseDuration('1.5h')).toBe(5400);
  expect(parseDuration('500ms')).toBe(0.5);
  expect(parseDuration('0')).toBe(0);
  expect(parseDuration('1d')).toBeNull();
  expect(parseDuration('')).toBeNull();
  expect(parseDuration('h')).toBeNull();
});
