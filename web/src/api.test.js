// Run: cd web && bun test src/api.test.js
import { afterEach, expect, test } from 'bun:test';
import { ApiClient } from './api';

const realFetch = globalThis.fetch;
globalThis.localStorage ??= { getItem: () => null, setItem: () => {} };

const respond = (status, body) => {
  globalThis.fetch = async () => new Response(JSON.stringify(body), { status });
};

afterEach(() => {
  globalThis.fetch = realFetch;
});

test('a 401 signs the client out with the server message and rejects the call', async () => {
  const client = new ApiClient();
  const heard = [];
  client.onSignedOut((message) => heard.push(message));
  respond(401, { error: 'id token expired' });

  const err = await client.getMe().catch((e) => e);

  expect(err.status).toBe(401);
  expect(err.message).toBe('id token expired');
  expect(heard).toEqual(['id token expired']);
});

test('other failures keep the client signed in', async () => {
  const client = new ApiClient();
  const heard = [];
  client.onSignedOut((message) => heard.push(message));
  respond(403, { error: 'forbidden' });

  const err = await client.getGrantPermissions('team-a', 'g1').catch((e) => e);

  expect([err.status, err.message]).toEqual([403, 'forbidden']);
  expect(heard).toEqual([]);
});

test('a listener stops hearing once it unsubscribes', async () => {
  const client = new ApiClient();
  const heard = [];
  client.onSignedOut((message) => heard.push(message))();
  respond(401, {});

  await client.getCapabilities().catch(() => {});

  expect(heard).toEqual([]);
});

test('a delete answered 204 with no body resolves', async () => {
  const client = new ApiClient();
  globalThis.fetch = async () => new Response(null, { status: 204 });

  await expect(client.deleteProfile('team-a', 'p1')).resolves.toBeUndefined();
});
