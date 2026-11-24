import { useState } from 'react';
import { Link } from 'react-router-dom';
import { SearchX } from 'lucide-react';
import { useData } from '../app/data';
import { useClientPage } from '../app/load';
import { isMe } from '../app/roles';
import { pendingOldestFirst } from '../app/waiting';
import { Identifier, PageHeader, Pagination, QuietLine, Row, RowList, Skeleton, cn } from '../components/ui';
import { formatDuration, profilePath } from '../app/format';
import { IdList } from './access/parts';
import { ActivationControl, ApprovalMeta } from './access/Activation';
import { linkClass } from './access/RequestRows';
import { activatable, myLiveGrants, type Activatable } from './home/home';
import { ProfileSearchField } from './profiles/ProfileSearchField';
import { searchProfiles } from './profiles/profile';

const PAGE = 20;

function ProfileRow({ item }: { item: Activatable }) {
  const { profile } = item;
  const name = profile.displayName || profile.name;
  return (
    <Row className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <div className="flex min-w-0 flex-col gap-1">
        <div className="flex min-w-0 flex-wrap items-baseline gap-x-3 gap-y-1">
          <h2 className="break-words text-body font-semibold text-text">{name}</h2>
          <ApprovalMeta profile={profile} />
        </div>
        {profile.description && <p className="max-w-prose text-label font-normal text-text-secondary">{profile.description}</p>}
        <dl className="flex flex-wrap gap-x-5 gap-y-1 text-label font-normal">
          <div className="flex min-w-0 gap-1">
            <dt className="text-text-secondary">Role</dt>
            <dd className="min-w-0 text-text">
              <Identifier>{profile.role.name}</Identifier>
            </dd>
          </div>
          <div className="flex min-w-0 gap-1">
            <dt className="text-text-secondary">Namespaces</dt>
            <dd className="min-w-0 text-text">
              <IdList items={profile.namespaces} empty="All" />
            </dd>
          </div>
          <div className="flex min-w-0 gap-1">
            <dt className="text-text-secondary">Clusters</dt>
            <dd className="min-w-0 text-text">
              <IdList items={profile.clusters} empty="None ready" />
            </dd>
          </div>
          <div className="flex min-w-0 gap-1">
            <dt className="text-text-secondary">Longest duration</dt>
            <dd className="tabular-nums text-text">{formatDuration(profile.maxDuration)}</dd>
          </div>
        </dl>
      </div>
      <div className="flex shrink-0 flex-wrap items-center gap-x-4 gap-y-2">
        <Link to={profilePath(profile)} className={cn(linkClass, 'text-label')}>
          View profile<span className="sr-only"> {name}</span>
        </Link>
        <ActivationControl item={item} />
      </div>
    </Row>
  );
}

export default function RequestAccess() {
  const { me, profiles, openRequests, activeGrants, loading } = useData();
  const [query, setQuery] = useState('');
  const mine = pendingOldestFirst(openRequests.items).filter((r) => isMe(me, r.requester));
  const offer = activatable(profiles, myLiveGrants(activeGrants.items, me), mine);
  const found = searchProfiles(
    offer.map((a) => a.profile),
    query,
  );
  const byProfile = new Map(offer.map((a) => [a.profile, a]));
  const paged = useClientPage(found, query.trim().toLocaleLowerCase('en'), PAGE);

  return (
    <>
      <PageHeader
        title="Activate access"
        description="Pick a profile, then choose clusters, duration and a reason. Some are granted at once, others need an approval."
      />
      {loading ? (
        <div className="flex flex-col gap-3" aria-busy="true">
          <span className="sr-only">Loading</span>
          <Skeleton className="h-5 w-1/3" />
          <Skeleton className="w-2/3" />
        </div>
      ) : offer.length === 0 ? (
        <QuietLine>No profile lists you as a requester yet. Ask a Protem admin to add one of your groups.</QuietLine>
      ) : (
        <>
          <ProfileSearchField value={query} onChange={setQuery} />
          {found.length === 0 ? (
            <p className="flex items-center gap-2 text-body text-text-secondary">
              <SearchX className="h-4 w-4 shrink-0" aria-hidden />
              No profiles match this search. Try a shorter search, or search by role.
            </p>
          ) : (
            <RowList label="Profiles">
              {paged.items.map((p) => (
                <ProfileRow key={`${p.namespace}/${p.name}`} item={byProfile.get(p)!} />
              ))}
            </RowList>
          )}
          <Pagination label="profiles" page={paged.page} pageSize={paged.pageSize} total={paged.total} onPageChange={paged.setPage} />
        </>
      )}
    </>
  );
}
