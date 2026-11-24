import { Identifier } from '../../components/ui';
import type { ClusterView } from '../../generated/views';

export function ClusterName({ cluster }: { cluster: ClusterView }) {
  if (!cluster.displayName || cluster.displayName === cluster.name) {
    return <Identifier className="text-body font-medium text-text">{cluster.name}</Identifier>;
  }
  return (
    <span className="flex min-w-0 flex-col">
      <span className="break-words text-body font-medium text-text">{cluster.displayName}</span>
      <Identifier className="text-caption text-text-secondary">{cluster.name}</Identifier>
    </span>
  );
}
