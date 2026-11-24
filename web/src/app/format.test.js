// Run: cd web && bun test src/app
import { expect, test } from 'bun:test';
import { formatClock, formatDuration, grantPath, profileName, requestFormPath, requestPath } from './format';

test('durations drop zero units; unparseable input is shown as is', () => {
  expect(formatDuration('1h0m0s')).toBe('1h');
  expect(formatDuration('15m0s')).toBe('15m');
  expect(formatDuration('1h30m0s')).toBe('1h 30m');
  expect(formatDuration('15m')).toBe('15m');
  expect(formatDuration('45s')).toBe('45s');
  expect(formatDuration('0s')).toBe('0s');
  expect(formatDuration('')).toBe('');
  expect(formatDuration('nonsense')).toBe('nonsense');
});

test('paths escape namespace and name', () => {
  expect(requestPath({ namespace: 'team a', name: 'req/1' })).toBe('/requests/team%20a/req%2F1');
  expect(grantPath({ namespace: 'default', name: 'g-1' })).toBe('/grants/default/g-1');
  expect(requestFormPath({ namespace: 'default', name: 'prod' })).toBe('/request/default/prod');
});

test('profile name prefers the display name in the same namespace', () => {
  const profiles = [
    { namespace: 'default', name: 'prod', displayName: 'Production' },
    { namespace: 'other', name: 'dev', displayName: 'Dev' },
    { namespace: 'default', name: 'bare', displayName: '' },
  ];
  expect(profileName(profiles, 'default', 'prod')).toBe('Production');
  expect(profileName(profiles, 'default', 'dev')).toBe('dev');
  expect(profileName(profiles, 'default', 'bare')).toBe('bare');
});

test('clock times read in 24-hour local time with two-digit hours and minutes', () => {
  expect(formatClock(new Date(2026, 10, 24, 22, 39))).toBe('22:39');
  expect(formatClock(new Date(2026, 10, 24, 9, 5).toISOString())).toBe('09:05');
  expect(formatClock(new Date(2026, 10, 24, 0, 0).getTime())).toBe('00:00');
});
