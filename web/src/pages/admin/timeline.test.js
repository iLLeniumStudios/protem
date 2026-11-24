// Run: cd web && bun test src/pages/admin
import { expect, test } from 'bun:test';
import { ROW_LIMIT, barGeometry, byEnd, groupTimeline, labelPlacement, parseTimelineQuery, rowGrants, ticks, timelineQueryString } from './timeline';

const H = 3_600_000;
const now = Date.parse('2026-11-24T12:00:00Z');
const at = (hours) => new Date(now + hours * H).toISOString();

const grant = (name, cluster, endsIn, over = {}) => ({
  namespace: 'team',
  name,
  requester: { username: name, email: `${name}@x.io` },
  role: { kind: 'ClusterRole', name: 'view' },
  clusters: [cluster],
  startedAt: at(-1),
  expiresAt: at(endsIn),
  phase: 'Active',
  ...over,
});
const cluster = (name, over = {}) => ({
  name,
  displayName: name,
  phase: 'Ready',
  ready: true,
  paused: false,
  activeGrantsCount: 0,
  agent: 'connected',
  ...over,
});

test('a bar runs from now to expiry as a fraction of the window', () => {
  expect(barGeometry(grant('a', 'c', 3), now, '6h')).toEqual({ left: 0, width: 0.5, cut: false });
  expect(barGeometry(grant('a', 'c', 6), now, '24h')).toEqual({ left: 0, width: 0.25, cut: false });
});

test('a grant ending beyond the window is cut at its edge and marked; one already over has no width', () => {
  expect(barGeometry(grant('a', 'c', 9), now, '6h')).toEqual({ left: 0, width: 1, cut: true });
  expect(barGeometry(grant('a', 'c', -1), now, '6h')).toEqual({ left: 0, width: 0, cut: false });
});

test('a grant that starts later starts its bar later, clamped to the window', () => {
  const later = barGeometry(grant('a', 'c', 4, { startedAt: at(3) }), now, '6h');
  expect(later).toMatchObject({ left: 0.5, cut: false });
  expect(later.width).toBeCloseTo(1 / 6);
  expect(barGeometry(grant('a', 'c', 9, { startedAt: at(7) }), now, '6h')).toEqual({ left: 1, width: 0, cut: true });
});

test('the label goes inside the bar when it fits, beside it when not', () => {
  expect(labelPlacement(300, 200)).toBe('inside');
  expect(labelPlacement(200, 200)).toBe('inside');
  expect(labelPlacement(120, 200)).toBe('beside');
});

test('clusters group by the env label; one "All clusters" group when none has it', () => {
  const groups = groupTimeline(
    [cluster('p1', { labels: { env: 'prod' } }), cluster('d1', { labels: { env: 'dev' } }), cluster('x')],
    [grant('g1', 'p1', 1), grant('g2', 'd1', 2), grant('g3', 'x', 3)],
    '',
  );
  expect(groups.map((g) => g.name)).toEqual(['dev', 'prod', 'No environment']);
  const one = groupTimeline([cluster('a'), cluster('b')], [grant('g', 'a', 1)], '');
  expect(one.map((g) => g.name)).toEqual(['All clusters']);
});

test('clusters with nothing live and nothing wrong collapse into the group summary; the rest keep a row', () => {
  const [g] = groupTimeline(
    [cluster('busy'), cluster('idle'), cluster('down', { agent: 'lost', ready: false }), cluster('held', { paused: true })],
    [grant('g1', 'busy', 1), grant('g2', 'busy', 2)],
    '',
  );
  expect(g.rows.map((r) => r.cluster.name)).toEqual(['down', 'held', 'busy']);
  expect(g.idle.map((c) => c.name)).toEqual(['idle']);
  expect(g).toMatchObject({ clusters: 4, withLive: 1, grants: 2, lost: 1, waiting: 0, paused: 1 });
});

test('a row lists its grants ending soonest first', () => {
  const [g] = groupTimeline([cluster('c')], [grant('late', 'c', 5), grant('soon', 'c', 1)], '');
  expect(g.rows[0].grants.map((x) => x.name)).toEqual(['soon', 'late']);
});

test('search keeps clusters matching by name with all their grants, else only grants matching by person or role', () => {
  const clusters = [cluster('prod-eu'), cluster('dev')];
  const grants = [
    grant('ann', 'prod-eu', 1),
    grant('bob', 'dev', 1, { role: { kind: 'ClusterRole', name: 'admin' } }),
    grant('cy', 'dev', 2),
  ];
  const rows = (query) => groupTimeline(clusters, grants, query).flatMap((g) => g.rows.map((r) => [r.cluster.name, r.grants.map((x) => x.name)]));
  expect(rows('prod')).toEqual([['prod-eu', ['ann']]]);
  expect(rows('ADMIN')).toEqual([['dev', ['bob']]]);
  expect(rows('cy@x')).toEqual([['dev', ['cy']]]);
  expect(groupTimeline(clusters, grants, 'nobody')).toEqual([]);
});

test('a row with more than three grants shows three until expanded', () => {
  const list = [1, 2, 3, 4, 5].map((n) => grant(`g${n}`, 'c', n));
  expect(ROW_LIMIT).toBe(3);
  expect(rowGrants(list, false)).toEqual({ shown: list.slice(0, 3), hidden: 2 });
  expect(rowGrants(list, true)).toEqual({ shown: list, hidden: 0 });
  expect(rowGrants(list.slice(0, 3), false)).toEqual({ shown: list.slice(0, 3), hidden: 0 });
});

test('the narrow list orders grants by end time', () => {
  expect(byEnd([grant('b', 'c', 2), grant('a', 'c', 1)]).map((g) => g.name)).toEqual(['a', 'b']);
});

test('ticks fall on whole steps inside the window, evenly spaced', () => {
  const six = ticks(now, '6h');
  expect(six.length).toBe(6);
  expect(six.every((t) => t.fraction > 0 && t.fraction <= 1)).toBe(true);
  expect(six[1].fraction - six[0].fraction).toBeCloseTo(1 / 6);
  expect(ticks(now, '24h')[1].fraction - ticks(now, '24h')[0].fraction).toBeCloseTo(2 / 24);
  expect(ticks(now, '7d').length).toBe(7);
});

test('zoom and search round-trip through the query string; 6 h is the default', () => {
  expect(parseTimelineQuery(new URLSearchParams(''))).toEqual({ q: '', zoom: '6h' });
  expect(timelineQueryString({ q: '', zoom: '6h' })).toBe('');
  expect(timelineQueryString({ q: 'ann', zoom: '7d' })).toBe('?q=ann&zoom=7d');
  expect(parseTimelineQuery(new URLSearchParams('q=ann&zoom=7d'))).toEqual({ q: 'ann', zoom: '7d' });
  expect(parseTimelineQuery(new URLSearchParams('zoom=1y'))).toEqual({ q: '', zoom: '6h' });
});

test('a tick too close to the Now label keeps its gridline but loses its label', () => {
  const halfPast = now + H / 2;
  const [first, second] = ticks(halfPast, '6h');
  expect(first.fraction).toBeCloseTo(0.5 / 6);
  expect([first.labelled, second.labelled]).toEqual([false, true]);
  expect(ticks(now, '6h')[0].labelled).toBe(true);
});
