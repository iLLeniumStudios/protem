import { useId, useRef, useState, type FormEvent } from 'react';
import { Plus } from 'lucide-react';
import { errorMessage } from '../../api';
import { useData } from '../../app/data';
import type { ClusterView, JoinView } from '../../generated/views';
import { Button, Dialog, ErrorBanner, Field, Input, Textarea } from '../../components/ui';
import { KeyValueRows } from '../profiles/KeyValueRows';
import { ConnectAgent, ConnectDone, connectTitle, useAgentJoined } from './ConnectAgent';
import { CLUSTER_NAME_MAX, addClusterPayload, type AddClusterErrors, type AddClusterForm } from './enrol';

const EMPTY: AddClusterForm = { name: '', displayName: '', description: '', labels: [] };

export function AddCluster({ hubName }: { hubName?: string }) {
  const { actions } = useData();
  const ids = useId();
  const formRef = useRef<HTMLFormElement>(null);
  const introRef = useRef<HTMLParagraphElement>(null);
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState<AddClusterForm>(EMPTY);
  const [submitted, setSubmitted] = useState(false);
  const [pending, setPending] = useState(false);
  const [serverError, setServerError] = useState<{ title: string; message: string } | null>(null);
  /** Created on the hub, but its join token is not issued yet: submitting again only retries the token. */
  const [added, setAdded] = useState<ClusterView | null>(null);
  const [join, setJoin] = useState<JoinView | null>(null);
  const joined = useAgentJoined(join);

  const openDialog = () => {
    setForm(EMPTY);
    setSubmitted(false);
    setPending(false);
    setServerError(null);
    setAdded(null);
    setJoin(null);
    setOpen(true);
  };

  const close = () => {
    setJoin(null);
    setOpen(false);
  };

  const { payload, errors } = addClusterPayload(form, hubName);
  const shown: AddClusterErrors = submitted ? errors : { name: form.name ? errors.name : undefined };

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setSubmitted(true);
    if (!added && Object.keys(errors).length) {
      requestAnimationFrame(() =>
        formRef.current?.querySelector<HTMLElement>('[aria-invalid="true"], [data-invalid="true"]')?.focus(),
      );
      return;
    }
    setPending(true);
    setServerError(null);
    let cluster = added;
    try {
      if (!cluster) {
        cluster = await actions.addCluster(payload);
        setAdded(cluster);
      }
    } catch (err) {
      setServerError({ title: 'The cluster was not added.', message: errorMessage(err) });
      setPending(false);
      return;
    }
    try {
      setJoin(await actions.joinCluster(cluster.name));
      requestAnimationFrame(() => introRef.current?.focus());
    } catch (err) {
      setServerError({ title: 'The cluster was added, but its join token was not issued.', message: errorMessage(err) });
    } finally {
      setPending(false);
    }
  };

  return (
    <>
      <Button variant="primary" icon={Plus} onClick={openDialog}>
        Add cluster
      </Button>
      <Dialog
        open={open}
        onOpenChange={(next) => !pending && (next ? setOpen(true) : close())}
        title={join ? connectTitle(added?.displayName || join.cluster) : 'New cluster'}
        footer={
          join ? (
            <ConnectDone joined={joined} onClose={close} />
          ) : (
            <>
              <Button variant="secondary" onClick={close} disabled={pending}>
                Cancel
              </Button>
              <Button type="submit" form={`${ids}-form`} variant="primary" loading={pending}>
                {added ? 'Issue join token' : 'Add cluster'}
              </Button>
            </>
          )
        }
      >
        {join ? (
          <ConnectAgent join={join} joined={joined} onJoin={setJoin} introRef={introRef} />
        ) : (
          <form id={`${ids}-form`} ref={formRef} noValidate onSubmit={submit} className="flex flex-col gap-6">
            {serverError && <ErrorBanner title={serverError.title} message={serverError.message} />}
            {added ? (
              <p className="text-body text-text">
                {added.displayName || added.name} is on the hub and waits for its agent. Issue its join token to continue, or close
                this and use Connect agent in the cluster list later.
              </p>
            ) : (
              <>
                <Field
                  label="Name"
                  htmlFor={`${ids}-name`}
                  required
                  hint={`The cluster's identifier: lowercase letters, digits and hyphens, at most ${CLUSTER_NAME_MAX} characters. It can't be changed later.`}
                  error={shown.name}
                >
                  <Input
                    value={form.name}
                    autoComplete="off"
                    spellCheck={false}
                    mono
                    onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
                  />
                </Field>
                <Field label="Display name" htmlFor={`${ids}-display`} required error={shown.displayName}>
                  <Input
                    value={form.displayName}
                    autoComplete="off"
                    onChange={(e) => setForm((f) => ({ ...f, displayName: e.target.value }))}
                  />
                </Field>
                <Field label="Description" htmlFor={`${ids}-description`}>
                  <Textarea value={form.description} onChange={(e) => setForm((f) => ({ ...f, description: e.target.value }))} />
                </Field>
                <KeyValueRows
                  id={`${ids}-labels`}
                  legend="Labels"
                  hint="Labels decide which profiles target this cluster."
                  rows={form.labels}
                  onChange={(labels) => setForm((f) => ({ ...f, labels }))}
                  error={shown.labels}
                />
              </>
            )}
          </form>
        )}
      </Dialog>
    </>
  );
}
