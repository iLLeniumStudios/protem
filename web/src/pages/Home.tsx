import { Link } from 'react-router-dom';
import { ArrowRight, KeyRound } from 'lucide-react';
import { useData } from '../app/data';
import { useClientPage } from '../app/load';
import { isApprover, isMe } from '../app/roles';
import { pendingOldestFirst, waitingForYou } from '../app/waiting';
import { EmptyState, PageHeader, Pagination, Panel, QuietLine, Row, RowList, Section, buttonClass, cn } from '../components/ui';
import { focusRing } from '../components/ui/cn';
import { activatable, forYouSummary, myLiveGrants, type Activatable } from './home/home';
import { ActivationControl, ApprovalMeta } from './access/Activation';
import { DecisionRow, GrantRow, MyRequestRow, linkClass } from './access/RequestRows';
import { RequestPane, useRequestPane } from './access/RequestPane';

const PAGE = 10;
const ACTIVATABLE_SHOWN = 6;

function CanActivate({ items }: { items: Activatable[] }) {
  return (
    <Section id="can-activate" title="Access you can activate" count={items.length}>
      {items.length === 0 ? (
        <QuietLine>No profile lists you as a requester yet. Ask a Protem admin to add one of your groups.</QuietLine>
      ) : (
        <RowList
          label="Access you can activate"
          footer={
            <Link to="/request" className={cn('flex items-center gap-1.5 px-4 py-3 text-label text-accent-text hover:underline', focusRing)}>
              Show all profiles
              <ArrowRight className="h-4 w-4 shrink-0" aria-hidden />
            </Link>
          }
        >
          {items.slice(0, ACTIVATABLE_SHOWN).map((item) => (
            <Row key={`${item.profile.namespace}/${item.profile.name}`} className="flex items-center justify-between gap-3 py-3">
              <span className="flex min-w-0 flex-col gap-0.5">
                <span className="break-words text-body font-semibold text-text">{item.profile.displayName || item.profile.name}</span>
                <ApprovalMeta profile={item.profile} />
              </span>
              <ActivationControl item={item} />
            </Row>
          ))}
        </RowList>
      )}
    </Section>
  );
}

export default function Home() {
  const data = useData();
  const { me, openRequests, activeGrants, profiles } = data;
  const pane = useRequestPane();
  const deciding = waitingForYou(openRequests.items);
  const live = myLiveGrants(activeGrants.items, me);
  const mine = pendingOldestFirst(openRequests.items).filter((r) => isMe(me, r.requester));
  const offer = activatable(profiles, live, mine);
  const decidingPage = useClientPage(deciding, '', PAGE);
  const approver = isApprover(data);
  const nothing = deciding.length === 0 && live.length === 0 && mine.length === 0;

  const requestAccess = (
    <Link to="/request" className={buttonClass('primary')}>
      <KeyRound className="h-5 w-5 shrink-0" aria-hidden />
      Request access
    </Link>
  );

  return (
    <>
      <PageHeader title="For you" description={forYouSummary({ deciding: deciding.length, live: live.length, waiting: mine.length })} actions={requestAccess} />
      {nothing && offer.length === 0 ? (
        <Panel>
          <EmptyState
            icon={KeyRound}
            title="You hold no access, and nothing waits for you"
            body="Pick a profile to request a role on a cluster for a set time. Once it is granted, it shows here with the time it has left."
            action={requestAccess}
          />
        </Panel>
      ) : (
        <div className={cn('grid grid-cols-1 items-start gap-8', !pane.current && 'xl:grid-cols-[minmax(0,1fr)_20rem]')}>
          <div className="flex min-w-0 flex-col gap-7">
            {approver && (
              <Section
                id="needs-decision"
                title="Needs your decision"
                count={deciding.length}
                action={
                  <Link to="/approvals" className={linkClass}>
                    Open approvals
                  </Link>
                }
              >
                {deciding.length === 0 ? (
                  <QuietLine>Nothing is waiting for your decision.</QuietLine>
                ) : (
                  <>
                    <RowList label="Needs your decision">
                      {decidingPage.items.map((r) => (
                        <DecisionRow
                          key={`${r.namespace}/${r.name}`}
                          request={r}
                          profiles={profiles}
                          selected={pane.isOpen(r)}
                          onView={() => pane.open(r)}
                        />
                      ))}
                    </RowList>
                    <Pagination
                      label="requests that need your decision"
                      page={decidingPage.page}
                      pageSize={decidingPage.pageSize}
                      total={decidingPage.total}
                      onPageChange={decidingPage.setPage}
                    />
                  </>
                )}
              </Section>
            )}
            <Section id="access-now" title="Your access now" count={live.length}>
              {live.length === 0 ? (
                <QuietLine>You hold no access right now.</QuietLine>
              ) : (
                <RowList label="Your access now">
                  {live.map((g) => (
                    <GrantRow key={`${g.namespace}/${g.name}`} grant={g} profiles={profiles} />
                  ))}
                </RowList>
              )}
            </Section>
            <Section
              id="your-requests"
              title="Your requests"
              count={`${mine.length} waiting`}
              action={
                <Link to="/requests" className={linkClass}>
                  Show all requests
                </Link>
              }
            >
              {mine.length === 0 ? (
                <QuietLine>None of your requests is waiting for approval.</QuietLine>
              ) : (
                <RowList label="Your requests">
                  {mine.map((r) => (
                    <MyRequestRow
                      key={`${r.namespace}/${r.name}`}
                      request={r}
                      profiles={profiles}
                      selected={pane.isOpen(r)}
                      onView={() => pane.open(r)}
                    />
                  ))}
                </RowList>
              )}
            </Section>
          </div>
          <div className="min-w-0 xl:sticky xl:top-7">
            <CanActivate items={offer} />
          </div>
        </div>
      )}
      <RequestPane pane={pane} />
    </>
  );
}
