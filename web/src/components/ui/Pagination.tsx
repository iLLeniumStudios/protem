import { ChevronLeft, ChevronRight } from 'lucide-react';
import { lastPage, rangeText } from '../../app/paging';
import { Button } from './Button';
import { cn } from './cn';

export interface PaginationProps {
  /** What is being paged, for the buttons' accessible names, e.g. "my requests". */
  label: string;
  /** 0-based. */
  page: number;
  pageSize: number;
  total: number;
  onPageChange: (page: number) => void;
  className?: string;
}

/** Previous and next page controls with the visible range; renders nothing when everything fits on one page. */
export function Pagination({ label, page, pageSize, total, onPageChange, className }: PaginationProps) {
  if (total <= pageSize && page === 0) return null;
  const last = lastPage(total, pageSize);
  return (
    <nav aria-label={`Pages of ${label}`} className={cn('flex flex-wrap items-center justify-between gap-3', className)}>
      <p className="text-body tabular-nums text-text-secondary" aria-live="polite">
        {rangeText(page, pageSize, total)}
      </p>
      <div className="flex gap-2">
        <Button size="sm" icon={ChevronLeft} disabled={page <= 0} onClick={() => onPageChange(page - 1)}>
          Previous page<span className="sr-only"> of {label}</span>
        </Button>
        <Button size="sm" icon={ChevronRight} disabled={page >= last} onClick={() => onPageChange(page + 1)}>
          Next page<span className="sr-only"> of {label}</span>
        </Button>
      </div>
    </nav>
  );
}
