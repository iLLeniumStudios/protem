import { useRef, type KeyboardEvent, type MouseEvent } from 'react';
import type { LucideIcon } from 'lucide-react';
import { cn, colorTransition, focusRing } from './cn';

export interface SegmentedOption<T extends string> {
  value: T;
  label: string;
  icon?: LucideIcon;
}

export interface SegmentedControlProps<T extends string> {
  value: T;
  onValueChange: (value: T) => void;
  options: SegmentedOption<T>[];
  label: string;
  className?: string;
}

const STEP: Record<string, number> = { ArrowRight: 1, ArrowDown: 1, ArrowLeft: -1, ArrowUp: -1 };

export function SegmentedControl<T extends string>({ value, onValueChange, options, label, className }: SegmentedControlProps<T>) {
  const refs = useRef<(HTMLButtonElement | null)[]>([]);
  const current = Math.max(0, options.findIndex((o) => o.value === value));

  const onKeyDown = (e: KeyboardEvent) => {
    let next: number;
    if (e.key in STEP) next = (current + STEP[e.key] + options.length) % options.length;
    else if (e.key === 'Home') next = 0;
    else if (e.key === 'End') next = options.length - 1;
    else return;
    e.preventDefault();
    onValueChange(options[next].value);
    refs.current[next]?.focus();
  };

  return (
    <div
      role="radiogroup"
      aria-label={label}
      onKeyDown={onKeyDown}
      className={cn('inline-flex max-w-full flex-wrap gap-1 rounded-md border border-border-strong bg-subtle p-1', className)}
    >
      {options.map((o, i) => {
        const checked = i === current;
        const Icon = o.icon;
        return (
          <button
            key={o.value}
            ref={(el) => {
              refs.current[i] = el;
            }}
            type="button"
            role="radio"
            aria-checked={checked}
            tabIndex={checked ? 0 : -1}
            onClick={() => onValueChange(o.value)}
            className={segmentClass(checked)}
          >
            {Icon && <Icon className="h-4 w-4 shrink-0" aria-hidden />}
            {o.label}
          </button>
        );
      })}
    </div>
  );
}

const segmentClass = (checked: boolean) =>
  cn(
    'inline-flex h-8 items-center justify-center gap-1.5 rounded-sm border px-3 text-label coarse:h-11',
    checked ? 'border-border-strong bg-surface text-text' : 'border-transparent text-text-secondary hover:bg-muted hover:text-text',
    colorTransition,
    focusRing,
  );

export interface SegmentedNavItem {
  href: string;
  label: string;
  current: boolean;
}

/** A segmented control whose options are places: each is a link, and the current one is marked current. */
export function SegmentedNav({
  label,
  items,
  onNavigate,
  className,
}: {
  label: string;
  items: SegmentedNavItem[];
  /** Called for a plain click; modified clicks (new tab, new window) keep the browser's behaviour. */
  onNavigate: (href: string) => void;
  className?: string;
}) {
  const go = (href: string) => (e: MouseEvent<HTMLAnchorElement>) => {
    if (e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    e.preventDefault();
    onNavigate(href);
  };
  return (
    <nav aria-label={label} className={cn('grid auto-cols-fr grid-flow-col gap-1 rounded-md border border-border-strong bg-muted p-1', className)}>
      {items.map((item) => (
        <a
          key={item.href}
          href={item.href}
          aria-current={item.current || undefined}
          onClick={go(item.href)}
          className={cn(segmentClass(item.current), 'min-w-0 truncate px-2', item.current && 'font-semibold')}
        >
          {item.label}
        </a>
      ))}
    </nav>
  );
}
