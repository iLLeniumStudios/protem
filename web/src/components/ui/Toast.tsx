import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { CircleAlert, CircleCheck, Info, X, type LucideIcon } from 'lucide-react';
import { IconButton } from './Button';

export type ToastTone = 'success' | 'danger' | 'info';

export interface ToastInput {
  title: string;
  tone: ToastTone;
}

interface ToastItem extends ToastInput {
  id: number;
}

const TOAST_MS = 5000;

const ICONS: Record<ToastTone, { icon: LucideIcon; className: string }> = {
  success: { icon: CircleCheck, className: 'text-success-text' },
  danger: { icon: CircleAlert, className: 'text-danger-text' },
  info: { icon: Info, className: 'text-info-text' },
};

const ToastContext = createContext<{ toast: (t: ToastInput) => void } | null>(null);

export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([]);
  const nextId = useRef(0);
  const timers = useRef(new Map<number, ReturnType<typeof setTimeout>>());

  const dismiss = useCallback((id: number) => {
    clearTimeout(timers.current.get(id));
    timers.current.delete(id);
    setItems((all) => all.filter((t) => t.id !== id));
  }, []);

  const toast = useCallback(
    (t: ToastInput) => {
      const id = nextId.current++;
      setItems((all) => [...all, { ...t, id }]);
      timers.current.set(
        id,
        setTimeout(() => dismiss(id), TOAST_MS),
      );
    },
    [dismiss],
  );

  useEffect(() => {
    const pending = timers.current;
    return () => pending.forEach(clearTimeout);
  }, []);

  const value = useMemo(() => ({ toast }), [toast]);

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div
        role="status"
        aria-live="polite"
        className="pointer-events-none fixed inset-x-4 bottom-4 z-50 flex flex-col items-center gap-2 sm:left-auto sm:items-end"
      >
        {items.map((t) => {
          const { icon: Icon, className } = ICONS[t.tone];
          return (
            <div
              key={t.id}
              className="pointer-events-auto flex w-full max-w-sm items-start gap-3 rounded-md border border-border-strong bg-surface py-2 pl-4 pr-2 text-text"
            >
              <Icon className={`mt-2.5 h-5 w-5 shrink-0 ${className}`} aria-hidden />
              <p className="min-w-0 flex-1 break-words py-2 text-body">{t.title}</p>
              <IconButton label="Dismiss notification" icon={X} onClick={() => dismiss(t.id)} />
            </div>
          );
        })}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast() {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error('useToast must be used inside <ToastProvider>');
  return ctx;
}
