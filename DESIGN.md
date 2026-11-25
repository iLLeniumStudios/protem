# Design

The durable visual decisions of the Protem UI (`web/`), as built. Read it before adding a screen. Who uses Protem and why is in [PRODUCT.md](PRODUCT.md); the fixed terms are in [GLOSSARY.md](GLOSSARY.md).

## Two workspaces

The frame (`web/src/app/Shell.tsx`) has a left navigation and one of two workspaces, chosen by the first path segment (`workspaceOf` in `web/src/app/roles.ts`):

- **My access**, for requesters and approvers: `/` (For you), `/request` and `/request/:ns/:profile` (Activate access and its form), `/requests` (My requests), `/requests/:ns/:name`, `/grants/:ns/:name`, `/approvals`.
- **Administration**, for administrators: `/admin` (Overview), `/timeline` (Access timeline), `/profiles`, `/clusters`, `/audit`.

The workspace switch shows only to someone with at least one Administration item. A new page joins one workspace by adding its item to `NAV` in `roles.ts`; never show an item the person cannot use.

## Tokens

All colour, type and radius comes from `web/src/styles/tokens.css` through Tailwind. No hex values, no Tailwind palette colours, no arbitrary font sizes in pages.

**Type scale.** One family (Public Sans) for UI, IBM Plex Mono for identifiers.

- `caption`: secondary facts under a name, badge text.
- `label`: controls, row facts, section counts.
- `body`: running text and row titles.
- `title`: section headings (`Section`), the brand.
- `heading`: dialog and side panel titles.
- `page`: the one page title in `PageHeader`.
- `display`, `stat`: defined for large figures; not used by any page yet.

**Colour roles.** Every role has a light and a dark value.

- Surfaces: `bg` (the page), `surface` (cards, panels), `subtle` and `muted` (hover, fills, tracks).
- Lines: `border` for dividers and cards, `border-strong` for control outlines.
- Text: `text`, `text-secondary` for supporting facts, `text-tertiary` for icons and counts only.
- `accent`: the brand blue, for the one primary action, links (`accent-text`), the current navigation item (`accent-soft`) and focus (`focus`).
- Status: `success`, `warning`, `danger`, `info`. Each has a solid (`success`), an ink to sit on it (`success-ink`), a soft background (`success-soft`) and a text colour (`success-text`). Use `*-text` for words and icons on a surface, `*-soft` behind them in a badge, the solid only for fills such as a progress bar or the destructive confirm button.
- `chart-*` and `scrim` exist for charts and overlays; there are no charts.

**Radii.** `sm` for chips, menu items and skeletons; `md` for buttons, inputs and menus; `lg` for cards, panels, row lists and dialogs; `full` for badges, pills, filter chips and avatars. `xl` is unused.

## Kit

Everything lives in `web/src/components/ui/` and is exported from `index.ts`. Add to the kit before a page builds its own.

- `PageHeader`: page title, one-line description and the page's actions. One per page.
- `Section`: a titled block with a quiet count and an optional link at the far end. Not a card; put a `RowList` or `Panel` inside.
- `Panel`: a titled card of facts or a form. Not for lists of things; use `RowList`.
- `RowList` and `Row`: the list of requests, grants or profiles on personal pages, one card divided by rules. `Row selected` marks the row whose detail is open in the side panel. Not for comparing many columns; use a table.
- `QuietLine`: what a section says when it has nothing to show. `EmptyState` only when the whole page has nothing.
- `Table` (with `THead`, `TR`, `TH`, `TD`): a small table inside a detail page. `DenseTable`: the console table, 40 px rows and a sticky header.
- `ConsoleCard`, `ConsoleToolbar`, `SearchField`, `FilterChip`, `FigureButton`, `TablePager`, `TextButton`, `ActionMenu`: the console pieces. Toolbar above a card; figures that filter when chosen; text-like buttons where a full button does not fit in a dense row; row actions beyond the first in a menu. Not for personal pages.
- `Pagination`: previous and next for row lists. Use `TablePager` under console tables.
- `SidePanel`: the detail of a row beside its list. From 1280 px it is non-modal and the frame makes room; narrower it covers the page and traps focus. Not for forms that create something; those are pages.
- `Dialog`, `ConfirmDialog`, `Sheet`: a dialog for a short task; `ConfirmDialog` for a decision with an optional or required reason; `Sheet` for the narrow-screen navigation. Never a dialog for reading.
- `Button` and `buttonClass` (for router links): variants `primary`, `secondary`, `ghost`, `quiet` (link-like action in a row), `approve` and `reject` (outlined decisions in a row), `danger` (destructive confirm only). `IconButton` needs a label.
- `Field`, `Input`, `Textarea`, `Select`, `Checkbox`, `SegmentedControl`, `SegmentedNav`: form controls. `Select` is a listbox, never a native `<select>`.
- `Badge` (tones) and `StatusPill` (a phase with its icon): short status, always with words.
- `Identifier`: names of Kubernetes objects, groups and users, in mono.
- `TimeLeft`, `TimeUsed`, `RelativeTime`, `Elapsed`: time, always in a `<time>` with an ISO `dateTime`.
- `Toast`: confirms that an action was sent. Not for errors that need reading; those stay on the page or in the dialog.
- `Skeleton`: loading placeholders in the shape of what comes.

## Page anatomy

- A `PageHeader` first, with at most one `primary` action (For you: "Request access"); other actions are `secondary`.
- Then `Section`s, the most urgent first (on For you: what needs your decision, your access now, your requests).
- Page content is centred in the space beside the navigation. Personal pages (My access) and the profile pages are capped at 1400 px (`max-w-[87.5rem]`); console pages (Administration) at 1920 px (`max-w-[120rem]`), so tables do not stretch across a very wide screen.
- Detail pages (request, grant) are a `PageHeader` with the status pill and object name under the title, then `Panel`s.

## Request rows

`web/src/pages/access/RequestRows.tsx` builds every request row:

- Line one: who and what ("alice requests Dev edit") or the profile name on your own requests, then how long it has waited.
- Line two, facts with leading icons: role and namespaces, clusters, duration, ticket, approvals so far, and the flagged-rules marker when the role has rules flagged for review.
- Line three: the reason in quotes, then the actions: "View request" (`quiet`), then "Reject request" and "Approve request" (outlined).
- Grant rows show what and where, a time-used bar with the end time, and "Revoke access" (outlined).

## Status and tone

- Never colour alone: every status carries an icon and words; colour only reinforces it.
- Solid red only on the confirming button of a destructive dialog. Reject and revoke elsewhere are outlined (`reject` variant).
- Warning tone for what needs a look: the last 30 minutes of a grant, a long wait, flagged rules.
- `text-tertiary` never carries meaning on its own.

## Wording

- Controls are a verb and an object ("Approve request", "Revoke access"); headings are nouns ("Your access now"); sentence case everywhere.
- Terms: hub, cluster, agent, profile, request, grant. Never "spoke" or "enforcer".
- Empty states say what is missing and what to do next, in one line.
- Errors say what failed, then the server's message.

## Clock

All absolute times are local and 24-hour, in one shape (`formatTimestamp` in `web/src/components/ui/format.ts`): "09:01" today, "24 Nov, 09:01" another day, "24 Nov 2025, 09:01" another year. Durations and relative times read "1h 30m", "2h left", "4 min ago". Machine-readable `dateTime` attributes stay ISO.

## Deliberately absent

No charts, no bulk approval, no saved table views, no export, no settings page, no notifications. Do not add controls that imply them.
