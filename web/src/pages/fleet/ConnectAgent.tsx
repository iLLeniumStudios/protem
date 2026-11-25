import { useEffect, useState, type RefObject } from 'react';
import { Check, CircleAlert, CircleCheck, Copy, Download, LoaderCircle, TriangleAlert } from 'lucide-react';
import { errorMessage } from '../../api';
import { useData } from '../../app/data';
import { formatClock } from '../../app/format';
import type { ClusterView, JoinView } from '../../generated/views';
import { Button, Identifier, cn, useToast } from '../../components/ui';
import { focusRing } from '../../components/ui/cn';
import { agentJoined, joinCountdown } from './enrol';

export const connectTitle = (displayName: string) => `Connect the agent to ${displayName}`;

type AgentSeen = Pick<ClusterView, 'agent' | 'lastHeartbeat'>;

/** Whether the agent reported in after `join` was issued; the state at issue is read from the snapshot refreshed by the issuing action. */
export function useAgentJoined(join: JoinView | null): boolean {
  const { clusters } = useData();
  const cluster = join ? clusters.find((c) => c.name === join.cluster) : undefined;
  const [atIssue, setAtIssue] = useState<{ join: JoinView | null; seen?: AgentSeen }>({ join: null });
  if (atIssue.join !== join) {
    setAtIssue({ join, seen: cluster && { agent: cluster.agent, lastHeartbeat: cluster.lastHeartbeat } });
  }
  return Boolean(join) && atIssue.join === join && agentJoined(cluster, atIssue.seen);
}

function useSecondTick() {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const id = setInterval(() => setNow(new Date()), 1000);
    return () => clearInterval(id);
  }, []);
  return now;
}

function download(filename: string, text: string) {
  const url = URL.createObjectURL(new Blob([text], { type: 'text/plain' }));
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 0);
}

function Block({ title, text, filename }: { title: string; text: string; filename?: string }) {
  const { toast } = useToast();
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      toast({ title: `Couldn't copy. Select the ${title} text and copy it.`, tone: 'danger' });
    }
  };
  return (
    <section className="flex min-w-0 flex-col gap-2">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="font-mono text-label text-text">{title}</h3>
        <div className="flex flex-wrap gap-2">
          <Button size="sm" icon={copied ? Check : Copy} onClick={copy}>
            {copied ? 'Copied' : 'Copy'}
            <span className="sr-only"> {title}</span>
          </Button>
          {filename && (
            <Button size="sm" icon={Download} onClick={() => download(filename, text)}>
              Download {filename}
            </Button>
          )}
        </div>
      </div>
      <pre
        tabIndex={0}
        aria-label={title}
        className={cn('max-h-48 overflow-auto rounded-md bg-subtle p-3 font-mono text-caption text-text', focusRing)}
      >
        {text}
      </pre>
    </section>
  );
}

function AgentProgress({ joined }: { joined: boolean }) {
  return (
    <p role="status" className="flex items-center gap-2 text-body">
      {joined ? (
        <>
          <CircleCheck className="h-4 w-4 shrink-0 text-success-text" aria-hidden />
          <span className="font-medium text-success-text">Agent connected</span>
        </>
      ) : (
        <>
          <LoaderCircle className="h-4 w-4 shrink-0 animate-spin text-text-secondary motion-reduce:animate-none" aria-hidden />
          <span className="text-text-secondary">Waiting for the agent</span>
        </>
      )}
    </p>
  );
}

export interface ConnectAgentProps {
  join: JoinView;
  joined: boolean;
  /** Receives the replacement when an expired join token is reissued. */
  onJoin: (join: JoinView) => void;
  introRef: RefObject<HTMLParagraphElement>;
}

export function ConnectAgent({ join, joined, onJoin, introRef }: ConnectAgentProps) {
  const { actions, clusters } = useData();
  const boundAs = clusters.find((c) => c.name === join.cluster)?.boundAs;
  const now = useSecondTick();
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const { expired, left } = joinCountdown(join.expiresAt, now);
  const until = new Date(join.expiresAt);

  const issueNew = async () => {
    setPending(true);
    setError(null);
    try {
      onJoin(await actions.joinCluster(join.cluster));
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setPending(false);
    }
  };

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-3">
        <p ref={introRef} tabIndex={-1} className={cn('rounded-sm text-body text-text', focusRing)}>
          Save both files in one directory, then run the command there with kubectl pointed at the target cluster.
        </p>
        <AgentProgress joined={joined} />
      </div>
      {expired ? (
        <div className="flex flex-col items-start gap-3 rounded-md border border-border bg-subtle px-4 py-3">
          <p className="flex flex-col gap-1 text-body">
            <span className="font-medium text-text">This join token has expired</span>
            <span className="text-text-secondary">Issue a new one and use the new files in place of any you saved.</span>
          </p>
          <Button variant="primary" loading={pending} onClick={issueNew}>
            Issue a new join token
          </Button>
          {error && (
            <p role="alert" className="flex items-start gap-1 text-caption text-danger-text">
              <CircleAlert className="mt-px h-4 w-4 shrink-0" aria-hidden />
              <span className="break-words">{error}</span>
            </p>
          )}
        </div>
      ) : (
        <>
          <Block title="hub.kubeconfig" text={join.kubeconfig} filename="hub.kubeconfig" />
          <Block title="kustomization.yaml" text={join.kustomization} filename="kustomization.yaml" />
          <Block title="Command" text={join.command} />
          <div className="flex flex-col gap-2 text-body">
            <p className="text-text-secondary">
              The join token works until{' '}
              <time dateTime={until.toISOString()} className="tabular-nums">
                {formatClock(until)}
              </time>{' '}
              (<span className="tabular-nums">{left}</span>). The agent replaces it with its own a few minutes after it starts.
            </p>
            <p className="flex items-start gap-2 text-warning-text">
              <TriangleAlert className="mt-0.5 h-4 w-4 shrink-0" aria-hidden />
              <span>This is shown once. Anyone holding hub.kubeconfig before it expires can act as this cluster's agent.</span>
            </p>
          </div>
        </>
      )}
      {joined && boundAs && (
        <div className="flex flex-col gap-3">
          <p className="text-body font-medium text-text">Check how this cluster names you</p>
          <Block title="Check command" text="kubectl auth whoami" />
          <p className="min-w-0 text-body text-text-secondary">
            It should print <Identifier className="text-text">{boundAs}</Identifier>. If it prints something else, change Subject in this
            cluster's settings; otherwise grants here will show as active but give no access.
          </p>
        </div>
      )}
    </div>
  );
}

export function ConnectDone({ joined, onClose }: { joined: boolean; onClose: () => void }) {
  return (
    <Button variant={joined ? 'primary' : 'secondary'} onClick={onClose}>
      {joined ? 'Done' : 'Close'}
    </Button>
  );
}
