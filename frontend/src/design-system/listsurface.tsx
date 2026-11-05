import { type ReactNode, useEffect, useState } from "react";
import { openingCase } from "../format/collate";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { useNarrowViewport } from "../lib/viewport";
import { OverflowMenu } from "./atoms";
import {
  type ListChip,
  type ListView,
  nextSortValue,
  type SortControl,
  type SortOption,
  sortDirection,
} from "./listsurface.dials";
import "./listtable.css";
import { Heading } from "./heading";
import { useTruncationTooltip } from "./tooltip";

// The list surface's shell: the header (view tabs, count, primary action),
// the caption and the toolbar (search, filter chips, an archived toggle and
// whatever presentation dials the caller owns). It knows nothing about
// columns, density, paging or rows, which belong to ListTable. The pipeline
// board reuses this same shell, as a contact list and a lead list do. The
// dials belong to the surface, not to what fills it.
//
// A view tab reports only its index. What a view MEANS — a sort, a set of
// filters, a stage — is the caller's to decide, which is what lets the board
// define views that are not a sort/filter preset at all. An index is fine to
// report a PRESS with, since it names a tab in the array just rendered; it is
// not an identity, which is why a tab carries its own `id` for the caller that
// has to remember which view is lit across a rename or an insertion.

// The dials' vocabulary is declared in the leaf beside this file; a caller that
// has always imported it from here goes on doing so.
export {
  type ListChip,
  type ListView,
  nextSortValue,
  type SortControl,
  type SortOption,
  sortDirection,
};

import { useCloseOnEscape, useCloseOnOutsideClick } from "./listmenu";
import { ArchivedSwitch, Toolbar } from "./listtoolbar";

export { Menu, useCloseOnEscape, useCloseOnOutsideClick } from "./listmenu";

const EMPTY_FILTERS: Readonly<Record<string, string>> = {};

export function ListSurface({
  title,
  bare = false,
  views = [],
  activeView = 0,
  onViewChange,
  count,
  action,
  caption,
  search,
  sort,
  sortOptions = [],
  chips = [],
  chosen = EMPTY_FILTERS,
  onChipChange,
  archived,
  tools,
  displayMenu,
  saveView,
  children,
  footer,
}: Readonly<{
  /**
   * The page's own name, when this surface IS the page.
   *
   * A list screen is a header row, a toolbar and rows: the name printed above
   * that card cost a whole band of a phone's screen to say one word the top
   * bar's trail had already said, and pushed the dials below the fold. Given
   * here it stands in the header it belongs to, on the line that already
   * carries the tabs and the count.
   *
   * It renders an h1, so the shell must not print one too — a screen that
   * passes this joins `SELF_HEADED_SCREENS` (app/pagemeta.ts) in the same
   * change, or the page is named twice at heading level.
   */
  title?: ReactNode;
  /**
   * No count and no toolbar, for a list with nothing to narrow. The archived
   * switch stays: the rows in hand never include archived ones.
   */
  bare?: boolean;
  views?: readonly ListView[];
  activeView?: number;
  onViewChange?: (index: number) => void;
  /** The reader's-eye summary line — the table's range, the board's deal count. */
  count?: ReactNode;
  /** The one primary action for this surface, e.g. "New contact". */
  action?: ReactNode;
  /**
   * A standing note about what this list is, when the list needs one. The
   * screen's name is not it: the shell already says which screen you are on,
   * and repeating it here would title the surface twice.
   */
  caption?: ReactNode;
  /** Omit for a list whose GET has no `q` param; the box is then not rendered. */
  search?: { value: string; onChange: (next: string) => void };
  chips?: readonly ListChip[];
  chosen?: Readonly<Record<string, string>>;
  /** Called with "" to clear. */
  onChipChange?: (key: string, value: string) => void;
  /** Omit for a body with no server sort at all. */
  sort?: SortControl;
  /**
   * The attributes the sort menu offers. A column header stays the other route
   * to the same state — this is not its replacement, and both go through
   * `nextSortValue`, so the two cannot disagree about which press descends.
   * Empty, or without `sort`, draws no menu: a dial over an order the server
   * cannot change is a control that does nothing.
   */
  sortOptions?: readonly SortOption[];
  archived?: { checked: boolean; onChange: (next: boolean) => void };
  /**
   * Controls that change HOW the body is shown rather than what is in it, kept
   * to the right: the table's Display menu, the board/table switch, the
   * pipeline being looked at.
   */
  tools?: ReactNode;
  /**
   * A popover the BODY brings to the row — the table's Display menu — drawn
   * between the sort dial and `tools`.
   *
   * A render prop, and that is the whole of why it exists: ONE MENU OPEN AT A
   * TIME IS A PROPERTY OF THE ROW, not of any dial standing on it, so the state
   * belongs where the row does. The body held its own `open` beside this one
   * and the two agreed about nothing — Sort and Display stood open side by
   * side, and Escape reached only whichever had spelled its own handler.
   */
  displayMenu?: (
    menu: Readonly<{ open: boolean; onToggle: () => void }>,
  ) => ReactNode;
  /**
   * Saving what the dials currently say, and it is a slot of its own so that
   * "last on the row" is a fact about this component rather than a habit each
   * screen keeps. It was the final node of every caller's `tools` fragment,
   * which held only while every one of them remembered to put it there.
   */
  saveView?: ReactNode;
  /** The body: a scrolling table, a Kanban board, whatever this surface holds. */
  children: ReactNode;
  /** Under the body — the table's pager, or nothing at all. */
  footer?: ReactNode;
}>) {
  const [openMenu, setOpenMenu] = useState<string | null>(null);
  useCloseOnOutsideClick(() => setOpenMenu(null));
  useCloseOnEscape(openMenu, () => setOpenMenu(null));

  return (
    <div className="lt">
      {(!bare || title || views.length > 0 || action) && (
        <div className="lt-head">
          {title && (
            // Out of the tab order; a press that takes its own button away
            // hands focus here (listfocus.ts).
            <Heading size="xlarge" className="lt-title t-display" tabIndex={-1}>
              {title}
            </Heading>
          )}
          {views.length > 0 && (
            <div className="lt-views">
              {views.map((view, index) => (
                <button
                  type="button"
                  key={view.id ?? view.label}
                  className={`lt-vtab${index === activeView ? " on" : ""}`}
                  aria-pressed={index === activeView}
                  onClick={() => onViewChange?.(index)}
                >
                  {view.label}
                </button>
              ))}
            </div>
          )}
          <div className="lt-head-end">
            {!bare && <HeadCount>{count}</HeadCount>}
            <HeadActions>{action}</HeadActions>
          </div>
        </div>
      )}

      {caption && <p className="lt-caption">{caption}</p>}

      {!bare && (
        <Toolbar
          search={search}
          sort={sort}
          sortOptions={sortOptions}
          chips={chips}
          chosen={chosen}
          onChipChange={onChipChange}
          archived={archived}
          tools={tools}
          displayMenu={displayMenu}
          saveView={saveView}
          openMenu={openMenu}
          setOpenMenu={setOpenMenu}
        />
      )}
      {bare && archived && (
        <div className="lt-tools">
          <ArchivedSwitch archived={archived} />
        </div>
      )}

      {children}

      {footer}
    </div>
  );
}

/**
 * A list header's verbs, folded into ONE menu once the row stops holding them.
 *
 * The SAME nodes move into the menu; nothing is rendered twice. That is what
 * `OverflowMenu` is for — it takes the caller's own controls as children and
 * never unmounts them once they are up, so a verb that opens a dialog still has
 * somewhere to hand focus back to. A second, hidden copy of every verb would
 * give a screen reader two controls of one name and mount every dialog twice.
 *
 * `keepMounted`, and it is load-bearing rather than a hint: the menu otherwise
 * defers its children to the first open, and a header verb that reads its
 * opening state once at mount is then dead — `#/deals/new` opened no create
 * dialog at all below this width, and pressing the menu opened one nobody had
 * asked for. One menu per page and a handful of buttons in it, so there is
 * nothing to defer here anyway.
 *
 * It reads the VIEWPORT rather than the header's own box: a container query
 * cannot move an element into a menu, and measuring the row would mean laying
 * it out overflowing first and then reflowing it, which a reader sees.
 */
function HeadActions({ children }: Readonly<{ children: ReactNode }>) {
  const t = useT();
  const narrow = useNarrowViewport();
  const verbs = narrow ? (
    <OverflowMenu label={t("list.headActions")} keepMounted>
      {children}
    </OverflowMenu>
  ) : (
    children
  );
  return children ? <span className="lt-actions">{verbs}</span> : null;
}

/**
 * The count sentence: the one thing on the header row that gives way, since the
 * tabs and verbs are what a reader presses (listtable.css `.lt-head-end`). It
 * arrives as a node, so the truncation tip reads the words the node rendered.
 */
function HeadCount({ children }: Readonly<{ children: ReactNode }>) {
  const [text, setText] = useState("");
  const tip = useTruncationTooltip<HTMLSpanElement>(text);
  useEffect(() => setText(tip.ref.current?.textContent ?? ""));
  return (
    <span className="lt-count" ref={tip.ref} {...tip.trigger}>
      {children}
      {tip.tip}
    </span>
  );
}

/**
 * What the reader is looking at: the visible range, the size of the set behind
 * it, and the ordering. Stated in words rather than left to a lone arrow, since
 * a sort applied from a saved view has no arrow to notice.
 */
export function CountLine({
  unit,
  first,
  last,
  total,
  more = false,
  sortedBy,
}: Readonly<{
  unit: string;
  first: number;
  last: number;
  total: number;
  /**
   * The server holds pages past the ones counted. `total` is then how many rows
   * have been LOADED, not how many exist — a keyset cursor never learns the
   * second number — and the line has to say which of the two it is rather than
   * report a total the client cannot know.
   */
  more?: boolean;
  sortedBy?: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  // All three figures are magnitudes in ONE sentence ("1–25 of 1,234 rows"), so
  // they group together or the line reads as two notations at once.
  const range = {
    first: formatNumber(first, locale),
    last: formatNumber(last, locale),
    count: formatNumber(total, locale),
    unit,
  };
  // An empty list says why in its body: none yet, none matching, a failed read.
  // A zero here would only repeat that sentence or contradict it.
  const counted =
    total === 0
      ? ""
      : more
        ? t("table.rangeLoaded", range)
        : t("table.range", range);
  const order = sortedBy
    ? t("table.sortedBy", { column: sortedBy })
    : undefined;
  // The surface owns the count's placement; this only says what it reads.
  //
  // The comma joins two clauses and is written only when there are two: with the
  // count withheld it had nothing on its left, and the line opened on a piece of
  // punctuation.
  return (
    <>
      {counted}
      {order && (counted ? `, ${order}` : openingCase(order, locale))}
    </>
  );
}
