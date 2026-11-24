import type { PhaseTone } from '../../components/ui';
import type { AddClusterPayload, CapabilitiesView, ClusterView } from '../../generated/views';
import type { LabelRow } from '../../app/labels';
import { DNS_LABEL, NAME_RULE } from '../profiles/editor';
import { unreservedLabels } from './settings';

/** The server's limit: the name is part of the namespace `protem-cluster-<name>`. */
export const CLUSTER_NAME_MAX = 40;

export function clusterNameError(name: string, hubName: string | undefined): string | undefined {
  if (!name) return 'Enter a name.';
  if (name.length > CLUSTER_NAME_MAX) return `The name has ${name.length} characters. Shorten it to ${CLUSTER_NAME_MAX} or fewer.`;
  if (!DNS_LABEL.test(name)) return NAME_RULE;
  if (name === hubName) return `${name} is the hub's own name. Choose another.`;
  return undefined;
}

export interface AddClusterForm {
  name: string;
  displayName: string;
  description: string;
  labels: LabelRow[];
}

export type AddClusterErrors = { name?: string; displayName?: string; labels?: string };

export function addClusterPayload(form: AddClusterForm, hubName: string | undefined): { payload: AddClusterPayload; errors: AddClusterErrors } {
  const errors: AddClusterErrors = {};
  const name = form.name.trim();
  const displayName = form.displayName.trim();
  const description = form.description.trim();
  const { labels, error } = unreservedLabels(form.labels);

  const nameError = clusterNameError(name, hubName);
  if (nameError) errors.name = nameError;
  if (!displayName) errors.displayName = 'Enter a display name.';
  if (error) errors.labels = error;

  const payload: AddClusterPayload = { name, displayName };
  if (description) payload.description = description;
  if (Object.keys(labels).length) payload.labels = labels;
  return { payload, errors };
}

export type ClusterAction = 'connect' | 'reissue' | 'remove';

/** The agent actions a row offers; none at all without `addClusters`, and none for the hub, which has no agent. */
export function clusterActions(
  caps: CapabilitiesView | null | undefined,
  cluster: Pick<ClusterView, 'agent' | 'managedBy'>,
): ClusterAction[] {
  if (!caps?.addClusters || cluster.agent === 'none') return [];
  const out: ClusterAction[] = [];
  if (cluster.agent === 'waiting' || cluster.agent === 'lost') out.push('connect');
  if (cluster.agent === 'connected' || cluster.agent === 'lost') out.push('reissue');
  if (!cluster.managedBy) out.push('remove');
  return out;
}

const AGENT_STATES: Record<string, { label: string; tone: PhaseTone }> = {
  none: { label: 'Hub (no agent)', tone: 'neutral' },
  waiting: { label: 'Waiting for agent', tone: 'warning' },
  connected: { label: 'Agent connected', tone: 'success' },
  lost: { label: 'Agent not reporting', tone: 'danger' },
};

export function agentState(agent: string): { label: string; tone: PhaseTone } {
  return Object.prototype.hasOwnProperty.call(AGENT_STATES, agent) ? AGENT_STATES[agent] : { label: 'Unknown', tone: 'neutral' };
}

export function joinCountdown(expiresAt: string, now: Date): { expired: boolean; left: string } {
  const ms = new Date(expiresAt).getTime() - now.getTime();
  if (!(ms > 0)) return { expired: true, left: '' };
  const seconds = Math.ceil(ms / 1000);
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  return { expired: false, left: m ? `${m}m ${String(s).padStart(2, '0')}s left` : `${s}s left` };
}

type AgentSeen = Pick<ClusterView, 'agent' | 'lastHeartbeat'>;

/**
 * Whether an agent reported in after credentials were issued. After a reissue the cut-off agent's last
 * heartbeat can still read as connected, so only a newer heartbeat counts.
 */
export function agentJoined(now: AgentSeen | undefined, atIssue: AgentSeen | undefined): boolean {
  if (now?.agent !== 'connected') return false;
  return atIssue?.agent !== 'connected' || now.lastHeartbeat !== atIssue.lastHeartbeat;
}
