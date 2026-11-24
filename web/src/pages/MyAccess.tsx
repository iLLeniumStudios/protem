import { Link } from 'react-router-dom';
import { api } from '../api';
import { useData } from '../app/data';
import { usePagedList } from '../app/load';
import { plural } from '../app/format';
import type { ProfileView } from '../generated/views';
import {
  PageHeader,
  Pagination,
  QuietLine,
  RowList,
  Section,
  Skeleton,
  StatusPill,
  buttonClass,
} from '../components/ui';
import { LoadFailed } from './access/parts';
import { MyRequestRow, linkClass, refKey } from './access/RequestRows';
import { RequestPane, useRequestPane, type RequestPaneState } from './access/RequestPane';
import { myLiveGrants } from './home/home';

const PAGE = 10;

function RequestList({
  state,
  title,
  empty,
  profiles,
  pane,
}: {
  state: 'open' | 'decided';
  title: string;
  empty: string;
  profiles: ProfileView[];
  pane: RequestPaneState;
}) {
  const paged = usePagedList((offset, limit) => api.getRequests({ mine: true, state, offset, limit }), `mine-${state}`, PAGE);
  return (
    <Section id={`requests-${state}`} title={title} count={paged.loading ? undefined : paged.total}>
      {paged.loading ? (
        <div className="flex flex-col gap-3" aria-busy="true">
          <span className="sr-only">Loading</span>
          <Skeleton className="w-full" />
          <Skeleton className="w-2/3" />
        </div>
      ) : paged.error && paged.total === 0 ? (
        <LoadFailed what={`your ${title.toLocaleLowerCase('en')}`} error={paged.error} retry={paged.reload} />
      ) : paged.total === 0 ? (
        <QuietLine>{empty}</QuietLine>
      ) : (
        <>
          <RowList label={title}>
            {paged.items.map((r) => (
              <MyRequestRow
                key={refKey(r)}
                request={r}
                profiles={profiles}
                selected={pane.isOpen(r)}
                onView={() => pane.open(r)}
                state={state === 'decided' && <StatusPill phase={r.phase} />}
              />
            ))}
          </RowList>
          <Pagination
            label={`my ${title.toLocaleLowerCase('en')}`}
            page={paged.page}
            pageSize={paged.pageSize}
            total={paged.total}
            onPageChange={paged.setPage}
          />
        </>
      )}
    </Section>
  );
}

export default function MyAccess() {
  const { me, activeGrants, profiles } = useData();
  const pane = useRequestPane();
  const live = myLiveGrants(activeGrants.items, me).length;

  return (
    <>
      <PageHeader
        title="My requests"
        description="Every request you have sent: open ones first, then the ones already decided."
        actions={
          <Link to="/request" className={buttonClass('secondary')}>
            Request access
          </Link>
        }
      />
      {live > 0 && (
        <p className="text-body text-text-secondary">
          You hold {plural(live, 'grant')} right now.{' '}
          <Link to="/" className={linkClass}>
            See your access on For you
          </Link>
        </p>
      )}
      <RequestList state="open" title="Open requests" empty="None of your requests is waiting." profiles={profiles} pane={pane} />
      <RequestList
        state="decided"
        title="History"
        empty="Requests you send appear here once they are decided."
        profiles={profiles}
        pane={pane}
      />
      <RequestPane pane={pane} />
    </>
  );
}
