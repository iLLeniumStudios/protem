import { Link } from 'react-router-dom';
import { CircleCheck, Clock, UserCheck, Zap } from 'lucide-react';
import { requestFormPath } from '../../app/format';
import type { ProfileView } from '../../generated/views';
import { buttonClass } from '../../components/ui';
import { describeActivation, type Activatable } from '../home/home';

/** How a request for the profile is decided: at once, or by whom. */
export function ApprovalMeta({ profile }: { profile: ProfileView }) {
  const { mode, line, title } = describeActivation(profile);
  const Icon = mode === 'at-once' ? Zap : UserCheck;
  return (
    <span className="flex min-w-0 items-start gap-1 text-caption text-text-secondary" title={title}>
      <Icon className="mt-px h-4 w-4 shrink-0 text-text-tertiary" aria-hidden />
      <span className="min-w-0 break-words">{line}</span>
    </span>
  );
}

/** The one thing to do with a profile: activate it, ask for approval, or nothing because it is held or asked for. */
export function ActivationControl({ item }: { item: Activatable }) {
  const name = item.profile.displayName || item.profile.name;
  if (item.state === 'active') {
    return (
      <span className="inline-flex shrink-0 items-center gap-1 text-label text-success-text">
        <CircleCheck className="h-4 w-4 shrink-0" aria-hidden />
        Active
      </span>
    );
  }
  if (item.state === 'waiting') {
    return (
      <span className="inline-flex shrink-0 items-center gap-1 text-label text-text-secondary">
        <Clock className="h-4 w-4 shrink-0" aria-hidden />
        Waiting
      </span>
    );
  }
  return (
    <Link to={requestFormPath(item.profile)} className={buttonClass('secondary', 'sm')}>
      {describeActivation(item.profile).control}
      <span className="sr-only"> for {name}</span>
    </Link>
  );
}
