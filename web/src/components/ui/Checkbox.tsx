import { useId, type ReactNode } from 'react';
import { Check } from 'lucide-react';
import { cn, colorTransition, focusRing } from './cn';

export interface CheckboxProps {
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  label: ReactNode;
  description?: ReactNode;
  disabled?: boolean;
  id?: string;
}

export function Checkbox({ checked, onCheckedChange, label, description, disabled, id }: CheckboxProps) {
  const autoId = useId();
  const inputId = id ?? autoId;
  const descriptionId = description ? `${inputId}-description` : undefined;
  return (
    <div className="flex min-w-0 items-start gap-3">
      <span className="relative mt-0.5 inline-flex h-5 w-5 shrink-0">
        <input
          id={inputId}
          type="checkbox"
          checked={checked}
          disabled={disabled}
          aria-describedby={descriptionId}
          onChange={(e) => onCheckedChange(e.target.checked)}
          className={cn(
            'peer h-5 w-5 cursor-pointer appearance-none rounded-sm border border-text-tertiary bg-surface disabled:opacity-50',
            'hover:border-text-secondary checked:border-accent checked:bg-accent disabled:cursor-not-allowed',
            colorTransition,
            focusRing,
          )}
        />
        <Check
          className="pointer-events-none absolute inset-0.5 hidden h-4 w-4 text-accent-ink peer-checked:block peer-disabled:opacity-50"
          strokeWidth={3}
          aria-hidden
        />
      </span>
      <span className="flex min-w-0 flex-col">
        <label
          htmlFor={inputId}
          className={cn('text-body', disabled ? 'cursor-not-allowed text-text-tertiary' : 'cursor-pointer text-text')}
        >
          {label}
        </label>
        {description && (
          <span id={descriptionId} className={cn('text-caption', disabled ? 'text-text-tertiary' : 'text-text-secondary')}>
            {description}
          </span>
        )}
      </span>
    </div>
  );
}
