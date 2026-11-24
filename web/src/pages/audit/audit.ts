import { toQuery } from '../../app/paging';
import type { AuditItem } from '../../generated/views';

export interface AuditFilters {
  query: string;
  event: string;
  cluster: string;
  order: 'asc' | 'desc';
}

export const ALL = 'all';
export const NO_FILTERS: AuditFilters = { query: '', event: ALL, cluster: ALL, order: 'desc' };

const EVENT_LABELS: Record<string, string> = {
  REQUEST_SUBMITTED: 'Request submitted',
  APPROVAL_GIVEN: 'Approval given',
  REQUEST_APPROVED: 'Request approved',
  REQUEST_REJECTED: 'Request rejected',
  REQUEST_CANCELLED: 'Request cancelled',
  REQUEST_TIMED_OUT: 'Request timed out',
  ACCESS_GRANTED: 'Access granted',
  ACCESS_REVOKED: 'Access revoked',
  ACCESS_EXPIRED: 'Access expired',
};

/** Every event type the server records, in lifecycle order. */
export const EVENT_TYPES = Object.keys(EVENT_LABELS);

export function eventLabel(type: string): string {
  if (Object.prototype.hasOwnProperty.call(EVENT_LABELS, type)) return EVENT_LABELS[type];
  const words = type.toLocaleLowerCase('en').replace(/_/g, ' ');
  return words.charAt(0).toLocaleUpperCase('en') + words.slice(1);
}

export function auditQuery(f: AuditFilters, offset: number, limit: number): string {
  return toQuery({
    q: f.query.trim(),
    event: f.event === ALL ? undefined : f.event,
    cluster: f.cluster === ALL ? undefined : f.cluster,
    order: f.order,
    offset,
    limit,
  });
}

/** The actor's comment, else the requester's reason and ticket. */
export function auditDetails(row: Pick<AuditItem, 'comment' | 'reason' | 'ticket'>): string {
  if (row.comment) return row.comment;
  if (row.reason && row.ticket) return `${row.reason} (${row.ticket})`;
  return row.reason || row.ticket || '';
}
