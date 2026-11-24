import { useId } from 'react';
import { Field, Input } from '../../components/ui';

export function ProfileSearchField({ value, onChange }: { value: string; onChange: (value: string) => void }) {
  const id = useId();
  return (
    <div role="search" aria-label="Search profiles" className="sm:max-w-sm">
      <Field label="Search profiles" htmlFor={id}>
        <Input
          type="search"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder="Name, description or role"
          autoComplete="off"
        />
      </Field>
    </div>
  );
}
