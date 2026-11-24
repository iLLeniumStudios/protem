import type { CapabilitiesView, ClusterView, ProfileView } from '../../generated/views';

type Caps = CapabilitiesView | null | undefined;

export function canCreateProfile(caps: Caps): boolean {
  return Boolean(caps?.management && caps.profileNamespaces?.length);
}

export function canEditProfile(caps: Caps, profile: Pick<ProfileView, 'namespace' | 'managedBy'>): boolean {
  const namespaces = caps?.profileNamespaces ?? [];
  return Boolean(caps?.management && !profile.managedBy && (namespaces.includes('*') || namespaces.includes(profile.namespace)));
}

export function canEditCluster(caps: Caps, cluster: Pick<ClusterView, 'managedBy'>): boolean {
  return Boolean(caps?.management && caps.manageClusters && !cluster.managedBy);
}

export function canSeeProfiles(caps: Caps): boolean {
  return canCreateProfile(caps) || Boolean(caps?.viewAll);
}
