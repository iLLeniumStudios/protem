import type { ClusterSettingsPayload, ClusterView } from '../../generated/views';
import { labelRows, rowsToLabels, type LabelRow } from '../../app/labels';

/** Label prefixes owned by Protem and GitOps tools; the server keeps them and refuses them in a payload. */
export const RESERVED_LABEL_PREFIXES = [
  'protem.dev/',
  'argocd.argoproj.io/',
  'kustomize.toolkit.fluxcd.io/',
  'helm.toolkit.fluxcd.io/',
  'app.kubernetes.io/',
];

const isReserved = (key: string) => RESERVED_LABEL_PREFIXES.some((p) => key.startsWith(p));

/** Rows as labels; `error` also refuses a key under a reserved prefix, as the server does. */
export function unreservedLabels(rows: LabelRow[]): { labels: Record<string, string>; error?: string } {
  const { labels, error } = rowsToLabels(rows);
  const reserved = Object.keys(labels).find(isReserved);
  if (error) return { labels, error };
  if (reserved) return { labels, error: `${reserved} uses a reserved prefix. Those labels are set by Protem or GitOps tools.` };
  return { labels };
}

export interface SettingsForm {
  paused: boolean;
  subject: string;
  labels: LabelRow[];
  /** Shown read-only; never sent. */
  reserved: [string, string][];
}

const editableLabels = (cluster: ClusterView) =>
  Object.fromEntries(Object.entries(cluster.labels ?? {}).filter(([k]) => !isReserved(k)));

export function settingsForm(cluster: ClusterView): SettingsForm {
  return {
    paused: cluster.paused,
    subject: cluster.subject ?? '',
    labels: labelRows(editableLabels(cluster)),
    reserved: Object.entries(cluster.labels ?? {}).filter(([k]) => isReserved(k)),
  };
}

function sameLabels(a: Record<string, string>, b: Record<string, string>) {
  const keys = Object.keys(a);
  return keys.length === Object.keys(b).length && keys.every((k) => Object.prototype.hasOwnProperty.call(b, k) && a[k] === b[k]);
}

/** Only the settings the user changed, and what is wrong with them. */
export function settingsChange(
  cluster: ClusterView,
  form: SettingsForm,
): { payload: ClusterSettingsPayload; errors: { subject?: string; labels?: string } } {
  const payload: ClusterSettingsPayload = {};
  const errors: { subject?: string; labels?: string } = {};

  if (form.paused !== cluster.paused) payload.paused = form.paused;

  const subject = form.subject.trim();
  if (subject !== (cluster.subject ?? '')) {
    if (!subject || subject.includes('{{.Email}}') || subject.includes('{{.Username}}')) payload.subject = subject;
    else errors.subject = 'Include {{.Email}} or {{.Username}}, or leave it empty. A fixed subject would give every user the same identity.';
  }

  const { labels, error } = unreservedLabels(form.labels);
  if (error) errors.labels = error;
  else if (!sameLabels(labels, editableLabels(cluster))) payload.labels = labels;

  return { payload, errors };
}
