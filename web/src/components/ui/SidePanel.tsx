import { createContext, useContext, useEffect, useRef, useState, useSyncExternalStore, type ReactNode } from 'react';
import * as RadixDialog from '@radix-ui/react-dialog';
import { X } from 'lucide-react';
import { IconButton } from './Button';
import { cn } from './cn';

/** Matches Tailwind's `xl`: from here the panel sits beside the page instead of covering it. */
const WIDE_QUERY = '(min-width: 1280px)';

function useMediaQuery(query: string): boolean {
  return useSyncExternalStore(
    (onChange) => {
      const mql = window.matchMedia(query);
      mql.addEventListener('change', onChange);
      return () => mql.removeEventListener('change', onChange);
    },
    () => window.matchMedia(query).matches,
  );
}

const SpaceContext = createContext<((open: boolean) => void) | null>(null);
const ReservedContext = createContext(false);

/** Lets the frame make room for an open `SidePanel`; wrap the frame and its page in it. */
export function SidePanelProvider({ children }: { children: ReactNode }) {
  const [reserved, setReserved] = useState(false);
  return (
    <SpaceContext.Provider value={setReserved}>
      <ReservedContext.Provider value={reserved}>{children}</ReservedContext.Provider>
    </SpaceContext.Provider>
  );
}

/** True while a `SidePanel` is open; the frame then keeps its content clear of the panel at wide widths. */
export function useSidePanelOpen() {
  return useContext(ReservedContext);
}

export interface SidePanelProps {
  open: boolean;
  onClose: () => void;
  /** Accessible name of the panel and its visible heading. */
  title: ReactNode;
  /** Shown under the heading. */
  meta?: ReactNode;
  /** Links or buttons beside the heading, before Close. */
  actions?: ReactNode;
  /** Pinned to the panel's bottom. */
  footer?: ReactNode;
  /** Changes when the panel shows a different item; focus then moves to the panel. */
  itemKey: string;
  /** Where focus goes once the panel closes; falls back to the page's main region. */
  returnFocus: () => HTMLElement | null;
  children: ReactNode;
}

/**
 * Detail beside a list. From 1280 px wide it is non-modal: the list stays usable and the frame makes room
 * for it. Narrower, it covers the page and traps focus like `Sheet`. Escape and Close both call `onClose`.
 */
export function SidePanel({ open, onClose, title, meta, actions, footer, itemKey, returnFocus, children }: SidePanelProps) {
  const wide = useMediaQuery(WIDE_QUERY);
  const reserve = useContext(SpaceContext);
  const panel = useRef<HTMLDivElement>(null);
  const shown = useRef<string | null>(null);

  useEffect(() => {
    if (!open || !wide || !reserve) return;
    reserve(true);
    return () => reserve(false);
  }, [open, wide, reserve]);

  useEffect(() => {
    if (open && shown.current !== null && shown.current !== itemKey) panel.current?.focus();
    shown.current = open ? itemKey : null;
  }, [open, itemKey]);

  return (
    <RadixDialog.Root open={open} onOpenChange={(next) => !next && onClose()} modal={!wide}>
      <RadixDialog.Portal>
        {!wide && <RadixDialog.Overlay className="fixed inset-0 z-40 bg-scrim" />}
        <RadixDialog.Content
          ref={panel}
          tabIndex={-1}
          aria-describedby={undefined}
          onOpenAutoFocus={(e) => {
            e.preventDefault();
            panel.current?.focus();
          }}
          onCloseAutoFocus={(e) => {
            e.preventDefault();
            const target = returnFocus();
            if (target?.isConnected) target.focus();
            else document.getElementById('main')?.focus();
          }}
          // Beside the list, working in the list must not close the panel.
          onInteractOutside={(e) => wide && e.preventDefault()}
          className={cn(
            'fixed inset-y-0 right-0 z-50 flex w-full max-w-xl flex-col border-l border-border bg-bg text-text shadow-xl',
            'xl:z-30 xl:shadow-none',
          )}
        >
          <div className="flex items-start gap-3 border-b border-border bg-surface px-4 py-4 sm:px-6">
            <div className="flex min-w-0 flex-1 flex-wrap items-start gap-x-4 gap-y-3">
              <div className="flex min-w-0 flex-1 basis-60 flex-col gap-1">
                <RadixDialog.Title className="break-words text-heading text-text">{title}</RadixDialog.Title>
                {meta}
              </div>
              {actions && <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div>}
            </div>
            <RadixDialog.Close asChild>
              <IconButton label="Close" icon={X} className="-mr-2 -mt-1 shrink-0" />
            </RadixDialog.Close>
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto px-4 py-5 sm:px-6">{children}</div>
          {footer && <div className="border-t border-border bg-surface px-4 py-4 sm:px-6">{footer}</div>}
        </RadixDialog.Content>
      </RadixDialog.Portal>
    </RadixDialog.Root>
  );
}
