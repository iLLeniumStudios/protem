// Run: cd web && bun test src/app
import { expect, test } from 'bun:test';
import { byTime, newestFirst, oldestFirst } from './order';

const obj = (namespace, name, at) => ({ namespace, name, at });
const at = (x) => x.at;
const names = (xs) => xs.map((x) => `${x.namespace}/${x.name}`);

const tied = [obj('b', 'x', '2026-11-24T12:00:00Z'), obj('a', 'y', '2026-11-24T12:00:00Z'), obj('a', 'x', '2026-11-24T12:00:00Z')];

test('equal timestamps fall back to namespace then name, whatever order they arrive in', () => {
  const want = ['a/x', 'a/y', 'b/x'];
  expect(names([...tied].sort(oldestFirst(at)))).toEqual(want);
  expect(names([...tied].reverse().sort(oldestFirst(at)))).toEqual(want);
  expect(names([...tied].sort(newestFirst(at)))).toEqual(want);
  expect(names([...tied].reverse().sort(newestFirst(at)))).toEqual(want);
});

test('time decides first, in either direction, and compares instants rather than strings', () => {
  const list = [obj('z', 'late', '2026-11-24T13:00:00+01:00'), obj('a', 'early', '2026-11-24T11:30:00Z'), obj('m', 'mid', '2026-11-24T12:30:00Z')];
  expect(list.slice().sort(oldestFirst(at)).map((x) => x.name)).toEqual(['early', 'late', 'mid']);
  expect(list.slice().sort(newestFirst(at)).map((x) => x.name)).toEqual(['mid', 'late', 'early']);
});

test('an unparseable time sorts as the oldest instead of breaking the order', () => {
  const list = [obj('a', 'ok', '2026-11-24T12:00:00Z'), obj('a', 'bad', ''), obj('a', 'bad2', 'soon')];
  expect(list.slice().sort(oldestFirst(at)).map((x) => x.name)).toEqual(['bad', 'bad2', 'ok']);
  expect(list.slice().reverse().sort(oldestFirst(at)).map((x) => x.name)).toEqual(['bad', 'bad2', 'ok']);
});

test('custom tie-breakers apply in the order given, missing values first', () => {
  const d = (user, rule) => ({ at: '2026-11-24T12:00:00Z', user, rule });
  const list = [d('bob', 'sre'), d('alice', 'sec'), d('alice', undefined), d('alice', 'eu')];
  const cmp = byTime('asc', (x) => x.at, (x) => x.user, (x) => x.rule);
  const want = [['alice', undefined], ['alice', 'eu'], ['alice', 'sec'], ['bob', 'sre']];
  expect(list.slice().sort(cmp).map((x) => [x.user, x.rule])).toEqual(want);
  expect(list.slice().reverse().sort(cmp).map((x) => [x.user, x.rule])).toEqual(want);
});
