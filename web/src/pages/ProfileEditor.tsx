import { useId, useRef, useState, type FormEvent, type ReactNode } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { ArrowDown, ArrowUp, CircleAlert, FileQuestion, Lock, Plus, RefreshCw, Trash2, TriangleAlert } from 'lucide-react';
import { ApiError, api, errorMessage } from '../api';
import { useData } from '../app/data';
import { useLive, useLoad } from '../app/load';
import { profilePath } from '../app/format';
import type { LabelSelectorRequirement } from '../generated/crd';
import type { ProfileDetailView } from '../generated/views';
import {
  Button,
  Checkbox,
  EmptyState,
  Field,
  IconButton,
  Identifier,
  Input,
  PageHeader,
  Panel,
  SegmentedControl,
  Select,
  Skeleton,
  Textarea,
  buttonClass,
  cn,
  useToast,
} from '../components/ui';
import { focusRing } from '../components/ui/cn';
import { Fact, Facts, IdList, LoadFailed } from './access/parts';
import {
  newProfileForm,
  newRuleForm,
  specToForm,
  formToSpec,
  validateForm,
  validateNew,
  type Errors,
  type ProfileForm,
  type RuleForm,
} from './profiles/editor';
import { KeyValueRows } from './profiles/KeyValueRows';
import { canCreateProfile, canEditProfile } from './profiles/manage';
import { selectorTerms } from './profiles/profile';

const DURATION_HINT = 'A duration such as 30m, 1h or 8h';
const OPTIONAL_DURATION_HINT = 'Optional; a duration such as 30m, 1h or 8h';
const ONE_PER_LINE = 'One per line';

function KeptExpressions({ expressions }: { expressions: LabelSelectorRequirement[] }) {
  if (!expressions.length) return null;
  return (
    <div className="flex min-w-0 flex-col gap-1 rounded-md border border-border bg-subtle px-3 py-2">
      <span className="flex items-center gap-1 text-caption text-text-secondary">
        <Lock className="h-4 w-4 shrink-0" aria-hidden />
        Also required, edited in YAML or Git and kept as they are:
      </span>
      <span className="text-body text-text">
        <IdList items={selectorTerms({ matchExpressions: expressions })} empty="" />
      </span>
    </div>
  );
}

function GroupError({ id, error }: { id: string; error?: string }) {
  if (!error) return null;
  return (
    <p id={id} className="flex items-start gap-1 text-caption text-danger-text">
      <CircleAlert className="mt-px h-4 w-4 shrink-0" aria-hidden />
      <span>{error}</span>
    </p>
  );
}

/** A labelled SegmentedControl that can carry an error and take focus for it. */
function Choice<T extends string>({
  id,
  label,
  value,
  onValueChange,
  options,
  error,
  children,
}: {
  id: string;
  label: string;
  value: T;
  onValueChange: (v: T) => void;
  options: { value: T; label: string }[];
  error?: string;
  children?: ReactNode;
}) {
  return (
    <div
      id={id}
      tabIndex={-1}
      data-invalid={error ? 'true' : undefined}
      className={cn('flex min-w-0 flex-col gap-2 rounded-sm', focusRing)}
    >
      <span className="text-label text-text">{label}</span>
      <SegmentedControl label={label} value={value} onValueChange={onValueChange} options={options} className="self-start" />
      {children}
      <GroupError id={`${id}-error`} error={error} />
    </div>
  );
}

function RuleEditor({
  rule,
  index,
  count,
  id,
  errors,
  onChange,
  onMove,
  onRemove,
}: {
  rule: RuleForm;
  index: number;
  count: number;
  id: (key: string) => string;
  errors: Errors;
  onChange: (patch: Partial<RuleForm>) => void;
  onMove: (by: -1 | 1) => void;
  onRemove: () => void;
}) {
  const key = (field: string) => `rules.${index}.${field}`;
  const title = `rule ${rule.name.trim() || index + 1}`;
  const manual = rule.mode === 'Manual';
  return (
    <li className="flex flex-col gap-4 py-6 first:pt-0 last:pb-0">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="text-label text-text-secondary tabular-nums">Rule {index + 1}</h3>
        <div className="flex items-center gap-1">
          <IconButton label={`Move ${title} up`} icon={ArrowUp} disabled={index === 0} onClick={() => onMove(-1)} />
          <IconButton label={`Move ${title} down`} icon={ArrowDown} disabled={index === count - 1} onClick={() => onMove(1)} />
          <IconButton label={`Remove ${title}`} icon={Trash2} onClick={onRemove} />
        </div>
      </div>
      <Field label="Rule name" htmlFor={id(key('name'))} required error={errors[key('name')]}>
        <Input value={rule.name} autoComplete="off" spellCheck={false} onChange={(e) => onChange({ name: e.target.value })} />
      </Field>
      <div className="flex flex-col gap-4 rounded-md border border-border p-4">
        <p className="text-body text-text-secondary">
          Applies to clusters with these labels and to requesters in these groups or users. Leave all empty to match every request.
        </p>
        <KeyValueRows
          id={id(key('clusterLabels'))}
          legend="Cluster labels"
          rows={rule.clusterLabels}
          onChange={(clusterLabels) => onChange({ clusterLabels })}
          error={errors[key('clusterLabels')]}
        >
          <KeptExpressions expressions={rule.clusterExpressions} />
        </KeyValueRows>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Requester groups" htmlFor={id(key('requesterGroups'))} hint={ONE_PER_LINE}>
            <Textarea
              rows={2}
              value={rule.requesterGroups}
              spellCheck={false}
              onChange={(e) => onChange({ requesterGroups: e.target.value })}
            />
          </Field>
          <Field label="Requester users" htmlFor={id(key('requesterUsers'))} hint={ONE_PER_LINE}>
            <Textarea
              rows={2}
              value={rule.requesterUsers}
              spellCheck={false}
              onChange={(e) => onChange({ requesterUsers: e.target.value })}
            />
          </Field>
        </div>
      </div>
      <Choice
        id={id(key('mode'))}
        label="Mode"
        value={rule.mode}
        onValueChange={(mode) => onChange({ mode })}
        options={[
          { value: 'Auto', label: 'Auto' },
          { value: 'Manual', label: 'Manual' },
        ]}
      >
        <p className="text-caption text-text-secondary">
          {manual ? 'Someone listed below must approve.' : 'Access goes live without anyone approving.'}
        </p>
      </Choice>
      {manual && (
        <>
          <fieldset
            id={id(key('approvers'))}
            tabIndex={-1}
            data-invalid={errors[key('approvers')] ? 'true' : undefined}
            aria-describedby={errors[key('approvers')] ? `${id(key('approvers'))}-error` : undefined}
            className={cn('flex min-w-0 flex-col gap-3 rounded-sm', focusRing)}
          >
            <legend className="text-label text-text">Approvers (at least one group or user)</legend>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field label="Groups" htmlFor={id(key('approverGroups'))} hint={ONE_PER_LINE}>
                <Textarea
                  rows={2}
                  value={rule.approverGroups}
                  spellCheck={false}
                  onChange={(e) => onChange({ approverGroups: e.target.value })}
                />
              </Field>
              <Field label="Users" htmlFor={id(key('approverUsers'))} hint={ONE_PER_LINE}>
                <Textarea
                  rows={2}
                  value={rule.approverUsers}
                  spellCheck={false}
                  onChange={(e) => onChange({ approverUsers: e.target.value })}
                />
              </Field>
            </div>
            <GroupError id={`${id(key('approvers'))}-error`} error={errors[key('approvers')]} />
          </fieldset>
          <Field
            label="Approvals required"
            htmlFor={id(key('requiredApprovals'))}
            required
            hint="How many different people must approve"
            error={errors[key('requiredApprovals')]}
            className="sm:max-w-xs"
          >
            <Input
              type="number"
              inputMode="numeric"
              min={1}
              step={1}
              value={rule.requiredApprovals}
              onChange={(e) => onChange({ requiredApprovals: e.target.value })}
            />
          </Field>
          <Checkbox
            checked={rule.allowSelfApproval}
            onCheckedChange={(allowSelfApproval) => onChange({ allowSelfApproval })}
            label="Allow self-approval"
            description="A requester who is also an approver may approve their own request."
          />
        </>
      )}
      <Field
        label="Longest duration under this rule"
        htmlFor={id(key('maxDuration'))}
        hint={OPTIONAL_DURATION_HINT}
        error={errors[key('maxDuration')]}
        className="sm:max-w-xs"
      >
        <Input
          value={rule.maxDuration}
          autoComplete="off"
          spellCheck={false}
          onChange={(e) => onChange({ maxDuration: e.target.value })}
        />
      </Field>
    </li>
  );
}

interface EditorProps {
  /** The profile being edited; absent when creating. */
  profile?: ProfileDetailView;
  /** Namespaces the caller may create in; "*" means any. */
  namespaces?: string[];
  onReload?: () => void;
  /** Set when the live profile no longer matches the one this form was loaded from. */
  stale?: 'changed' | 'deleted' | null;
}

function ProfileEditorForm({ profile, namespaces = [], onReload, stale }: EditorProps) {
  const creating = !profile;
  const { actions } = useData();
  const { toast } = useToast();
  const navigate = useNavigate();
  const ids = useId();
  const id = (key: string) => `${ids}-${key.replace(/\./g, '-')}`;
  const formRef = useRef<HTMLFormElement>(null);
  const bannerRef = useRef<HTMLDivElement>(null);

  const anyNamespace = namespaces.includes('*');
  const [form, setForm] = useState<ProfileForm>(() => (profile ? specToForm(profile.spec) : newProfileForm()));
  const [namespace, setNamespace] = useState(() => (!anyNamespace && namespaces.length === 1 ? namespaces[0] : ''));
  const [name, setName] = useState('');
  const [submitted, setSubmitted] = useState(false);
  const [saving, setSaving] = useState(false);
  const [serverError, setServerError] = useState<{ status: number; message: string } | null>(null);

  const set = (patch: Partial<ProfileForm>) => setForm((f) => ({ ...f, ...patch }));
  const setRules = (update: (rules: RuleForm[]) => RuleForm[]) => setForm((f) => ({ ...f, rules: update(f.rules) }));

  const allErrors: Errors = {
    ...(creating ? validateNew(namespace.trim(), name.trim()) : {}),
    ...validateForm(form),
  };
  const errors: Errors = submitted ? allErrors : {};

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setSubmitted(true);
    if (Object.keys(allErrors).length) {
      requestAnimationFrame(() =>
        formRef.current?.querySelector<HTMLElement>('[aria-invalid="true"], [data-invalid="true"]')?.focus(),
      );
      return;
    }
    setSaving(true);
    setServerError(null);
    const spec = formToSpec(form);
    try {
      const saved = profile
        ? await actions.updateProfile(profile.namespace, profile.name, { resourceVersion: profile.resourceVersion, spec })
        : await actions.createProfile(namespace.trim(), { name: name.trim(), spec });
      toast({ title: profile ? 'Profile saved' : 'Profile created', tone: 'success' });
      navigate(profilePath(saved ?? { namespace: namespace.trim(), name: name.trim() }));
    } catch (err) {
      setServerError({
        status: err instanceof ApiError ? err.status : 0,
        message: errorMessage(err),
      });
      setSaving(false);
      requestAnimationFrame(() => bannerRef.current?.focus());
    }
  };

  const conflict = !creating && serverError?.status === 409;
  const cancelTo = profile ? profilePath(profile) : '/profiles';

  return (
    <form ref={formRef} noValidate onSubmit={submit} className="flex min-w-0 flex-col gap-6">
      {stale && !saving && !conflict && (
        <div
          role="status"
          className="flex flex-col gap-3 rounded-md border border-warning bg-warning-soft px-4 py-3 text-body text-warning-text sm:flex-row sm:items-start sm:justify-between"
        >
          <div className="flex min-w-0 items-start gap-2">
            <TriangleAlert className="mt-0.5 h-4 w-4 shrink-0" aria-hidden />
            <div className="flex min-w-0 flex-col gap-1">
              <span className="font-medium">This profile changed since you opened it</span>
              <span>
                {stale === 'deleted'
                  ? 'Someone deleted it. Saving will fail.'
                  : 'Someone saved a newer version. Saving now would be refused.'}{' '}
                Reload to see the current state. Your changes here will be lost.
              </span>
            </div>
          </div>
          {onReload && (
            <Button size="sm" icon={RefreshCw} onClick={onReload}>
              Reload profile
            </Button>
          )}
        </div>
      )}
      {serverError && (
        <div
          ref={bannerRef}
          tabIndex={-1}
          role="alert"
          className="flex flex-col gap-3 rounded-md border border-danger bg-danger-soft px-4 py-3 text-body text-danger-text sm:flex-row sm:items-start sm:justify-between"
        >
          <div className="flex min-w-0 items-start gap-2">
            <CircleAlert className="mt-0.5 h-4 w-4 shrink-0" aria-hidden />
            <div className="flex min-w-0 flex-col gap-1">
              <span className="font-medium">
                {conflict ? 'This profile changed since you opened it' : `The profile was not ${creating ? 'created' : 'saved'}`}
              </span>
              <span className="break-words">{serverError.message}</span>
              {conflict && <span>Reload to see the current version. Your changes here will be lost.</span>}
            </div>
          </div>
          {conflict && onReload && (
            <Button size="sm" icon={RefreshCw} onClick={onReload}>
              Reload profile
            </Button>
          )}
        </div>
      )}

      <Panel title="Profile">
        <div className="flex flex-col gap-4">
          {profile ? (
            <Facts>
              <Fact label="Namespace">
                <Identifier>{profile.namespace}</Identifier>
              </Fact>
              <Fact label="Name">
                <Identifier>{profile.name}</Identifier>
              </Fact>
            </Facts>
          ) : (
            <div className="grid gap-4 sm:grid-cols-2">
              {anyNamespace ? (
                <Field label="Namespace" htmlFor={id('namespace')} required error={errors.namespace}>
                  <Input value={namespace} autoComplete="off" spellCheck={false} onChange={(e) => setNamespace(e.target.value)} />
                </Field>
              ) : (
                <div
                  id={id('namespace')}
                  tabIndex={-1}
                  data-invalid={errors.namespace ? 'true' : undefined}
                  className={cn('flex min-w-0 flex-col gap-2 rounded-sm', focusRing)}
                >
                  <Select
                    label="Namespace (required)"
                    value={namespace}
                    onValueChange={setNamespace}
                    placeholder="Choose a namespace"
                    options={namespaces.map((n) => ({ value: n, label: n }))}
                  />
                  <GroupError id={`${id('namespace')}-error`} error={errors.namespace} />
                </div>
              )}
              <Field
                label="Name"
                htmlFor={id('name')}
                required
                hint="The name in Kubernetes. It can't be changed later."
                error={errors.name}
              >
                <Input value={name} autoComplete="off" spellCheck={false} onChange={(e) => setName(e.target.value)} />
              </Field>
            </div>
          )}
          <Field label="Display name" htmlFor={id('displayName')} required error={errors.displayName}>
            <Input value={form.displayName} onChange={(e) => set({ displayName: e.target.value })} />
          </Field>
          <Field label="Description" htmlFor={id('description')} hint="Optional. Tell requesters when to use this profile.">
            <Textarea rows={2} value={form.description} onChange={(e) => set({ description: e.target.value })} />
          </Field>
        </div>
      </Panel>

      <Panel title="Role and scope">
        <div className="flex flex-col gap-6">
          <div className="grid gap-4 sm:grid-cols-2">
            <Choice
              id={id('role.kind')}
              label="Role kind"
              value={form.roleKind}
              onValueChange={(roleKind) => set({ roleKind })}
              options={[
                { value: 'ClusterRole', label: 'ClusterRole' },
                { value: 'Role', label: 'Role' },
              ]}
            />
            <Field label="Role name" htmlFor={id('role.name')} required error={errors['role.name']}>
              <Input
                value={form.roleName}
                autoComplete="off"
                spellCheck={false}
                onChange={(e) => set({ roleName: e.target.value })}
              />
            </Field>
          </div>
          <Choice
            id={id('scope')}
            label="Scope"
            value={form.scope}
            onValueChange={(scope) => set({ scope })}
            options={[
              { value: 'Namespaced', label: 'Namespaced' },
              { value: 'Cluster', label: 'Cluster' },
            ]}
            error={errors.scope}
          >
            {form.scope === 'Cluster' && (
              <p className="text-body text-text">
                Grants the role across every namespace on each cluster. Needs a ClusterRole.
              </p>
            )}
          </Choice>
          {form.scope !== 'Cluster' && (
            <Field
              label="Namespaces"
              htmlFor={id('namespaces')}
              required
              hint="Where the role is bound on each cluster, one per line"
              error={errors.namespaces}
            >
              <Textarea rows={3} value={form.namespaces} spellCheck={false} onChange={(e) => set({ namespaces: e.target.value })} />
            </Field>
          )}
        </div>
      </Panel>

      <Panel title="Clusters" description="Choose clusters by name or by label, not both.">
        <div className="flex flex-col gap-4">
          <Choice
            id={id('clusters')}
            label="Cluster selection"
            value={form.clusterMode}
            onValueChange={(clusterMode) => set({ clusterMode })}
            options={[
              { value: 'names', label: 'By name' },
              { value: 'selector', label: 'By label' },
            ]}
            error={errors.clusters}
          />
          {form.clusterMode === 'names' ? (
            <Field label="Cluster names" htmlFor={id('clusterNames')} hint={ONE_PER_LINE}>
              <Textarea rows={3} value={form.clusterNames} spellCheck={false} onChange={(e) => set({ clusterNames: e.target.value })} />
            </Field>
          ) : (
            <KeyValueRows
              id={id('clusterLabels')}
              legend="Cluster labels"
              hint="Every registered cluster with all of these labels. Add at least one."
              rows={form.clusterLabels}
              onChange={(clusterLabels) => set({ clusterLabels })}
              error={errors.clusterLabels}
            >
              <KeptExpressions expressions={form.clusterExpressions} />
            </KeyValueRows>
          )}
        </div>
      </Panel>

      <Panel title="Request settings">
        <div className="flex flex-col gap-4">
          <fieldset
            id={id('requesters')}
            tabIndex={-1}
            data-invalid={errors.requesters ? 'true' : undefined}
            aria-describedby={errors.requesters ? `${id('requesters')}-error` : undefined}
            className={cn('flex min-w-0 flex-col gap-3 rounded-sm', focusRing)}
          >
            <legend className="text-label text-text">Requesters (at least one group or user)</legend>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field label="Requester groups" htmlFor={id('requesterGroups')} hint="Members of these groups can request this profile. One per line.">
                <Textarea rows={2} value={form.requesterGroups} spellCheck={false} onChange={(e) => set({ requesterGroups: e.target.value })} />
              </Field>
              <Field label="Requester users" htmlFor={id('requesterUsers')} hint="Usernames or emails that can request this profile. One per line.">
                <Textarea rows={2} value={form.requesterUsers} spellCheck={false} onChange={(e) => set({ requesterUsers: e.target.value })} />
              </Field>
            </div>
            <GroupError id={`${id('requesters')}-error`} error={errors.requesters} />
          </fieldset>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field
              label="Default duration"
              htmlFor={id('duration.default')}
              required
              hint={DURATION_HINT}
              error={errors['duration.default']}
            >
              <Input value={form.defaultDuration} autoComplete="off" spellCheck={false} onChange={(e) => set({ defaultDuration: e.target.value })} />
            </Field>
            <Field
              label="Longest duration"
              htmlFor={id('duration.max')}
              required
              hint={DURATION_HINT}
              error={errors['duration.max']}
            >
              <Input value={form.maxDuration} autoComplete="off" spellCheck={false} onChange={(e) => set({ maxDuration: e.target.value })} />
            </Field>
          </div>
          <Choice
            id={id('reason')}
            label="Reason"
            value={form.reason}
            onValueChange={(reason) => set({ reason })}
            options={[
              { value: 'Required', label: 'Required' },
              { value: 'Optional', label: 'Optional' },
            ]}
          />
          <Field
            label="Ticket pattern"
            htmlFor={id('ticketPattern')}
            hint={
              <>
                Optional. A regular expression every ticket must match, such as <Identifier>^(INC|OPS)-[0-9]+$</Identifier>.
              </>
            }
            error={errors.ticketPattern}
          >
            <Input
              value={form.ticketPattern}
              autoComplete="off"
              spellCheck={false}
              mono
              onChange={(e) => set({ ticketPattern: e.target.value })}
            />
          </Field>
        </div>
      </Panel>

      <Panel
        title="Approval"
        description="Rules are checked in order; the first rule that matches a cluster decides for it. A cluster that matches no rule is rejected."
      >
        <div className="flex flex-col gap-6">
          <Field
            label="Pending timeout"
            htmlFor={id('pendingTimeout')}
            hint="Optional. A request nobody approves within this time expires. Use a duration such as 24h."
            error={errors.pendingTimeout}
            className="sm:max-w-xs"
          >
            <Input value={form.pendingTimeout} autoComplete="off" spellCheck={false} onChange={(e) => set({ pendingTimeout: e.target.value })} />
          </Field>
          <div
            id={id('rules')}
            tabIndex={-1}
            data-invalid={errors.rules ? 'true' : undefined}
            className={cn('flex min-w-0 flex-col gap-4 rounded-sm', focusRing)}
          >
            {form.rules.length > 0 && (
              <ol aria-label="Approval rules" className="flex flex-col divide-y divide-border">
                {form.rules.map((rule, i) => (
                  <RuleEditor
                    key={rule.key}
                    rule={rule}
                    index={i}
                    count={form.rules.length}
                    id={id}
                    errors={errors}
                    onChange={(patch) => setRules((rules) => rules.map((r, j) => (j === i ? { ...r, ...patch } : r)))}
                    onMove={(by) =>
                      setRules((rules) => {
                        const next = [...rules];
                        [next[i], next[i + by]] = [next[i + by], next[i]];
                        return next;
                      })
                    }
                    onRemove={() => setRules((rules) => rules.filter((_, j) => j !== i))}
                  />
                ))}
              </ol>
            )}
            <GroupError id={`${id('rules')}-error`} error={errors.rules} />
            <Button icon={Plus} className="self-start" onClick={() => setRules((rules) => [...rules, newRuleForm()])}>
              Add rule
            </Button>
          </div>
        </div>
      </Panel>

      <div className="flex flex-col-reverse gap-3 sm:flex-row sm:items-center sm:justify-end">
        <Link to={cancelTo} className={buttonClass('ghost')}>
          Cancel
        </Link>
        <Button type="submit" variant="primary" loading={saving}>
          {creating ? 'Create profile' : 'Save changes'}
        </Button>
      </div>
    </form>
  );
}

function LoadingPanel() {
  return (
    <Panel>
      <div className="flex flex-col gap-3" aria-busy="true">
        <span className="sr-only">Loading</span>
        <Skeleton className="h-5 w-1/3" />
        <Skeleton className="w-2/3" />
        <Skeleton className="w-1/2" />
      </div>
    </Panel>
  );
}

function NotEditable({ title, body, back }: { title: string; body: ReactNode; back: string }) {
  return (
    <Panel>
      <EmptyState
        icon={Lock}
        title={title}
        body={body}
        action={
          <Link to={back} className={buttonClass('secondary')}>
            Go back
          </Link>
        }
        className="py-6"
      />
    </Panel>
  );
}

/** True from the first render where `on` holds: losing a right mid-edit must not unmount the form and drop input. */
function useLatch(on: boolean): boolean {
  const latched = useRef(false);
  if (on) latched.current = true;
  return latched.current;
}

function NewProfile() {
  const { capabilities } = useData();
  const allowed = useLatch(canCreateProfile(capabilities));
  return (
    <>
      <PageHeader title="New profile" description="Define a role people can request, where it applies, and who approves it." />
      {allowed ? (
        <ProfileEditorForm namespaces={capabilities?.profileNamespaces ?? []} />
      ) : (
        <NotEditable
          title="You can't create profiles here"
          body="Profiles are managed outside this UI, or you have no namespace to create them in."
          back="/profiles"
        />
      )}
    </>
  );
}

function EditProfile({ ns, name }: { ns: string; name: string }) {
  const { capabilities, profiles, loading: polling } = useData();
  const { data: profile, error, loading, reload } = useLoad(() => api.getProfile(ns, name), []);
  const live = useLive(() => api.getProfile(ns, name), []);
  const allowed = useLatch(Boolean(profile && canEditProfile(capabilities, profile)));
  const deleted =
    (live.error instanceof ApiError && live.error.status === 404) ||
    (!polling && !profiles.some((p) => p.namespace === ns && p.name === name));
  const changed = Boolean(profile && live.data && live.data.resourceVersion !== profile.resourceVersion);
  const stale = deleted ? 'deleted' : changed ? 'changed' : null;

  if (loading) {
    return (
      <>
        <PageHeader title="Profile" />
        <LoadingPanel />
      </>
    );
  }

  if (!profile) {
    const missing = error instanceof ApiError && error.status === 404;
    return (
      <>
        <PageHeader title="Profile" />
        <Panel>
          {missing || !error ? (
            <EmptyState
              icon={FileQuestion}
              title="Profile not found"
              body={
                <>
                  There is no profile <Identifier>{name}</Identifier> in <Identifier>{ns}</Identifier>.
                </>
              }
              action={
                <Link to="/profiles" className={buttonClass('primary')}>
                  View all profiles
                </Link>
              }
              className="py-6"
            />
          ) : (
            <LoadFailed what="this profile" error={error} retry={reload} />
          )}
        </Panel>
      </>
    );
  }

  const title = profile.displayName || profile.name;
  return (
    <>
      <PageHeader
        title={title}
        description={
          <>
            Editing <Identifier>{`${profile.namespace}/${profile.name}`}</Identifier>
          </>
        }
      />
      {allowed ? (
        <ProfileEditorForm
          key={profile.resourceVersion}
          profile={profile}
          onReload={() => {
            reload();
            live.reload();
          }}
          stale={stale}
        />
      ) : (
        <NotEditable
          title="You can't edit this profile here"
          body={
            profile.managedBy
              ? `Managed by ${profile.managedBy}. Change it in Git; edits here would be reverted.`
              : 'Editing profiles is turned off, or you may not change profiles in this namespace.'
          }
          back={profilePath(profile)}
        />
      )}
    </>
  );
}

export default function ProfileEditor() {
  const { ns, name } = useParams();
  return (
    <div className="flex w-full max-w-[87.5rem] flex-col gap-6">
      {ns && name ? <EditProfile key={`${ns}/${name}`} ns={ns} name={name} /> : <NewProfile />}
    </div>
  );
}
