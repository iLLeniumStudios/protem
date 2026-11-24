// Run: cd web && bun test src/components/ui
import { expect, test } from 'bun:test';
import { endingSoon, formatElapsed, formatRelative, formatTimeLeft, formatTimestamp, timeUsedFraction } from './format';
import { phaseTone } from './StatusPill';

const now = new Date('2026-11-24T12:00:00Z');
const plus = (ms) => new Date(now.getTime() + ms);
const MIN = 60_000;

test('an absolute time reads as a clock time today, with day and month on another day, and the year in another year', () => {
  const today = new Date(2026, 10, 24, 15, 30);
  expect(formatTimestamp(new Date(2026, 10, 24, 9, 1, 12), today)).toBe('09:01');
  expect(formatTimestamp(new Date(2026, 10, 25, 21, 1).toISOString(), today)).toBe('25 Nov, 21:01');
  expect(formatTimestamp(new Date(2026, 8, 3, 0, 5), today)).toBe('3 Sep, 00:05');
  expect(formatTimestamp(new Date(2025, 10, 24, 9, 1), today)).toBe('24 Nov 2025, 09:01');
});

test('relative time older than a month falls back to the absolute shape', () => {
  const today = new Date(2026, 10, 24, 15, 30);
  expect(formatRelative(new Date(2026, 6, 14, 8, 0), today)).toBe('14 Jul, 08:00');
});

test('time left counts down in hours and minutes, rounding partial minutes up', () => {
  expect(formatTimeLeft(plus(102 * MIN), now)).toBe('1h 42m left');
  expect(formatTimeLeft(plus(120 * MIN), now)).toBe('2h left');
  expect(formatTimeLeft(plus(3 * MIN), now)).toBe('3m left');
  expect(formatTimeLeft(plus(20_000), now)).toBe('1m left');
  expect(formatTimeLeft(plus(26 * 60 * MIN), now)).toBe('1d 2h left');
});

test('time left reads Expired at and after the deadline', () => {
  expect(formatTimeLeft(now, now)).toBe('Expired');
  expect(formatTimeLeft(plus(-5 * MIN).toISOString(), now)).toBe('Expired');
});

test('relative time reads in minutes, hours and days ago', () => {
  expect(formatRelative(plus(-20_000), now)).toBe('just now');
  expect(formatRelative(plus(5_000), now)).toBe('just now');
  expect(formatRelative(plus(-12 * MIN), now)).toBe('12 min ago');
  expect(formatRelative(plus(-3 * 60 * MIN).toISOString(), now)).toBe('3 h ago');
  expect(formatRelative(plus(-2 * 24 * 60 * MIN), now)).toBe('2 d ago');
});

test('status pill tone follows the phase meaning', () => {
  expect(phaseTone('Pending')).toBe('warning');
  for (const p of ['Approved', 'Active', 'Ready', 'Applied']) expect(phaseTone(p)).toBe('success');
  for (const p of ['Rejected', 'Revoked', 'Failed', 'NotReady']) expect(phaseTone(p)).toBe('danger');
  for (const p of ['Cancelled', 'Expired', 'Whatever', undefined]) expect(phaseTone(p)).toBe('neutral');
});

test('elapsed time reads as a span for "waiting ..." labels', () => {
  expect(formatElapsed(plus(-20_000), now)).toBe('under 1 min');
  expect(formatElapsed(plus(-4 * MIN), now)).toBe('4 min');
  expect(formatElapsed(plus(-125 * MIN).toISOString(), now)).toBe('2 h');
  expect(formatElapsed(plus(-50 * 60 * MIN), now)).toBe('2 d');
});

test('time used is 0 before the grant starts, 1 once it is due, and clamped in between', () => {
  const started = '2026-11-24T12:00:00Z';
  const expires = '2026-11-24T13:00:00Z';
  expect(timeUsedFraction(started, expires, new Date('2026-11-24T11:00:00Z'))).toBe(0);
  expect(timeUsedFraction(started, expires, new Date('2026-11-24T12:30:00Z'))).toBe(0.5);
  expect(timeUsedFraction(started, expires, new Date('2026-11-24T14:00:00Z'))).toBe(1);
});

test('time used falls back to 1 for an unparseable or non-positive span', () => {
  expect(timeUsedFraction('nope', '2026-11-24T13:00:00Z', new Date())).toBe(1);
  expect(timeUsedFraction('2026-11-24T13:00:00Z', '2026-11-24T12:00:00Z', new Date())).toBe(1);
});

test('a grant is ending soon in its last 30 minutes, and not once it has ended', () => {
  expect(endingSoon(plus(29 * MIN), now)).toBe(true);
  expect(endingSoon(plus(31 * MIN), now)).toBe(false);
  expect(endingSoon(plus(-1 * MIN), now)).toBe(false);
});
