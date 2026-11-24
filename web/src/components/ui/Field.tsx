import {
  createContext,
  forwardRef,
  useContext,
  type InputHTMLAttributes,
  type ReactNode,
  type TextareaHTMLAttributes,
} from 'react';
import { CircleAlert } from 'lucide-react';
import { cn, colorTransition, focusRing } from './cn';

interface FieldState {
  id?: string;
  describedBy?: string;
  invalid?: boolean;
  required?: boolean;
}

const FieldContext = createContext<FieldState>({});

export interface FieldProps {
  label: string;
  hint?: ReactNode;
  error?: ReactNode;
  required?: boolean;
  htmlFor: string;
  children: ReactNode;
  className?: string;
}

export function Field({ label, hint, error, required, htmlFor, children, className }: FieldProps) {
  const hintId = hint ? `${htmlFor}-hint` : undefined;
  const errorId = error ? `${htmlFor}-error` : undefined;
  const describedBy = [hintId, errorId].filter(Boolean).join(' ') || undefined;
  return (
    <div className={cn('flex min-w-0 flex-col gap-2', className)}>
      <div className="flex min-w-0 flex-col gap-1">
        <label htmlFor={htmlFor} className="text-label text-text">
          {label}
          {required && <span className="font-normal text-text-secondary"> (required)</span>}
        </label>
        {hint && (
          <p id={hintId} className="max-w-prose text-caption text-text-secondary">
            {hint}
          </p>
        )}
      </div>
      <FieldContext.Provider value={{ id: htmlFor, describedBy, invalid: Boolean(error), required }}>
        {children}
      </FieldContext.Provider>
      {error && (
        <p id={errorId} className="flex items-start gap-1 text-caption text-danger-text">
          <CircleAlert className="mt-px h-4 w-4 shrink-0" aria-hidden />
          <span>{error}</span>
        </p>
      )}
    </div>
  );
}

const control = cn(
  'w-full min-w-0 rounded-md border bg-surface px-3 text-title font-normal text-text placeholder:text-text-tertiary sm:text-body',
  'hover:border-text-secondary disabled:cursor-not-allowed disabled:opacity-50',
  colorTransition,
  focusRing,
);

function useFieldProps(props: { id?: string; required?: boolean; 'aria-describedby'?: string; 'aria-invalid'?: unknown }) {
  const field = useContext(FieldContext);
  const invalid = props['aria-invalid'] ?? (field.invalid || undefined);
  return {
    id: props.id ?? field.id,
    required: props.required ?? field.required,
    'aria-describedby': props['aria-describedby'] ?? field.describedBy,
    'aria-invalid': invalid as boolean | undefined,
    borderClass: invalid ? 'border-danger hover:border-danger' : 'border-text-tertiary',
  };
}

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement> & { mono?: boolean }>(function Input(
  { className, mono, ...rest },
  ref,
) {
  const { borderClass, ...field } = useFieldProps(rest);
  return <input ref={ref} {...rest} {...field} className={cn(control, 'h-10 coarse:h-11', mono && 'font-mono', borderClass, className)} />;
});

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaHTMLAttributes<HTMLTextAreaElement>>(function Textarea(
  { className, rows = 3, ...rest },
  ref,
) {
  const { borderClass, ...field } = useFieldProps(rest);
  return <textarea ref={ref} rows={rows} {...rest} {...field} className={cn(control, 'py-2', borderClass, className)} />;
});
