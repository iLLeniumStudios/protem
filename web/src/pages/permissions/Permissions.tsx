import type { ReactNode } from 'react';
import { TriangleAlert } from 'lucide-react';
import type { PermissionRuleView, PermissionsView } from '../../generated/views';
import { Fact, Facts, IdList } from '../access/parts';
import { MobileRow, TBody, TD, TH, THead, TR, Table } from '../../components/ui';
import { cellValues, flaggedFirst, rulesSummary } from './rules';

const COLUMNS: { label: string; cell: (r: PermissionRuleView) => ReactNode }[] = [
  { label: 'API groups', cell: (r) => <IdList items={cellValues(r.apiGroups, 'group')} empty="None" /> },
  { label: 'Resources', cell: (r) => <IdList items={cellValues(r.resources)} empty="None" /> },
  { label: 'Verbs', cell: (r) => <IdList items={cellValues(r.verbs)} empty="None" /> },
  {
    label: 'Resource names',
    cell: (r) => <IdList items={cellValues(r.resourceNames)} empty={r.resources?.length ? 'Any' : 'None'} />,
  },
  { label: 'Non-resource URLs', cell: (r) => <IdList items={cellValues(r.nonResourceURLs)} empty="None" /> },
];

function Risks({ risks }: { risks: string[] }) {
  return (
    <div className="flex items-start gap-2 rounded-md bg-warning-soft px-3 py-2 text-body text-warning-text">
      <TriangleAlert className="mt-0.5 h-4 w-4 shrink-0" aria-hidden />
      <div className="flex min-w-0 flex-col gap-1">
        <span className="font-medium">Flagged for review</span>
        <ul className="flex list-disc flex-col gap-1 pl-5">
          {risks.map((risk) => (
            <li key={risk} className="break-words">
              {risk}
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
}

function RulesTable({ rules, label }: { rules: PermissionRuleView[]; label: string }) {
  return (
    <>
      <div className="hidden md:block">
        <Table label={label}>
          <THead>
            <TR>
              {COLUMNS.map((c) => (
                <TH key={c.label}>{c.label}</TH>
              ))}
            </TR>
          </THead>
          <TBody>
            {rules.flatMap((r, i) => {
              const row = (
                <TR key={i} className={r.risks?.length ? 'border-b-0' : undefined}>
                  {COLUMNS.map((c) => (
                    <TD key={c.label} className="min-w-28 max-w-xs">
                      {c.cell(r)}
                    </TD>
                  ))}
                </TR>
              );
              if (!r.risks?.length) return [row];
              return [
                row,
                <TR key={`${i}-risks`}>
                  <TD colSpan={COLUMNS.length} className="pt-0">
                    <Risks risks={r.risks} />
                  </TD>
                </TR>,
              ];
            })}
          </TBody>
        </Table>
      </div>
      <ul className="flex flex-col divide-y divide-border md:hidden" aria-label={label}>
        {rules.map((r, i) => (
          <MobileRow key={i}>
            {r.risks?.length ? <Risks risks={r.risks} /> : null}
            <Facts>
              {COLUMNS.map((c) => (
                <Fact key={c.label} label={c.label}>
                  {c.cell(r)}
                </Fact>
              ))}
            </Facts>
          </MobileRow>
        ))}
      </ul>
    </>
  );
}

/** What a role grants: where the answer came from, a count of flagged rules, and the rules, flagged first. */
export function Permissions({
  view,
  source,
  label = 'Permission rules',
}: {
  view: PermissionsView;
  source: string;
  label?: string;
}) {
  const rules = flaggedFirst(view.rules ?? []);
  const flagged = rules.some((r) => r.risks?.length);
  return (
    <div className="flex min-w-0 flex-col gap-4">
      <p className="text-label text-text-secondary">{source}</p>
      {!view.found ? (
        <p className="break-words text-body text-text">{view.message || 'The role could not be read.'}</p>
      ) : (
        <>
          <p className={flagged ? 'flex items-start gap-2 text-body font-medium text-warning-text' : 'text-body text-text'}>
            {flagged && <TriangleAlert className="mt-0.5 h-4 w-4 shrink-0" aria-hidden />}
            {rulesSummary(rules)}
          </p>
          {rules.length > 0 && <RulesTable rules={rules} label={label} />}
        </>
      )}
    </div>
  );
}
