// Run: cd web && bun test src/pages/profiles
import { expect, test } from 'bun:test';
import { canCreateProfile, canEditCluster, canEditProfile, canSeeProfiles } from './manage';

const caps = (over = {}) => ({ management: true, profileNamespaces: [], manageClusters: false, viewFleet: false, viewAll: false, ...over });
const profile = (namespace, managedBy) => ({ namespace, name: 'p', managedBy });
const cluster = (managedBy) => ({ name: 'c', managedBy });

test('a profile is editable with management on, no GitOps owner, and write access to its namespace', () => {
  expect(canEditProfile(caps({ profileNamespaces: ['team-a'] }), profile('team-a'))).toBe(true);
  expect(canEditProfile(caps({ profileNamespaces: ['*'] }), profile('team-b', ''))).toBe(true);
  expect(canEditProfile(caps({ profileNamespaces: ['team-a'] }), profile('team-b'))).toBe(false);
  expect(canEditProfile(caps({ profileNamespaces: ['*'] }), profile('team-a', 'Argo CD'))).toBe(false);
  expect(canEditProfile(caps({ profileNamespaces: [] }), profile('team-a'))).toBe(false);
  expect(canEditProfile(null, profile('team-a'))).toBe(false);
});

test('creating a profile needs management on and at least one writable namespace', () => {
  expect(canCreateProfile(caps({ profileNamespaces: ['team-a'] }))).toBe(true);
  expect(canCreateProfile(caps({ profileNamespaces: ['*'] }))).toBe(true);
  expect(canCreateProfile(caps({ profileNamespaces: [] }))).toBe(false);
  expect(canCreateProfile(caps({ profileNamespaces: null }))).toBe(false);
  expect(canCreateProfile(null)).toBe(false);
});

test('a cluster is editable with management on, cluster write access, and no GitOps owner', () => {
  expect(canEditCluster(caps({ manageClusters: true }), cluster())).toBe(true);
  expect(canEditCluster(caps({ manageClusters: true }), cluster('Flux'))).toBe(false);
  expect(canEditCluster(caps({ manageClusters: false }), cluster())).toBe(false);
  expect(canEditCluster(null, cluster())).toBe(false);
});

test('the profiles page is for people who may create profiles or read everything', () => {
  expect(canSeeProfiles(caps({ profileNamespaces: ['team-a'] }))).toBe(true);
  expect(canSeeProfiles(caps({ management: false, viewAll: true }))).toBe(true);
  expect(canSeeProfiles(caps({ management: false, profileNamespaces: ['*'] }))).toBe(false);
  expect(canSeeProfiles(null)).toBe(false);
});

test('with management off, no create or edit control is ever offered, whatever else the caller may do', () => {
  const bools = [false, true];
  const namespaceSets = [[], ['*'], ['team-a'], ['team-a', '*']];
  const owners = [undefined, '', 'Argo CD'];
  for (const manageClusters of bools)
    for (const viewFleet of bools)
      for (const viewAll of bools)
        for (const profileNamespaces of namespaceSets) {
          const c = caps({ management: false, manageClusters, viewFleet, viewAll, profileNamespaces });
          expect(canCreateProfile(c)).toBe(false);
          for (const managedBy of owners) {
            expect(canEditProfile(c, profile('team-a', managedBy))).toBe(false);
            expect(canEditCluster(c, cluster(managedBy))).toBe(false);
          }
        }
});
