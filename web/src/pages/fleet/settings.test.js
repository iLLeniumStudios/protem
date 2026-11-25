// Run: cd web && bun test src/pages/fleet
import { expect, test } from 'bun:test';
import { boundAs, settingsChange, settingsForm } from './settings';

const cluster = (over = {}) => ({
  name: 'prod-eu',
  displayName: 'prod-eu',
  phase: 'Ready',
  ready: true,
  activeGrantsCount: 0,
  paused: false,
  subject: 'oidc:{{.Email}}',
  labels: { env: 'prod', 'app.kubernetes.io/managed-by': 'Helm', 'protem.dev/region': 'eu' },
  ...over,
});

test('the form edits only labels outside the reserved prefixes and shows the rest apart', () => {
  const form = settingsForm(cluster());
  expect(form.labels).toEqual([{ key: 'env', value: 'prod' }]);
  expect(form.reserved).toEqual([
    ['app.kubernetes.io/managed-by', 'Helm'],
    ['protem.dev/region', 'eu'],
  ]);
});

test('nothing changed sends nothing', () => {
  const c = cluster();
  expect(settingsChange(c, settingsForm(c))).toEqual({ payload: {}, errors: {} });
  const bare = cluster({ subject: undefined, labels: undefined });
  expect(settingsChange(bare, settingsForm(bare))).toEqual({ payload: {}, errors: {} });
});

test('only the fields the user changed are sent', () => {
  const c = cluster();
  expect(settingsChange(c, { ...settingsForm(c), paused: true }).payload).toEqual({ paused: true });
  expect(settingsChange(c, { ...settingsForm(c), subject: ' {{.Username}} ' }).payload).toEqual({ subject: '{{.Username}}' });
  const labels = [
    { key: 'env', value: 'staging' },
    { key: '', value: '' },
  ];
  expect(settingsChange(c, { ...settingsForm(c), labels }).payload).toEqual({ labels: { env: 'staging' } });
  expect(settingsChange(c, { ...settingsForm(c), labels: [] }).payload).toEqual({ labels: {} });
});

test('reserved labels are never sent; typing one is an error', () => {
  const c = cluster();
  const form = { ...settingsForm(c), labels: [{ key: 'env', value: 'prod' }, { key: 'argocd.argoproj.io/instance', value: 'x' }] };
  const out = settingsChange(c, form);
  expect(Object.keys(out.errors)).toEqual(['labels']);
  expect(JSON.stringify(out.payload)).not.toContain('argocd.argoproj.io/');
  expect(JSON.stringify(settingsChange(c, { ...settingsForm(c), labels: [{ key: 'team', value: 'a' }] }).payload)).not.toContain(
    'protem.dev/',
  );
});

test('a label row needs a key, and keys are unique', () => {
  const c = cluster();
  expect(Object.keys(settingsChange(c, { ...settingsForm(c), labels: [{ key: '', value: 'x' }] }).errors)).toEqual(['labels']);
  const dup = [
    { key: 'env', value: 'a' },
    { key: 'env', value: 'b' },
  ];
  expect(Object.keys(settingsChange(c, { ...settingsForm(c), labels: dup }).errors)).toEqual(['labels']);
});

test('a non-empty subject without a user placeholder is rejected', () => {
  const c = cluster();
  const out = settingsChange(c, { ...settingsForm(c), subject: 'admin' });
  expect(Object.keys(out.errors)).toEqual(['subject']);
  expect(out.payload).toEqual({});
  expect(settingsChange(c, { ...settingsForm(c), subject: 'u:{{.Email}}' }).errors).toEqual({});
});

test('clearing a subject sends an empty one, meaning the hub default', () => {
  const c = cluster();
  expect(settingsChange(c, { ...settingsForm(c), subject: '  ' })).toEqual({ payload: { subject: '' }, errors: {} });
  const unset = cluster({ subject: undefined });
  expect(settingsChange(unset, { ...settingsForm(unset), subject: '' })).toEqual({ payload: {}, errors: {} });
});

test('the hub cluster never sends a subject, whatever the form holds', () => {
  const hub = cluster({ agent: 'none' });
  const form = { ...settingsForm(hub), subject: '{{.Username}}', paused: true };
  expect(settingsChange(hub, form)).toEqual({ payload: { paused: true }, errors: {} });
  expect(settingsChange(hub, { ...settingsForm(hub), subject: 'admin' })).toEqual({ payload: {}, errors: {} });
  expect(settingsChange(cluster({ agent: 'connected' }), form).payload).toEqual({ paused: true, subject: '{{.Username}}' });
});

test('the bound name follows the server: empty is the email, placeholders filled, literal text kept', () => {
  const alice = { email: 'alice@company.com', username: 'alice' };
  expect(boundAs('', alice)).toBe('alice@company.com');
  expect(boundAs('oidc:{{.Email}}', alice)).toBe('oidc:alice@company.com');
  expect(boundAs('{{.Username}}', alice)).toBe('alice');
  expect(boundAs('aks:{{.Username}}', { email: 'bob@company.com', username: 'bob' })).toBe('aks:bob');
  expect(boundAs('{{.Email}}', { email: '', username: 'alice' })).toBe('alice');
  expect(boundAs('static', alice)).toBe('alice@company.com');
});
