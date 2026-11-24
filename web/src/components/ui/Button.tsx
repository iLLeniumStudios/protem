import { forwardRef, type ButtonHTMLAttributes, type MouseEvent } from 'react';
import { LoaderCircle, type LucideIcon } from 'lucide-react';
import { cn, colorTransition, focusRing } from './cn';

export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger' | 'quiet' | 'reject' | 'approve';
export type ButtonSize = 'sm' | 'md';

const VARIANTS: Record<ButtonVariant, string> = {
  primary: 'bg-accent text-accent-ink hover:bg-accent-hover active:bg-accent-hover',
  secondary: 'border border-border-strong bg-transparent text-text hover:bg-subtle active:bg-muted',
  ghost: 'text-text-secondary hover:bg-subtle hover:text-text active:bg-muted',
  danger: 'bg-danger text-danger-ink hover:bg-danger-hover active:bg-danger-hover',
  /** A link-like action inside a row, such as "View request". */
  quiet: 'text-accent-text hover:bg-accent-soft active:bg-accent-soft',
  /** Outlined decisions in a list row, quieter than `danger` and `primary`. */
  reject: 'border border-border-strong bg-transparent text-danger-text hover:bg-danger-soft active:bg-danger-soft',
  approve: 'border border-border-strong bg-transparent text-success-text hover:bg-success-soft active:bg-success-soft',
};

const SIZES: Record<ButtonSize, string> = {
  sm: 'h-8 gap-1.5 px-3 text-label coarse:h-11',
  md: 'h-10 gap-2 px-4 text-body font-medium coarse:h-11',
};

/** Classes for a router link that should look like a button. */
export function buttonClass(variant: ButtonVariant = 'secondary', size: ButtonSize = 'md') {
  return cn(
    'inline-flex shrink-0 select-none items-center justify-center whitespace-nowrap rounded-md',
    'disabled:pointer-events-none disabled:opacity-50',
    colorTransition,
    focusRing,
    VARIANTS[variant],
    SIZES[size],
  );
}

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  loading?: boolean;
  icon?: LucideIcon;
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { variant = 'secondary', size = 'md', loading = false, icon: Icon, className, children, onClick, type = 'button', ...rest },
  ref,
) {
  const iconClass = size === 'sm' ? 'h-4 w-4 shrink-0' : 'h-5 w-5 shrink-0';
  return (
    <button
      ref={ref}
      type={type}
      aria-busy={loading || undefined}
      onClick={(e: MouseEvent<HTMLButtonElement>) => {
        if (loading) e.preventDefault();
        else onClick?.(e);
      }}
      className={cn(buttonClass(variant, size), loading && 'cursor-progress', className)}
      {...rest}
    >
      {loading ? (
        <LoaderCircle className={cn(iconClass, 'animate-spin')} aria-hidden />
      ) : (
        Icon && <Icon className={iconClass} aria-hidden />
      )}
      {children}
    </button>
  );
});

export interface IconButtonProps extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'children'> {
  label: string;
  icon: LucideIcon;
  variant?: ButtonVariant;
}

export const IconButton = forwardRef<HTMLButtonElement, IconButtonProps>(function IconButton(
  { label, icon: Icon, variant = 'ghost', className, type = 'button', ...rest },
  ref,
) {
  return (
    <button
      ref={ref}
      type={type}
      aria-label={label}
      title={label}
      className={cn(buttonClass(variant), 'w-10 px-0 coarse:w-11', className)}
      {...rest}
    >
      <Icon className="h-5 w-5 shrink-0" aria-hidden />
    </button>
  );
});
