// Run: cd web && bun test src/pages/access
import { expect, test } from 'bun:test';
import { approvalProgress, defaultClusters, durationOptions, parseRequestRef, ticketError, withRequestRef } from './model';

test('duration options are the presets up to the max, plus the default when it is not a preset', () => {
  expect(durationOptions({ defaultDuration: '30m0s', maxDuration: '2h0m0s' })).toEqual(['15m', '30m', '1h', '2h']);
  expect(durationOptions({ defaultDuration: '1h0m0s', maxDuration: '8h0m0s' })).toEqual(['15m', '30m', '1h', '2h', '4h', '8h']);
  expect(durationOptions({ defaultDuration: '45m0s', maxDuration: '1h0m0s' })).toEqual(['15m', '30m', '45m', '1h']);
  expect(durationOptions({ defaultDuration: '10m0s', maxDuration: '10m0s' })).toEqual(['10m']);
});

test('approval progress counts manual approvals once per rule, capped at what is required', () => {
  const rule = (cluster, ruleName, required, approvedBy, mode = 'Manual') => ({ cluster, rule: ruleName, mode, required, approvedBy });
  // One rule over two clusters needs 2 approvals in total, not 2 per cluster.
  const spread = [rule('a', 'sre', 2, ['bob']), rule('b', 'sre', 2, ['bob']), rule('c', 'sec', 1, ['eve', 'mal']), rule('d', 'auto', 0, [], 'Auto')];
  expect(approvalProgress({ requirements: spread })).toEqual({ approved: 2, required: 3 });
  expect(approvalProgress({ requirements: [rule('a', 'sre', 1, null)] })).toEqual({ approved: 0, required: 1 });
  expect(approvalProgress({ requirements: [rule('a', 'auto', 0, [], 'Auto')] })).toBeNull();
  expect(approvalProgress({ requirements: undefined })).toBeNull();
});

test('clusters offered are exactly the ready clusters the server lists for the profile', () => {
  expect(defaultClusters({ clusters: ['prod-us-east-1', 'local'] })).toEqual(['prod-us-east-1', 'local']);
  expect(defaultClusters({ clusters: [] })).toEqual([]);
  expect(defaultClusters({ clusters: null })).toEqual([]);
});

test('a ticket must match the profile pattern when one is set', () => {
  const pattern = '^(INC|OPS)-[0-9]+$';
  expect(ticketError(pattern, 'INC-42')).toBeUndefined();
  expect(ticketError(pattern, ' OPS-7 ')).toBeUndefined();
  expect(ticketError(pattern, 'JIRA-1')).toBe('Use the format ^(INC|OPS)-[0-9]+$.');
  expect(ticketError(pattern, '')).toBe('Enter a ticket ID.');
  expect(ticketError(undefined, 'anything')).toBeUndefined();
});

test('a pattern JavaScript cannot compile in unicode mode is left to the server', () => {
  expect(ticketError('(?i)^inc-\\d+$', 'whatever')).toBeUndefined();
  expect(ticketError('^\\p{Lu}+-\\d+$', 'INC-1')).toBeUndefined();
  expect(ticketError('^\\p{Lu}+-\\d+$', 'inc-1')).toBe('Use the format ^\\p{Lu}+-\\d+$.');
});

test('the request parameter names one request as <namespace>/<name>, and nothing else', () => {
  expect(parseRequestRef('team-a/req-1')).toEqual({ namespace: 'team-a', name: 'req-1' });
  for (const bad of [null, '', 'req-1', 'team-a/', '/req-1', 'team-a/req-1/x']) expect(parseRequestRef(bad)).toBeNull();
});

test('setting the request parameter keeps the other parameters and a readable slash; clearing removes it', () => {
  expect(withRequestRef('?page=2', { namespace: 'team-a', name: 'req-1' })).toBe('?page=2&request=team-a/req-1');
  expect(withRequestRef('?request=team-a/old&page=2', { namespace: 'team-a', name: 'new' })).toBe('?page=2&request=team-a/new');
  expect(withRequestRef('?request=team-a/req-1&page=2', null)).toBe('?page=2');
  expect(withRequestRef('?request=team-a%2Freq-1', null)).toBe('');
  const search = withRequestRef('', { namespace: 'team a', name: 'r&1' });
  expect(parseRequestRef(new URLSearchParams(search).get('request'))).toEqual({ namespace: 'team a', name: 'r&1' });
});
