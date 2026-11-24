// Run: cd web && bun test src/pages/approvals
import { expect, test } from 'bun:test';
import { decidedAt, isLongWait } from './queue';

const req = (name, over = {}) => ({
  namespace: 'default',
  name,
  requester: { username: 'alice', email: 'alice@company.com' },
  profile: 'prod-troubleshooter',
  reason: 'r',
  duration: '15m0s',
  phase: 'Pending',
  createdAt: '2026-11-24T12:00:00Z',
  actions: { approve: false, reject: false, cancel: false },
  ...over,
});

test('decided time is the latest decision, else creation', () => {
  expect(decidedAt(req('a'))).toBe('2026-11-24T12:00:00Z');
  const decisions = [
    { user: 'b', decision: 'Approved', at: '2026-11-24T12:20:00Z' },
    { user: 'c', decision: 'Approved', at: '2026-11-24T12:30:00Z' },
    { user: 'd', decision: 'Approved', at: '2026-11-24T12:25:00Z' },
  ];
  expect(decidedAt(req('b', { approvals: decisions }))).toBe('2026-11-24T12:30:00Z');
});

test('a wait is long after thirty minutes', () => {
  const now = new Date('2026-11-24T12:30:00Z');
  expect(isLongWait('2026-11-24T12:00:00Z', now)).toBe(false);
  expect(isLongWait('2026-11-24T11:59:00Z', now)).toBe(true);
});
