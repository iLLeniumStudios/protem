import * as DropdownMenu from '@radix-ui/react-dropdown-menu';
import { Check, ChevronsUpDown, LogOut } from 'lucide-react';
import { cn, colorTransition, focusRing } from './cn';
import { useTheme, type ThemeMode } from './Theme';

const THEME_OPTIONS: { value: ThemeMode; label: string }[] = [
  { value: 'system', label: 'System' },
  { value: 'light', label: 'Light' },
  { value: 'dark', label: 'Dark' },
];

export interface AccountMenuPersona {
  value: string;
  label: string;
}

export interface AccountMenuProps {
  name: string;
  email: string;
  role: string;
  devAuth: boolean;
  personas: AccountMenuPersona[];
  activePersona: string;
  onSwitchPersona: (email: string) => void;
  signOutURL?: string;
  className?: string;
}

function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return '?';
  const first = parts[0]?.[0] ?? '';
  const last = parts.length > 1 ? (parts[parts.length - 1]?.[0] ?? '') : '';
  return (first + last).toLocaleUpperCase('en');
}

const content = cn(
  'z-50 w-64 max-w-[calc(100vw-2rem)] overflow-hidden rounded-md border border-border-strong bg-surface p-1 text-text shadow-lg',
  'max-h-[var(--radix-dropdown-menu-content-available-height)] overflow-y-auto',
);

const radioItem = cn(
  'relative flex min-h-9 cursor-default select-none items-center gap-2 rounded-sm py-2 pl-8 pr-3 text-body coarse:min-h-11',
  colorTransition,
  focusRing,
  'data-[highlighted]:bg-accent-soft data-[highlighted]:text-accent-text',
);

const menuItem = cn(
  'relative flex min-h-9 cursor-default select-none items-center gap-2 rounded-sm px-3 py-2 text-body coarse:min-h-11',
  colorTransition,
  focusRing,
  'data-[highlighted]:bg-accent-soft data-[highlighted]:text-accent-text',
);

export function AccountMenu({ name, email, role, devAuth, personas, activePersona, onSwitchPersona, signOutURL, className }: AccountMenuProps) {
  const { mode, setMode } = useTheme();
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button
          type="button"
          className={cn(
            'flex min-w-0 items-center gap-3 rounded-md border border-border px-3 py-2 text-left',
            'hover:bg-subtle',
            colorTransition,
            focusRing,
            className,
          )}
        >
          <span
            aria-hidden
            className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-accent-soft text-label font-medium text-accent-text"
          >
            {initials(name)}
          </span>
          <span className="flex min-w-0 flex-1 flex-col">
            <span className="truncate text-body font-medium text-text">{name}</span>
            <span className="truncate text-caption text-text-secondary">{role}</span>
          </span>
          <ChevronsUpDown className="h-4 w-4 shrink-0 text-text-secondary" aria-hidden />
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content align="start" side="top" sideOffset={8} className={content}>
          <DropdownMenu.Label className="flex flex-col px-3 py-2">
            <span className="truncate text-body font-medium text-text">{name}</span>
            <span className="truncate text-caption text-text-secondary">{email}</span>
          </DropdownMenu.Label>
          <DropdownMenu.Separator className="my-1 h-px bg-border" />
          <DropdownMenu.Label className="px-3 py-1 text-caption text-text-secondary">Theme</DropdownMenu.Label>
          <DropdownMenu.RadioGroup value={mode} onValueChange={(v) => setMode(v as ThemeMode)}>
            {THEME_OPTIONS.map((option) => (
              <DropdownMenu.RadioItem key={option.value} value={option.value} className={radioItem}>
                <DropdownMenu.ItemIndicator className="absolute left-2 inline-flex">
                  <Check className="h-4 w-4 shrink-0" aria-hidden />
                </DropdownMenu.ItemIndicator>
                {option.label}
              </DropdownMenu.RadioItem>
            ))}
          </DropdownMenu.RadioGroup>
          {devAuth && (
            <>
              <DropdownMenu.Separator className="my-1 h-px bg-border" />
              <DropdownMenu.Label className="px-3 py-1 text-caption text-text-secondary">Switch persona</DropdownMenu.Label>
              <DropdownMenu.RadioGroup value={activePersona} onValueChange={onSwitchPersona}>
                {personas.map((persona) => (
                  <DropdownMenu.RadioItem key={persona.value} value={persona.value} className={radioItem}>
                    <DropdownMenu.ItemIndicator className="absolute left-2 inline-flex">
                      <Check className="h-4 w-4 shrink-0" aria-hidden />
                    </DropdownMenu.ItemIndicator>
                    {persona.label}
                  </DropdownMenu.RadioItem>
                ))}
              </DropdownMenu.RadioGroup>
            </>
          )}
          {signOutURL && (
            <>
              <DropdownMenu.Separator className="my-1 h-px bg-border" />
              <DropdownMenu.Item asChild className={menuItem}>
                <a href={signOutURL}>
                  <LogOut className="h-4 w-4 shrink-0" aria-hidden />
                  Sign out
                </a>
              </DropdownMenu.Item>
            </>
          )}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}
