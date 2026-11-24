import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { ArrowDownUp, ScrollText, SearchX } from 'lucide-react';
import { api } from '../api';
import {
  Button,
  ConsoleCard,
  ConsoleToolbar,
  DenseTable,
  EmptyState,
  FilterChip,
  Identifier,
  PageHeader,
  RelativeTime,
  SearchField,
  Skeleton,
  TBody,
  TD,
  TH,
  THead,
  TR,
  TablePager,
  cn,
} from '../components/ui';
import { focusRing } from '../components/ui/cn';
import { useData } from '../app/data';
import { grantPath, profileName, requestPath } from '../app/format';
import { usePagedList } from '../app/load';
import type { AuditItem } from '../generated/views';
import { ALL, EVENT_TYPES, NO_FILTERS, auditDetails, auditQuery, eventLabel, type AuditFilters } from './audit/audit';

const PAGE = 25;
const SEARCH_DELAY_MS = 300;
const linkClass = cn('whitespace-nowrap rounded-sm font-medium text-accent-text underline-offset-4 hover:underline', focusRing);

function Muted({ children }: { children: string }) {
  return <span className="text-text-secondary">{children}</span>;
}

function ClusterList({ clusters }: { clusters?: string[] }) {
  if (!clusters?.length) return <Muted>None</Muted>;
  return <Identifier>{clusters.join(', ')}</Identifier>;
}

function RowLinks({ row }: { row: AuditItem }) {
  return (
    <span className="flex flex-wrap gap-x-3 gap-y-1 lg:flex-nowrap">
      <Link to={requestPath({ namespace: row.namespace, name: row.request })} className={linkClass}>
        View request<span className="sr-only"> {row.request}</span>
      </Link>
      {row.grant && (
        <Link to={grantPath({ namespace: row.namespace, name: row.grant })} className={linkClass}>
          View grant<span className="sr-only"> {row.grant}</span>
        </Link>
      )}
    </span>
  );
}

const anyOf = (v: string) => (v === ALL ? '' : v);
const orAll = (v: string) => v || ALL;

export default function Audit() {
  const { clusters, profiles } = useData();
  const [filters, setFilters] = useState<AuditFilters>(NO_FILTERS);
  const [text, setText] = useState('');

  useEffect(() => {
    const timer = setTimeout(() => setFilters((f) => (f.query === text ? f : { ...f, query: text })), SEARCH_DELAY_MS);
    return () => clearTimeout(timer);
  }, [text]);

  const filterKey = auditQuery(filters, 0, PAGE);
  const paged = usePagedList((offset, limit) => api.getAudit(auditQuery(filters, offset, limit)), filterKey, PAGE);
  const filtered = filters.query.trim() !== '' || filters.event !== ALL || filters.cluster !== ALL;

  const set = (patch: Partial<AuditFilters>) => setFilters((f) => ({ ...f, ...patch }));
  const clear = () => {
    setText('');
    setFilters(NO_FILTERS);
  };
  const toggleOrder = () => set({ order: filters.order === 'desc' ? 'asc' : 'desc' });

  const label = (r: AuditItem) => profileName(profiles, r.namespace, r.profile);
  const by = (r: AuditItem) => (r.actor ? <Identifier>{r.actor}</Identifier> : <Muted>System</Muted>);
  const details = (r: AuditItem) => auditDetails(r) || 'None';

  const clearButton = (
    <Button size="sm" onClick={clear}>
      Clear filters
    </Button>
  );

  return (
    <>
      <PageHeader title="Audit" description="Everything that happened to each request and grant." />
      <ConsoleToolbar label="Filter audit events">
        <SearchField label="Search audit events" placeholder="Email, profile, ticket, reason or comment" value={text} onChange={setText} />
        <FilterChip
          label="Event"
          anyLabel="All events"
          value={anyOf(filters.event)}
          options={EVENT_TYPES.map((t) => ({ value: t, label: eventLabel(t) }))}
          onValueChange={(event) => set({ event: orAll(event) })}
        />
        <FilterChip
          label="Cluster"
          anyLabel="All clusters"
          value={anyOf(filters.cluster)}
          options={clusters.map((c) => ({ value: c.name, label: c.displayName || c.name }))}
          onValueChange={(cluster) => set({ cluster: orAll(cluster) })}
        />
        <Button size="sm" variant="ghost" icon={ArrowDownUp} onClick={toggleOrder}>
          {filters.order === 'desc' ? 'Newest first' : 'Oldest first'}
          <span className="sr-only">, change order</span>
        </Button>
        {filtered && clearButton}
      </ConsoleToolbar>

      <ConsoleCard label="Audit events">
        {paged.loading ? (
          <div className="flex flex-col gap-3 p-4" aria-busy="true">
            <span className="sr-only">Loading</span>
            <Skeleton className="w-full" />
            <Skeleton className="w-full" />
            <Skeleton className="w-2/3" />
          </div>
        ) : paged.total === 0 ? (
          filtered ? (
            <EmptyState
              icon={SearchX}
              title="No events match these filters"
              body="Try a shorter search, or a different event or cluster."
              action={clearButton}
            />
          ) : (
            <EmptyState icon={ScrollText} title="No audit events yet" body="Events appear here once someone requests access." />
          )
        ) : (
          <>
            <div className="hidden lg:block">
              <DenseTable label="Audit events">
                <THead>
                  <TR>
                    <TH sort={filters.order} onSort={toggleOrder}>
                      Time
                    </TH>
                    {['Event', 'Requester', 'Profile', 'Clusters', 'Details'].map((h) => (
                      <TH key={h}>{h}</TH>
                    ))}
                    <TH>
                      <span className="sr-only">Links</span>
                    </TH>
                  </TR>
                </THead>
                <TBody>
                  {paged.items.map((r) => (
                    <TR key={r.id}>
                      <TD className="whitespace-nowrap">
                        <RelativeTime at={r.timestamp} />
                      </TD>
                      <TD className="whitespace-nowrap">{eventLabel(r.eventType)}</TD>
                      <TD>
                        <span className="flex max-w-48 flex-col leading-tight">
                          <Identifier>{r.requester}</Identifier>
                          <span className="flex min-w-0 gap-1 text-caption text-text-secondary">
                            by {by(r)}
                          </span>
                        </span>
                      </TD>
                      <TD className="max-w-48">
                        <span className="block max-w-48 truncate" title={label(r)}>
                          {label(r)}
                        </span>
                      </TD>
                      <TD>
                        <span className="block max-w-32 truncate">
                          <ClusterList clusters={r.clusters} />
                        </span>
                      </TD>
                      <TD className="w-full max-w-0">
                        <span className="block truncate" title={details(r)}>
                          {details(r)}
                        </span>
                      </TD>
                      <TD>
                        <RowLinks row={r} />
                      </TD>
                    </TR>
                  ))}
                </TBody>
              </DenseTable>
            </div>
            <ul className="flex flex-col divide-y divide-border lg:hidden" aria-label="Audit events">
              {paged.items.map((r) => (
                <li key={r.id} className="flex flex-col gap-2 px-4 py-3 text-label font-normal">
                  <span className="flex min-w-0 flex-col">
                    <span className="font-medium text-text">{eventLabel(r.eventType)}</span>
                    <RelativeTime at={r.timestamp} className="text-caption text-text-secondary" />
                  </span>
                  <span className="break-words text-text">{label(r)}</span>
                  <dl className="flex flex-col gap-1">
                    {(
                      [
                        ['Requester', <Identifier key="v">{r.requester}</Identifier>],
                        ['By', by(r)],
                        ['Clusters', <ClusterList key="v" clusters={r.clusters} />],
                      ] as const
                    ).map(([term, value]) => (
                      <div key={term} className="flex min-w-0 gap-3">
                        <dt className="w-20 shrink-0 text-text-secondary">{term}</dt>
                        <dd className="min-w-0 text-text">{value}</dd>
                      </div>
                    ))}
                  </dl>
                  <p className="break-words text-text-secondary">{details(r)}</p>
                  <RowLinks row={r} />
                </li>
              ))}
            </ul>
            <TablePager
              label="audit events"
              page={paged.page}
              pageSize={paged.pageSize}
              total={paged.total}
              onPageChange={paged.setPage}
              className="border-t border-border px-4 py-2"
            />
          </>
        )}
      </ConsoleCard>
    </>
  );
}
