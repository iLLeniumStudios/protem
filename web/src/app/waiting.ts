import type { RequestView } from '../generated/views';
import { oldestFirst } from './order';

/** Every pending request the user can see, oldest first: the fleet-wide oversight list. */
export function pendingOldestFirst(requests: RequestView[]): RequestView[] {
  return requests
    .filter((r) => r.phase === 'Pending')
    .sort(oldestFirst((r) => r.createdAt));
}

/** Pending requests the server lets this user decide, oldest first. */
export function waitingForYou(requests: RequestView[]): RequestView[] {
  return pendingOldestFirst(requests).filter((r) => r.actions.approve || r.actions.reject);
}
