import { CircleCheck, CircleMinus, CircleX, Clock, type LucideIcon } from 'lucide-react';
import { Badge } from './Badge';

export type PhaseTone = 'warning' | 'success' | 'danger' | 'neutral';

const TONE_BY_PHASE: Record<string, PhaseTone> = {
  Pending: 'warning',
  Approved: 'success',
  Active: 'success',
  Ready: 'success',
  Applied: 'success',
  Rejected: 'danger',
  Revoked: 'danger',
  Failed: 'danger',
  NotReady: 'danger',
  Disconnected: 'danger',
  Maintenance: 'warning',
  Paused: 'warning',
};

export function phaseTone(phase?: string): PhaseTone {
  return phase && Object.prototype.hasOwnProperty.call(TONE_BY_PHASE, phase) ? TONE_BY_PHASE[phase] : 'neutral';
}

const ICONS: Record<PhaseTone, LucideIcon> = {
  warning: Clock,
  success: CircleCheck,
  danger: CircleX,
  neutral: CircleMinus,
};

const LABELS: Record<string, string> = { NotReady: 'Not ready', TimedOut: 'Timed out' };

/** A phase's pill; `tone` and `label` stand in for a phase when the state is not a phase. */
export function StatusPill({ phase, tone: toneOverride, label }: { phase?: string; tone?: PhaseTone; label?: string }) {
  const tone = toneOverride ?? phaseTone(phase);
  const Icon = ICONS[tone];
  return (
    <Badge tone={tone}>
      <Icon className="h-4 w-4 shrink-0" aria-hidden />
      {label ?? (phase ? (LABELS[phase] ?? phase) : 'Unknown')}
    </Badge>
  );
}
