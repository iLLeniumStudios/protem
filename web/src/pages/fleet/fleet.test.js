// Run: cd web && bun test src/pages/fleet
import { expect, test } from 'bun:test';
import { clusterHealth, clusterStatus, liveGrants } from './fleet';

const cluster = (name, ready, over = {}) => ({
  name,
  displayName: name,
  phase: ready ? 'Ready' : 'Disconnected',
  ready,
  paused: false,
  activeGrantsCount: 0,
  ...over,
});

test('cluster health counts ready and paused clusters and lists the unhealthy rest first-by-name', () => {
  const health = clusterHealth([
    cluster('b', false),
    cluster('c', true),
    cluster('a', false),
    cluster('p', false, { paused: true, phase: 'Ready' }),
  ]);
  expect(health.ready).toBe(1);
  expect(health.paused).toBe(1);
  expect(health.total).toBe(4);
  expect(health.unhealthy.map((s) => s.name)).toEqual(['a', 'b']);
});

test('a paused cluster reads as paused, never as not ready', () => {
  expect(clusterStatus(cluster('p', false, { paused: true, phase: 'Ready' }))).toEqual({
    state: 'Paused',
    detail: 'New requests cannot target it',
  });
  expect(clusterStatus(cluster('p', true, { paused: true }))).toEqual({ state: 'Paused', detail: 'New requests cannot target it' });
});

test('the raw phase is shown only when it adds to the pill, never when it contradicts it', () => {
  expect(clusterStatus(cluster('a', true))).toEqual({ state: 'Ready' });
  expect(clusterStatus(cluster('a', false))).toEqual({ state: 'NotReady', detail: 'Disconnected' });
  expect(clusterStatus(cluster('a', false, { phase: 'Ready' }))).toEqual({ state: 'NotReady' });
  expect(clusterStatus(cluster('a', false, { phase: '' }))).toEqual({ state: 'NotReady' });
  expect(clusterStatus(cluster('a', true, { phase: 'Degraded' }))).toEqual({ state: 'Ready' });
});

test('live grants are the active ones, soonest to expire first', () => {
  const g = (name, phase, expiresAt) => ({ name, phase, expiresAt });
  const out = liveGrants([
    g('late', 'Active', '2026-11-24T14:00:00Z'),
    g('gone', 'Revoked', '2026-11-24T13:00:00Z'),
    g('soon', 'Active', '2026-11-24T12:30:00Z'),
  ]);
  expect(out.map((x) => x.name)).toEqual(['soon', 'late']);
});

