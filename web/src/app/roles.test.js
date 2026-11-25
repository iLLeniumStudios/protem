// Run: cd web && bun test src/app
import { expect, test } from 'bun:test';
import { canOpen, canViewFleet, isAdmin, isApprover, isMe, navFor, pageTitle, REDIRECTS, roleLabel, workspaceOf } from './roles';

const caps = (over = {}) => ({ management: false, profileNamespaces: [], manageClusters: false, viewFleet: false, viewAll: false, ...over });
const viewer = (capabilities, ...canApprove) => ({ capabilities, profiles: canApprove.map((c) => ({ canApprove: c })) });

test('admin is whoever may read every request and grant', () => {
  expect(isAdmin(viewer(caps({ viewAll: true })))).toBe(true);
  expect(isAdmin(viewer(caps({ viewFleet: true, manageClusters: true }), true))).toBe(false);
  expect(isAdmin(viewer(null))).toBe(false);
});

test('approver is an admin or anyone who may approve at least one profile', () => {
  expect(isApprover(viewer(caps({ viewAll: true })))).toBe(true);
  expect(isApprover(viewer(caps(), false, true))).toBe(true);
  expect(isApprover(viewer(caps(), false, false))).toBe(false);
  expect(isApprover(viewer(null))).toBe(false);
});

test('fleet is visible exactly when the caller may list target clusters', () => {
  expect(canViewFleet(viewer(caps({ viewFleet: true })))).toBe(true);
  expect(canViewFleet(viewer(caps({ viewAll: true }), true))).toBe(false);
  expect(canViewFleet(viewer(null))).toBe(false);
});

test('role label names the highest role', () => {
  expect(roleLabel(viewer(caps({ viewAll: true }), true))).toBe('Admin');
  expect(roleLabel(viewer(caps(), true))).toBe('Approver');
  expect(roleLabel(viewer(caps({ viewFleet: true }), false))).toBe('Requester');
});

test('a requester is me when the username or the email matches, never on two empty values', () => {
  const me = { username: 'alice', email: 'alice@company.com' };
  expect(isMe(me, { username: 'alice', email: '' })).toBe(true);
  expect(isMe(me, { username: '', email: 'alice@company.com' })).toBe(true);
  expect(isMe(me, { username: 'bob', email: 'bob@company.com' })).toBe(false);
  expect(isMe({ username: '', email: '' }, { username: '', email: '' })).toBe(false);
  expect(isMe(null, { username: 'alice', email: 'alice@company.com' })).toBe(false);
  expect(isMe({ username: '1163', email: 'alice@company.com' }, { username: 'alice@company.com', email: '' })).toBe(true);
  expect(isMe({ username: '1163', email: 'alice@company.com' }, { username: 'alice@company.com', email: 'other@company.com' })).toBe(false);
});

test('the five administration paths and their children belong to Administration, everything else to My access', () => {
  for (const p of ['/admin', '/timeline', '/profiles', '/profiles/ns/a/edit', '/profiles/new', '/clusters', '/audit', '/audit/']) {
    expect(workspaceOf(p)).toBe('admin');
  }
  for (const p of ['/', '/request', '/request/ns/p', '/requests', '/requests/ns/r', '/grants/ns/g', '/approvals', '/administrator', '/nowhere']) {
    expect(workspaceOf(p)).toBe('access');
  }
});

const paths = (ws, v) => navFor(ws, v).map((i) => i.to);

test('My access shows Approvals only to approvers, in the fixed order', () => {
  expect(paths('access', viewer(caps(), false))).toEqual(['/', '/request', '/requests']);
  expect(paths('access', viewer(caps(), true))).toEqual(['/', '/request', '/requests', '/approvals']);
  expect(navFor('access', viewer(caps(), true)).find((i) => i.to === '/approvals').badge).toBe(true);
});

test('Administration items follow capabilities, never roles', () => {
  expect(paths('admin', viewer(caps(), true))).toEqual([]);
  expect(paths('admin', viewer(null))).toEqual([]);
  expect(paths('admin', viewer(caps({ viewFleet: true })))).toEqual(['/clusters']);
  expect(paths('admin', viewer(caps({ management: true, profileNamespaces: ['team-a'] })))).toEqual(['/profiles']);
  expect(paths('admin', viewer(caps({ viewAll: true, viewFleet: true })))).toEqual([
    '/admin',
    '/timeline',
    '/profiles',
    '/clusters',
    '/audit',
  ]);
});

test('old paths redirect to a page the navigation still offers', () => {
  expect(REDIRECTS).toEqual({ '/me': '/requests', '/fleet': '/clusters', '/overview': '/admin' });
  const all = viewer(caps({ viewAll: true, viewFleet: true }), true);
  const offered = [...paths('access', all), ...paths('admin', all)];
  for (const to of Object.values(REDIRECTS)) expect(offered).toContain(to);
});

test('page titles follow the navigation labels', () => {
  expect(pageTitle('/')).toBe('For you · Protem');
  expect(pageTitle('/request')).toBe('Activate access · Protem');
  expect(pageTitle('/request/ns/p')).toBe('New request · Protem');
  expect(pageTitle('/requests')).toBe('My requests · Protem');
  expect(pageTitle('/requests/ns/r')).toBe('Request · Protem');
  expect(pageTitle('/admin')).toBe('Overview · Protem');
  expect(pageTitle('/timeline')).toBe('Access timeline · Protem');
  expect(pageTitle('/clusters')).toBe('Clusters · Protem');
  expect(pageTitle('/profiles/ns/p/edit')).toBe('Editing profile · Protem');
  expect(pageTitle('/nowhere')).toBe('Page not found · Protem');
});

test('a page opens by address only for someone its navigation item shows to; other paths open as before', () => {
  const requester = viewer(caps(), false);
  for (const p of ['/admin', '/timeline', '/audit', '/clusters', '/clusters/', '/approvals', '/profiles']) expect(canOpen(p, requester)).toBe(false);
  for (const p of ['/', '/request', '/request/ns/p', '/requests', '/requests/ns/x', '/grants/ns/x', '/nope']) expect(canOpen(p, requester)).toBe(true);

  const approver = viewer(caps(), true);
  expect(canOpen('/approvals', approver)).toBe(true);
  expect(canOpen('/admin', approver)).toBe(false);

  const admin = viewer(caps({ viewAll: true, viewFleet: true }));
  for (const p of ['/admin', '/timeline', '/audit', '/clusters', '/approvals', '/profiles', '/profiles/ns/name']) expect(canOpen(p, admin)).toBe(true);

  expect(canOpen('/profiles/ns/name', requester)).toBe(false);
  expect(canOpen('/profiles/ns/name', viewer(caps({ management: true, profileNamespaces: ['ns'] })))).toBe(true);
});
