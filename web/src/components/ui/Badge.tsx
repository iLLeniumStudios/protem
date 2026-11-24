import type { ReactNode } from 'react';
import { cn } from './cn';

export type Tone = 'neutral' | 'accent' | 'success' | 'warning' | 'danger' | 'info' | 'count';

const TONES: Record<Tone, string> = {
  neutral: 'bg-muted text-text-secondary',
  accent: 'bg-accent-soft text-accent-text',
  success: 'bg-success-soft text-success-text',
  warning: 'bg-warning-soft text-warning-text',
  danger: 'bg-danger-soft text-danger-text',
  info: 'bg-info-soft text-info-text',
  count: 'bg-accent text-accent-ink',
};

export function Badge({ tone = 'neutral', children, className }: { tone?: Tone; children: ReactNode; className?: string }) {
  return (
    <span
      className={cn(
        'inline-flex max-w-full items-center gap-1 whitespace-nowrap rounded-full px-2 py-0.5 text-caption font-medium',
        TONES[tone],
        className,
      )}
    >
      {children}
    </span>
  );
}
