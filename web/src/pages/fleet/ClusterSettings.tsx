import { useId, useRef, useState, type FormEvent } from 'react';
import { Lock, TriangleAlert } from 'lucide-react';
import { errorMessage } from '../../api';
import { useData } from '../../app/data';
import type { ClusterView } from '../../generated/views';
import { Button, Checkbox, Dialog, ErrorBanner, Field, Identifier, Input, useToast } from '../../components/ui';
import { IdList } from '../access/parts';
import { KeyValueRows } from '../profiles/KeyValueRows';
import { boundAs, settingsChange, settingsForm, type SettingsForm } from './settings';

/** The settings dialog; its form starts from the cluster as it is each time the dialog opens. */
export function ClusterSettings({ cluster, open, onOpenChange }: { cluster: ClusterView; open: boolean; onOpenChange: (open: boolean) => void }) {
  const { actions, me } = useData();
  const { toast } = useToast();
  const ids = useId();
  const formRef = useRef<HTMLFormElement>(null);
  const [wasOpen, setWasOpen] = useState(open);
  const [opened, setOpened] = useState(cluster);
  const [form, setForm] = useState<SettingsForm>(() => settingsForm(cluster));
  const [submitted, setSubmitted] = useState(false);
  const [pending, setPending] = useState(false);
  const [serverError, setServerError] = useState<string | null>(null);

  if (open !== wasOpen) {
    setWasOpen(open);
    if (open) {
      setOpened(cluster);
      setForm(settingsForm(cluster));
      setSubmitted(false);
      setPending(false);
      setServerError(null);
    }
  }
  const { payload, errors } = settingsChange(opened, form);
  const shown = submitted ? errors : {};
  const title = cluster.displayName || cluster.name;
  const subject = form.subject.trim();
  const boundName =
    subject === (opened.subject ?? '') ? opened.boundAs : boundAs(subject, { email: me?.email ?? '', username: me?.username ?? '' });

  const save = async (e: FormEvent) => {
    e.preventDefault();
    setSubmitted(true);
    if (Object.keys(errors).length) {
      requestAnimationFrame(() =>
        formRef.current?.querySelector<HTMLElement>('[aria-invalid="true"], [data-invalid="true"]')?.focus(),
      );
      return;
    }
    if (!Object.keys(payload).length) {
      onOpenChange(false);
      return;
    }
    setPending(true);
    setServerError(null);
    try {
      await actions.updateCluster(cluster.name, payload);
      toast({ title: `Settings for ${title} saved`, tone: 'success' });
      onOpenChange(false);
    } catch (err) {
      setServerError(errorMessage(err));
    } finally {
      setPending(false);
    }
  };

  return (
    <>
      <Dialog
        open={open}
        onOpenChange={(next) => !pending && onOpenChange(next)}
        title={`Settings for ${title}`}
        footer={
          <>
            <Button variant="secondary" onClick={() => onOpenChange(false)} disabled={pending}>
              Cancel
            </Button>
            <Button type="submit" form={`${ids}-form`} variant="primary" loading={pending}>
              Save settings
            </Button>
          </>
        }
      >
        <form id={`${ids}-form`} ref={formRef} noValidate onSubmit={save} className="flex flex-col gap-6">
          {serverError && <ErrorBanner title="Settings were not saved." message={serverError} />}
          <Checkbox
            checked={form.paused}
            onCheckedChange={(paused) => setForm((f) => ({ ...f, paused }))}
            label="Paused"
            description="New requests cannot target a paused cluster. Access already live keeps running."
          />
          <Field
            label="Subject"
            htmlFor={`${ids}-subject`}
            hint={
              <>
                The username each person gets on this cluster. It must match how this cluster's API server names users. If
                set, it must include <code className="font-mono">{'{{.Email}}'}</code> or{' '}
                <code className="font-mono">{'{{.Username}}'}</code>, for example{' '}
                <code className="font-mono">{'oidc:{{.Email}}'}</code>. Leave empty to use the hub's default.
              </>
            }
            error={shown.subject}
          >
            <Input
              value={form.subject}
              autoComplete="off"
              spellCheck={false}
              mono
              onChange={(e) => setForm((f) => ({ ...f, subject: e.target.value }))}
            />
            <div className="flex min-w-0 flex-col gap-1 text-caption text-text-secondary">
              <p className="min-w-0">
                You would be bound as <Identifier className="text-text">{boundName}</Identifier>
              </p>
              <p>
                Run <code className="font-mono">kubectl auth whoami</code> on this cluster; it should print this name.
              </p>
            </div>
          </Field>
          <KeyValueRows
            id={`${ids}-labels`}
            legend="Labels"
            hint={
              <span className="flex items-start gap-1">
                <TriangleAlert className="mt-px h-4 w-4 shrink-0" aria-hidden />
                Labels decide which profiles target this cluster; changing them can add or remove access.
              </span>
            }
            rows={form.labels}
            onChange={(labels) => setForm((f) => ({ ...f, labels }))}
            error={shown.labels}
          >
            {form.reserved.length > 0 && (
              <div className="flex min-w-0 flex-col gap-1 rounded-md border border-border bg-subtle px-3 py-2">
                <span className="flex items-center gap-1 text-caption text-text-secondary">
                  <Lock className="h-4 w-4 shrink-0" aria-hidden />
                  Set by Protem or GitOps tools; not editable here:
                </span>
                <span className="text-body text-text">
                  <IdList items={form.reserved.map(([k, v]) => `${k}=${v}`)} empty="" />
                </span>
              </div>
            )}
          </KeyValueRows>
        </form>
      </Dialog>
    </>
  );
}
