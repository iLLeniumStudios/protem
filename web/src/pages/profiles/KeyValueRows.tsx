import type { ReactNode } from 'react';
import { CircleAlert, Plus, Trash2 } from 'lucide-react';
import type { LabelRow } from '../../app/labels';
import { Button, IconButton, Input, cn } from '../../components/ui';
import { focusRing } from '../../components/ui/cn';

export interface KeyValueRowsProps {
  id: string;
  legend: string;
  hint?: ReactNode;
  rows: LabelRow[];
  onChange: (rows: LabelRow[]) => void;
  error?: string;
  /** Shown under the editable rows, read-only. */
  children?: ReactNode;
}

/** Label key/value rows; the fieldset takes focus when it holds the first error. */
export function KeyValueRows({ id, legend, hint, rows, onChange, error, children }: KeyValueRowsProps) {
  const describedBy = [hint && `${id}-hint`, error && `${id}-error`].filter(Boolean).join(' ') || undefined;
  const update = (i: number, patch: Partial<LabelRow>) => onChange(rows.map((r, j) => (j === i ? { ...r, ...patch } : r)));
  return (
    <fieldset
      id={id}
      tabIndex={-1}
      data-invalid={error ? 'true' : undefined}
      aria-describedby={describedBy}
      className={cn('flex min-w-0 flex-col gap-2 rounded-sm', focusRing)}
    >
      <legend className="text-label text-text">{legend}</legend>
      {hint && (
        <p id={`${id}-hint`} className="text-caption text-text-secondary">
          {hint}
        </p>
      )}
      {rows.length > 0 && (
        <ul className="flex flex-col gap-2">
          {rows.map((row, i) => (
            <li key={i} className="flex min-w-0 items-center gap-2">
              <Input
                aria-label={`Key, row ${i + 1}`}
                placeholder="key"
                value={row.key}
                autoComplete="off"
                spellCheck={false}
                aria-invalid={Boolean(error) || undefined}
                mono
                onChange={(e) => update(i, { key: e.target.value })}
              />
              <span aria-hidden className="text-text-secondary">
                =
              </span>
              <Input
                aria-label={`Value, row ${i + 1}`}
                placeholder="value"
                value={row.value}
                autoComplete="off"
                spellCheck={false}
                mono
                onChange={(e) => update(i, { value: e.target.value })}
              />
              <IconButton
                label={`Remove label ${row.key.trim() || `row ${i + 1}`}`}
                icon={Trash2}
                className="shrink-0"
                onClick={() => onChange(rows.filter((_, j) => j !== i))}
              />
            </li>
          ))}
        </ul>
      )}
      <Button size="sm" icon={Plus} className="self-start" onClick={() => onChange([...rows, { key: '', value: '' }])}>
        Add label
      </Button>
      {children}
      {error && (
        <p id={`${id}-error`} className="flex items-start gap-1 text-caption text-danger-text">
          <CircleAlert className="mt-px h-4 w-4 shrink-0" aria-hidden />
          <span>{error}</span>
        </p>
      )}
    </fieldset>
  );
}
