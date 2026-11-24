// Run: cd web && bun test src/app
import { expect, test } from 'bun:test';
import { pendingOldestFirst, waitingForYou } from './waiting';

const req = (name, over = {}) => ({
  namespace: 'default',
  name,
  phase: 'Pending',
  createdAt: '2026-11-24T12:00:00Z',
  actions: { approve: false, reject: false, cancel: false },
  ...over,
});
const can = { approve: true, reject: true, cancel: false };

const list = [
  req('newer', { createdAt: '2026-11-24T12:10:00Z', actions: can }),
  req('approved-already', { createdAt: '2026-11-24T12:05:00Z', actions: { approve: false, reject: true, cancel: false } }),
  req('older', { createdAt: '2026-11-24T11:00:00Z', actions: can }),
  req('not-mine', { createdAt: '2026-11-24T10:00:00Z', actions: { approve: false, reject: false, cancel: true } }),
  req('decided', { phase: 'Approved', actions: can }),
];

test('pending is every pending request, oldest first', () => {
  expect(pendingOldestFirst(list).map((r) => r.name)).toEqual(['not-mine', 'older', 'approved-already', 'newer']);
});

test('waiting for you keeps only pending requests the user may decide, oldest first', () => {
  expect(waitingForYou(list).map((r) => r.name)).toEqual(['older', 'approved-already', 'newer']);
});
