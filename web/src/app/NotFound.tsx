import { Link } from 'react-router-dom';
import { MapPinOff } from 'lucide-react';
import { EmptyState, PageHeader, buttonClass } from '../components/ui';

export function NotFound() {
  return (
    <>
      <PageHeader title="Page not found" description="This address does not match any page in Protem." />
      <EmptyState
        icon={MapPinOff}
        title="Nothing lives at this address"
        body="The link may be old, or the request or grant may have been removed."
        action={
          <Link to="/" className={buttonClass('primary')}>
            Go to For you
          </Link>
        }
      />
    </>
  );
}
