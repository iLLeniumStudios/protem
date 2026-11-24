// Run: cd web && bun test src/pages/admin
import { expect, test } from 'bun:test';
import { attentionClusters, waitingLongest } from './attention';

const cluster = (name, agent, lastHeartbeat) => ({ name, displayName: name, agent, lastHeartbeat, ready: agent === 'connected', paused: false });

test('clusters whose agent is not reporting or never reported need attention, longest silent first', () => {
  const out = attentionClusters([
    cluster('ok', 'connected', '2026-11-24T12:00:00Z'),
    cluster('lost-late', 'lost', '2026-11-24T11:00:00Z'),
    cluster('lost-early', 'lost', '2026-11-24T09:00:00Z'),
    cluster('new', 'waiting', undefined),
    cluster('hub', 'none', undefined),
  ]);
  expect(out.map((c) => c.name)).toEqual(['new', 'lost-early', 'lost-late']);
});

test('requests waiting for a decision, longest waiting first, at most n', () => {
  const r = (name, phase, createdAt) => ({ namespace: 'n', name, phase, createdAt });
  const out = waitingLongest(
    [r('b', 'Pending', '2026-11-24T10:00:00Z'), r('done', 'Approved', '2026-11-24T08:00:00Z'), r('a', 'Pending', '2026-11-24T09:00:00Z'), r('c', 'Pending', '2026-11-24T11:00:00Z')],
    2,
  );
  expect(out.map((x) => x.name)).toEqual(['a', 'b']);
});
