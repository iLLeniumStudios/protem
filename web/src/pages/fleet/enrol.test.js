// Run: cd web && bun test src/pages/fleet
import { expect, test } from 'bun:test';
import { addClusterPayload, agentJoined, agentState, clusterActions, clusterNameError, joinCountdown } from './enrol';

test('a cluster name is a lowercase DNS label of at most 40 characters that is not the hub', () => {
  expect(clusterNameError('prod-eu-1', 'hub')).toBeUndefined();
  expect(clusterNameError('a'.repeat(40), 'hub')).toBeUndefined();
  expect(clusterNameError('', 'hub')).toBe('Enter a name.');
  expect(clusterNameError('Prod', 'hub')).toContain('lowercase');
  expect(clusterNameError('-prod', 'hub')).toContain('lowercase');
  expect(clusterNameError('prod-', 'hub')).toContain('lowercase');
  expect(clusterNameError('prod.eu', 'hub')).toContain('lowercase');
  expect(clusterNameError('a'.repeat(41), 'hub')).toContain('40');
  expect(clusterNameError('hub', 'hub')).toContain('hub');
  expect(clusterNameError('hub', undefined)).toBeUndefined();
});

test('the add form sends trimmed values, leaves out what is empty, and refuses reserved labels', () => {
  const form = { name: ' prod-eu ', displayName: ' Production EU ', description: ' ', labels: [{ key: 'env', value: 'prod' }, { key: '', value: '' }] };
  expect(addClusterPayload(form, 'hub')).toEqual({
    payload: { name: 'prod-eu', displayName: 'Production EU', labels: { env: 'prod' } },
    errors: {},
  });
  const bad = addClusterPayload({ name: 'hub', displayName: '', description: '', labels: [{ key: 'protem.dev/region', value: 'eu' }] }, 'hub');
  expect(Object.keys(bad.errors).sort()).toEqual(['displayName', 'labels', 'name']);
  expect(bad.errors.labels).toContain('protem.dev/region');
});

const caps = (addClusters) => ({ management: true, profileNamespaces: [], manageClusters: true, addClusters, viewFleet: true, viewAll: true });
const cluster = (agent, managedBy) => ({ name: 'c', agent, managedBy });

test('each agent state offers its own actions, and a GitOps-managed cluster cannot be removed here', () => {
  expect(clusterActions(caps(true), cluster('none'))).toEqual([]);
  expect(clusterActions(caps(true), cluster('waiting'))).toEqual(['connect', 'remove']);
  expect(clusterActions(caps(true), cluster('connected'))).toEqual(['reissue', 'remove']);
  expect(clusterActions(caps(true), cluster('lost'))).toEqual(['connect', 'reissue', 'remove']);
  expect(clusterActions(caps(true), cluster('none', 'Argo CD'))).toEqual([]);
  expect(clusterActions(caps(true), cluster('waiting', 'Argo CD'))).toEqual(['connect']);
  expect(clusterActions(caps(true), cluster('connected', 'Argo CD'))).toEqual(['reissue']);
  expect(clusterActions(caps(true), cluster('lost', 'Argo CD'))).toEqual(['connect', 'reissue']);
});

test('without addClusters no row offers any action, whatever its state', () => {
  for (const agent of ['none', 'waiting', 'connected', 'lost']) {
    for (const managedBy of [undefined, 'Flux']) {
      expect(clusterActions(caps(false), cluster(agent, managedBy))).toEqual([]);
      expect(clusterActions(null, cluster(agent, managedBy))).toEqual([]);
    }
  }
});

test('the join token counts down to the second and is expired exactly at zero', () => {
  const now = new Date('2026-11-24T12:00:00Z');
  expect(joinCountdown('2026-11-24T12:15:00Z', now)).toEqual({ expired: false, left: '15m 00s left' });
  expect(joinCountdown('2026-11-24T12:01:05Z', now)).toEqual({ expired: false, left: '1m 05s left' });
  expect(joinCountdown('2026-11-24T12:00:00.500Z', now)).toEqual({ expired: false, left: '1s left' });
  expect(joinCountdown('2026-11-24T12:00:00Z', now)).toEqual({ expired: true, left: '' });
  expect(joinCountdown('2026-11-24T11:59:00Z', now)).toEqual({ expired: true, left: '' });
  expect(joinCountdown('not a time', now)).toEqual({ expired: true, left: '' });
});

test('each agent state reads as a sentence-case label in a status tone', () => {
  expect(agentState('none')).toEqual({ label: 'Hub (no agent)', tone: 'neutral' });
  expect(agentState('waiting')).toEqual({ label: 'Waiting for agent', tone: 'warning' });
  expect(agentState('connected')).toEqual({ label: 'Agent connected', tone: 'success' });
  expect(agentState('lost')).toEqual({ label: 'Agent not reporting', tone: 'danger' });
});

test('the agent counts as joined only once it reports after the token was issued', () => {
  const at = (agent, lastHeartbeat) => ({ agent, lastHeartbeat });
  expect(agentJoined(at('waiting'), at('waiting'))).toBe(false);
  expect(agentJoined(at('connected', 't1'), at('waiting'))).toBe(true);
  expect(agentJoined(at('connected', 't1'), at('lost', 't0'))).toBe(true);
  // Reissued: the old agent's last heartbeat is still fresh, so connected alone proves nothing.
  expect(agentJoined(at('connected', 't0'), at('connected', 't0'))).toBe(false);
  expect(agentJoined(at('connected', 't1'), at('connected', 't0'))).toBe(true);
  expect(agentJoined(undefined, at('waiting'))).toBe(false);
});
