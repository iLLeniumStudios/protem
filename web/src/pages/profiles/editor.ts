import type {
  AccessProfileSpec,
  ApprovalApprovers,
  ApprovalRule,
  ApprovalRuleWhen,
  LabelSelector,
  LabelSelectorRequirement,
} from '../../generated/crd';
import { labelRows, rowsToLabels, type LabelRow } from '../../app/labels';

export type ClusterMode = 'names' | 'selector';

/** Lists are edited as one item per line. */
export interface RuleForm {
  key: number;
  base: ApprovalRule | null;
  name: string;
  clusterLabels: LabelRow[];
  clusterExpressions: LabelSelectorRequirement[];
  requesterGroups: string;
  requesterUsers: string;
  mode: string;
  approverGroups: string;
  approverUsers: string;
  requiredApprovals: string;
  allowSelfApproval: boolean;
  maxDuration: string;
}

export interface ProfileForm {
  base: AccessProfileSpec | null;
  displayName: string;
  description: string;
  roleKind: string;
  roleName: string;
  scope: string;
  namespaces: string;
  clusterMode: ClusterMode;
  clusterNames: string;
  clusterLabels: LabelRow[];
  clusterExpressions: LabelSelectorRequirement[];
  defaultDuration: string;
  maxDuration: string;
  requesterGroups: string;
  requesterUsers: string;
  reason: string;
  ticketPattern: string;
  pendingTimeout: string;
  rules: RuleForm[];
}

/** Field errors keyed by the spec path they belong to, e.g. `duration.default` or `rules.1.name`. */
export type Errors = Record<string, string>;

let nextKey = 0;

export const lines = (text: string) =>
  text
    .split('\n')
    .map((s) => s.trim())
    .filter(Boolean);

const text = (items: string[] | null | undefined) => (items ?? []).join('\n');

export function newRuleForm(name = ''): RuleForm {
  return {
    key: nextKey++,
    base: null,
    name,
    clusterLabels: [],
    clusterExpressions: [],
    requesterGroups: '',
    requesterUsers: '',
    mode: 'Manual',
    approverGroups: '',
    approverUsers: '',
    requiredApprovals: '1',
    allowSelfApproval: false,
    maxDuration: '',
  };
}

export function newProfileForm(): ProfileForm {
  return {
    base: null,
    displayName: '',
    description: '',
    roleKind: 'ClusterRole',
    roleName: '',
    scope: 'Namespaced',
    namespaces: '',
    clusterMode: 'names',
    clusterNames: '',
    clusterLabels: [],
    clusterExpressions: [],
    defaultDuration: '1h',
    maxDuration: '8h',
    requesterGroups: '',
    requesterUsers: '',
    reason: 'Required',
    ticketPattern: '',
    pendingTimeout: '',
    rules: [newRuleForm('default')],
  };
}

function ruleToForm(rule: ApprovalRule): RuleForm {
  const selector = rule.when?.clusterSelector;
  return {
    key: nextKey++,
    base: rule,
    name: rule.name ?? '',
    clusterLabels: labelRows(selector?.matchLabels),
    clusterExpressions: selector?.matchExpressions ?? [],
    requesterGroups: text(rule.when?.requesterGroups),
    requesterUsers: text(rule.when?.requesterUsers),
    mode: rule.mode || 'Manual',
    approverGroups: text(rule.approvers?.groups),
    approverUsers: text(rule.approvers?.users),
    requiredApprovals: String(rule.requiredApprovals ?? 1),
    allowSelfApproval: Boolean(rule.allowSelfApproval),
    maxDuration: shortDuration(rule.maxDuration ?? ''),
  };
}

export function specToForm(spec: AccessProfileSpec): ProfileForm {
  const selector = spec.clusters?.selector;
  return {
    base: spec,
    displayName: spec.displayName ?? '',
    description: spec.description ?? '',
    roleKind: spec.role?.kind || 'ClusterRole',
    roleName: spec.role?.name ?? '',
    scope: spec.scope || 'Namespaced',
    namespaces: text(spec.namespaces),
    clusterMode: selector && !spec.clusters.names?.length ? 'selector' : 'names',
    clusterNames: text(spec.clusters?.names),
    clusterLabels: labelRows(selector?.matchLabels),
    clusterExpressions: selector?.matchExpressions ?? [],
    defaultDuration: shortDuration(spec.duration?.default ?? ''),
    maxDuration: shortDuration(spec.duration?.max ?? ''),
    requesterGroups: text(spec.requesters?.groups),
    requesterUsers: text(spec.requesters?.users),
    reason: spec.justification?.reason || 'Required',
    ticketPattern: spec.justification?.ticketPattern ?? '',
    pendingTimeout: shortDuration(spec.approval?.pendingTimeout ?? ''),
    rules: (spec.approval?.rules ?? []).map(ruleToForm),
  };
}

function selector(rows: LabelRow[], expressions: LabelSelectorRequirement[]): LabelSelector {
  const out: LabelSelector = {};
  const { labels } = rowsToLabels(rows);
  if (Object.keys(labels).length) out.matchLabels = labels;
  if (expressions.length) out.matchExpressions = expressions;
  return out;
}

// Fields hidden for an Auto rule are not editable there, so an Auto rule keeps what it was loaded with.
function ruleToSpec(r: RuleForm): ApprovalRule {
  const base = r.base;
  const rule: ApprovalRule = { name: r.name.trim(), mode: r.mode };

  const when: ApprovalRuleWhen = {};
  const clusterSelector = selector(r.clusterLabels, r.clusterExpressions);
  if (Object.keys(clusterSelector).length || base?.when?.clusterSelector) when.clusterSelector = clusterSelector;
  const requesterGroups = lines(r.requesterGroups);
  if (requesterGroups.length) when.requesterGroups = requesterGroups;
  const requesterUsers = lines(r.requesterUsers);
  if (requesterUsers.length) when.requesterUsers = requesterUsers;
  if (Object.keys(when).length || base?.when) rule.when = when;

  if (r.maxDuration.trim()) rule.maxDuration = r.maxDuration.trim();

  if (r.mode !== 'Manual') {
    if (base?.approvers) rule.approvers = base.approvers;
    if (base?.requiredApprovals !== undefined) rule.requiredApprovals = base.requiredApprovals;
    if (base?.allowSelfApproval !== undefined) rule.allowSelfApproval = base.allowSelfApproval;
    return rule;
  }
  const approvers: ApprovalApprovers = {};
  const groups = lines(r.approverGroups);
  const users = lines(r.approverUsers);
  if (groups.length) approvers.groups = groups;
  if (users.length) approvers.users = users;
  if (Object.keys(approvers).length || base?.approvers) rule.approvers = approvers;
  const required = Number(r.requiredApprovals.trim() || NaN);
  if (base?.requiredApprovals !== undefined || required !== 1) rule.requiredApprovals = required;
  if (r.allowSelfApproval || base?.allowSelfApproval !== undefined) rule.allowSelfApproval = r.allowSelfApproval;
  return rule;
}

/** The spec the form describes; anything the form cannot edit is carried over from the loaded spec untouched. */
export function formToSpec(f: ProfileForm): AccessProfileSpec {
  const names = lines(f.clusterNames);
  const spec: AccessProfileSpec = {
    displayName: f.displayName,
    role: { kind: f.roleKind, name: f.roleName.trim() },
    scope: f.scope,
    clusters:
      f.clusterMode === 'names' ? (names.length ? { names } : {}) : { selector: selector(f.clusterLabels, f.clusterExpressions) },
    duration: { default: f.defaultDuration.trim(), max: f.maxDuration.trim() },
    requesters: {},
    justification: { reason: f.reason },
    approval: { rules: f.rules.map(ruleToSpec) },
  };
  if (f.description) spec.description = f.description;
  const requesterGroups = lines(f.requesterGroups);
  const requesterUsers = lines(f.requesterUsers);
  if (requesterGroups.length) spec.requesters.groups = requesterGroups;
  if (requesterUsers.length) spec.requesters.users = requesterUsers;
  const namespaces = f.scope === 'Cluster' ? (f.base?.scope === 'Cluster' ? f.base.namespaces : undefined) : lines(f.namespaces);
  if (namespaces?.length) spec.namespaces = namespaces;
  if (f.ticketPattern) spec.justification.ticketPattern = f.ticketPattern;
  if (f.pendingTimeout.trim()) spec.approval.pendingTimeout = f.pendingTimeout.trim();
  return spec;
}

const UNIT_SECONDS: Record<string, number> = { ns: 1e-9, us: 1e-6, 'µs': 1e-6, 'μs': 1e-6, ms: 1e-3, s: 1, m: 60, h: 3600 };

/** Seconds in a Go duration ("1h30m", "1.5h", "500ms"); null when Go's time.ParseDuration would reject it. */
export function parseDuration(s: string): number | null {
  if (s === '0') return 0;
  if (!s) return null;
  const part = /(\d+(?:\.\d*)?|\.\d+)(ns|us|µs|μs|ms|s|m|h)/y;
  let total = 0;
  while (part.lastIndex < s.length) {
    const m = part.exec(s);
    if (!m) return null;
    total += Number(m[1]) * UNIT_SECONDS[m[2]];
  }
  return total;
}

const DURATION_HINT = 'Use a duration such as 30m, 1h or 8h.';

// Go's RE2 accepts a leading flag group such as (?i), which JavaScript does not.
function regexError(pattern: string): string | undefined {
  try {
    new RegExp(pattern.replace(/^\(\?[imsU]+\)/, ''));
    return undefined;
  } catch (e) {
    return `This regular expression doesn't compile: ${e instanceof Error ? e.message : String(e)}`;
  }
}

const positive = (d: string | undefined) => {
  const seconds = parseDuration(d?.trim() ?? '');
  return seconds !== null && seconds > 0 ? seconds : null;
};

export function validateSpec(spec: AccessProfileSpec): Errors {
  const e: Errors = {};
  if (!spec.displayName?.trim()) e.displayName = 'Enter a display name. Requesters pick profiles by it.';
  if (!spec.role?.name?.trim()) e['role.name'] = 'Enter the name of the role to bind.';
  if (spec.scope === 'Cluster' && spec.role?.kind !== 'ClusterRole') {
    e.scope = 'Cluster scope needs a ClusterRole. A Role exists only inside one namespace.';
  }
  if (spec.scope !== 'Cluster' && !spec.namespaces?.length) e.namespaces = 'List at least one namespace.';

  const named = Boolean(spec.clusters?.names?.length);
  const selected = Boolean(spec.clusters?.selector);
  if (named && selected) e.clusters = 'Choose clusters by name or by label, not both.';
  else if (!named && !selected) e.clusters = 'Name at least one cluster, or choose clusters by label.';
  else if (selected && !Object.keys(spec.clusters.selector?.matchLabels ?? {}).length && !spec.clusters.selector?.matchExpressions?.length) {
    e.clusterLabels = 'Add at least one label. A profile cannot target every cluster by leaving this empty.';
  }

  const def = positive(spec.duration?.default);
  const max = positive(spec.duration?.max);
  if (def === null) e['duration.default'] = DURATION_HINT;
  if (max === null) e['duration.max'] = DURATION_HINT;
  if (def !== null && max !== null && def > max) e['duration.default'] = "The default can't be longer than the maximum.";

  if (!spec.requesters?.groups?.length && !spec.requesters?.users?.length) {
    e.requesters = 'Add at least one group or user. Only they can request this profile.';
  }
  const pattern = spec.justification?.ticketPattern;
  const patternError = pattern ? regexError(pattern) : undefined;
  if (patternError) e.ticketPattern = patternError;
  if (spec.approval?.pendingTimeout && positive(spec.approval.pendingTimeout) === null) e.pendingTimeout = DURATION_HINT;

  const rules = spec.approval?.rules ?? [];
  if (!rules.length) e.rules = 'Add at least one approval rule.';
  const seen = new Set<string>();
  rules.forEach((rule, i) => {
    const name = rule.name?.trim();
    if (!name) e[`rules.${i}.name`] = 'Enter a rule name.';
    else if (seen.has(name)) e[`rules.${i}.name`] = 'Another rule already uses this name.';
    seen.add(name);
    if (rule.maxDuration && positive(rule.maxDuration) === null) e[`rules.${i}.maxDuration`] = DURATION_HINT;
    if (rule.mode !== 'Manual') return;
    if (!rule.approvers?.groups?.length && !rule.approvers?.users?.length) {
      e[`rules.${i}.approvers`] = 'Add at least one approver group or user.';
    }
    const required = rule.requiredApprovals ?? 1;
    if (!Number.isInteger(required) || required < 1) e[`rules.${i}.requiredApprovals`] = 'Enter a whole number, 1 or more.';
  });
  return e;
}

export function validateForm(f: ProfileForm): Errors {
  const e = validateSpec(formToSpec(f));
  const clusterLabels = rowsToLabels(f.clusterLabels).error;
  if (f.clusterMode === 'selector' && clusterLabels) e.clusterLabels = clusterLabels;
  f.rules.forEach((r, i) => {
    const error = rowsToLabels(r.clusterLabels).error;
    if (error) e[`rules.${i}.clusterLabels`] = error;
  });
  return e;
}

export const DNS_LABEL = /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/;
const DNS_SUBDOMAIN = /^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$/;
export const NAME_RULE = 'Use lowercase letters, digits and hyphens, starting and ending with a letter or digit.';

export function validateNew(namespace: string, name: string): Errors {
  const e: Errors = {};
  if (!namespace) e.namespace = 'Choose a namespace.';
  else if (namespace.length > 63 || !DNS_LABEL.test(namespace)) e.namespace = NAME_RULE;
  if (!name) e.name = 'Enter a name.';
  else if (name.length > 253 || !DNS_SUBDOMAIN.test(name)) e.name = `${NAME_RULE} Dots are allowed.`;
  return e;
}

/** A Go duration in its short form ("1h0m0s" as "1h"); anything it cannot shorten exactly comes back as is. */
export function shortDuration(s: string): string {
  const total = parseDuration(s);
  if (total === null || !Number.isInteger(total)) return s;
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const sec = total % 60;
  return `${h ? `${h}h` : ''}${m ? `${m}m` : ''}${sec || !total ? `${sec}s` : ''}`;
}
