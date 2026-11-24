import { Link, useSearchParams } from 'react-router-dom';
import { CircleCheck, Clock, Layers, Plus, SearchX, Split, type LucideIcon } from 'lucide-react';
import { useData } from '../app/data';
import { formatDuration, plural, profilePath } from '../app/format';
import { clampPage, slicePage } from '../app/paging';
import type { ProfileView } from '../generated/views';
import {
  Button,
  ConsoleCard,
  ConsoleToolbar,
  DenseTable,
  EmptyState,
  FilterChip,
  Identifier,
  PageHeader,
  SearchField,
  TBody,
  TD,
  TH,
  THead,
  TR,
  TablePager,
  buttonClass,
  cn,
} from '../components/ui';
import { focusRing } from '../components/ui/cn';
import {
  activationOf,
  filterProfiles,
  firstFew,
  parseProfileQuery,
  profileFacets,
  profileQueryString,
  whoCanRequest,
  type Few,
  type GrantMode,
  type ProfileQuery,
} from './profiles/list';
import { approvers } from './access/model';
import { canCreateProfile } from './profiles/manage';
import { ManagedBadge } from './profiles/ManagedBadge';

const PAGE = 25;
const GRANTED: Record<GrantMode, { label: string; icon: LucideIcon; className: string }> = {
  'at-once': { label: 'At once', icon: CircleCheck, className: 'text-success-text' },
  approval: { label: 'With approval', icon: Clock, className: 'text-warning-text' },
  mixed: { label: 'Depends on cluster', icon: Split, className: 'text-text' },
};
const GRANT_OPTIONS = (Object.keys(GRANTED) as GrantMode[]).map((value) => ({ value, label: GRANTED[value].label }));

function ProfileName({ profile }: { profile: ProfileView }) {
  return (
    <span className="flex min-w-0 flex-col">
      <span className="flex min-w-0 items-center gap-2">
        <Link
          to={profilePath(profile)}
          className={cn('truncate rounded-sm font-semibold text-text underline-offset-4 hover:text-accent-text hover:underline', focusRing)}
        >
          {profile.displayName || profile.name}
        </Link>
        <ManagedBadge by={profile.managedBy} />
      </span>
      <Identifier className="text-caption text-text-secondary">
        {profile.namespace}/{profile.name}
      </Identifier>
    </span>
  );
}

function RoleScope({ profile }: { profile: ProfileView }) {
  const ns = profile.namespaces ?? [];
  const where = profile.scope === 'Cluster' ? 'cluster-wide' : ns.length === 1 ? `in ${ns[0]}` : ns.length ? `in ${ns.length} namespaces` : 'in its namespaces';
  return (
    <span className="flex min-w-0 items-baseline gap-1.5 whitespace-nowrap" title={ns.length > 1 ? ns.join(', ') : undefined}>
      <Identifier className="text-text">{profile.role.name}</Identifier>
      <span className="text-text-secondary">{where}</span>
    </span>
  );
}

function FewList({ few: { shown, more, all }, empty, mono }: { few: Few; empty: string; mono?: boolean }) {
  if (!all.length) return <span className="min-w-0 text-text-secondary">{empty}</span>;
  return (
    <span className="flex min-w-0 items-baseline gap-1.5" title={all.join(', ')}>
      <span className={cn('min-w-0 truncate text-text', mono && 'font-mono text-caption')}>{shown.join(', ')}</span>
      {more > 0 && <span className="shrink-0 text-caption text-text-secondary">+{more} more</span>}
    </span>
  );
}

function Granted({ profile }: { profile: ProfileView }) {
  const mode = activationOf(profile);
  const { label, icon: Icon, className } = GRANTED[mode];
  const who = approvers(profile);
  const title =
    mode === 'mixed'
      ? 'Granted at once on some clusters, after approval on others'
      : mode === 'approval' && who.length
        ? `Approved by ${who.join(', ')}`
        : undefined;
  return (
    <span className={cn('inline-flex items-center gap-1.5 whitespace-nowrap', className)} title={title}>
      <Icon className="h-4 w-4 shrink-0" aria-hidden />
      {label}
    </span>
  );
}

export default function Profiles() {
  const { profiles, capabilities } = useData();
  const [params, setParams] = useSearchParams();
  const query = parseProfileQuery(params);
  const setQuery = (next: ProfileQuery) => setParams(new URLSearchParams(profileQueryString(next)), { replace: true });
  const patch = (change: Partial<ProfileQuery>) => setQuery({ ...query, page: 0, ...change });

  const canCreate = canCreateProfile(capabilities);
  const { namespaces, roles } = profileFacets(profiles);
  const found = filterProfiles(profiles, query);
  const page = clampPage(query.page, found.length, PAGE);
  const rows = slicePage(found, page, PAGE);
  const filtered = Boolean(query.q.trim() || query.ns || query.role || query.grant);
  const newProfile = canCreate && (
    <Link to="/profiles/new" className={buttonClass('primary')}>
      <Plus className="h-5 w-5 shrink-0" aria-hidden />
      Create profile
    </Link>
  );
  const clear = (
    <Button size="sm" onClick={() => setQuery({ q: '', page: 0 })}>
      Clear filters
    </Button>
  );

  return (
    <>
      <PageHeader title={profiles.length ? plural(profiles.length, 'profile') : 'Profiles'} actions={newProfile} />
      {profiles.length > 0 && (
        <ConsoleToolbar label="Filter profiles">
          <SearchField label="Search profiles" placeholder="Search by name, description or role" value={query.q} onChange={(q) => patch({ q })} />
          <FilterChip
            label="Namespace"
            value={query.ns ?? ''}
            options={namespaces.map((n) => ({ value: n, label: n }))}
            onValueChange={(ns) => patch({ ns: ns || undefined })}
          />
          <FilterChip label="Role" value={query.role ?? ''} options={roles.map((r) => ({ value: r, label: r }))} onValueChange={(role) => patch({ role: role || undefined })} />
          <FilterChip
            label="Granted"
            value={query.grant ?? ''}
            options={GRANT_OPTIONS}
            onValueChange={(grant) => patch({ grant: (grant || undefined) as GrantMode | undefined })}
          />
          {filtered && clear}
        </ConsoleToolbar>
      )}

      <ConsoleCard label="Profiles">
        {profiles.length === 0 ? (
          <EmptyState
            icon={Layers}
            title="No profiles yet"
            body={canCreate ? 'Create the first one here, or add one through Git.' : 'Profiles appear here once an admin adds them.'}
          />
        ) : found.length === 0 ? (
          <EmptyState icon={SearchX} title="No profiles match these filters" body="Try a shorter search or another filter." action={clear} />
        ) : (
          <>
            <div className="hidden lg:block">
              <DenseTable label="Profiles">
                <THead>
                  <TR>
                    <TH className="w-1/4">Profile</TH>
                    <TH>Role and scope</TH>
                    <TH>Clusters</TH>
                    <TH>Who can request</TH>
                    <TH>How it is granted</TH>
                    <TH className="text-right">Max duration</TH>
                  </TR>
                </THead>
                <TBody>
                  {rows.map((p) => (
                    <TR key={`${p.namespace}/${p.name}`}>
                      <TD className="max-w-80 py-1.5">
                        <ProfileName profile={p} />
                      </TD>
                      <TD className="max-w-64">
                        <RoleScope profile={p} />
                      </TD>
                      <TD className="max-w-56">
                        <FewList few={firstFew(p.clusters ?? [])} empty="None ready" mono />
                      </TD>
                      <TD className="max-w-56">
                        <FewList few={whoCanRequest(p)} empty="No one" />
                      </TD>
                      <TD>
                        <Granted profile={p} />
                      </TD>
                      <TD className="whitespace-nowrap text-right">{formatDuration(p.maxDuration)}</TD>
                    </TR>
                  ))}
                </TBody>
              </DenseTable>
            </div>
            <ul aria-label="Profiles" className="flex flex-col divide-y divide-border lg:hidden">
              {rows.map((p) => (
                <li key={`${p.namespace}/${p.name}`} className="flex flex-col gap-2 px-4 py-3 text-label font-normal">
                  <ProfileName profile={p} />
                  <RoleScope profile={p} />
                  <div className="flex flex-wrap items-center gap-x-4 gap-y-1">
                    <Granted profile={p} />
                    <span className="text-text-secondary">
                      Up to <span className="text-text">{formatDuration(p.maxDuration)}</span>
                    </span>
                    <span className="text-text-secondary">{plural(p.clusters?.length ?? 0, 'cluster')}</span>
                  </div>
                  <div className="flex min-w-0 items-center gap-2">
                    <span className="shrink-0 text-text-secondary">Who can request</span>
                    <FewList few={whoCanRequest(p)} empty="No one" />
                  </div>
                </li>
              ))}
            </ul>
            <TablePager
              label="profiles"
              page={page}
              pageSize={PAGE}
              total={found.length}
              onPageChange={(p) => setQuery({ ...query, page: p })}
              className="border-t border-border px-4 py-2"
            />
          </>
        )}
      </ConsoleCard>
    </>
  );
}
