import { forwardRef, useId, useRef, type ButtonHTMLAttributes, type ReactNode } from 'react';
import * as DropdownMenu from '@radix-ui/react-dropdown-menu';
import { Check, ChevronDown, ChevronLeft, ChevronRight, MoreHorizontal, Search, type LucideIcon } from 'lucide-react';
import { lastPage } from '../../app/paging';
import { Button } from './Button';
import { cn, colorTransition, focusRing } from './cn';

/** A row of filters above a console table: a search field, filter chips, then anything pushed to the end. */
export function ConsoleToolbar({ label, children, className }: { label: string; children: ReactNode; className?: string }) {
  return (
    <div role="search" aria-label={label} className={cn('flex flex-wrap items-center gap-2', className)}>
      {children}
    </div>
  );
}

/** The card a console table, list or timeline sits in: no padding, so rows run to its edges. */
export function ConsoleCard({ label, children, className }: { label: string; children: ReactNode; className?: string }) {
  return (
    <section aria-label={label} className={cn('min-w-0 overflow-clip rounded-lg border border-border bg-surface', className)}>
      {children}
    </section>
  );
}

export interface TextButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  /** `accent` reads as a link ("Show 3 more"); `plain` as a heading that toggles. */
  tone?: 'accent' | 'plain';
}

/** A button that reads as text inside dense rows, where a full button would not fit. */
export const TextButton = forwardRef<HTMLButtonElement, TextButtonProps>(function TextButton(
  { tone = 'accent', className, type = 'button', ...rest },
  ref,
) {
  return (
    <button
      ref={ref}
      type={type}
      className={cn(
        'inline-flex shrink-0 items-center gap-1 rounded-sm',
        tone === 'accent' ? 'font-semibold text-accent-text hover:underline' : 'px-1 text-text hover:bg-muted',
        colorTransition,
        focusRing,
        className,
      )}
      {...rest}
    />
  );
});

export interface SearchFieldProps {
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  className?: string;
}

export function SearchField({ label, value, onChange, placeholder, className }: SearchFieldProps) {
  const id = useId();
  return (
    <div
      className={cn(
        'flex h-8 w-full min-w-0 items-center gap-2 rounded-md border border-text-tertiary bg-surface px-2.5 text-text-secondary coarse:h-11 sm:w-80',
        'hover:border-text-secondary focus-within:outline focus-within:outline-2 focus-within:outline-offset-1 focus-within:outline-focus',
        colorTransition,
        className,
      )}
    >
      <Search className="h-4 w-4 shrink-0" aria-hidden />
      <label htmlFor={id} className="sr-only">
        {label}
      </label>
      <input
        id={id}
        type="search"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        autoComplete="off"
        spellCheck={false}
        className="h-full min-w-0 flex-1 bg-transparent text-label font-normal text-text outline-none placeholder:text-text-tertiary"
      />
    </div>
  );
}

const menuContent = cn(
  'z-50 min-w-48 max-w-[calc(100vw-2rem)] overflow-y-auto rounded-md border border-border-strong bg-surface p-1 text-text shadow-lg',
  'max-h-[var(--radix-dropdown-menu-content-available-height)]',
);

const menuItem = cn(
  'relative flex min-h-9 cursor-default select-none items-center gap-2 rounded-sm px-3 py-2 text-body coarse:min-h-11',
  colorTransition,
  focusRing,
  'data-[highlighted]:bg-accent-soft data-[highlighted]:text-accent-text',
);

export interface FilterOption {
  value: string;
  label: string;
}

export interface FilterChipProps {
  label: string;
  /** '' is "any". */
  value: string;
  options: FilterOption[];
  onValueChange: (value: string) => void;
  anyLabel?: string;
}

/** A compact filter: its name and current choice; the choices open in a menu. */
export function FilterChip({ label, value, options, onValueChange, anyLabel = 'Any' }: FilterChipProps) {
  const current = options.find((o) => o.value === value)?.label ?? anyLabel;
  const set = value !== '';
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button
          type="button"
          className={cn(
            'inline-flex h-8 max-w-full shrink-0 items-center gap-1.5 rounded-full border pl-3 pr-2 text-label coarse:h-11',
            set
              ? 'border-accent bg-accent-soft text-accent-text'
              : 'border-dashed border-border-strong text-text-secondary hover:bg-subtle hover:text-text',
            colorTransition,
            focusRing,
          )}
        >
          {label}
          <span className={cn('min-w-0 truncate font-semibold', set ? 'text-accent-text' : 'text-text')}>{current}</span>
          <ChevronDown className="h-4 w-4 shrink-0" aria-hidden />
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content align="start" sideOffset={4} className={menuContent}>
          <DropdownMenu.RadioGroup value={value} onValueChange={onValueChange}>
            {[{ value: '', label: anyLabel }, ...options].map((o) => (
              <DropdownMenu.RadioItem key={o.value} value={o.value} className={cn(menuItem, 'pl-8')}>
                <DropdownMenu.ItemIndicator className="absolute left-2 inline-flex">
                  <Check className="h-4 w-4 shrink-0" aria-hidden />
                </DropdownMenu.ItemIndicator>
                {o.label}
              </DropdownMenu.RadioItem>
            ))}
          </DropdownMenu.RadioGroup>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}

const FIGURE_TONES = {
  success: 'text-success-text',
  warning: 'text-warning-text',
  danger: 'text-danger-text',
  neutral: 'text-text-secondary',
};

export interface FigureButtonProps {
  count: number;
  label: string;
  icon: LucideIcon;
  tone?: keyof typeof FIGURE_TONES;
  pressed: boolean;
  onClick: () => void;
}

/** A summary count that filters the list behind it when chosen, and shows that it is chosen. */
export function FigureButton({ count, label, icon: Icon, tone = 'neutral', pressed, onClick }: FigureButtonProps) {
  return (
    <button
      type="button"
      aria-pressed={pressed}
      onClick={onClick}
      className={cn(
        'inline-flex h-10 shrink-0 items-center gap-2 rounded-md border pl-3 pr-3.5 text-label coarse:h-11',
        pressed
          ? 'border-accent bg-accent-soft text-accent-text ring-1 ring-inset ring-accent'
          : 'border-border bg-surface text-text-secondary hover:border-border-strong hover:bg-subtle',
        colorTransition,
        focusRing,
      )}
    >
      <Icon className={cn('h-4 w-4 shrink-0', pressed ? 'text-accent-text' : FIGURE_TONES[tone])} aria-hidden />
      <span className={cn('text-title tabular-nums', pressed ? 'text-accent-text' : 'text-text')}>{count}</span>
      {label}
    </button>
  );
}

/**
 * A table for console pages: 40 px rows and a header that stays in view while the rows scroll inside it.
 * Use the kit's THead/TR/TH/TD inside.
 */
export function DenseTable({ label, children, className }: { label: string; children: ReactNode; className?: string }) {
  return (
    <div
      role="region"
      aria-label={label}
      tabIndex={0}
      className={cn('max-h-[calc(100dvh-15rem)] min-h-0 w-full overflow-auto', focusRing, className)}
    >
      <table
        className={cn(
          'w-full border-separate border-spacing-0 text-left text-label font-normal tabular-nums',
          '[&_th]:sticky [&_th]:top-0 [&_th]:z-10 [&_th]:h-9 [&_th]:border-b [&_th]:border-border [&_th]:bg-subtle [&_th]:px-3 [&_th]:py-0 [&_th]:text-caption [&_th]:font-semibold',
          '[&_td]:h-10 [&_td]:border-b [&_td]:border-border [&_td]:px-3 [&_td]:py-1 [&_td]:align-middle',
          '[&_th:first-child]:pl-4 [&_td:first-child]:pl-4 [&_th:last-child]:pr-4 [&_td:last-child]:pr-4',
          '[&_tbody_tr:hover_td]:bg-bg',
        )}
      >
        {children}
      </table>
    </div>
  );
}

export interface TablePagerProps {
  /** What is being paged, for the buttons' accessible names, e.g. "clusters". */
  label: string;
  /** 0-based. */
  page: number;
  pageSize: number;
  total: number;
  onPageChange: (page: number) => void;
  className?: string;
}

/** "1 to 25 of 64" and, when there is more than one page, the page and its previous and next buttons. */
export function TablePager({ label, page, pageSize, total, onPageChange, className }: TablePagerProps) {
  const last = lastPage(total, pageSize);
  const from = total ? page * pageSize + 1 : 0;
  const to = Math.min(total, (page + 1) * pageSize);
  return (
    <nav aria-label={`Pages of ${label}`} className={cn('flex flex-wrap items-center gap-3 text-label font-normal text-text-secondary', className)}>
      <p aria-live="polite" className="tabular-nums">
        <span className="font-semibold text-text">
          {from} to {to}
        </span>{' '}
        of {total}
      </p>
      {last > 0 && (
        <div className="ml-auto flex items-center gap-2">
          <span className="tabular-nums">
            Page {page + 1} of {last + 1}
          </span>
          <Button size="sm" icon={ChevronLeft} disabled={page <= 0} onClick={() => onPageChange(page - 1)}>
            Previous page<span className="sr-only"> of {label}</span>
          </Button>
          <Button size="sm" icon={ChevronRight} disabled={page >= last} onClick={() => onPageChange(page + 1)}>
            Next page<span className="sr-only"> of {label}</span>
          </Button>
        </div>
      )}
    </nav>
  );
}

export interface ActionMenuItem {
  key: string;
  label: string;
  icon?: LucideIcon;
  danger?: boolean;
  onSelect: () => void;
}

/**
 * A row's "more actions" menu. An item runs once the menu has closed and focus is back on the trigger,
 * so a dialog it opens returns focus there.
 */
export function ActionMenu({ label, items }: { label: string; items: ActionMenuItem[] }) {
  const trigger = useRef<HTMLButtonElement>(null);
  const pending = useRef<(() => void) | null>(null);
  if (!items.length) return null;
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button
          ref={trigger}
          type="button"
          aria-label={label}
          title={label}
          className={cn(
            'inline-flex h-8 w-8 items-center justify-center rounded-md text-text-secondary hover:bg-muted hover:text-text coarse:h-11 coarse:w-11',
            colorTransition,
            focusRing,
          )}
        >
          <MoreHorizontal className="h-5 w-5 shrink-0" aria-hidden />
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          align="end"
          sideOffset={4}
          className={menuContent}
          onCloseAutoFocus={(e) => {
            const run = pending.current;
            pending.current = null;
            if (!run) return;
            e.preventDefault();
            trigger.current?.focus();
            run();
          }}
        >
          {items.map(({ key, label: text, icon: Icon, danger, onSelect }) => (
            <DropdownMenu.Item
              key={key}
              onSelect={() => {
                pending.current = onSelect;
              }}
              className={cn(menuItem, danger && 'text-danger-text data-[highlighted]:bg-danger-soft data-[highlighted]:text-danger-text')}
            >
              {Icon && <Icon className="h-4 w-4 shrink-0" aria-hidden />}
              {text}
            </DropdownMenu.Item>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}
