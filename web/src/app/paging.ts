/** Page indexes are 0-based; what people read is 1-based. */

export function lastPage(total: number, size: number): number {
  return Math.max(0, Math.ceil(total / size) - 1);
}

/** The page to show: never past the last page that has rows, so removing the last row of a page steps back. */
export function clampPage(page: number, total: number, size: number): number {
  return Math.min(Math.max(0, page), lastPage(total, size));
}

/** The remembered page, or the first page when the filters changed since it was chosen. */
export function pageFor(state: { key: string; page: number }, key: string): number {
  return state.key === key ? state.page : 0;
}

export function rangeText(page: number, size: number, total: number): string {
  if (total <= 0) return '0 of 0';
  const from = page * size + 1;
  return `${from}–${Math.min(total, from + size - 1)} of ${total}`;
}

export function slicePage<T>(items: T[], page: number, size: number): T[] {
  return items.slice(page * size, (page + 1) * size);
}

export type QueryValue = string | number | boolean | undefined;

/** A query string from the values that are set: empty strings, `false` and `undefined` are left out. */
export function toQuery(params: Record<string, QueryValue>): string {
  const q = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === '' || v === false) continue;
    q.set(k, String(v));
  }
  const s = q.toString();
  return s ? `?${s}` : '';
}
