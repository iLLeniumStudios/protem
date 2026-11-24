// Run: cd web && bun test src/pages/audit
import { expect, test } from 'bun:test';
import { ALL, EVENT_TYPES, NO_FILTERS, auditDetails, auditQuery, eventLabel } from './audit';

test('filters become query parameters; empty and "all" filters are left out', () => {
  expect(auditQuery(NO_FILTERS, 0, 25)).toBe('?order=desc&offset=0&limit=25');
  expect(
    auditQuery({ query: '  CHG-77 ', event: 'ACCESS_REVOKED', cluster: 'prod eu', order: 'asc' }, 50, 25),
  ).toBe('?q=CHG-77&event=ACCESS_REVOKED&cluster=prod+eu&order=asc&offset=50&limit=25');
  expect(auditQuery({ ...NO_FILTERS, query: '   ', event: ALL, cluster: ALL }, 25, 25)).toBe('?order=desc&offset=25&limit=25');
});

test('every event type has a plain sentence-case label', () => {
  expect(EVENT_TYPES.map(eventLabel)).toEqual([
    'Request submitted',
    'Approval given',
    'Request approved',
    'Request rejected',
    'Request cancelled',
    'Request timed out',
    'Access granted',
    'Access revoked',
    'Access expired',
  ]);
  expect(eventLabel('SOMETHING_NEW')).toBe('Something new');
});

test("details show the actor's comment, else the requester's reason and ticket", () => {
  expect(auditDetails({ comment: 'Looks fine', reason: 'Fix outage', ticket: 'INC-1' })).toBe('Looks fine');
  expect(auditDetails({ reason: 'Fix outage', ticket: 'INC-1' })).toBe('Fix outage (INC-1)');
  expect(auditDetails({ reason: 'Fix outage' })).toBe('Fix outage');
  expect(auditDetails({ ticket: 'INC-1' })).toBe('INC-1');
  expect(auditDetails({})).toBe('');
});
