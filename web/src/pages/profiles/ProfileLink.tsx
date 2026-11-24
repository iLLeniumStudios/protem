import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { cn } from '../../components/ui';
import { focusRing } from '../../components/ui/cn';
import { profilePath } from '../../app/format';

export function ProfileLink({ namespace, name, children }: { namespace: string; name: string; children: ReactNode }) {
  return (
    <Link
      to={profilePath({ namespace, name })}
      className={cn('rounded-sm font-medium text-accent-text underline-offset-4 hover:underline', focusRing)}
    >
      {children}
    </Link>
  );
}
