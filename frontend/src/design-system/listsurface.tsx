import {
  type ComponentProps,
  type ReactNode,
  useEffect,
  useRef,
  useState,
} from "react";
import { openingCase } from "../format/collate";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { useNarrowViewport } from "../lib/viewport";
import { OverflowMenu } from "./atoms";
import { useCloseOnEscape, useCloseOnOutsideClick } from "./listmenu";
import {
  type ListChip,
  type ListView,
  nextSortValue,
  type SortControl,
  type SortOption,
  sortDirection,
} from "./listsurface.dials";
import { ArchivedSwitch, Toolbar, VIEW_MENU } from "./listtoolbar";
import type { ViewRow, ViewToggle } from "./listviewmenu";
import "./listtable.css";
import { Heading } from "./heading";
import { useTruncationTooltip } from "./tooltip";

// The list surface's shell: header, caption and toolbar. It knows nothing of
// columns, density, paging or rows, so the pipeline board reuses it too.
//
// A view tab reports only its index. What a view MEANS — a sort, a set of
// filters, a stage — is the caller's to decide, which is what lets the board
// define views that are not a sort/filter preset at all. An index is fine to
// report a PRESS with, since it names a tab in the array just rendered; it is
// not an identity, which is why a tab carries its own `id` for the caller that
// has to remember which view is lit across a rename or an insertion.

export { Menu, useCloseOnEscape, useCloseOnOutsideClick } from "./listmenu";
export type { ViewRow, ViewToggle } from "./listviewmenu";

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

const EMPTY_FILTERS: Readonly<Record<string, string>> = {};
const NO_ROWS: readonly ViewRow[] = [];
const NO_TOGGLES: readonly ViewToggle[] = [];

export function ListSurface({
  title,
  bare = false,
  views = [],
  activeView = 0,
  onViewChange,
  count,
  holdCount = false,
  action,
  caption,
  search,
  sort,
  sortOptions = [],
  chips = [],
  chosen = EMPTY_FILTERS,
  onChipChange,
  archived,
  layout,
  viewRows = NO_ROWS,
  displayRows = NO_ROWS,
  displayToggles = NO_TOGGLES,
  changed = false,
  tools,
  saveView,
  notice,
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
  /**
   * The reader's-eye summary line: the table's range, the board's deal count.
   * Omitted when the list has nothing to say, and then no line is drawn.
   */
  count?: ReactNode;
  /**
   * The count line keeps its height, blank, while a count may be refilled: a
   * count is loading, or a narrowed list has no rows yet.
   */
  holdCount?: boolean;
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
  /** How the body is drawn (Table or Board), at the top of the View panel. */
  layout?: ReactNode;
  /** The screen's own dials, as the View panel's first rows (a pipeline). */
  viewRows?: readonly ViewRow[];
  /** The body's presentation dials, as the View panel's last rows. */
  displayRows?: readonly ViewRow[];
  /** The body's two-state dials, after Show archived in the panel's end group. */
  displayToggles?: readonly ViewToggle[];
  /** The list departs from every view it offers; View carries a dot. */
  changed?: boolean;
  /**
   * A verb that is not a dial (a company's Coverage explorer), at the end of
   * the toolbar row.
   */
  tools?: ReactNode;
  /** Save view and Manage views, in the View panel's footer. */
  saveView?: ReactNode;
  /**
   * A line of its own under the search, for what the dials depend on and
   * could not get: a failed saved-views read, with its Retry.
   */
  notice?: ReactNode;
  /** The body: a scrolling table, a Kanban board, whatever this surface holds. */
  children: ReactNode;
  /** Under the body — the table's pager, or nothing at all. */
  footer?: ReactNode;
}>) {
  const [openMenu, setOpenMenu] = useState<string | null>(null);
  useCloseOnOutsideClick(() => setOpenMenu(null));
  // The View panel and its phone sheet close on Escape and hand focus back
  // themselves. A second handler here would land focus inside the sheet.
  useCloseOnEscape(openMenu === VIEW_MENU ? null : openMenu, () =>
    setOpenMenu(null),
  );
  // A list can turn short under an open panel. It keeps its toolbar until the
  // panel closes, so the panel never vanishes under the reader.
  const chromeless = bare && openMenu === null;
  const root = useRef<HTMLDivElement>(null);
  const focused = useRef<Element | null>(null);
  useEffect(() => {
    const element = root.current;
    const remember = (event: FocusEvent) => {
      focused.current = event.target instanceof Element ? event.target : null;
    };
    element?.addEventListener("focusin", remember);
    return () => element?.removeEventListener("focusin", remember);
  }, []);
  // The toolbar and the bare switch swap whole, so focus taken with one goes
  // to the other's control for the same dials.
  useEffect(() => {
    const lost = focused.current;
    if (
      lost === null ||
      lost.isConnected ||
      document.activeElement !== document.body
    ) {
      return;
    }
    root.current
      ?.querySelector<HTMLElement>(
        chromeless ? ".lt-tools .lt-vtoggle" : ".lt-tools .lt-vtrigger",
      )
      ?.focus();
  }, [chromeless]);

  return (
    <div className="lt" ref={root}>
      {(!chromeless || title || views.length > 0 || action) && (
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
            {!chromeless && <HeadCount hold={holdCount}>{count}</HeadCount>}
            <HeadActions>{action}</HeadActions>
          </div>
        </div>
      )}

      {caption && <p className="lt-caption">{caption}</p>}

      {!chromeless && (
        <Toolbar
          search={search}
          sort={sort}
          sortOptions={sortOptions}
          chips={chips}
          chosen={chosen}
          onChipChange={onChipChange}
          archived={archived}
          layout={layout}
          viewRows={viewRows}
          displayRows={displayRows}
          displayToggles={displayToggles}
          changed={changed}
          tools={tools}
          saveView={saveView}
          notice={notice}
          openMenu={openMenu}
          setOpenMenu={setOpenMenu}
        />
      )}
      {chromeless && archived && (
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
function HeadCount({
  hold,
  children,
}: Readonly<{ hold: boolean; children: ReactNode }>) {
  const says =
    children !== undefined && children !== null && children !== false;
  // A first load holds the line, since most lists have rows to count.
  const [held, setHeld] = useState(true);
  if (!hold && held !== says) {
    setHeld(says);
  }
  const [text, setText] = useState("");
  const tip = useTruncationTooltip<HTMLSpanElement>(text);
  useEffect(() => setText(tip.ref.current?.textContent ?? ""));
  if (!says && !(hold && held)) {
    return null;
  }
  return (
    <span className="lt-count" ref={tip.ref} {...tip.trigger}>
      {children}
      {tip.tip}
    </span>
  );
}

/** The table's range line, or none while it is pending or has nothing to say. */
export function rangeLine(
  pending: boolean,
  line: ComponentProps<typeof CountLine>,
): ReactNode {
  return !pending && countSays(line.total, line.sortedBy) ? (
    <CountLine {...line} />
  ) : undefined;
}

/** Whether `CountLine` has words: rows to count, or an order to name. */
export function countSays(total: number, sortedBy?: string): boolean {
  return total > 0 || Boolean(sortedBy);
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
  scope,
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
  /** What the counted rows belong to, when the list reads one part of a set (a pipeline). */
  scope?: string;
  sortedBy?: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  if (!countSays(total, sortedBy)) {
    return null;
  }
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
  const ranged = more ? t("table.rangeLoaded", range) : t("table.range", range);
  const scoped = scope ? t("table.rangeIn", { range: ranged, scope }) : ranged;
  const counted = total === 0 ? "" : scoped;
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
