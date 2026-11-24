import { useEffect, useState } from 'react';
import { TriangleAlert } from 'lucide-react';
import { cn } from './cn';
import { endingSoon, formatElapsed, formatRelative, formatTimeLeft, formatTimestamp, timeUsedFraction } from './format';

const TICK_MS = 30_000;

function useNow() {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const id = setInterval(() => setNow(new Date()), TICK_MS);
    return () => clearInterval(id);
  }, []);
  return now;
}

function iso(v: Date | string) {
  const d = new Date(v);
  return Number.isNaN(d.getTime()) ? undefined : d.toISOString();
}

export function TimeLeft({ expiresAt, className }: { expiresAt: Date | string; className?: string }) {
  const now = useNow();
  return (
    <time dateTime={iso(expiresAt)} title={formatTimestamp(expiresAt)} className={cn('tabular-nums', className)}>
      {formatTimeLeft(expiresAt, now)}
    </time>
  );
}

export function RelativeTime({ at, className }: { at: Date | string; className?: string }) {
  const now = useNow();
  return (
    <time dateTime={iso(at)} title={formatTimestamp(at)} className={cn('tabular-nums', className)}>
      {formatRelative(at, now)}
    </time>
  );
}

/** How long since `since`, as a span ("4 min"); for "waiting ..." labels. */
export function Elapsed({ since, className }: { since: Date | string; className?: string }) {
  const now = useNow();
  return (
    <time dateTime={iso(since)} title={formatTimestamp(since)} className={cn('tabular-nums', className)}>
      {formatElapsed(since, now)}
    </time>
  );
}

/** A grant's time left in words above a bar of the time used; the last 30 minutes read as a warning. */
export function TimeUsed({ startedAt, expiresAt, label, className }: { startedAt: string; expiresAt: string; label: string; className?: string }) {
  const now = useNow();
  const used = Math.round(timeUsedFraction(startedAt, expiresAt, now) * 100);
  const left = formatTimeLeft(expiresAt, now);
  const low = endingSoon(expiresAt, now);
  const until = formatTimestamp(expiresAt, new Date(now));
  return (
    <div className={cn('flex min-w-0 flex-col gap-1.5', className)}>
      <div className="flex items-baseline justify-between gap-3">
        <span className={cn('inline-flex items-center gap-1 text-label tabular-nums', low ? 'text-warning-text' : 'text-text')}>
          {low && <TriangleAlert className="h-4 w-4 shrink-0 self-center" aria-hidden />}
          {left}
        </span>
        <time dateTime={iso(expiresAt)} title={formatTimestamp(expiresAt)} className="text-caption tabular-nums text-text-secondary">
          until {until}
        </time>
      </div>
      <div
        role="progressbar"
        aria-label={label}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={used}
        aria-valuetext={`${left}, until ${until}`}
        className="h-1.5 w-full overflow-hidden rounded-full bg-muted"
      >
        <div className={cn('h-full rounded-full', low ? 'bg-warning' : 'bg-accent')} style={{ width: `${used}%` }} />
      </div>
    </div>
  );
}
