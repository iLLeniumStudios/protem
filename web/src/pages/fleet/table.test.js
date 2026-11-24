// Run: cd web && bun test src/pages/fleet
import { expect, test } from 'bun:test';
import {
  allowedRolesText,
  chooseFigure,
  clusterFigures,
  clusterQueryString,
  envOptions,
  figureOf,
  filterClusters,
  parseClusterQuery,
  sortClusters,
} from './table';

const c = (name, over = {}) => ({
  name,
  displayName: name.toUpperCase(),
  phase: 'Ready',
  ready: true,
  paused: false,
  activeGrantsCount: 0,
  agent: 'connected',
  ...over,
});

const fleet = [
  c('prod-eu', { labels: { env: 'prod', team: 'payments' } }),
  c('prod-us', { labels: { env: 'prod' }, agent: 'lost', ready: false, phase: 'Disconnected' }),
  c('dev-1', { labels: { env: 'dev' }, paused: true }),
  c('stage', { displayName: 'Staging Frankfurt', agent: 'waiting', ready: false, phase: 'Pending' }),
  c('hub', { agent: 'none' }),
];
const names = (list) => list.map((x) => x.name);
const q = (over = {}) => ({ q: '', page: 0, ...over });

test('a figure filters to its clusters: ready, agent not reporting, paused, waiting for agent', () => {
  expect(names(filterClusters(fleet, q({ status: 'ready' })))).toEqual(['prod-eu', 'hub']);
  expect(names(filterClusters(fleet, q({ status: 'paused' })))).toEqual(['dev-1']);
  expect(names(filterClusters(fleet, q({ agent: 'lost' })))).toEqual(['prod-us']);
  expect(names(filterClusters(fleet, q({ agent: 'waiting' })))).toEqual(['stage']);
});

test('the figures count what their filter shows, and waiting for agent only when there is one', () => {
  expect(clusterFigures(fleet)).toEqual([
    { figure: 'ready', count: 2 },
    { figure: 'lost', count: 1 },
    { figure: 'paused', count: 1 },
    { figure: 'waiting', count: 1 },
  ]);
  expect(clusterFigures(fleet.filter((x) => x.agent !== 'waiting')).map((f) => f.figure)).toEqual(['ready', 'lost', 'paused']);
});

test('search matches name, display name and labels in any case; environment and agent state narrow further', () => {
  expect(names(filterClusters(fleet, q({ q: 'frank' })))).toEqual(['stage']);
  expect(names(filterClusters(fleet, q({ q: 'PAYMENTS' })))).toEqual(['prod-eu']);
  expect(names(filterClusters(fleet, q({ q: 'env=dev' })))).toEqual(['dev-1']);
  expect(names(filterClusters(fleet, q({ env: 'prod' })))).toEqual(['prod-eu', 'prod-us']);
  expect(names(filterClusters(fleet, q({ env: 'prod', agent: 'connected' })))).toEqual(['prod-eu']);
});

test('choosing a figure replaces any other figure and the agent state; choosing it again clears it', () => {
  const base = q({ q: 'x', env: 'prod', agent: 'connected', page: 3 });
  const lost = chooseFigure(base, 'lost');
  expect(lost).toEqual(q({ q: 'x', env: 'prod', agent: 'lost' }));
  expect(figureOf(lost)).toBe('lost');
  const ready = chooseFigure(lost, 'ready');
  expect(ready).toEqual(q({ q: 'x', env: 'prod', status: 'ready' }));
  expect(figureOf(ready)).toBe('ready');
  expect(chooseFigure(ready, 'ready')).toEqual(q({ q: 'x', env: 'prod' }));
  expect(figureOf(q({ status: 'ready', agent: 'connected' }))).toBeUndefined();
});

test('clusters that need attention come first, then by name', () => {
  const sorted = sortClusters([c('b'), c('a'), c('p', { paused: true }), c('z', { agent: 'lost', ready: false }), c('m', { agent: 'waiting', ready: false })]);
  expect(names(sorted)).toEqual(['m', 'z', 'p', 'a', 'b']);
});

test('environments come from the env label, sorted; none when no cluster has one', () => {
  expect(envOptions(fleet)).toEqual(['dev', 'prod']);
  expect(envOptions([c('a'), c('b', { labels: { team: 'x' } })])).toEqual([]);
});

test('the query string round-trips and drops what it does not know', () => {
  const full = { q: 'prod eu', status: 'ready', agent: 'lost', env: 'prod', page: 2 };
  expect(clusterQueryString(full)).toBe('?q=prod+eu&status=ready&agent=lost&env=prod&page=3');
  expect(parseClusterQuery(new URLSearchParams(clusterQueryString(full)))).toEqual(full);
  expect(clusterQueryString(q())).toBe('');
  expect(parseClusterQuery(new URLSearchParams('agent=lost'))).toEqual(q({ agent: 'lost' }));
  expect(parseClusterQuery(new URLSearchParams('agent=gone&status=odd&page=abc'))).toEqual(q());
  expect(parseClusterQuery(new URLSearchParams('page=-4'))).toEqual(q());
});

test('allowed roles read as the agent reported them', () => {
  expect(allowedRolesText(c('a', { allowedRoles: ['view', 'edit'] }))).toEqual({ kind: 'roles', roles: ['view', 'edit'], partial: false });
  expect(allowedRolesText(c('a', { allowedRoles: ['view'], allowedRolesPartial: true }))).toEqual({ kind: 'roles', roles: ['view'], partial: true });
  expect(allowedRolesText(c('a', { allowedRoles: ['*'] }))).toEqual({ kind: 'any' });
  expect(allowedRolesText(c('a', { allowedRoles: ['*'], allowedRolesPartial: true }))).toEqual({ kind: 'any' });
  expect(allowedRolesText(c('a'))).toEqual({ kind: 'unreported' });
  expect(allowedRolesText(c('a', { allowedRoles: [] }))).toEqual({ kind: 'roles', roles: [], partial: false });
  expect(allowedRolesText(c('hub', { agent: 'none' }))).toEqual({ kind: 'operator' });
});
