import { Children, type ReactNode } from 'react';
import type { LucideIcon } from 'lucide-react';
import { cn } from './cn';

export interface PanelProps {
  title?: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
  children: ReactNode;
  as?: 'section' | 'div' | 'article' | 'aside';
  className?: string;
}

export function Panel({ title, description, actions, children, as: Tag = 'section', className }: PanelProps) {
  const hasHeader = title || description || actions;
  return (
    <Tag className={cn('min-w-0 rounded-lg border border-border bg-surface', className)}>
      {hasHeader && (
        <div className="flex flex-col gap-3 border-b border-border px-4 py-4 sm:flex-row sm:items-start sm:justify-between sm:px-6">
          <div className="flex min-w-0 flex-col gap-1">
            {title && <h2 className="text-title text-text">{title}</h2>}
            {description && <p className="max-w-prose text-body text-text-secondary">{description}</p>}
          </div>
          {actions && <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div>}
        </div>
      )}
      <div className="p-4 sm:p-6">{children}</div>
    </Tag>
  );
}

/** One row of a `divide-y` list that stands in for a table's row at narrow widths. */
export function MobileRow({ children, className }: { children: ReactNode; className?: string }) {
  return <li className={cn('flex flex-col gap-3 py-4 first:pt-0 last:pb-0', className)}>{children}</li>;
}

/** A `divide-y` list of `MobileRow`s inside its own card, for a section that is not a full `Panel`. */
export function CardList({ label, children, className }: { label: string; children: ReactNode; className?: string }) {
  return (
    <ul aria-label={label} className={cn('flex flex-col divide-y divide-border rounded-lg border border-border bg-surface px-4 sm:px-6', className)}>
      {children}
    </ul>
  );
}

export interface PageHeaderProps {
  title: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
}

export function PageHeader({ title, description, actions }: PageHeaderProps) {
  return (
    <header className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
      <div className="flex min-w-0 flex-col gap-0.5">
        <h1 className="break-words text-page text-text">{title}</h1>
        {description && <p className="max-w-prose text-body text-text-secondary">{description}</p>}
      </div>
      {actions && <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div>}
    </header>
  );
}

export interface SectionProps {
  id: string;
  title: ReactNode;
  /** A quiet figure beside the heading: a count, or a short phrase such as "1 waiting". */
  count?: ReactNode;
  /** A link at the heading's far end. */
  action?: ReactNode;
  children: ReactNode;
  className?: string;
}

/** A titled block of a page: heading, quiet count, optional link, then its content. */
export function Section({ id, title, count, action, children, className }: SectionProps) {
  return (
    <section aria-labelledby={id} className={cn('flex min-w-0 flex-col gap-2.5', className)}>
      <div className="flex min-w-0 flex-wrap items-baseline gap-x-2">
        <h2 id={id} className="text-title text-text">
          {title}
        </h2>
        {count !== undefined && <span className="text-label tabular-nums text-text-tertiary">{count}</span>}
        {action && <div className="ml-auto text-label">{action}</div>}
      </div>
      {children}
    </section>
  );
}

const rowListClass = 'flex min-w-0 flex-col divide-y divide-border overflow-hidden rounded-lg border border-border bg-surface';

/** A card of rows divided by rules; each child is a `Row`. `footer`, such as a "show all" link, closes the card. */
export function RowList({ label, children, footer, className }: { label: string; children: ReactNode; footer?: ReactNode; className?: string }) {
  if (!footer) {
    return (
      <ul aria-label={label} className={cn(rowListClass, className)}>
        {children}
      </ul>
    );
  }
  return (
    <div className={cn(rowListClass, className)}>
      <ul aria-label={label} className="flex min-w-0 flex-col divide-y divide-border">
        {children}
      </ul>
      {footer}
    </div>
  );
}

/** One row of a `RowList`; `selected` marks the row whose detail is open beside the list. */
export function Row({ selected, children, className, ...rest }: { selected?: boolean; children: ReactNode; className?: string } & Record<`data-${string}`, string>) {
  return (
    <li
      {...rest}
      aria-current={selected || undefined}
      className={cn(
        'relative min-w-0 px-4 py-3.5',
        selected && 'bg-accent-soft before:absolute before:inset-y-0 before:left-0 before:w-1 before:bg-accent',
        className,
      )}
    >
      {children}
    </li>
  );
}

/** What a section says when it has nothing to show: one line, not an empty card. */
export function QuietLine({ children }: { children: ReactNode }) {
  return <p className="text-body text-text-secondary">{children}</p>;
}

export interface EmptyStateProps {
  icon: LucideIcon;
  title: ReactNode;
  body?: ReactNode;
  action?: ReactNode;
  className?: string;
}

export function EmptyState({ icon: Icon, title, body, action, className }: EmptyStateProps) {
  return (
    <div className={cn('flex flex-col items-center justify-center gap-3 px-4 py-12 text-center', className)}>
      <Icon className="h-5 w-5 text-text-tertiary" aria-hidden />
      <div className="flex max-w-prose flex-col gap-1">
        <p className="text-title text-text">{title}</p>
        {body && <p className="text-body text-text-secondary">{body}</p>}
      </div>
      {action && <div className="mt-2 flex flex-wrap justify-center gap-2">{action}</div>}
    </div>
  );
}

export function Skeleton({ className }: { className?: string }) {
  return <div aria-hidden className={cn('h-4 animate-pulse rounded-sm bg-muted motion-reduce:animate-none', className)} />;
}

export function Identifier({ children, className }: { children: ReactNode; className?: string }) {
  const text = Children.toArray(children)
    .filter((c) => typeof c === 'string' || typeof c === 'number')
    .join('');
  return (
    <span title={text || undefined} className={cn('inline-block max-w-full truncate align-bottom font-mono', className)}>
      {children}
    </span>
  );
}
