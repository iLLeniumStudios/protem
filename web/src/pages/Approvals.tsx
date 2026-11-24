import { Link } from 'react-router-dom';
import { Eye } from 'lucide-react';
import { api } from '../api';
import { useData } from '../app/data';
import { profileName } from '../app/format';
import { useClientPage, usePagedList } from '../app/load';
import { waitingForYou } from '../app/waiting';
import {
  Button,
  Identifier,
  PageHeader,
  Pagination,
  QuietLine,
  RelativeTime,
  Row,
  RowList,
  Section,
  Skeleton,
  StatusPill,
} from '../components/ui';
import { decidedAt } from './approvals/queue';
import { SnapshotLimit } from './access/parts';
import { DecisionRow, FlaggedRules, linkClass, refKey, requesterName } from './access/RequestRows';
import { RequestPane, useRequestPane } from './access/RequestPane';

const WAITING_PAGE = 20;
const DECIDED_PAGE = 10;

export default function Approvals() {
  const { profiles, openRequests, capabilities } = useData();
  const pane = useRequestPane();
  const everyone = Boolean(capabilities?.viewAll);
  const waiting = waitingForYou(openRequests.items);
  const waitingPage = useClientPage(waiting, '', WAITING_PAGE);
  const decided = usePagedList(
    (offset, limit) => api.getRequests({ state: 'decided', approver: everyone ? undefined : true, offset, limit }),
    everyone ? 'decided-all' : 'decided-mine',
    DECIDED_PAGE,
  );

  return (
    <>
      <PageHeader title="Approvals" description="Requests waiting for your decision, oldest first." />

      <Section id="waiting-for-you" title="Waiting for you" count={waiting.length}>
        {waiting.length === 0 ? (
          <QuietLine>Nothing is waiting for you. Requests that need your approval appear here as soon as they are made.</QuietLine>
        ) : (
          <>
            <RowList label="Waiting for you">
              {waitingPage.items.map((r) => (
                <DecisionRow
                  key={refKey(r)}
                  request={r}
                  profiles={profiles}
                  selected={pane.isOpen(r)}
                  onView={() => pane.open(r)}
                />
              ))}
            </RowList>
            <Pagination
              label="requests waiting for you"
              page={waitingPage.page}
              pageSize={waitingPage.pageSize}
              total={waitingPage.total}
              onPageChange={waitingPage.setPage}
            />
          </>
        )}
        <SnapshotLimit
          shown={openRequests.items.length}
          total={openRequests.total}
          what="open requests"
          next="The oldest are listed once newer ones are decided."
        />
      </Section>

      <Section
        id="recent-decisions"
        title="Recent decisions"
        count={decided.loading ? undefined : decided.total}
        action={
          <Link to="/audit" className={linkClass}>
            Open audit
          </Link>
        }
      >
        <p className="text-body text-text-secondary">
          {everyone
            ? 'Every request that has been decided, newest first.'
            : 'Requests you were asked to approve that have been decided, newest first.'}
        </p>
        {decided.loading ? (
          <div className="flex flex-col gap-3" aria-busy="true">
            <span className="sr-only">Loading</span>
            <Skeleton className="w-full" />
            <Skeleton className="w-full" />
          </div>
        ) : decided.total === 0 ? (
          <QuietLine>No decisions yet.</QuietLine>
        ) : (
          <>
            <RowList label="Recent decisions">
              {decided.items.map((r) => {
                const profile = profileName(profiles, r.namespace, r.profile);
                return (
                  <Row
                    key={refKey(r)}
                    selected={pane.isOpen(r)}
                    className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2"
                  >
                    <span className="flex min-w-0 flex-col gap-0.5">
                      <span className="break-words text-body font-semibold text-text">{profile}</span>
                      <span className="flex min-w-0 flex-wrap items-center gap-x-2 text-label font-normal text-text-secondary">
                        <Identifier>{r.requester.email || requesterName(r)}</Identifier>
                        <span aria-hidden>·</span>
                        <span>
                          decided <RelativeTime at={decidedAt(r)} />
                        </span>
                      </span>
                    </span>
                    <span className="flex shrink-0 flex-wrap items-center gap-3">
                      <FlaggedRules count={r.flaggedRules} />
                      <StatusPill phase={r.phase} />
                      <Button
                        variant="quiet"
                        size="sm"
                        icon={Eye}
                        data-request-view={refKey(r)}
                        onClick={() => pane.open(r)}
                      >
                        View request<span className="sr-only"> {profile} from {requesterName(r)}</span>
                      </Button>
                    </span>
                  </Row>
                );
              })}
            </RowList>
            <Pagination
              label="recent decisions"
              page={decided.page}
              pageSize={decided.pageSize}
              total={decided.total}
              onPageChange={decided.setPage}
            />
          </>
        )}
      </Section>

      <RequestPane pane={pane} />
    </>
  );
}
