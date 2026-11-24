/** Capabilities are re-read on every Nth poll; they change rarely and cost a SubjectAccessReview each. */
export const CAPABILITIES_EVERY = 10;

/** Structural equality for JSON values (what the API returns). */
export function deepEqual(a: unknown, b: unknown): boolean {
  if (a === b) return true;
  if (typeof a !== 'object' || typeof b !== 'object' || a === null || b === null) return false;
  if (Array.isArray(a) !== Array.isArray(b)) return false;
  const ka = Object.keys(a);
  const kb = Object.keys(b);
  if (ka.length !== kb.length) return false;
  const ra = a as Record<string, unknown>;
  const rb = b as Record<string, unknown>;
  return ka.every((k) => Object.prototype.hasOwnProperty.call(rb, k) && deepEqual(ra[k], rb[k]));
}

/** `prev` when `next` has the same content, so a state setter given it bails out of re-rendering. */
export function keepIfEqual<T>(prev: T, next: T): T {
  return deepEqual(prev, next) ? prev : next;
}

export function capabilitiesDue(poll: number, userChanged: boolean, requested: boolean): boolean {
  return userChanged || requested || poll % CAPABILITIES_EVERY === 0;
}

export interface LiveResult<T> {
  data: T | null;
  error: Error | null;
}

export type Outcome<T> = { ok: true; data: T } | { ok: false; error: Error };

/**
 * The next state of a live read: an unchanged answer keeps `prev`; a failed refetch keeps the last good
 * data, except a 404, which means the object is gone.
 */
export function settle<T>(prev: LiveResult<T> | null, outcome: Outcome<T>): LiveResult<T> {
  if (outcome.ok) return prev && !prev.error && deepEqual(prev.data, outcome.data) ? prev : { data: outcome.data, error: null };
  const gone = 'status' in outcome.error && outcome.error.status === 404;
  if (prev?.data != null && !gone) return prev;
  return { data: null, error: outcome.error };
}
