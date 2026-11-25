import { useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { Check, CircleCheck, Clock, Copy, FileQuestion } from 'lucide-react';
import { ApiError, api } from '../api';
import { useData } from '../app/data';
import { useLive } from '../app/load';
import { formatDuration, profilePath, requestFormPath } from '../app/format';
import type { ApprovalRule, ClusterTargeting } from '../generated/crd';
import type { ProfileDetailView } from '../generated/views';
import {
  Badge,
  Button,
  ConfirmDialog,
  EmptyState,
  Identifier,
  MobileRow,
  PageHeader,
  Panel,
  Skeleton,
  buttonClass,
  cn,
  useToast,
} from '../components/ui';
import { focusRing } from '../components/ui/cn';
import { Fact, Facts, IdList, LoadFailed } from './access/parts';
import { Permissions } from './permissions/Permissions';
import { canEditProfile } from './profiles/manage';
import { scopeText, selectorTerms } from './profiles/profile';

function DeleteProfile({ profile }: { profile: ProfileDetailView }) {
  const { actions } = useData();
  const { toast } = useToast();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const title = profile.displayName || profile.name;
  return (
    <>
      <Button variant="reject" onClick={() => setOpen(true)}>
        Delete profile
      </Button>
      <ConfirmDialog
        open={open}
        onOpenChange={setOpen}
        title={`Delete ${title}?`}
        body={
          <>
            The profile <Identifier>{`${profile.namespace}/${profile.name}`}</Identifier> is removed. Pending requests for it
            will be rejected, and grants already issued keep running until they expire. This can't be undone.
          </>
        }
        confirmLabel="Delete profile"
        tone="danger"
        onConfirm={async () => {
          await actions.deleteProfile(profile.namespace, profile.name);
          toast({ title: `${title} deleted`, tone: 'success' });
          navigate('/profiles');
        }}
      />
    </>
  );
}

const HUB_SOURCE = 'As defined on the hub cluster. A cluster can define the same role differently.';

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

function WhatItGrants({ profile }: { profile: ProfileDetailView }) {
  const permissions = useLive(() => api.getProfilePermissions(profile.namespace, profile.name), []);
  return (
    <Panel title="Permissions">
      <div className="flex flex-col gap-6">
        <Facts>
          <Fact label="Role">
            <Identifier>{profile.role.name}</Identifier> <span className="text-text-secondary">({profile.role.kind})</span>
          </Fact>
          <Fact label="Scope">
            {scopeText(profile.scope)}
            {profile.scope !== 'Cluster' && (
              <>
                {' in '}
                <IdList items={profile.namespaces} empty="no namespaces" />
              </>
            )}
          </Fact>
        </Facts>
        <section aria-labelledby="profile-permissions" className="flex flex-col gap-3 border-t border-border pt-6">
          <h3 id="profile-permissions" className="text-label text-text-secondary">
            Permission rules
          </h3>
          {permissions.loading ? (
            <Skeleton className="w-2/3" />
          ) : permissions.error ? (
            <LoadFailed what="the permissions" error={permissions.error} retry={permissions.reload} />
          ) : (
            permissions.data && <Permissions view={permissions.data} source={HUB_SOURCE} />
          )}
        </section>
      </div>
    </Panel>
  );
}

function ChosenBy({ clusters }: { clusters: ClusterTargeting }) {
  if (clusters.names?.length) {
    return (
      <>
        By name: <IdList items={clusters.names} empty="" />
      </>
    );
  }
  const terms = selectorTerms(clusters.selector);
  if (!terms.length) return <>Every registered cluster (the label selector is empty)</>;
  return (
    <>
      Clusters with labels <IdList items={terms} empty="" />
    </>
  );
}

function Where({ profile }: { profile: ProfileDetailView }) {
  return (
    <Panel title="Clusters">
      <Facts>
        <Fact label="Clusters it applies to now">
          <IdList items={profile.clusters} empty="No ready cluster right now" />
        </Fact>
        <Fact label="How clusters are chosen">
          <ChosenBy clusters={profile.spec.clusters} />
        </Fact>
      </Facts>
    </Panel>
  );
}

function WhoCanRequest({ profile }: { profile: ProfileDetailView }) {
  return (
    <Panel title="Request settings">
      <Facts>
        <Fact label="Requester groups" wide>
          <IdList items={profile.spec.requesters.groups} empty="None" />
        </Fact>
        <Fact label="Requester users" wide>
          <IdList items={profile.spec.requesters.users} empty="None" />
        </Fact>
        <Fact label="Default duration">
          <span className="tabular-nums">{formatDuration(profile.defaultDuration)}</span>
        </Fact>
        <Fact label="Longest duration">
          <span className="tabular-nums">{formatDuration(profile.maxDuration)}</span>
        </Fact>
        <Fact label="Reason">{profile.reasonRequired ? 'Required' : 'Optional'}</Fact>
        <Fact label="Ticket pattern">
          {profile.ticketPattern ? (
            <Identifier>{profile.ticketPattern}</Identifier>
          ) : (
            <span className="text-text-secondary">None; a ticket is optional</span>
          )}
        </Fact>
      </Facts>
    </Panel>
  );
}

function AppliesWhen({ rule }: { rule: ApprovalRule }) {
  const terms = selectorTerms(rule.when?.clusterSelector);
  const groups = rule.when?.requesterGroups ?? [];
  const users = rule.when?.requesterUsers ?? [];
  if (!terms.length && !groups.length && !users.length) return <>Every request</>;
  return (
    <span className="flex flex-col gap-1">
      {terms.length > 0 && (
        <span>
          Clusters with labels <IdList items={terms} empty="" />
        </span>
      )}
      {groups.length > 0 && (
        <span>
          Requesters in <IdList items={groups} empty="" />
        </span>
      )}
      {users.length > 0 && (
        <span>
          {groups.length > 0 ? 'Or requested by' : 'Requested by'} <IdList items={users} empty="" />
        </span>
      )}
    </span>
  );
}

function ApprovalRuleItem({ rule, index }: { rule: ApprovalRule; index: number }) {
  const manual = rule.mode === 'Manual';
  return (
    <MobileRow>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <span className="text-label text-text-secondary tabular-nums">Rule {index + 1}</span>
        <Identifier className="min-w-0 text-body font-medium text-text">{rule.name}</Identifier>
        {manual ? (
          <Badge tone="warning">
            <Clock className="h-4 w-4 shrink-0" aria-hidden />
            Manual approval
          </Badge>
        ) : (
          <Badge tone="success">
            <CircleCheck className="h-4 w-4 shrink-0" aria-hidden />
            No approval needed
          </Badge>
        )}
      </div>
      <Facts>
        <Fact label="Applies to" wide>
          <AppliesWhen rule={rule} />
        </Fact>
        {manual && (
          <>
            <Fact label="Approver groups">
              <IdList items={rule.approvers?.groups} empty="None" />
            </Fact>
            <Fact label="Approver users">
              <IdList items={rule.approvers?.users} empty="None" />
            </Fact>
            <Fact label="Approvals required">
              <span className="tabular-nums">{rule.requiredApprovals ?? 1}</span>
            </Fact>
            <Fact label="Self-approval">{rule.allowSelfApproval ? 'Allowed' : 'Not allowed'}</Fact>
          </>
        )}
        {rule.maxDuration && (
          <Fact label="Longest duration under this rule">
            <span className="tabular-nums">{formatDuration(rule.maxDuration)}</span>
          </Fact>
        )}
      </Facts>
    </MobileRow>
  );
}

function Approval({ profile }: { profile: ProfileDetailView }) {
  const { pendingTimeout, rules } = profile.spec.approval;
  return (
    <Panel
      title="Approval"
      description={
        <>
          {pendingTimeout
            ? `A request nobody approves within ${formatDuration(pendingTimeout)} expires. `
            : 'No pending timeout is set. '}
          Rules are checked in order; the first that matches a cluster decides for it.
        </>
      }
    >
      <ol aria-label="Approval rules" className="flex flex-col divide-y divide-border">
        {(rules ?? []).map((rule, i) => (
          <ApprovalRuleItem key={rule.name} rule={rule} index={i} />
        ))}
      </ol>
    </Panel>
  );
}

function Yaml({ yaml }: { yaml: string }) {
  const { toast } = useToast();
  const [copied, setCopied] = useState(false);
  const [open, setOpen] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(yaml);
      setCopied(true);
      toast({ title: 'YAML copied to the clipboard', tone: 'success' });
      setTimeout(() => setCopied(false), 2000);
    } catch {
      setOpen(true);
      toast({ title: "Couldn't copy. Select the YAML below and copy it.", tone: 'danger' });
    }
  };
  return (
    <Panel
      title="YAML"
      description="The manifest for this profile, ready to commit to a GitOps repository."
      actions={
        <Button icon={copied ? Check : Copy} onClick={copy}>
          {copied ? 'Copied' : 'Copy YAML'}
        </Button>
      }
    >
      <details open={open} onToggle={(e) => setOpen(e.currentTarget.open)}>
        <summary className={cn('w-fit cursor-pointer rounded-sm text-body font-medium text-accent-text', focusRing)}>
          {open ? 'Hide YAML' : 'Show YAML'}
        </summary>
        <pre
          tabIndex={0}
          aria-label="Profile YAML"
          className={cn('mt-3 max-h-96 overflow-auto rounded-md bg-subtle p-4 font-mono text-caption text-text', focusRing)}
        >
          {yaml}
        </pre>
      </details>
    </Panel>
  );
}

function ProfilePage({ ns, name }: { ns: string; name: string }) {
  const { capabilities } = useData();
  const { data: profile, error, loading, reload } = useLive(() => api.getProfile(ns, name), []);

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
                <Link to="/request" className={buttonClass('primary')}>
                  Browse profiles
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

  return (
    <>
      <PageHeader
        title={profile.displayName || profile.name}
        description={
          <span className="flex flex-col gap-2">
            {profile.description && <span>{profile.description}</span>}
            <span className="flex flex-wrap items-center gap-2">
              <Identifier className="min-w-0">
                {profile.namespace}/{profile.name}
              </Identifier>
              {profile.managedBy && !capabilities?.management && <Badge tone="info">Managed by {profile.managedBy}</Badge>}
            </span>
            {profile.managedBy && capabilities?.management && (
              <span>Managed by {profile.managedBy}. Change it in Git; edits here would be reverted.</span>
            )}
          </span>
        }
        actions={
          (profile.canRequest || canEditProfile(capabilities, profile)) && (
            <>
              {profile.canRequest && (
                <Link to={requestFormPath(profile)} className={buttonClass('primary')}>
                  Request access
                </Link>
              )}
              {canEditProfile(capabilities, profile) && (
                <>
                  <Link to={`${profilePath(profile)}/edit`} className={buttonClass('secondary')}>
                    Edit profile
                  </Link>
                  <DeleteProfile profile={profile} />
                </>
              )}
            </>
          )
        }
      />
      <WhatItGrants profile={profile} />
      <Where profile={profile} />
      <WhoCanRequest profile={profile} />
      <Approval profile={profile} />
      <Yaml yaml={profile.yaml} />
    </>
  );
}

export default function ProfileDetail() {
  const { ns = '', name = '' } = useParams();
  return (
    <div className="mx-auto flex w-full max-w-[87.5rem] flex-col gap-6">
      <ProfilePage key={`${ns}/${name}`} ns={ns} name={name} />
    </div>
  );
}
