// Run: cd web && bun test src/app
import { expect, test } from 'bun:test';
import { CAPABILITIES_EVERY, capabilitiesDue, deepEqual, keepIfEqual, settle } from './sync';

test('deep equality compares JSON values structurally', () => {
  expect(deepEqual({ a: [1, { b: 'x' }], c: null }, { c: null, a: [1, { b: 'x' }] })).toBe(true);
  expect(deepEqual({ a: [1, 2] }, { a: [2, 1] })).toBe(false);
  expect(deepEqual({ a: 1 }, { a: 1, b: 2 })).toBe(false);
  expect(deepEqual({ a: undefined }, { b: undefined })).toBe(false);
  expect(deepEqual([], {})).toBe(false);
  expect(deepEqual(null, {})).toBe(false);
  expect(deepEqual('1', 1)).toBe(false);
});

test('an unchanged answer keeps the current object, so React skips the re-render', () => {
  const prev = { requests: [{ name: 'r1', phase: 'Pending' }] };
  const same = structuredClone(prev);
  expect(keepIfEqual(prev, same)).toBe(prev);
  const changed = { requests: [{ name: 'r1', phase: 'Approved' }] };
  expect(keepIfEqual(prev, changed)).toBe(changed);
  expect(keepIfEqual(null, same)).toBe(same);
});

test(`capabilities are fetched on the first poll, every ${CAPABILITIES_EVERY}th poll after, on persona change and after a write`, () => {
  const due = [];
  for (let poll = 0; poll < 25; poll++) if (capabilitiesDue(poll, false, false)) due.push(poll);
  expect(due).toEqual([0, 10, 20]);
  expect(capabilitiesDue(3, true, false)).toBe(true);
  expect(capabilitiesDue(3, false, true)).toBe(true);
});

test('a refetch keeps the last good data on failure, but a 404 means the object is gone', () => {
  const good = { data: { name: 'p' }, error: null };
  const boom = new Error('network down');
  const gone = Object.assign(new Error('not found'), { status: 404 });

  expect(settle(good, { ok: false, error: boom })).toBe(good);
  expect(settle(good, { ok: false, error: gone })).toEqual({ data: null, error: gone });
  expect(settle(null, { ok: false, error: boom })).toEqual({ data: null, error: boom });
  expect(settle(good, { ok: true, data: { name: 'p' } })).toBe(good);
  expect(settle(good, { ok: true, data: { name: 'q' } })).toEqual({ data: { name: 'q' }, error: null });
  const failed = { data: null, error: boom };
  expect(settle(failed, { ok: true, data: { name: 'p' } })).toEqual({ data: { name: 'p' }, error: null });
});
