import { plural } from '../../app/format';
import { oldestFirst } from '../../app/order';
import { isMe } from '../../app/roles';
import { approvers } from '../access/model';
import type { GrantView, ProfileView, RequestView, UserInfo } from '../../generated/views';

export function myLiveGrants(grants: GrantView[], me: Pick<UserInfo, 'username' | 'email'> | null): GrantView[] {
  return grants.filter((g) => g.phase === 'Active' && isMe(me, g.requester)).sort(oldestFirst((g) => g.expiresAt));
}

/** Where the person stands with a profile: holds it, waits for it, or may activate it at once or ask for approval. */
export type ActivationState = 'active' | 'waiting' | 'activate' | 'approval';

export interface Activatable {
  profile: ProfileView;
  state: ActivationState;
}

const key = (namespace: string, name: string) => `${namespace}/${name}`;

export interface ActivationCopy {
  mode: 'at-once' | 'approval' | 'mixed';
  control: 'Activate access' | 'Request approval';
  line: string;
  title?: string;
}

const SHOWN_APPROVERS = 2;

/** How a request for the profile is decided, in words; an unknown or empty `activation` reads as approval. */
export function describeActivation(profile: Pick<ProfileView, 'activation' | 'approverGroups' | 'approverUsers'>): ActivationCopy {
  if (profile.activation === 'at-once') return { mode: 'at-once', control: 'Activate access', line: 'Granted at once' };
  if (profile.activation === 'mixed') {
    return {
      mode: 'mixed',
      control: 'Request approval',
      line: 'Approval on some clusters',
      title: 'Granted at once on some clusters, after approval on others',
    };
  }
  const names = approvers(profile);
  if (!names.length) return { mode: 'approval', control: 'Request approval', line: 'Needs approval' };
  const more = names.length > SHOWN_APPROVERS ? ` +${names.length - SHOWN_APPROVERS} more` : '';
  return {
    mode: 'approval',
    control: 'Request approval',
    line: `Approval by ${names.slice(0, SHOWN_APPROVERS).join(', ')}${more}`,
    title: names.join(', '),
  };
}

/** Profiles the person may request, alphabetical. `myGrants` and `myRequests` must already be the person's own. */
export function activatable(profiles: ProfileView[], myGrants: GrantView[], myRequests: RequestView[]): Activatable[] {
  const held = new Set(myGrants.filter((g) => g.phase === 'Active').map((g) => key(g.namespace, g.profile)));
  const asked = new Set(myRequests.filter((r) => r.phase === 'Pending').map((r) => key(r.namespace, r.profile)));
  return profiles
    .filter((p) => p.canRequest)
    .sort((a, b) => (a.displayName || a.name).localeCompare(b.displayName || b.name, 'en'))
    .map((profile) => {
      const k = key(profile.namespace, profile.name);
      const atOnce = describeActivation(profile).mode === 'at-once';
      const state: ActivationState = held.has(k) ? 'active' : asked.has(k) ? 'waiting' : atOnce ? 'activate' : 'approval';
      return { profile, state };
    });
}

export function forYouSummary({ deciding, live, waiting }: { deciding: number; live: number; waiting: number }): string {
  const parts: string[] = [];
  if (deciding) parts.push(`${plural(deciding, 'request')} ${deciding === 1 ? 'waits' : 'wait'} for your decision.`);
  if (live) parts.push(`You hold ${plural(live, 'grant')}.`);
  if (waiting) parts.push(`${waiting} of your requests ${waiting === 1 ? 'is' : 'are'} waiting for approval.`);
  return parts.join(' ') || 'Nothing needs you right now.';
}
