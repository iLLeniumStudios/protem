// Run: cd web && bun test src/app
import { expect, test } from 'bun:test';
import { clampPage, lastPage, pageFor, rangeText, slicePage, toQuery } from './paging';

test('the range reads "first–last of total", 1-based, and is empty with no items', () => {
  expect(rangeText(0, 25, 312)).toBe('1–25 of 312');
  expect(rangeText(12, 25, 312)).toBe('301–312 of 312');
  expect(rangeText(0, 25, 7)).toBe('1–7 of 7');
  expect(rangeText(0, 25, 0)).toBe('0 of 0');
});

test('the last page index covers a partial last page and is 0 when empty', () => {
  expect(lastPage(312, 25)).toBe(12);
  expect(lastPage(300, 25)).toBe(11);
  expect(lastPage(1, 25)).toBe(0);
  expect(lastPage(0, 25)).toBe(0);
});

test('a page past the end steps back to the last page that has rows', () => {
  expect(clampPage(12, 300, 25)).toBe(11);
  expect(clampPage(3, 26, 25)).toBe(1);
  expect(clampPage(2, 0, 25)).toBe(0);
  expect(clampPage(4, 312, 25)).toBe(4);
  expect(clampPage(-1, 312, 25)).toBe(0);
});

test('a new filter returns to the first page; the same filter keeps the page', () => {
  expect(pageFor({ key: 'event=A', page: 3 }, 'event=A')).toBe(3);
  expect(pageFor({ key: 'event=A', page: 3 }, 'event=B')).toBe(0);
});

test('client paging slices the requested page', () => {
  const items = Array.from({ length: 45 }, (_, i) => i);
  expect(slicePage(items, 0, 20)).toEqual(items.slice(0, 20));
  expect(slicePage(items, 2, 20)).toEqual([40, 41, 42, 43, 44]);
});

test('query strings leave out empty values and encode the rest', () => {
  expect(toQuery({ state: 'open', mine: true, approver: false, q: '', cluster: undefined, offset: 0, limit: 10 })).toBe(
    '?state=open&mine=true&offset=0&limit=10',
  );
  expect(toQuery({ q: 'a&b c' })).toBe('?q=a%26b+c');
  expect(toQuery({})).toBe('');
});
