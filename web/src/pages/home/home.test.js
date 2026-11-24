// Run: cd web && bun test src/pages/home
import { expect, test } from 'bun:test';
import { activatable, describeActivation, forYouSummary, myLiveGrants } from './home';

const me = { username: 'alice', email: 'alice@company.com' };

const profile = (name, over = {}) => ({
  namespace: 'team-payments',
  name,
  displayName: name,
  role: { kind: 'ClusterRole', name: 'view' },
  scope: 'Namespaced',
  clusters: ['local'],
  defaultDuration: '1h',
  maxDuration: '4h',
  reasonRequired: false,
  activation: 'at-once',
  canRequest: true,
  canApprove: false,
  ...over,
});

const grant = (profileName, over = {}) => ({
  namespace: 'team-payments',
  name: `grant-${profileName}`,
  request: `req-${profileName}`,
  requester: me,
  profile: profileName,
  role: { kind: 'ClusterRole', name: 'view' },
  scope: 'Namespaced',
  clusters: ['local'],
  startedAt: '2026-11-24T12:00:00Z',
  expiresAt: '2026-11-24T13:00:00Z',
  phase: 'Active',
  remainingSeconds: 1800,
  actions: { revoke: true },
  ...over,
});

const request = (profileName, over = {}) => ({
  namespace: 'team-payments',
  name: `req-${profileName}`,
  requester: me,
  profile: profileName,
  reason: '',
  duration: '1h0m0s',
  phase: 'Pending',
  createdAt: '2026-11-24T12:00:00Z',
  actions: { approve: false, reject: false, cancel: true },
  ...over,
});

test('my live grants are my active ones, the soonest to end first', () => {
  const grants = [
    grant('late', { expiresAt: '2026-11-24T15:00:00Z' }),
    grant('other', { requester: { username: 'bob', email: 'bob@company.com' } }),
    grant('ended', { phase: 'Expired' }),
    grant('soon', { expiresAt: '2026-11-24T12:30:00Z' }),
  ];
  expect(myLiveGrants(grants, me).map((g) => g.profile)).toEqual(['soon', 'late']);
});

test('activatable profiles: requestable only, alphabetical, held ones Active, asked ones Waiting', () => {
  const profiles = [
    profile('zulu'),
    profile('held'),
    profile('asked', { activation: 'approval', approverGroups: ['sre'] }),
    profile('locked', { canRequest: false }),
    profile('mixed', { activation: 'mixed', approverGroups: ['sre'] }),
    profile('manual', { activation: 'approval' }),
    profile('named', { activation: 'approval', approverUsers: ['bob@company.com'] }),
  ];
  const states = activatable(profiles, [grant('held')], [request('asked')]).map((a) => [a.profile.name, a.state]);
  expect(states).toEqual([
    ['asked', 'waiting'],
    ['held', 'active'],
    ['manual', 'approval'],
    ['mixed', 'approval'],
    ['named', 'approval'],
    ['zulu', 'activate'],
  ]);
});

test('a request that is no longer pending does not make a profile Waiting', () => {
  const [only] = activatable([profile('p')], [], [request('p', { phase: 'Rejected' })]);
  expect(only.state).toBe('activate');
});

test('activation reads from the profile: the control, the line under the name and its title', () => {
  const cases = [
    [{ activation: 'at-once' }, { mode: 'at-once', control: 'Activate access', line: 'Granted at once' }],
    [
      { activation: 'mixed', approverGroups: ['sre'] },
      {
        mode: 'mixed',
        control: 'Request approval',
        line: 'Approval on some clusters',
        title: 'Granted at once on some clusters, after approval on others',
      },
    ],
    [{ activation: 'approval' }, { mode: 'approval', control: 'Request approval', line: 'Needs approval' }],
    [
      { activation: 'approval', approverUsers: ['bob@company.com'] },
      { mode: 'approval', control: 'Request approval', line: 'Approval by bob@company.com', title: 'bob@company.com' },
    ],
    [
      { activation: 'approval', approverGroups: ['sre'], approverUsers: ['bob@company.com'] },
      { mode: 'approval', control: 'Request approval', line: 'Approval by sre, bob@company.com', title: 'sre, bob@company.com' },
    ],
    [
      { activation: 'approval', approverGroups: ['sre', 'dba'], approverUsers: ['bob@company.com', 'eve@company.com'] },
      {
        mode: 'approval',
        control: 'Request approval',
        line: 'Approval by sre, dba +2 more',
        title: 'sre, dba, bob@company.com, eve@company.com',
      },
    ],
    [{ activation: 'sometimes' }, { mode: 'approval', control: 'Request approval', line: 'Needs approval' }],
    [{ activation: '' }, { mode: 'approval', control: 'Request approval', line: 'Needs approval' }],
  ];
  for (const [over, want] of cases) expect(describeActivation(profile('p', over))).toEqual(want);
});

test('the For you summary names what waits and what is held, and says so when nothing is', () => {
  expect(forYouSummary({ deciding: 3, live: 2, waiting: 0 })).toBe('3 requests wait for your decision. You hold 2 grants.');
  expect(forYouSummary({ deciding: 1, live: 0, waiting: 1 })).toBe('1 request waits for your decision. 1 of your requests is waiting for approval.');
  expect(forYouSummary({ deciding: 0, live: 1, waiting: 2 })).toBe('You hold 1 grant. 2 of your requests are waiting for approval.');
  expect(forYouSummary({ deciding: 0, live: 0, waiting: 0 })).toBe('Nothing needs you right now.');
});
