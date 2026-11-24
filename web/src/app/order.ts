type Key<T> = (x: T) => string | undefined;
export type Comparator<T> = (a: T, b: T) => number;

const instant = (at: string | undefined) => {
  const ms = Date.parse(at ?? '');
  return Number.isNaN(ms) ? -Infinity : ms;
};

const compare = <V extends string | number>(a: V, b: V) => (a < b ? -1 : a > b ? 1 : 0);

/**
 * Time order in `dir`, then each tie-breaker ascending (missing first), compared by code unit so the
 * result never depends on locale. With unique tie-breakers no two items tie, so rows never swap between polls.
 */
export function byTime<T>(dir: 'asc' | 'desc', at: Key<T>, ...ties: Key<T>[]): Comparator<T> {
  const sign = dir === 'asc' ? 1 : -1;
  return (a, b) => {
    const byAt = sign * compare(instant(at(a)), instant(at(b)));
    if (byAt) return byAt;
    for (const key of ties) {
      const c = compare(key(a) ?? '', key(b) ?? '');
      if (c) return c;
    }
    return 0;
  };
}

type Named = { namespace: string; name: string };

export const oldestFirst = <T extends Named>(at: Key<T>) => byTime<T>('asc', at, (x) => x.namespace, (x) => x.name);

export const newestFirst = <T extends Named>(at: Key<T>) => byTime<T>('desc', at, (x) => x.namespace, (x) => x.name);
