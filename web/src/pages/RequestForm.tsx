import { useId, useRef, useState, type FormEvent } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { CircleAlert, ShieldOff } from 'lucide-react';
import { errorMessage } from '../api';
import { useData } from '../app/data';
import type { ClusterView, ProfileView } from '../generated/views';
import {
  Button,
  Checkbox,
  EmptyState,
  Field,
  Identifier,
  Input,
  PageHeader,
  Panel,
  SegmentedControl,
  Skeleton,
  Textarea,
  buttonClass,
  useToast,
} from '../components/ui';
import { formatDuration, plural, profilePath, requestPath } from '../app/format';
import { approvers, defaultClusters, defaultDuration, durationOptions, ticketError } from './access/model';
import { Fact, Facts, IdList } from './access/parts';
import { describeActivation } from './home/home';

const NONE_READY = "None of this profile's clusters is ready right now. Try again later.";

function Summary({ profile, clusterCount }: { profile: ProfileView; clusterCount: number }) {
  return (
    <Panel as="aside" title="Summary" className="lg:sticky lg:top-8 lg:order-last lg:self-start">
      <Facts>
        <Fact label="Approvers" wide>
          {describeActivation(profile).mode === 'at-once' ? (
            'Nobody. Access goes live as soon as you submit.'
          ) : (
            <IdList items={approvers(profile)} empty="Not listed on the profile" />
          )}
        </Fact>
        <Fact label="Longest duration" wide>
          <span className="tabular-nums">{formatDuration(profile.maxDuration)}</span>
        </Fact>
        <Fact label="Access" wide>
          <Identifier>{profile.role.name}</Identifier> ({profile.role.kind}){' '}
          {profile.namespaces?.length ? (
            <>
              in <IdList items={profile.namespaces} empty="" />
            </>
          ) : (
            'across all namespaces'
          )}{' '}
          on {plural(clusterCount, 'cluster')}
        </Fact>
      </Facts>
    </Panel>
  );
}

function ProfileForm({ profile, clusters }: { profile: ProfileView; clusters: ClusterView[] }) {
  const { actions } = useData();
  const { toast } = useToast();
  const navigate = useNavigate();
  const ids = useId();
  const allowed = defaultClusters(profile);
  const options = durationOptions(profile);

  const [picked, setPicked] = useState<string[]>(allowed);
  const chosen = picked.filter((s) => allowed.includes(s));
  const [duration, setDuration] = useState(() => defaultDuration(profile));
  const [ticket, setTicket] = useState('');
  const [reason, setReason] = useState('');
  const [touched, setTouched] = useState({ ticket: false, reason: false });
  const [submitted, setSubmitted] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [serverError, setServerError] = useState<string | null>(null);
  const bannerRef = useRef<HTMLDivElement>(null);

  const errors = {
    clusters: !allowed.length ? NONE_READY : !chosen.length ? 'Choose at least one cluster.' : undefined,
    ticket: ticketError(profile.ticketPattern, ticket),
    reason: profile.reasonRequired && !reason.trim() ? 'Enter a reason. Approvers and the audit log see it.' : undefined,
  };
  const shown = {
    clusters: errors.clusters,
    ticket: touched.ticket || submitted ? errors.ticket : undefined,
    reason: touched.reason || submitted ? errors.reason : undefined,
  };

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setSubmitted(true);
    if (!allowed.length) return;
    const firstInvalid = (['clusters', 'ticket', 'reason'] as const).find((k) => errors[k]);
    if (firstInvalid) {
      document.getElementById(`${ids}-${firstInvalid}`)?.focus();
      return;
    }
    setSubmitting(true);
    setServerError(null);
    try {
      const created = await actions.createRequest(profile.namespace, {
        profileRef: profile.name,
        duration,
        reason: reason.trim(),
        ticket: ticket.trim() || undefined,
        targetNamespaces: profile.namespaces?.length ? profile.namespaces : undefined,
        targetClusters: chosen,
      });
      toast({
        title: describeActivation(profile).mode === 'at-once' ? 'Request sent. Access goes live in a moment.' : 'Request sent for approval',
        tone: 'success',
      });
      navigate(requestPath(created), { state: { request: created } });
    } catch (err) {
      setServerError(errorMessage(err));
      setSubmitting(false);
      requestAnimationFrame(() => bannerRef.current?.focus());
    }
  };

  const clusterByName = new Map(clusters.map((c) => [c.name, c]));

  return (
    <div className="grid gap-6 lg:grid-cols-3">
      <Summary profile={profile} clusterCount={chosen.length} />
      <form noValidate onSubmit={submit} className="flex min-w-0 flex-col gap-6 lg:col-span-2">
        {serverError && (
          <div
            ref={bannerRef}
            tabIndex={-1}
            role="alert"
            className="flex items-start gap-2 rounded-md border border-danger bg-danger-soft px-4 py-3 text-body text-danger-text"
          >
            <CircleAlert className="mt-0.5 h-4 w-4 shrink-0" aria-hidden />
            <span className="flex min-w-0 flex-col gap-1">
              <span className="font-medium">Your request was not sent.</span>
              <span className="break-words">{serverError}</span>
            </span>
          </div>
        )}
        <Panel title="Clusters" description={allowed.length ? 'Access is set up on each cluster you choose.' : undefined}>
          <fieldset className="flex flex-col gap-3" aria-describedby={shown.clusters ? `${ids}-clusters-error` : undefined}>
            <legend className="sr-only">Clusters</legend>
            {allowed.map((name, i) => (
              <Checkbox
                key={name}
                id={i === 0 ? `${ids}-clusters` : undefined}
                checked={chosen.includes(name)}
                onCheckedChange={(on) => setPicked(on ? [...chosen, name] : chosen.filter((s) => s !== name))}
                label={clusterByName.get(name)?.displayName || name}
                description={<Identifier>{name}</Identifier>}
              />
            ))}
            {shown.clusters && (
              <p id={`${ids}-clusters-error`} className="flex items-start gap-1 text-caption text-danger-text">
                <CircleAlert className="mt-px h-4 w-4 shrink-0" aria-hidden />
                {shown.clusters}
              </p>
            )}
          </fieldset>
        </Panel>
        <Panel title="Duration" description={`Access ends on its own after this time. Up to ${formatDuration(profile.maxDuration)}.`}>
          <SegmentedControl
            label="Duration"
            value={duration}
            onValueChange={setDuration}
            options={options.map((d) => ({ value: d, label: formatDuration(d) }))}
          />
        </Panel>
        <Panel title="Justification">
          <div className="flex flex-col gap-4">
            <Field
              label="Ticket"
              htmlFor={`${ids}-ticket`}
              required={Boolean(profile.ticketPattern)}
              hint={
                profile.ticketPattern ? (
                  <>
                    Must match <Identifier>{profile.ticketPattern}</Identifier>
                  </>
                ) : (
                  'Optional. An incident or change ID helps approvers decide.'
                )
              }
              error={shown.ticket}
            >
              <Input
                value={ticket}
                maxLength={64}
                autoComplete="off"
                spellCheck={false}
                onChange={(e) => setTicket(e.target.value)}
                onBlur={() => setTouched((t) => ({ ...t, ticket: true }))}
              />
            </Field>
            <Field
              label="Reason"
              htmlFor={`${ids}-reason`}
              required={profile.reasonRequired}
              hint="What you need to do and why. Approvers and the audit log see this."
              error={shown.reason}
            >
              <Textarea
                rows={4}
                value={reason}
                onChange={(e) => setReason(e.target.value)}
                onBlur={() => setTouched((t) => ({ ...t, reason: true }))}
              />
            </Field>
          </div>
        </Panel>
        <div className="flex flex-col-reverse gap-3 sm:flex-row sm:items-center sm:justify-end">
          <Link to="/request" className={buttonClass('ghost')}>
            Choose another profile
          </Link>
          {!allowed.length && (
            <p id={`${ids}-blocked`} className="text-body text-text-secondary">
              Can't send yet: none of this profile's clusters is ready.
            </p>
          )}
          <Button
            type="submit"
            variant="primary"
            loading={submitting}
            disabled={!allowed.length}
            aria-describedby={allowed.length ? undefined : `${ids}-blocked`}
          >
            Request access
          </Button>
        </div>
      </form>
    </div>
  );
}

export default function RequestForm() {
  const { ns = '', profile: name = '' } = useParams();
  const { profiles, clusters, loading } = useData();
  const profile = profiles.find((p) => p.namespace === ns && p.name === name && p.canRequest);

  if (loading) {
    return (
      <>
        <PageHeader title="New request" />
        <Panel>
          <div className="flex flex-col gap-3">
            <Skeleton className="h-5 w-1/3" />
            <Skeleton className="w-2/3" />
            <Skeleton className="w-1/2" />
          </div>
        </Panel>
      </>
    );
  }

  if (!profile) {
    return (
      <>
        <PageHeader title="New request" />
        <Panel>
          <EmptyState
            icon={ShieldOff}
            title="This profile isn't available to you"
            body={
              <>
                There is no profile <Identifier>{name}</Identifier> you can request in{' '}
                <Identifier>{ns}</Identifier>.
              </>
            }
            action={
              <Link to="/request" className={buttonClass('primary')}>
                Browse profiles
              </Link>
            }
            className="py-6"
          />
        </Panel>
      </>
    );
  }

  return (
    <>
      <PageHeader
        title={profile.displayName || profile.name}
        description={profile.description || 'Request time-boxed access with this profile.'}
        actions={
          <Link to={profilePath(profile)} className={buttonClass('secondary')}>
            View profile
          </Link>
        }
      />
      <ProfileForm key={`${ns}/${name}`} profile={profile} clusters={clusters} />
    </>
  );
}
