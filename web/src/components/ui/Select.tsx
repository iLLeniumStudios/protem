import { useId } from 'react';
import * as RadixSelect from '@radix-ui/react-select';
import { Check, ChevronDown } from 'lucide-react';
import { cn, colorTransition, focusRing } from './cn';

export interface SelectOption {
  value: string;
  label: string;
}

export interface SelectProps {
  value: string;
  onValueChange: (value: string) => void;
  options: SelectOption[];
  label: string;
  placeholder?: string;
  disabled?: boolean;
  className?: string;
}

export function Select({ value, onValueChange, options, label, placeholder, disabled, className }: SelectProps) {
  const labelId = useId();
  return (
    <div className={cn('flex min-w-0 flex-col gap-1', className)}>
      <span id={labelId} className="text-label text-text">
        {label}
      </span>
      <RadixSelect.Root value={value} onValueChange={onValueChange} disabled={disabled}>
        <RadixSelect.Trigger
          aria-labelledby={labelId}
          className={cn(
            'inline-flex h-10 w-full min-w-0 items-center justify-between gap-2 rounded-md border border-text-tertiary bg-surface pl-3 pr-2 text-left text-body text-text coarse:h-11',
            'hover:border-text-secondary disabled:cursor-not-allowed disabled:opacity-50 data-[placeholder]:text-text-tertiary',
            colorTransition,
            focusRing,
          )}
        >
          <span className="min-w-0 truncate">
            <RadixSelect.Value placeholder={placeholder} />
          </span>
          <RadixSelect.Icon>
            <ChevronDown className="h-4 w-4 shrink-0 text-text-secondary" aria-hidden />
          </RadixSelect.Icon>
        </RadixSelect.Trigger>
        <RadixSelect.Portal>
          <RadixSelect.Content
            position="popper"
            sideOffset={4}
            className="z-50 max-h-popover min-w-trigger overflow-hidden rounded-md border border-border-strong bg-surface text-text"
          >
            <RadixSelect.Viewport className="p-1">
              {options.map((o) => (
                <RadixSelect.Item
                  key={o.value}
                  value={o.value}
                  className={cn(
                    'relative flex min-h-9 cursor-default select-none items-center rounded-sm py-2 pl-8 pr-3 text-body coarse:min-h-11',
                    'focus-visible:outline focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-focus',
                    'data-[highlighted]:bg-accent-soft data-[highlighted]:text-accent-text data-[disabled]:opacity-50',
                  )}
                >
                  <RadixSelect.ItemIndicator className="absolute left-2 inline-flex">
                    <Check className="h-4 w-4" aria-hidden />
                  </RadixSelect.ItemIndicator>
                  <RadixSelect.ItemText>{o.label}</RadixSelect.ItemText>
                </RadixSelect.Item>
              ))}
            </RadixSelect.Viewport>
          </RadixSelect.Content>
        </RadixSelect.Portal>
      </RadixSelect.Root>
    </div>
  );
}
