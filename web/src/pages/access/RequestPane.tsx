import { useId, useRef, useState } from 'react';
import { Link, useLocation, useNavigate } from 'react-router-dom';
import { Check, FileText, Info, X } from 'lucide-react';
import { api, errorMessage } from '../../api';
import { useData } from '../../app/data';
import { useLive } from '../../app/load';
import { formatDuration, profileName, requestPath } from '../../app/format';
import type { RequestView } from '../../generated/views';
import {
  Button,
  ConfirmDialog,
  ErrorBanner,
  Field,
  Input,
  RelativeTime,
  SidePanel,
  Skeleton,
  StatusPill,
  buttonClass,
  useToast,
} from '../../components/ui';
import { approvalProgress, approvers, parseRequestRef, withRequestRef, type RequestRef } from './model';
import { CancelRequest, Unavailable } from './parts';
import { RequestBody } from './RequestBody';
import { refKey, requesterName } from './RequestRows';

/**
 * The request open beside a list, kept in the URL as `?request=<namespace>/<name>`. `open` remembers what had
 * focus so closing returns there; after a reload, focus returns to the row's "View request" button.
 */
export interface RequestPaneState {
  current: RequestRef | null;
  isOpen: (r: RequestRef) => boolean;
  open: (r: RequestRef) => void;
  close: () => void;
  returnFocus: () => HTMLElement | null;
}

export function useRequestPane(): RequestPaneState {
  const { search } = useLocation();
  const navigate = useNavigate();
  const opener = useRef<HTMLElement | null>(null);
  const current = parseRequestRef(new URLSearchParams(search).get('request'));
  return {
    current,
    isOpen: (r: RequestRef) => current !== null && refKey(current) === refKey(r),
    open: (r: RequestRef) => {
      opener.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
      navigate({ search: withRequestRef(search, r) }, { replace: current !== null });
    },
    close: () => navigate({ search: withRequestRef(search, null) }, { replace: true }),
    returnFocus: () =>
      opener.current?.isConnected
        ? opener.current
        : current && document.querySelector<HTMLElement>(`[data-request-view="${CSS.escape(refKey(current))}"]`),
  };
}

function DecisionBar({ request, profile }: { request: RequestView; profile: string }) {
  const { actions } = useData();
  const { toast } = useToast();
  const commentId = useId();
  const [comment, setComment] = useState('');
  const [missing, setMissing] = useState(false);
  const [approving, setApproving] = useState(false);
  const [rejecting, setRejecting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const who = request.requester.email || requesterName(request);
  const progress = approvalProgress(request);
  const last = !progress || progress.required - progress.approved <= 1;

  const approve = async () => {
    setApproving(true);
    setError(null);
    try {
      await actions.approve(request, comment.trim() || undefined);
      toast({ title: `Approved ${profile} for ${who}`, tone: 'success' });
      setComment('');
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setApproving(false);
    }
  };

  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-col gap-3">
        <Field
          label={request.actions.reject ? 'Comment, required to reject' : 'Comment, optional'}
          htmlFor={commentId}
          error={missing ? 'Enter a reason to reject this request. The requester sees it.' : undefined}
        >
          <Input
            value={comment}
            placeholder={`Visible to ${who} and kept in the audit trail`}
            onChange={(e) => {
              setComment(e.target.value);
              if (e.target.value.trim()) setMissing(false);
            }}
          />
        </Field>
        <div className="flex flex-wrap justify-end gap-2">
          {request.actions.reject && (
            <Button
              variant="reject"
              icon={X}
              className="flex-1 sm:flex-none"
              onClick={() => (comment.trim() ? setRejecting(true) : setMissing(true))}
            >
              Reject request
            </Button>
          )}
          {request.actions.approve && (
            <Button variant="primary" icon={Check} loading={approving} onClick={approve} className="flex-1 sm:flex-none">
              Approve request
            </Button>
          )}
        </div>
      </div>
      {error && <ErrorBanner title="Couldn't approve request" message={error} />}
      {request.actions.approve && (
        <p className="flex items-start gap-1.5 text-caption text-text-secondary">
          <Info className="mt-px h-4 w-4 shrink-0" aria-hidden />
          {last
            ? `Approving grants ${formatDuration(request.duration)} on ${(request.clusters ?? []).join(', ') || 'the chosen clusters'}. Access is removed on its own when the time is up.`
            : `Your approval is one of ${progress?.required}. Access goes live once all of them are in.`}
        </p>
      )}
      <ConfirmDialog
        open={rejecting}
        onOpenChange={setRejecting}
        title={`Reject ${profile} for ${who}?`}
        body={
          <>
            The request ends as Rejected and no access is granted. {who} sees your reason: <q>{comment.trim()}</q>
          </>
        }
        confirmLabel="Reject request"
        tone="danger"
        onConfirm={async () => {
          await actions.reject(request, comment.trim());
          toast({ title: `Rejected ${profile} for ${who}`, tone: 'info' });
          setComment('');
        }}
      />
    </div>
  );
}

/** The request named by the URL, beside the list; render once per page that lists requests. */
export function RequestPane({ pane }: { pane: RequestPaneState }) {
  const { profiles } = useData();
  const ref = pane.current;
  const key = ref ? refKey(ref) : '';
  const loaded = useLive(() => (ref ? api.getRequest(ref.namespace, ref.name) : Promise.resolve(null)), [key]);
  const request = loaded.data && refKey(loaded.data) === key ? loaded.data : null;
  const profile = request ? profileName(profiles, request.namespace, request.profile) : '';
  const decide = request && (request.actions.approve || request.actions.reject);
  const groups = request ? [...new Set((request.requirements ?? []).flatMap(approvers))] : [];

  return (
    <SidePanel
      open={ref !== null}
      onClose={pane.close}
      itemKey={key}
      returnFocus={pane.returnFocus}
      title={request ? `${requesterName(request)} requests ${profile}` : 'Request'}
      meta={
        request && (
          <p className="flex flex-wrap items-center gap-x-3 gap-y-1 text-label font-normal text-text-secondary">
            <StatusPill phase={request.phase || 'Pending'} />
            <span>
              Sent <RelativeTime at={request.createdAt} />
            </span>
            {groups.length > 0 && <span>Approvers {groups.join(', ')}</span>}
          </p>
        )
      }
      actions={
        ref && (
          <Link to={requestPath(ref)} className={buttonClass('secondary', 'sm')}>
            <FileText className="h-4 w-4 shrink-0" aria-hidden />
            Open request page
          </Link>
        )
      }
      footer={
        request &&
        (decide ? (
          <DecisionBar key={key} request={request} profile={profile} />
        ) : request.actions.cancel ? (
          <div className="flex justify-end">
            <CancelRequest request={request} profile={profile} />
          </div>
        ) : null)
      }
    >
      {!ref ? null : request ? (
        <RequestBody request={request} profile={profile} />
      ) : loaded.error ? (
        <Unavailable kind="request" ns={ref.namespace} name={ref.name} error={loaded.error} retry={loaded.reload} />
      ) : (
        <div className="flex flex-col gap-3" aria-busy="true">
          <span className="sr-only">Loading</span>
          <Skeleton className="h-5 w-1/3" />
          <Skeleton className="w-2/3" />
          <Skeleton className="w-1/2" />
        </div>
      )}
    </SidePanel>
  );
}
