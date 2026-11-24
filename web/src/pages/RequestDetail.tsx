import { Link, useLocation, useParams } from 'react-router-dom';
import { api } from '../api';
import { useData } from '../app/data';
import { useLive } from '../app/load';
import type { RequestView } from '../generated/views';
import { Identifier, PageHeader, Panel, Skeleton, StatusPill, buttonClass } from '../components/ui';
import { grantPath, profileName } from '../app/format';
import { CancelRequest, DecideButtons, Unavailable } from './access/parts';
import { RequestBody } from './access/RequestBody';
import { requesterName } from './access/RequestRows';

export default function RequestDetail() {
  const { ns = '', name = '' } = useParams();
  return <RequestPage key={`${ns}/${name}`} ns={ns} name={name} />;
}

function RequestPage({ ns, name }: { ns: string; name: string }) {
  const location = useLocation();
  const { profiles } = useData();
  // The request just created, passed along by the form, shows until the first answer arrives.
  const fresh = (location.state as { request?: RequestView } | null)?.request;
  const loaded = useLive(
    () => api.getRequest(ns, name),
    [],
    fresh?.namespace === ns && fresh.name === name ? fresh : undefined,
  );
  const request = loaded.data;

  if (loaded.loading) {
    return (
      <>
        <PageHeader title="Request" />
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

  if (!request) {
    return (
      <>
        <PageHeader title="Request" />
        <Unavailable kind="request" ns={ns} name={name} error={loaded.error} retry={loaded.reload} />
      </>
    );
  }

  const profile = profileName(profiles, request.namespace, request.profile);
  const { actions: can } = request;

  return (
    <div className="flex max-w-4xl flex-col gap-6">
      <PageHeader
        title={`${requesterName(request)} requests ${profile}`}
        description={
          <span className="flex flex-wrap items-center gap-2">
            <StatusPill phase={request.phase || 'Pending'} />
            <Identifier className="min-w-0">{request.name}</Identifier>
          </span>
        }
        actions={
          (can.approve || can.reject || can.cancel || request.grantRef) && (
            <>
              {request.grantRef && (
                <Link to={grantPath({ namespace: request.namespace, name: request.grantRef })} className={buttonClass('secondary')}>
                  View grant
                </Link>
              )}
              {can.cancel && <CancelRequest request={request} profile={profile} />}
              <DecideButtons request={request} profile={profile} />
            </>
          )
        }
      />
      <RequestBody request={request} profile={profile} />
    </div>
  );
}
