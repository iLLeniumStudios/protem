import { Lock } from 'lucide-react';
import { Badge } from '../../components/ui';

/** Shown only when a GitOps tool or lock owns the object; the UI cannot change it then. */
export function ManagedBadge({ by }: { by?: string }) {
  if (!by) return null;
  return (
    <span title={`Managed by ${by}; change it in Git`} className="inline-flex shrink-0">
      <Badge tone="info">
        <Lock className="h-3.5 w-3.5 shrink-0" aria-hidden />
        <span className="sr-only">Managed by </span>
        {by}
      </Badge>
    </span>
  );
}
