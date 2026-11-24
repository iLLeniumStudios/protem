import { useRef, useState } from 'react';
import { KeyRound, Plug, Settings, Trash2 } from 'lucide-react';
import { errorMessage } from '../../api';
import { useData } from '../../app/data';
import type { ClusterView, JoinView } from '../../generated/views';
import { ActionMenu, Button, ConfirmDialog, Dialog, ErrorBanner, Identifier, useToast, type ActionMenuItem } from '../../components/ui';
import { ClusterSettings } from './ClusterSettings';
import { ConnectAgent, ConnectDone, connectTitle, useAgentJoined } from './ConnectAgent';
import { clusterActions } from './enrol';

/** The row's menu: edit settings, connect the agent, reissue credentials, remove; each opens its dialog. */
export function ClusterActions({ cluster, editable }: { cluster: ClusterView; editable: boolean }) {
  const { capabilities, actions } = useData();
  const { toast } = useToast();
  const introRef = useRef<HTMLParagraphElement>(null);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [connecting, setConnecting] = useState(false);
  const [reissueOpen, setReissueOpen] = useState(false);
  const [reissuing, setReissuing] = useState(false);
  const [reissueError, setReissueError] = useState<string | null>(null);
  const [join, setJoin] = useState<JoinView | null>(null);
  const [removeOpen, setRemoveOpen] = useState(false);
  const joined = useAgentJoined(join);
  const offered = clusterActions(capabilities, cluster);
  const title = cluster.displayName || cluster.name;

  const connect = async () => {
    setConnecting(true);
    try {
      setJoin(await actions.joinCluster(cluster.name));
    } catch (err) {
      toast({ title: `No join token was issued for ${title}. ${errorMessage(err)}`, tone: 'danger' });
    } finally {
      setConnecting(false);
    }
  };

  const reissue = async () => {
    setReissuing(true);
    setReissueError(null);
    try {
      setJoin(await actions.reissueCluster(cluster.name));
      requestAnimationFrame(() => introRef.current?.focus());
    } catch (err) {
      setReissueError(errorMessage(err));
    } finally {
      setReissuing(false);
    }
  };

  const close = () => {
    setJoin(null);
    setReissueOpen(false);
  };

  const items: ActionMenuItem[] = [];
  if (editable) items.push({ key: 'settings', label: 'Edit settings', icon: Settings, onSelect: () => setSettingsOpen(true) });
  if (offered.includes('connect')) {
    items.push({ key: 'connect', label: connecting ? 'Connecting agent' : 'Connect agent', icon: Plug, onSelect: connect });
  }
  if (offered.includes('reissue')) {
    items.push({
      key: 'reissue',
      label: 'Reissue credentials',
      icon: KeyRound,
      onSelect: () => {
        setReissueError(null);
        setReissueOpen(true);
      },
    });
  }
  if (offered.includes('remove')) {
    items.push({ key: 'remove', label: 'Remove cluster', icon: Trash2, danger: true, onSelect: () => setRemoveOpen(true) });
  }
  if (!items.length) return null;

  return (
    <>
      <ActionMenu label={`Actions for ${title}`} items={items} />
      {editable && <ClusterSettings cluster={cluster} open={settingsOpen} onOpenChange={setSettingsOpen} />}
      <Dialog
        open={Boolean(join) || reissueOpen}
        onOpenChange={(next) => !next && !reissuing && close()}
        title={join ? connectTitle(title) : `Reissue credentials for ${title}?`}
        footer={
          join ? (
            <ConnectDone joined={joined} onClose={close} />
          ) : (
            <>
              <Button variant="secondary" onClick={close} disabled={reissuing}>
                Cancel
              </Button>
              <Button variant="danger" loading={reissuing} onClick={reissue}>
                Reissue credentials
              </Button>
            </>
          )
        }
      >
        {join ? (
          <ConnectAgent join={join} joined={joined} onJoin={setJoin} introRef={introRef} />
        ) : (
          <div className="flex flex-col gap-4">
            <p className="text-body text-text">
              The agent running on <Identifier>{cluster.name}</Identifier> is cut off from the hub at once. It stays cut off
              until you reinstall it with the new files shown next.
            </p>
            {reissueError && <ErrorBanner title="Credentials were not reissued." message={reissueError} />}
          </div>
        )}
      </Dialog>
      <ConfirmDialog
        open={removeOpen}
        onOpenChange={setRemoveOpen}
        title={`Remove ${title}?`}
        body={
          <>
            <Identifier>{cluster.name}</Identifier> is removed from the hub and its agent loses access to the hub. Bindings
            already on the cluster stay until they expire.
          </>
        }
        confirmLabel="Remove cluster"
        tone="danger"
        onConfirm={async () => {
          await actions.removeCluster(cluster.name);
          toast({ title: `${title} removed`, tone: 'success' });
        }}
      />
    </>
  );
}
