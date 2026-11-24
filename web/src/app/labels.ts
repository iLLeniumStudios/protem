export interface LabelRow {
  key: string;
  value: string;
}

export function labelRows(labels: Record<string, string> | null | undefined): LabelRow[] {
  return Object.entries(labels ?? {}).map(([key, value]) => ({ key, value }));
}

/** Rows as a label map, skipping blank rows; `error` is set when a row has a value but no key, or a key repeats. */
export function rowsToLabels(rows: LabelRow[]): { labels: Record<string, string>; error?: string } {
  const labels = new Map<string, string>();
  let error: string | undefined;
  for (const row of rows) {
    const key = row.key.trim();
    const value = row.value.trim();
    if (!key) {
      if (value) error ??= 'Each label needs a key. Enter one or remove the row.';
      continue;
    }
    if (labels.has(key)) error ??= `The key ${key} appears more than once.`;
    labels.set(key, value);
  }
  return { labels: Object.fromEntries(labels), ...(error && { error }) };
}
