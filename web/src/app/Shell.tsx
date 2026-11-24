import { useEffect, useState } from 'react';
import { Link, NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom';
import {
  Activity,
  CircleAlert,
  House,
  Inbox,
  KeyRound,
  LayoutDashboard,
  Layers,
  Menu,
  ScrollText,
  Send,
  Server,
  type LucideIcon,
} from 'lucide-react';
import { api } from '../api';
import {
  AccountMenu,
  Badge,
  Button,
  IconButton,
  Panel,
  SegmentedNav,
  Sheet,
  SidePanelProvider,
  Skeleton,
  buttonClass,
  cn,
  useSidePanelOpen,
} from '../components/ui';
import { colorTransition, focusRing } from '../components/ui/cn';
import { ProtemMark } from '../components/ui/Logo';
import { useData } from './data';
import { isApprover, navFor, pageTitle, roleLabel, workspaceOf, type Workspace } from './roles';
import { waitingForYou } from './waiting';

const ICONS: Record<string, LucideIcon> = {
  '/': House,
  '/request': KeyRound,
  '/requests': Send,
  '/approvals': Inbox,
  '/admin': LayoutDashboard,
  '/timeline': Activity,
  '/profiles': Layers,
  '/clusters': Server,
  '/audit': ScrollText,
};

const WORKSPACES: { workspace: Workspace; href: string; label: string }[] = [
  { workspace: 'access', href: '/', label: 'My access' },
  { workspace: 'admin', href: '/admin', label: 'Administration' },
];

const PERSONAS = [
  { value: 'alice@company.com', label: 'Alice, requester' },
  { value: 'bob@company.com', label: 'Bob, approver' },
  { value: 'admin@company.com', label: 'Admin' },
];

function Brand() {
  return (
    <Link to="/" className={cn('flex min-w-0 items-center gap-2 rounded-md', focusRing)}>
      <ProtemMark className="h-6 w-6 shrink-0 text-accent-text" aria-hidden />
      <span className="text-title text-text">Protem</span>
    </Link>
  );
}

/** The workspace the frame shows: the path's own, unless the person has no Administration item to show. */
function useWorkspace() {
  const data = useData();
  const { pathname } = useLocation();
  const hasAdmin = navFor('admin', data).length > 0;
  return { workspace: hasAdmin ? workspaceOf(pathname) : ('access' as const), hasAdmin };
}

function WorkspaceSwitch({ onNavigate }: { onNavigate?: () => void }) {
  const { workspace, hasAdmin } = useWorkspace();
  const navigate = useNavigate();
  if (!hasAdmin) return null;
  return (
    <SegmentedNav
      label="Workspace"
      items={WORKSPACES.map((w) => ({ href: w.href, label: w.label, current: w.workspace === workspace }))}
      onNavigate={(href) => {
        onNavigate?.();
        navigate(href);
      }}
    />
  );
}

function NavList({ waiting, onNavigate }: { waiting: number; onNavigate?: () => void }) {
  const data = useData();
  const { workspace } = useWorkspace();
  return (
    <nav aria-label={workspace === 'admin' ? 'Administration' : 'My access'} className="flex flex-col gap-0.5">
      {navFor(workspace, data).map(({ to, label, badge }) => {
        const Icon = ICONS[to];
        return (
          <NavLink
            key={to}
            to={to}
            end={to === '/'}
            onClick={onNavigate}
            className={({ isActive }) =>
              cn(
                'flex h-9 items-center gap-2.5 rounded-md px-2.5 text-body font-medium coarse:h-11',
                colorTransition,
                focusRing,
                isActive ? 'bg-accent-soft font-semibold text-accent-text' : 'text-text-secondary hover:bg-muted hover:text-text',
              )
            }
          >
            <Icon className="h-4 w-4 shrink-0" aria-hidden />
            <span className="min-w-0 flex-1 truncate">{label}</span>
            {badge && waiting > 0 && (
              <Badge tone="count" className="tabular-nums">
                {waiting}
                <span className="sr-only"> waiting</span>
              </Badge>
            )}
          </NavLink>
        );
      })}
    </nav>
  );
}

function AccountPanel() {
  const data = useData();
  const { me, refresh } = data;
  const navigate = useNavigate();
  const switchPersona = async (email: string) => {
    api.setActiveUser(email);
    await refresh();
    navigate('/');
  };
  if (!me) return <Skeleton className="h-12 w-full rounded-md" />;
  return (
    <AccountMenu
      name={me.username}
      email={me.email}
      role={roleLabel(data)}
      devAuth={me.devAuth}
      personas={PERSONAS}
      activePersona={me.email}
      onSwitchPersona={switchPersona}
      signOutURL={me.signOutURL}
      className="w-full border-transparent hover:bg-muted"
    />
  );
}

/** Switcher, navigation and account: the sidebar's content, also shown in the narrow-screen sheet. */
function SideContent({ waiting, onNavigate }: { waiting: number; onNavigate?: () => void }) {
  return (
    <div className="flex min-h-full flex-col gap-4 px-3 pb-3 pt-4">
      <WorkspaceSwitch onNavigate={onNavigate} />
      <div className="flex-1">
        <NavList waiting={waiting} onNavigate={onNavigate} />
      </div>
      <AccountPanel />
    </div>
  );
}

function LoadError({ message }: { message: string }) {
  const { refresh } = useData();
  const [retrying, setRetrying] = useState(false);
  return (
    <div
      role="alert"
      className="flex flex-col gap-3 rounded-md border border-danger bg-danger-soft px-4 py-3 sm:flex-row sm:items-center sm:justify-between"
    >
      <div className="flex min-w-0 items-start gap-2 text-danger-text">
        <CircleAlert className="mt-0.5 h-4 w-4 shrink-0" aria-hidden />
        <div className="flex min-w-0 flex-col">
          <span className="text-body font-medium">Can't reach Protem. Trying again every few seconds.</span>
          <span className="break-words text-caption">{message}</span>
        </div>
      </div>
      <Button
        size="sm"
        loading={retrying}
        onClick={async () => {
          setRetrying(true);
          await refresh();
          setRetrying(false);
        }}
      >
        Try again
      </Button>
    </div>
  );
}

function PageSkeleton() {
  return (
    <div className="flex flex-col gap-8" aria-busy="true">
      <span className="sr-only">Loading</span>
      <div className="flex flex-col gap-2">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="w-72 max-w-full" />
      </div>
      <Skeleton className="h-40 w-full rounded-lg" />
      <Skeleton className="h-24 w-full rounded-lg" />
    </div>
  );
}

function SignedOut({ message }: { message: string }) {
  return (
    <main id="main" className="flex min-h-dvh items-center justify-center p-4">
      <Panel as="div" className="flex w-full max-w-md flex-col items-start gap-6">
        <Brand />
        <div className="flex min-w-0 flex-col gap-2">
          <h1 className="text-page text-text">You're signed out</h1>
          <p className="break-words text-body text-text-secondary">
            {message || 'Sign in to see and request access.'}
          </p>
        </div>
        <Button variant="primary" onClick={() => window.location.reload()}>
          Sign in
        </Button>
      </Panel>
    </main>
  );
}

function Frame() {
  const data = useData();
  const { openRequests, loading, error, signedOut } = data;
  const { pathname } = useLocation();
  const { workspace } = useWorkspace();
  const paneOpen = useSidePanelOpen();
  const [menuOpen, setMenuOpen] = useState(false);
  const waiting = waitingForYou(openRequests.items).length;

  useEffect(() => {
    document.title = signedOut === null ? pageTitle(pathname) : 'Signed out · Protem';
    setMenuOpen(false);
  }, [pathname, signedOut]);

  if (signedOut !== null) return <SignedOut message={signedOut} />;

  return (
    <div className="relative min-h-dvh">
      <a
        href="#main"
        className={cn(buttonClass('secondary'), 'fixed left-4 top-0 z-50 -translate-y-full bg-surface focus:top-4 focus:translate-y-0')}
      >
        Skip to content
      </a>

      <aside className="hidden border-r border-border bg-subtle lg:fixed lg:inset-y-0 lg:left-0 lg:flex lg:w-60 lg:flex-col">
        <div className="px-5 pt-5">
          <Brand />
        </div>
        <div className="flex-1 overflow-y-auto">
          <SideContent waiting={waiting} />
        </div>
      </aside>

      <header className="flex h-14 items-center gap-2 border-b border-border bg-surface px-2 lg:hidden">
        <IconButton label="Open navigation" icon={Menu} onClick={() => setMenuOpen(true)} />
        <Brand />
        {isApprover(data) && (
          <Link
            to="/approvals"
            aria-label={`Approvals, ${waiting} waiting`}
            className={cn(buttonClass('ghost'), 'ml-auto gap-2 px-3')}
          >
            <Inbox className="h-5 w-5 shrink-0" aria-hidden />
            <span className="tabular-nums">{waiting}</span>
          </Link>
        )}
      </header>

      <Sheet open={menuOpen} onOpenChange={setMenuOpen} side="left" title="Protem">
        <SideContent waiting={waiting} onNavigate={() => setMenuOpen(false)} />
      </Sheet>

      <div className={cn('lg:pl-60', paneOpen && 'xl:pr-[36rem]')}>
        <main
          id="main"
          tabIndex={-1}
          className={cn(
            'flex w-full flex-col gap-6 px-4 py-6 lg:px-10 lg:py-7',
            workspace === 'access' && !paneOpen && 'max-w-[87.5rem]',
          )}
        >
          {error && <LoadError message={error} />}
          {loading ? <PageSkeleton /> : <Outlet />}
        </main>
      </div>
    </div>
  );
}

export function Shell() {
  return (
    <SidePanelProvider>
      <Frame />
    </SidePanelProvider>
  );
}
