import { useEffect, useState, type DependencyList } from 'react';
import { usePollVersion } from './data';
import { clampPage, pageFor, slicePage } from './paging';
import { settle, type LiveResult } from './sync';

export interface Loaded<T> {
  data: T | null;
  error: Error | null;
  /** True until the first answer; a reload keeps showing the previous answer. */
  loading: boolean;
  reload: () => void;
}

const asError = (e: unknown) => (e instanceof Error ? e : new Error(String(e)));

/** One on-demand API read outside the 3 s poll, reloaded when `deps` change. */
export function useLoad<T>(load: () => Promise<T>, deps: DependencyList): Loaded<T> {
  const [result, setResult] = useState<{ data: T | null; error: Error | null } | null>(null);
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    let alive = true;
    load().then(
      (data) => alive && setResult({ data, error: null }),
      (e) => alive && setResult({ data: null, error: asError(e) }),
    );
    return () => {
      alive = false;
    };
  }, [...deps, attempt]);

  return {
    data: result?.data ?? null,
    error: result?.error ?? null,
    loading: result === null,
    reload: () => setAttempt((n) => n + 1),
  };
}

/**
 * An API read refetched after every poll. It shows the previous answer while refetching, keeps it when a
 * refetch fails (except a 404: the object is gone), and does not re-render when the answer is unchanged.
 * `initial`, when given, is shown until the first answer arrives.
 */
export function useLive<T>(load: () => Promise<T>, deps: DependencyList, initial?: T): Loaded<T> {
  const version = usePollVersion();
  const [result, setResult] = useState<LiveResult<T> | null>(initial === undefined ? null : { data: initial, error: null });
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    let alive = true;
    load().then(
      (data) => alive && setResult((prev) => settle(prev, { ok: true, data })),
      (e) => alive && setResult((prev) => settle(prev, { ok: false, error: asError(e) })),
    );
    return () => {
      alive = false;
    };
  }, [...deps, attempt, version]);

  return {
    data: result?.data ?? null,
    error: result?.error ?? null,
    loading: result === null,
    reload: () => setAttempt((n) => n + 1),
  };
}

export interface Paged<T> {
  items: T[];
  total: number;
  /** 0-based. */
  page: number;
  pageSize: number;
  setPage: (page: number) => void;
}

/**
 * One page of a server-paged list, refetched with every poll. A new `filterKey` returns to the first page;
 * a page that empties (its last row went away) steps back to the last page with rows.
 */
export function usePagedList<T>(
  fetchPage: (offset: number, limit: number) => Promise<{ items: T[]; total: number }>,
  filterKey: string,
  pageSize: number,
): Paged<T> & Pick<Loaded<unknown>, 'loading' | 'error' | 'reload'> {
  const [state, setState] = useState({ key: filterKey, page: 0 });
  const page = pageFor(state, filterKey);
  const live = useLive(() => fetchPage(page * pageSize, pageSize), [filterKey, page, pageSize]);
  const data = live.data;

  useEffect(() => {
    if (!data || data.items.length || page === 0) return;
    const back = clampPage(page, data.total, pageSize);
    if (back !== page) setState({ key: filterKey, page: back });
  }, [data, page, pageSize, filterKey]);

  return {
    items: data?.items ?? [],
    total: data?.total ?? 0,
    page,
    pageSize,
    setPage: (next) => setState({ key: filterKey, page: next }),
    loading: live.loading,
    error: live.error,
    reload: live.reload,
  };
}

/** One page of a list held in full on the client; same paging rules as `usePagedList`. */
export function useClientPage<T>(all: T[], filterKey: string, pageSize: number): Paged<T> {
  const [state, setState] = useState({ key: filterKey, page: 0 });
  const page = clampPage(pageFor(state, filterKey), all.length, pageSize);
  return {
    items: slicePage(all, page, pageSize),
    total: all.length,
    page,
    pageSize,
    setPage: (next) => setState({ key: filterKey, page: next }),
  };
}
