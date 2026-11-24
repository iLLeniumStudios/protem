import { toQuery } from './app/paging';
import {
  MeView,
  ProfileView,
  RequestView,
  RequestPage,
  GrantView,
  GrantPage,
  StatsView,
  AuditPage,
  ClusterView,
  CreateRequestPayload,
  CapabilitiesView,
  ProfileDetailView,
  PermissionsView,
  ClusterPermissionsView,
  ClusterSettingsPayload,
  ProfilePayload,
  AddClusterPayload,
  JoinView,
} from './generated/views';

export type RequestQuery = { state?: 'open' | 'decided'; mine?: boolean; approver?: boolean; offset?: number; limit?: number };
export type GrantQuery = { state?: 'active' | 'ended'; mine?: boolean; offset?: number; limit?: number };

const objectPath = (kind: 'requests' | 'grants', namespace: string, name: string) =>
  `/namespaces/${encodeURIComponent(namespace)}/${kind}/${encodeURIComponent(name)}`;

const API_BASE = '/api';

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
    super(message);
  }
}

export class ApiClient {
  private activeUser: string = 'alice@company.com';
  private signedOutListeners = new Set<(message: string) => void>();

  /** Calls `listener` with the server's message whenever a call is answered 401; returns the unsubscribe. */
  onSignedOut(listener: (message: string) => void): () => void {
    this.signedOutListeners.add(listener);
    return () => this.signedOutListeners.delete(listener);
  }

  setActiveUser(user: string) {
    this.activeUser = user;
    localStorage.setItem('protem_test_user', user);
  }

  getActiveUser(): string {
    return localStorage.getItem('protem_test_user') || this.activeUser;
  }

  private getHeaders(): HeadersInit {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
    };
    const user = this.getActiveUser();
    if (user) {
      headers['X-Test-User'] = user;
    }
    return headers;
  }

  private async call<T>(path: string, init?: { method: string; body?: unknown }): Promise<T> {
    const res = await fetch(`${API_BASE}${path}`, {
      method: init?.method,
      headers: this.getHeaders(),
      body: init?.body === undefined ? undefined : JSON.stringify(init.body),
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      const message: string = err.error || '';
      if (res.status === 401) this.signedOutListeners.forEach((l) => l(message));
      throw new ApiError(message || res.statusText || `Request failed with status ${res.status}`, res.status);
    }
    return res.status === 204 ? (undefined as T) : res.json();
  }

  getMe(): Promise<MeView> {
    return this.call('/me');
  }

  getCapabilities(): Promise<CapabilitiesView> {
    return this.call('/capabilities');
  }

  getProfile(namespace: string, name: string): Promise<ProfileDetailView> {
    return this.call(`/namespaces/${encodeURIComponent(namespace)}/profiles/${encodeURIComponent(name)}`);
  }

  createProfile(namespace: string, payload: ProfilePayload): Promise<ProfileDetailView> {
    return this.call(`/namespaces/${encodeURIComponent(namespace)}/profiles`, { method: 'POST', body: payload });
  }

  updateProfile(namespace: string, name: string, payload: ProfilePayload): Promise<ProfileDetailView> {
    return this.call(`/namespaces/${encodeURIComponent(namespace)}/profiles/${encodeURIComponent(name)}`, {
      method: 'PUT',
      body: payload,
    });
  }

  deleteProfile(namespace: string, name: string): Promise<void> {
    return this.call(`/namespaces/${encodeURIComponent(namespace)}/profiles/${encodeURIComponent(name)}`, { method: 'DELETE' });
  }

  updateCluster(name: string, payload: ClusterSettingsPayload): Promise<ClusterView> {
    return this.call(`/clusters/${encodeURIComponent(name)}`, { method: 'PATCH', body: payload });
  }

  addCluster(payload: AddClusterPayload): Promise<ClusterView> {
    return this.call('/clusters', { method: 'POST', body: payload });
  }

  joinCluster(name: string): Promise<JoinView> {
    return this.call(`/clusters/${encodeURIComponent(name)}/join`, { method: 'POST' });
  }

  reissueCluster(name: string): Promise<JoinView> {
    return this.call(`/clusters/${encodeURIComponent(name)}/reissue`, { method: 'POST' });
  }

  removeCluster(name: string): Promise<void> {
    return this.call(`/clusters/${encodeURIComponent(name)}`, { method: 'DELETE' });
  }

  getProfilePermissions(namespace: string, name: string): Promise<PermissionsView> {
    return this.call(`/namespaces/${encodeURIComponent(namespace)}/profiles/${encodeURIComponent(name)}/permissions`);
  }

  getGrantPermissions(namespace: string, name: string): Promise<ClusterPermissionsView[]> {
    return this.call(`/namespaces/${encodeURIComponent(namespace)}/grants/${encodeURIComponent(name)}/permissions`);
  }

  getClusters(): Promise<ClusterView[]> {
    return this.call('/clusters');
  }

  getProfiles(): Promise<ProfileView[]> {
    return this.call('/profiles');
  }

  getRequests(query: RequestQuery): Promise<RequestPage> {
    return this.call(`/requests${toQuery(query)}`);
  }

  getRequest(namespace: string, name: string): Promise<RequestView> {
    return this.call(objectPath('requests', namespace, name));
  }

  createRequest(namespace: string, payload: CreateRequestPayload): Promise<RequestView> {
    return this.call(`/namespaces/${encodeURIComponent(namespace)}/requests`, { method: 'POST', body: payload });
  }

  private decide(req: RequestView, action: 'approve' | 'reject' | 'cancel', body: unknown): Promise<RequestView> {
    return this.call(
      `/namespaces/${encodeURIComponent(req.namespace)}/requests/${encodeURIComponent(req.name)}/${action}`,
      { method: 'POST', body },
    );
  }

  approveRequest(req: RequestView, comment?: string): Promise<RequestView> {
    return this.decide(req, 'approve', { comment });
  }

  rejectRequest(req: RequestView, reason: string): Promise<RequestView> {
    return this.decide(req, 'reject', { reason });
  }

  cancelRequest(req: RequestView, reason?: string): Promise<RequestView> {
    return this.decide(req, 'cancel', { reason });
  }

  getGrants(query: GrantQuery): Promise<GrantPage> {
    return this.call(`/grants${toQuery(query)}`);
  }

  getGrant(namespace: string, name: string): Promise<GrantView> {
    return this.call(objectPath('grants', namespace, name));
  }

  revokeGrant(grant: GrantView, reason?: string): Promise<GrantView> {
    return this.call(
      `/namespaces/${encodeURIComponent(grant.namespace)}/grants/${encodeURIComponent(grant.name)}/revoke`,
      { method: 'POST', body: { reason } },
    );
  }

  getStats(): Promise<StatsView> {
    return this.call('/stats');
  }

  /** `query` comes from `auditQuery`. */
  getAudit(query: string): Promise<AuditPage> {
    return this.call(`/audit${query}`);
  }
}

export const api = new ApiClient();

/** The server's message, or a fallback that still names the problem and the next step. */
export function errorMessage(err: unknown): string {
  return err instanceof Error && err.message ? err.message : 'The request failed. Try again.';
}
