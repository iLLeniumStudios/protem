import type { CapabilitiesView, ProfileView, RequesterView, UserInfo } from '../generated/views';
import { canSeeProfiles } from '../pages/profiles/manage';

/** What the hub says the caller may do; the shape `useData()` returns. */
export interface Viewer {
  capabilities: CapabilitiesView | null;
  profiles: Pick<ProfileView, 'canApprove'>[];
}

/** Same person as the server decides it: a matching non-empty username or a matching non-empty email. */
export function isMe(me: Pick<UserInfo, 'username' | 'email'> | null | undefined, who: RequesterView): boolean {
  if (!me) return false;
  return (Boolean(me.username) && me.username === who.username) || (Boolean(me.email) && me.email === who.email);
}

export function isAdmin(v: Viewer): boolean {
  return Boolean(v.capabilities?.viewAll);
}

export function isApprover(v: Viewer): boolean {
  return isAdmin(v) || v.profiles.some((p) => p.canApprove);
}

export function canViewFleet(v: Viewer): boolean {
  return Boolean(v.capabilities?.viewFleet);
}

export function roleLabel(v: Viewer): string {
  if (isAdmin(v)) return 'Admin';
  if (isApprover(v)) return 'Approver';
  return 'Requester';
}

export type Workspace = 'access' | 'admin';

export interface NavItem {
  to: string;
  label: string;
  /** Shows the count of requests waiting for the viewer. */
  badge?: boolean;
}

const NAV: Record<Workspace, (NavItem & { visible: (v: Viewer) => boolean })[]> = {
  access: [
    { to: '/', label: 'For you', visible: () => true },
    { to: '/request', label: 'Activate access', visible: () => true },
    { to: '/requests', label: 'My requests', visible: () => true },
    { to: '/approvals', label: 'Approvals', visible: isApprover, badge: true },
  ],
  admin: [
    { to: '/admin', label: 'Overview', visible: isAdmin },
    { to: '/timeline', label: 'Access timeline', visible: isAdmin },
    { to: '/profiles', label: 'Profiles', visible: (v) => canSeeProfiles(v.capabilities) },
    { to: '/clusters', label: 'Clusters', visible: canViewFleet },
    { to: '/audit', label: 'Audit', visible: isAdmin },
  ],
};

const section = (pathname: string) => pathname.split('/').filter(Boolean)[0] ?? '';

export function workspaceOf(pathname: string): Workspace {
  const first = `/${section(pathname)}`;
  return NAV.admin.some((item) => item.to === first) ? 'admin' : 'access';
}

/** The navigation items of `workspace` this viewer may use, in display order. */
export function navFor(workspace: Workspace, v: Viewer): NavItem[] {
  return NAV[workspace].filter((item) => item.visible(v)).map(({ visible: _, ...item }) => item);
}

export const REDIRECTS: Record<string, string> = { '/me': '/requests', '/fleet': '/clusters', '/overview': '/admin' };

const TITLES: Record<string, string> = {
  ...Object.fromEntries([...NAV.access, ...NAV.admin].map((item) => [section(item.to), item.label])),
  grants: 'Grant',
};

export function pageTitle(pathname: string): string {
  const parts = pathname.split('/').filter(Boolean);
  const first = parts[0] ?? '';
  let title = Object.prototype.hasOwnProperty.call(TITLES, first) ? TITLES[first] : 'Page not found';
  if (first === 'request' && parts.length > 1) title = 'New request';
  if (first === 'requests' && parts.length > 1) title = 'Request';
  if (first === 'profiles' && parts.length > 1) {
    title = parts[1] === 'new' ? 'New profile' : parts[3] === 'edit' ? 'Editing profile' : 'Profile';
  }
  return `${title} · Protem`;
}
