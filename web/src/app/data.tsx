import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { api } from '../api';
import { capabilitiesDue, keepIfEqual } from './sync';
import type {
  AddClusterPayload,
  CapabilitiesView,
  ClusterSettingsPayload,
  ClusterView,
  CreateRequestPayload,
  GrantPage,
  GrantView,
  JoinView,
  MeView,
  ProfileDetailView,
  ProfilePayload,
  ProfileView,
  RequestPage,
  RequestView,
  StatsView,
} from '../generated/views';

const POLL_MS = 3000;
/** The most open requests and active grants the poll holds; history is paged by the page that shows it. */
export const SNAPSHOT_LIMIT = 100;
const EMPTY_PAGE = { items: [], total: 0 };

interface Snapshot {
  me: MeView;
  profiles: ProfileView[];
  openRequests: RequestPage;
  activeGrants: GrantPage;
  clusters: ClusterView[];
  stats: StatsView;
}

export interface DataActions {
  approve: (req: RequestView, comment?: string) => Promise<RequestView>;
  reject: (req: RequestView, reason: string) => Promise<RequestView>;
  cancel: (req: RequestView, reason?: string) => Promise<RequestView>;
  createRequest: (namespace: string, payload: CreateRequestPayload) => Promise<RequestView>;
  revoke: (grant: GrantView, reason?: string) => Promise<GrantView>;
  createProfile: (namespace: string, payload: ProfilePayload) => Promise<ProfileDetailView>;
  updateProfile: (namespace: string, name: string, payload: ProfilePayload) => Promise<ProfileDetailView>;
  deleteProfile: (namespace: string, name: string) => Promise<void>;
  updateCluster: (name: string, payload: ClusterSettingsPayload) => Promise<ClusterView>;
  addCluster: (payload: AddClusterPayload) => Promise<ClusterView>;
  joinCluster: (name: string) => Promise<JoinView>;
  reissueCluster: (name: string) => Promise<JoinView>;
  removeCluster: (name: string) => Promise<void>;
}

export interface DataState {
  me: MeView | null;
  /** What the hub lets the current user do; fetched on load and on persona change, not polled. */
  capabilities: CapabilitiesView | null;
  profiles: ProfileView[];
  /** Pending requests the caller may see, newest first; `total` may exceed the items held. */
  openRequests: RequestPage;
  /** Active grants the caller may see; `total` may exceed the items held. */
  activeGrants: GrantPage;
  clusters: ClusterView[];
  stats: StatsView | null;
  /** True until the first successful load. */
  loading: boolean;
  /** Message of the last failed load; null once a load succeeds. */
  error: string | null;
  /** The server's message from the first 401 (possibly empty); null while signed in. Polling stops once set. */
  signedOut: string | null;
  refresh: () => Promise<void>;
  actions: DataActions;
}

const DataContext = createContext<DataState | null>(null);
const VersionContext = createContext(0);

async function fetchSnapshot(): Promise<Snapshot> {
  const [me, profiles, openRequests, activeGrants, clusters, stats] = await Promise.all([
    api.getMe(),
    api.getProfiles(),
    api.getRequests({ state: 'open', limit: SNAPSHOT_LIMIT }),
    api.getGrants({ state: 'active', limit: SNAPSHOT_LIMIT }),
    api.getClusters(),
    api.getStats(),
  ]);
  // Go encodes an empty slice as null.
  return {
    me,
    profiles: profiles ?? [],
    openRequests,
    activeGrants,
    clusters: clusters ?? [],
    stats,
  };
}

export function DataProvider({ children }: { children: ReactNode }) {
  const [data, setData] = useState<Snapshot | null>(null);
  const [capabilities, setCapabilities] = useState<CapabilitiesView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [signedOut, setSignedOut] = useState<string | null>(null);
  const [version, setVersion] = useState(0);
  const seq = useRef(0);
  const polls = useRef(0);
  const capabilitiesUser = useRef<string | null>(null);
  const capabilitiesWanted = useRef(false);
  const stopped = useRef(false);

  useEffect(
    () =>
      api.onSignedOut((message) => {
        stopped.current = true;
        setSignedOut((prev) => prev ?? message);
      }),
    [],
  );

  const refresh = useCallback(async () => {
    const mine = ++seq.current;
    const user = api.getActiveUser();
    const due = capabilitiesDue(polls.current++, capabilitiesUser.current !== user, capabilitiesWanted.current);
    try {
      const [next, caps] = await Promise.all([fetchSnapshot(), due ? api.getCapabilities() : null]);
      // A newer refresh started meanwhile (poll vs. action), or the persona changed: this answer is stale.
      if (mine !== seq.current || user !== api.getActiveUser()) return;
      if (caps) {
        capabilitiesUser.current = user;
        capabilitiesWanted.current = false;
        setCapabilities((prev) => keepIfEqual(prev, caps));
      }
      setData((prev) => keepIfEqual(prev, next));
      setError(null);
      setVersion((v) => v + 1);
    } catch (e) {
      if (mine !== seq.current) return;
      setError(e instanceof Error ? e.message : String(e));
    }
  }, []);

  useEffect(() => {
    let alive = true;
    let timer: ReturnType<typeof setTimeout>;
    const tick = async () => {
      await refresh();
      if (alive && !stopped.current) timer = setTimeout(tick, POLL_MS);
    };
    tick();
    return () => {
      alive = false;
      clearTimeout(timer);
    };
  }, [refresh]);

  const actions = useMemo<DataActions>(() => {
    const thenRefresh = async <T,>(p: Promise<T>) => {
      const result = await p;
      capabilitiesWanted.current = true;
      await refresh();
      return result;
    };
    return {
      approve: (req, comment) => thenRefresh(api.approveRequest(req, comment)),
      reject: (req, reason) => thenRefresh(api.rejectRequest(req, reason)),
      cancel: (req, reason) => thenRefresh(api.cancelRequest(req, reason)),
      createRequest: (namespace, payload) => thenRefresh(api.createRequest(namespace, payload)),
      revoke: (grant, reason) => thenRefresh(api.revokeGrant(grant, reason)),
      createProfile: (namespace, payload) => thenRefresh(api.createProfile(namespace, payload)),
      updateProfile: (namespace, name, payload) => thenRefresh(api.updateProfile(namespace, name, payload)),
      deleteProfile: (namespace, name) => thenRefresh(api.deleteProfile(namespace, name)),
      updateCluster: (name, payload) => thenRefresh(api.updateCluster(name, payload)),
      addCluster: (payload) => thenRefresh(api.addCluster(payload)),
      joinCluster: (name) => thenRefresh(api.joinCluster(name)),
      reissueCluster: (name) => thenRefresh(api.reissueCluster(name)),
      removeCluster: (name) => thenRefresh(api.removeCluster(name)),
    };
  }, [refresh]);

  const value = useMemo<DataState>(
    () => ({
      me: data?.me ?? null,
      capabilities,
      profiles: data?.profiles ?? [],
      openRequests: data?.openRequests ?? EMPTY_PAGE,
      activeGrants: data?.activeGrants ?? EMPTY_PAGE,
      clusters: data?.clusters ?? [],
      stats: data?.stats ?? null,
      loading: data === null || capabilities === null,
      error,
      signedOut,
      refresh,
      actions,
    }),
    [data, capabilities, error, signedOut, refresh, actions],
  );

  return (
    <DataContext.Provider value={value}>
      <VersionContext.Provider value={version}>{children}</VersionContext.Provider>
    </DataContext.Provider>
  );
}

export function useData() {
  const ctx = useContext(DataContext);
  if (!ctx) throw new Error('useData must be used inside <DataProvider>');
  return ctx;
}

/** Increases after each successful poll; kept out of `useData()` so a poll re-renders only its readers. */
export function usePollVersion() {
  return useContext(VersionContext);
}
