import { useEffect, useId, useRef, useState, type ReactNode } from 'react';
import * as RadixDialog from '@radix-ui/react-dialog';
import { CircleAlert, X } from 'lucide-react';
import { errorMessage } from '../../api';
import { Button, IconButton } from './Button';
import { Field, Textarea } from './Field';
import { cn } from './cn';

const overlay = 'fixed inset-0 z-40 bg-scrim';

// Radix returns focus only to a Dialog.Trigger; our dialogs open from plain buttons,
// so remember whatever had focus on open and put it back on close.
function useReturnFocus() {
  const opener = useRef<HTMLElement | null>(null);
  return {
    onOpenAutoFocus: () => {
      opener.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    },
    onCloseAutoFocus: (e: Event) => {
      e.preventDefault();
      if (opener.current?.isConnected) opener.current.focus();
      else document.getElementById('main')?.focus();
      opener.current = null;
    },
  };
}

export interface DialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: ReactNode;
  description?: ReactNode;
  children?: ReactNode;
  footer?: ReactNode;
}

export function Dialog({ open, onOpenChange, title, description, children, footer }: DialogProps) {
  const returnFocus = useReturnFocus();
  return (
    <RadixDialog.Root open={open} onOpenChange={onOpenChange}>
      <RadixDialog.Portal>
        <RadixDialog.Overlay className={overlay} />
        <RadixDialog.Content
          {...returnFocus}
          {...(description ? {} : { 'aria-describedby': undefined })}
          className={cn(
            'fixed inset-0 z-50 flex flex-col bg-surface text-text',
            'sm:inset-auto sm:left-1/2 sm:top-1/2 sm:max-h-dialog sm:w-full sm:max-w-lg sm:-translate-x-1/2 sm:-translate-y-1/2 sm:rounded-lg sm:border sm:border-border sm:shadow-xl',
          )}
        >
          <div className="flex items-start justify-between gap-4 border-b border-border px-4 py-4 sm:px-6">
            <div className="flex min-w-0 flex-col gap-1">
              <RadixDialog.Title className="text-heading text-text">{title}</RadixDialog.Title>
              {description && (
                <RadixDialog.Description className="text-body text-text-secondary">{description}</RadixDialog.Description>
              )}
            </div>
            <RadixDialog.Close asChild>
              <IconButton label="Close" icon={X} className="-mr-2 -mt-1" />
            </RadixDialog.Close>
          </div>
          {children && <div className="flex-1 overflow-y-auto px-4 py-4 sm:px-6">{children}</div>}
          {footer && (
            <div className="mt-auto flex flex-col-reverse gap-3 border-t border-border px-4 py-4 sm:flex-row sm:justify-end sm:px-6">
              {footer}
            </div>
          )}
        </RadixDialog.Content>
      </RadixDialog.Portal>
    </RadixDialog.Root>
  );
}

export interface ConfirmDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: ReactNode;
  body: ReactNode;
  confirmLabel: string;
  /** "Cancel" unless that would read as cancelling the request itself. */
  cancelLabel?: string;
  tone: 'primary' | 'danger';
  reason?: 'required' | 'optional';
  reasonLabel?: string;
  onConfirm: (reason: string) => Promise<void>;
}

export function ConfirmDialog({
  open,
  onOpenChange,
  title,
  body,
  confirmLabel,
  cancelLabel = 'Cancel',
  tone,
  reason,
  reasonLabel = 'Reason',
  onConfirm,
}: ConfirmDialogProps) {
  const reasonId = useId();
  const [text, setText] = useState('');
  const [missing, setMissing] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setText('');
    setMissing(false);
    setPending(false);
    setError(null);
  }, [open]);

  const confirm = async () => {
    const value = text.trim();
    if (reason === 'required' && !value) {
      setMissing(true);
      return;
    }
    setPending(true);
    setError(null);
    try {
      await onConfirm(value);
      onOpenChange(false);
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setPending(false);
    }
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => !pending && onOpenChange(next)}
      title={title}
      footer={
        <>
          <Button variant="secondary" onClick={() => onOpenChange(false)} disabled={pending}>
            {cancelLabel}
          </Button>
          <Button variant={tone} loading={pending} onClick={confirm}>
            {confirmLabel}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <div className="text-body text-text">{body}</div>
        {reason && (
          <Field
            label={reasonLabel}
            htmlFor={reasonId}
            required={reason === 'required'}
            error={missing ? `Enter a ${reasonLabel.toLocaleLowerCase('en')}.` : undefined}
          >
            <Textarea
              value={text}
              onChange={(e) => {
                setText(e.target.value);
                if (e.target.value.trim()) setMissing(false);
              }}
            />
          </Field>
        )}
        {error && <ErrorBanner title={`Couldn't ${confirmLabel.toLocaleLowerCase('en')}`} message={error} />}
      </div>
    </Dialog>
  );
}

/** A server error inside a dialog: what failed, then the server's message. */
export function ErrorBanner({ title, message }: { title: ReactNode; message: string }) {
  return (
    <div role="alert" className="flex items-start gap-2 rounded-md border border-danger bg-danger-soft px-3 py-2 text-body text-danger-text">
      <CircleAlert className="mt-0.5 h-4 w-4 shrink-0" aria-hidden />
      <span className="flex min-w-0 flex-col gap-1">
        <span className="font-medium">{title}</span>
        <span className="break-words">{message}</span>
      </span>
    </div>
  );
}

export interface SheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  side?: 'left';
  title: ReactNode;
  children: ReactNode;
}

export function Sheet({ open, onOpenChange, title, children }: SheetProps) {
  const returnFocus = useReturnFocus();
  return (
    <RadixDialog.Root open={open} onOpenChange={onOpenChange}>
      <RadixDialog.Portal>
        <RadixDialog.Overlay className={overlay} />
        <RadixDialog.Content
          {...returnFocus}
          aria-describedby={undefined}
          className="fixed inset-y-0 left-0 z-50 flex w-full max-w-xs flex-col border-r border-border bg-surface text-text shadow-xl"
        >
          <div className="flex items-center justify-between gap-4 border-b border-border px-4 py-3">
            <RadixDialog.Title className="text-title text-text">{title}</RadixDialog.Title>
            <RadixDialog.Close asChild>
              <IconButton label="Close" icon={X} />
            </RadixDialog.Close>
          </div>
          <div className="flex-1 overflow-y-auto">{children}</div>
        </RadixDialog.Content>
      </RadixDialog.Portal>
    </RadixDialog.Root>
  );
}
