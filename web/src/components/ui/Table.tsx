import type { HTMLAttributes, ReactNode, TdHTMLAttributes, ThHTMLAttributes } from 'react';
import { ArrowDown, ArrowUp, ArrowUpDown } from 'lucide-react';
import { cn, colorTransition, focusRing } from './cn';

export interface TableProps extends HTMLAttributes<HTMLTableElement> {
  /** Names the scroll region; also makes it keyboard-scrollable. */
  label?: string;
}

export function Table({ label, className, children, ...rest }: TableProps) {
  return (
    <div
      role={label ? 'region' : undefined}
      aria-label={label}
      tabIndex={label ? 0 : undefined}
      className={cn('w-full min-w-0 overflow-x-auto rounded-md', focusRing)}
    >
      <table className={cn('w-full border-collapse text-left text-body tabular-nums', className)} {...rest}>
        {children}
      </table>
    </div>
  );
}

export function THead({ className, ...rest }: HTMLAttributes<HTMLTableSectionElement>) {
  return <thead className={cn('bg-subtle', className)} {...rest} />;
}

export function TBody(props: HTMLAttributes<HTMLTableSectionElement>) {
  return <tbody {...props} />;
}

export function TR({ className, ...rest }: HTMLAttributes<HTMLTableRowElement>) {
  return <tr className={cn('border-b border-border last:border-b-0', className)} {...rest} />;
}

export type SortDirection = 'asc' | 'desc' | 'none';

export interface THProps extends ThHTMLAttributes<HTMLTableCellElement> {
  sort?: SortDirection;
  onSort?: () => void;
  children?: ReactNode;
}

const ARIA_SORT = { asc: 'ascending', desc: 'descending', none: 'none' } as const;
const SORT_ICON = { asc: ArrowUp, desc: ArrowDown, none: ArrowUpDown };

export function TH({ sort, onSort, className, children, scope = 'col', ...rest }: THProps) {
  const Icon = sort ? SORT_ICON[sort] : null;
  return (
    <th
      scope={scope}
      aria-sort={sort ? ARIA_SORT[sort] : undefined}
      className={cn('whitespace-nowrap px-4 py-3 text-label text-text-secondary', className)}
      {...rest}
    >
      {sort && onSort && Icon ? (
        <button
          type="button"
          onClick={onSort}
          className={cn(
            '-mx-1 inline-flex min-h-6 items-center gap-1 rounded-sm px-1 hover:text-text coarse:min-h-11',
            colorTransition,
            focusRing,
          )}
        >
          {children}
          <Icon className="h-4 w-4 shrink-0" aria-hidden />
        </button>
      ) : (
        children
      )}
    </th>
  );
}

export function TD({ className, ...rest }: TdHTMLAttributes<HTMLTableCellElement>) {
  return <td className={cn('px-4 py-3 align-top text-text', className)} {...rest} />;
}
