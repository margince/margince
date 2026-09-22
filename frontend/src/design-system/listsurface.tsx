import {
  ArrowDown,
  ArrowDownUp,
  ArrowUp,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  Filter,
  MoreVertical,
  Plus,
  Search,
} from "lucide-react";
import { type ReactNode, useEffect, useId, useRef, useState } from "react";
import { useNarrowViewport } from "../app/viewport";
import { openingCase } from "../format/collate";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { Button, Checkbox, OverflowMenu, Radio } from "./atoms";
import { ChipValueList } from "./listfiltervalues";
import "./listtable.css";
import { Heading } from "./heading";

// The list surface's shell: the header (view tabs, count, primary action),
// the caption and the toolbar (search, filter chips, an archived toggle and
// whatever presentation dials the caller owns). It knows nothing about
// columns, density, paging or rows — those stay in listtable.tsx, and the
// pipeline board reuses this same shell for the same reason a contact list
// and a lead list do: the dials belong to the surface, not to what fills it.
//
// A view tab reports only its index. What a view MEANS — a sort, a set of
// filters, a stage — is the caller's to decide, which is what lets the board
// define views that are not a sort/filter preset at all. An index is fine to
// report a PRESS with, since it names a tab in the array just rendered; it is
// not an identity, which is why a tab carries its own `id` for the caller that
// has to remember which view is lit across a rename or an insertion.

/**
 * One filter chip. Single-select by design: the list endpoints take one value
 * per filter param, so a multi-select chip would compose a query the API
 * cannot answer.
 */
export type ListChip = {
  key: string;
  label: string;
  /** The "no filter" entry, which is also how a chosen value is cleared. */
  allLabel: string;
  options: readonly { value: string; label: string }[];
  /**
   * A relation filter too large to list whole (a workspace's companies): when
   * present, the value step searches this instead of walking `options`. The
   * "all" entry still clears the filter, and `options` stays required so a
   * chip declared without `search` needs no separate shape.
   */
  search?: (
    query: string,
  ) => Promise<readonly { value: string; label: string }[]>;
};

/** A saved view: a named tab whose meaning is entirely the caller's. */
export type ListView = {
  /**
   * What identifies this tab, when the caller has something steadier than its
   * name. Two saved views may share a name, so a rail keyed on the label
   * collides on the pair and React renders one of them; the label stays the
   * fallback for a rail whose tabs are a fixed set the caller wrote, where the
   * name IS the identity.
   */
  id?: string;
  label: string;
  sort?: string;
  filters?: Readonly<Record<string, string>>;
};

/**
 * One attribute the sort pill can order by. ListSurface never learns what a
 * column is — the table hands it this plain list, keyed by the same server
 * sort field a column header already uses.
 */
export type SortControl = {
  /** The server sort string, e.g. `-created_at`. */
  value: string;
  onChange: (next: string) => void;
};

/**
 * One attribute a list can be ordered by, named as the reader sees it.
 *
 * `field` is the same server sort string the matching column header sends, so
 * the menu and the header are two routes to ONE state rather than two states
 * that can disagree about what the list is ordered by.
 */
export type SortOption = {
  field: string;
  label: string;
  /** Biggest first on the opening press, as the numeric column header does. */
  numeric?: boolean;
};

/** Which way `value` orders `field`, or null when it orders something else. */
export function sortDirection(
  field: string,
  value: string,
): "asc" | "desc" | null {
  if (value === field) {
    return "asc";
  }
  return value === `-${field}` ? "desc" : null;
}

/**
 * The sort a press on `option` produces, given where it stands now.
 *
 * One function for the header and the menu both: a reader who flips a column
 * from its header and then reopens the sort menu must find the direction they
 * just chose, and two copies of this arithmetic are how the two ends up
 * disagreeing about which press descends.
 */
export function nextSortValue(
  option: Readonly<{ field: string; numeric?: boolean }>,
  direction: "asc" | "desc" | null,
): string {
  if (direction === "asc") {
    return `-${option.field}`;
  }
  if (direction === "desc") {
    return option.field;
  }
  // Unsorted, a number almost always wants its biggest value first.
  return option.numeric ? `-${option.field}` : option.field;
}

const EMPTY_FILTERS: Readonly<Record<string, string>> = {};

export function ListSurface({
  title,
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
   * pipeline being looked at, and the reader's own Save view last.
   */
  tools?: ReactNode;
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
      <div className="lt-head">
        {title && (
          <Heading size="xlarge" className="lt-title t-display">
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
        {/* Wrapped here rather than by the caller, so every body's count sits
            in the same place: pushed right of the view tabs, left of the
            action. */}
        {/* Always rendered, even with nothing to say yet: this element carries
            the margin that pushes itself and the action to the right, so a
            count that only arrives with the rows would otherwise let the action
            start at the left and jump across once they land. */}
        <span className="lt-count">{count}</span>
        <HeadActions>{action}</HeadActions>
      </div>

      {caption && <p className="lt-caption">{caption}</p>}

      <Toolbar
        search={search}
        sort={sort}
        sortOptions={sortOptions}
        chips={chips}
        chosen={chosen}
        onChipChange={onChipChange}
        archived={archived}
        tools={tools}
        openMenu={openMenu}
        setOpenMenu={setOpenMenu}
      />

      {children}

      {footer}
    </div>
  );
}

/**
 * A list header's verbs, folded into ONE menu once the row stops holding them.
 *
 * The header is view tabs, a count sentence and the verbs, and the sentence is
 * the part that grows: "1–25 of 200 contacts loaded so far, sorted by Created"
 * does not wrap and does not shrink, so below about a laptop's width it ran
 * straight through the buttons and the three verbs on the contacts list drew on
 * top of each other. Widths do not fix that — the sentence is a translated
 * string and German runs a third longer again.
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
  if (!children) {
    return null;
  }
  if (!narrow) {
    return children;
  }
  return (
    <OverflowMenu label={t("list.headActions")} keepMounted>
      {children}
    </OverflowMenu>
  );
}

/**
 * The one Filter trigger: a searchable list of the attributes not yet
 * applied, then — once one is picked — that attribute's value list. Reads as
 * the "Filter" button (its icon and label) until a filter exists, and as a
 * bare "+" once the first applied row is standing — the vocabulary the
 * button offers hasn't changed, only what it's called once there is
 * something beside it to add to. Its own component because the two steps,
 * plus the search text narrowing the first one, were most of the toolbar's
 * cognitive complexity.
 */
function FilterMenu({
  chips,
  chosen,
  onChipChange,
  onRemember,
  open,
  onToggle,
  hasApplied,
}: Readonly<{
  chips: readonly ListChip[];
  chosen: Readonly<Record<string, string>>;
  onChipChange?: (key: string, value: string) => void;
  /** Reports the label a value was picked under, for a searched chip. */
  onRemember: (key: string, value: string, label: string) => void;
  open: boolean;
  onToggle: () => void;
  hasApplied: boolean;
}>) {
  const t = useT();
  const [attributeKey, setAttributeKey] = useState<string | null>(null);
  const [query, setQuery] = useState("");
  const searchRef = useRef<HTMLInputElement>(null);
  // Only the back button sets this, so opening the menu with a pointer does not
  // pull focus into a search box the reader has not asked for.
  const steppingBack = useRef(false);

  // A closed menu always reopens at the attribute search, never wherever the
  // reader last left it — the button says "Filter", not "Filter: Status".
  useEffect(() => {
    if (!open) {
      setAttributeKey(null);
      setQuery("");
    }
  }, [open]);

  // Stepping back leaves the reader where they came FROM: the attribute row
  // they pressed went with the level it belonged to, so focus would otherwise
  // fall to the top of the document.
  useEffect(() => {
    if (attributeKey === null && steppingBack.current) {
      steppingBack.current = false;
      searchRef.current?.focus();
    }
  }, [attributeKey]);

  // Only the attributes with no filter row of their own yet: once a filter is
  // applied it stands as its own row (FilterRow below), so offering it again
  // here would be a second way to reach the same value, through a menu that
  // no longer names the attribute it belongs to.
  const addable = chips.filter((chip) => !chosen[chip.key]);
  const attribute = addable.find((chip) => chip.key === attributeKey);
  const narrowed = addable.filter((chip) =>
    chip.label.toLowerCase().includes(query.toLowerCase()),
  );

  return (
    <span className="lt-menu-wrap">
      {/* The "+" carries no caret: the word it dropped is what the caret
          qualifies, and a glyph-only trigger with one reads as two marks. */}
      {hasApplied ? (
        <Button
          iconOnly
          aria-expanded={open}
          aria-label={t("table.addFilter")}
          onClick={onToggle}
        >
          <Plus strokeWidth={1.8} aria-hidden="true" />
        </Button>
      ) : (
        <Button aria-expanded={open} onClick={onToggle}>
          <Filter strokeWidth={1.6} aria-hidden="true" />
          {t("table.filter")}
          <ChevronDown className="lt-caret" aria-hidden="true" />
        </Button>
      )}
      <Menu
        open={open}
        head={attribute ? attribute.label : t("table.filter")}
        back={
          attribute && {
            label: t("table.filterBack"),
            onBack: () => {
              steppingBack.current = true;
              setAttributeKey(null);
            },
          }
        }
      >
        {attribute ? (
          <ChipValueList
            chip={attribute}
            value={chosen[attribute.key] ?? ""}
            onPick={(value, label) => {
              onRemember(attribute.key, value, label);
              onChipChange?.(attribute.key, value);
              onToggle();
            }}
          />
        ) : (
          <>
            <label className="lt-fsearch">
              <span className="sr-only">{t("table.filterSearch")}</span>
              <input
                ref={searchRef}
                className="lt-fsearch-input"
                value={query}
                placeholder={t("table.filterSearch")}
                onChange={(event) => setQuery(event.target.value)}
              />
            </label>
            {narrowed.map((chip) => (
              <button
                type="button"
                key={chip.key}
                className="lt-mi"
                onClick={() => setAttributeKey(chip.key)}
              >
                {chip.label}
                {/* This row opens the attribute's own values rather than
                    picking one, and the glyph is all that says so. */}
                <ChevronRight
                  size={12}
                  strokeWidth={1.8}
                  aria-hidden="true"
                  className="lt-mi-more"
                />
              </button>
            ))}
          </>
        )}
      </Menu>
    </span>
  );
}

/**
 * The condition segment of an applied filter row. Its menu offers exactly
 * one entry — the list endpoints compare for equality only — but it is a
 * real menu rather than a disabled label, so a second condition can join it
 * later without a shape change here.
 */
function FilterConditionMenu({
  open,
  onToggle,
}: Readonly<{ open: boolean; onToggle: () => void }>) {
  const t = useT();
  const group = useId();
  return (
    <span className="lt-menu-wrap">
      <button
        type="button"
        className="lt-frow-seg"
        aria-expanded={open}
        onClick={onToggle}
      >
        {t("table.filterIs")}
      </button>
      <Menu open={open} head={t("table.filterCondition")}>
        {/* The one condition, standing. `defaultChecked` and not `checked`,
            because a group of one has no second answer for a change event to
            report: the press closes the menu and nothing else, and a controlled
            radio whose `onChange` can never fire would be a handler that lies
            about what runs. */}
        <Radio
          className="lt-mi"
          name={group}
          defaultChecked
          label={t("table.filterIs")}
          onClick={onToggle}
        />
      </Menu>
    </span>
  );
}

/** Which segment of a filter row currently owns the one open popover. */
type FilterRowSegment = "cond" | "value" | "menu";

function isFilterRowSegment(value: string): value is FilterRowSegment {
  return value === "cond" || value === "value" || value === "menu";
}

/**
 * Reads which segment (if any) of one row's own popover the toolbar's single
 * `openMenu` key names — a plain string key rather than per-row state, so
 * the existing one-menu-open-at-a-time behaviour covers filter rows too.
 */
function openRowSegment(
  openMenu: string | null,
  key: string,
): FilterRowSegment | null {
  const prefix = `row:${key}:`;
  if (!openMenu?.startsWith(prefix)) {
    return null;
  }
  const segment = openMenu.slice(prefix.length);
  return isFilterRowSegment(segment) ? segment : null;
}

/**
 * An applied filter, read as one row: the attribute, its condition, its
 * value, and a "⋮" menu to delete it — a compound pill rather than a bare
 * chip, so a second condition (already possible in FilterConditionMenu) has
 * somewhere to live without the row changing shape again.
 */
function FilterRow({
  chip,
  value,
  pickedLabel,
  openSegment,
  onToggleSegment,
  onPick,
  onDelete,
}: Readonly<{
  chip: ListChip;
  value: string;
  /**
   * What the reader picked, when the chip searched for it rather than listing
   * it. A searched chip carries no `options` to look the value back up in, so
   * without this the row would name a company by its id.
   */
  pickedLabel?: string;
  openSegment: FilterRowSegment | null;
  onToggleSegment: (segment: FilterRowSegment) => void;
  onPick: (value: string, label: string) => void;
  onDelete: () => void;
}>) {
  const t = useT();
  const active = chip.options.find((option) => option.value === value);
  const valueLabel = active?.label ?? pickedLabel ?? value;
  return (
    <fieldset className="lt-frow" aria-label={`${chip.label}: ${valueLabel}`}>
      <span className="lt-frow-attr">{chip.label}</span>
      <FilterConditionMenu
        open={openSegment === "cond"}
        onToggle={() => onToggleSegment("cond")}
      />
      <span className="lt-menu-wrap">
        <button
          type="button"
          className="lt-frow-seg lt-frow-value"
          aria-expanded={openSegment === "value"}
          onClick={() => onToggleSegment("value")}
        >
          {valueLabel}
        </button>
        <Menu open={openSegment === "value"} head={chip.label}>
          <ChipValueList chip={chip} value={value} onPick={onPick} />
        </Menu>
      </span>
      <span className="lt-menu-wrap">
        <button
          type="button"
          className="lt-frow-more"
          aria-label={t("table.filterMore", { filter: chip.label })}
          aria-expanded={openSegment === "menu"}
          onClick={() => onToggleSegment("menu")}
        >
          <MoreVertical size={14} strokeWidth={1.8} aria-hidden="true" />
        </button>
        <Menu open={openSegment === "menu"} head={chip.label} align="right">
          <button type="button" className="lt-mi" onClick={onDelete}>
            {t("table.deleteFilter")}
          </button>
        </Menu>
      </span>
    </fieldset>
  );
}

/**
 * The sort dial: every attribute the server can order this list by, in one menu.
 *
 * A column header is the other route to the same state, and it is not enough on
 * its own. A header can only offer the columns currently SHOWN, at a width a
 * phone does not have; and a sort restored by a saved view arrives with no
 * header pressed at all, so there is nowhere for the reader to see what else
 * they could have asked for. This is that one place.
 *
 * Pressing the active attribute flips its direction and pressing another takes
 * its own opening direction, exactly as clicking a header does, because both
 * ends read `nextSortValue`. "Default order" is offered because it is a state a
 * reader can reach — a saved view that names no sort asks for the server's own
 * order — and a state they can reach is one they must be able to ask for.
 */
function SortMenu({
  sort,
  options,
  open,
  onToggle,
}: Readonly<{
  sort: SortControl;
  options: readonly SortOption[];
  open: boolean;
  onToggle: () => void;
}>) {
  const t = useT();
  // WHAT THE LIST IS ORDERED BY, on the control that changes it. A dial labelled
  // only "Sort" leaves the reader to open it to find out — and a sort can arrive
  // from three places they did not press: a saved view, a column header, the
  // address they pasted. Naming it here is the one place all three are visible.
  const active = options.find(
    (option) => sortDirection(option.field, sort.value) !== null,
  );
  const activeDirection = active
    ? sortDirection(active.field, sort.value)
    : null;
  const group = useId();
  return (
    <span className="lt-menu-wrap">
      <Button aria-expanded={open} onClick={onToggle}>
        <SortGlyph direction={activeDirection} />
        {active
          ? t("table.sortNamed", { column: active.label })
          : t("table.sort")}
        {/* The glyph is the sighted reader's half. Which WAY the list runs has
            to be said as well, or the dial names a column and not an order. */}
        {activeDirection && (
          <span className="sr-only">
            {activeDirection === "asc"
              ? t("table.sortAscending")
              : t("table.sortDescending")}
          </span>
        )}
        <ChevronDown className="lt-caret" aria-hidden="true" />
      </Button>
      <Menu open={open} head={t("table.sortMenu")} align="right">
        {options.map((option) => {
          const direction = sortDirection(option.field, sort.value);
          return (
            <SortItem
              key={option.field}
              group={group}
              label={option.label}
              direction={direction}
              onPick={() => sort.onChange(nextSortValue(option, direction))}
            />
          );
        })}
        {/* Offered because it is a state a reader can REACH — a saved view that
            names no sort asks for the server's own order — and a state they can
            reach is one they must be able to ask for. It has no direction to
            flip, so it carries no arrow. */}
        <SortItem
          group={group}
          label={t("table.sortDefault")}
          direction={sort.value ? null : "none"}
          onPick={() => sort.onChange("")}
        />
      </Menu>
    </span>
  );
}

/**
 * Which way the list runs, on the dial that changes it — or, unsorted, the pair
 * of arrows that says an order is what this control offers.
 */
function SortGlyph({
  direction,
}: Readonly<{ direction: "asc" | "desc" | null }>) {
  if (direction === "asc") {
    return <ArrowUp strokeWidth={1.5} aria-hidden="true" />;
  }
  if (direction === "desc") {
    return <ArrowDown strokeWidth={1.5} aria-hidden="true" />;
  }
  return <ArrowDownUp strokeWidth={1.5} aria-hidden="true" />;
}

/**
 * One attribute in the sort menu: its name, which way it is ordering, and
 * whether it is the one in force.
 *
 * ONE ORDER AT A TIME, so it is a RADIO — the server takes one field plus the
 * house tie-breaker, and a column of boxes would promise a combination this list
 * cannot be in. The direction arrow sits against the label, because it qualifies
 * THAT name rather than being a second column of its own.
 */
function SortItem({
  group,
  label,
  direction,
  onPick,
}: Readonly<{
  /** The radio name every entry of one sort menu shares. */
  group: string;
  label: string;
  /** `"none"` is the server's own order: in force, with no direction to state. */
  direction: "asc" | "desc" | "none" | null;
  onPick: () => void;
}>) {
  const t = useT();
  return (
    <Radio
      className="lt-mi"
      name={group}
      checked={direction !== null}
      onChange={onPick}
      // Pressing the order ALREADY IN FORCE flips its direction, and a radio
      // that is already checked reports no change — so the press is read here
      // too. Guarded on the direction this entry rendered with, and the guard is
      // what keeps a press from being answered twice: the click runs before the
      // change, so on any entry that was not standing this does nothing and the
      // handler above takes it.
      onClick={() => {
        if (direction !== null) {
          onPick();
        }
      }}
      label={
        <>
          {label}
          {(direction === "asc" || direction === "desc") && (
            <span className="lt-mi-dir">
              {direction === "asc" ? (
                <ArrowUp size={12} strokeWidth={1.8} aria-hidden="true" />
              ) : (
                <ArrowDown size={12} strokeWidth={1.8} aria-hidden="true" />
              )}
              {/* The arrow is the sighted reader's half of this. The direction
                  has to be said as well, or the standing entry announces only
                  that it is the sort and not which way. */}
              <span className="sr-only">
                {direction === "asc"
                  ? t("table.sortAscending")
                  : t("table.sortDescending")}
              </span>
            </span>
          )}
        </>
      }
    />
  );
}

export function Menu({
  open,
  head,
  align = "left",
  back,
  children,
}: Readonly<{
  open: boolean;
  head: string;
  align?: "left" | "right";
  /**
   * The step this menu can return to, for a menu that has more than one. The
   * label and the handler arrive together because a head drawn as a button
   * still owes a reader the word "back": its face is the step's own name, which
   * says where you are rather than what pressing it does.
   */
  back?: { label: string; onBack: () => void };
  children: ReactNode;
}>) {
  return (
    // A fieldset because that is what this is: a set of controls that belong
    // together. Named with it, so both a screen reader and a test can say
    // which menu — and which step of it — they are in.
    <fieldset
      className={`lt-menu${open ? " open" : ""}${align === "right" ? " right" : ""}`}
      // A closed menu leaves the tab order entirely; otherwise Tab walks every
      // option of every filter before reaching the body below it.
      inert={!open}
      aria-label={head}
    >
      {back ? (
        <button
          type="button"
          className="lt-mhead lt-mback"
          aria-label={back.label}
          onClick={back.onBack}
        >
          <ChevronLeft size={12} strokeWidth={1.8} aria-hidden="true" />
          {head}
        </button>
      ) : (
        <div className="lt-mhead">{head}</div>
      )}
      {children}
    </fieldset>
  );
}

/**
 * Click anywhere that is not a menu or its trigger, and the menus close.
 *
 * The handler is read through a ref so the listener is attached once for the
 * life of the surface rather than being torn down and re-added on every
 * render — a document-level listener that churns is a real cost on a page
 * this size.
 */
/**
 * Escape closes the open popup and hands focus back to whatever opened it.
 *
 * Without it the only way out of a filter or column menu is a pointer click
 * elsewhere — and a reader who tabbed into the menu would be returned to the
 * top of the document when it closed, rather than to the control they were
 * standing on.
 *
 * Keyed on WHICH popup is open rather than on whether one is: moving straight
 * from one trigger to the next never passes through a closed state, so a
 * boolean would hold the first trigger and send focus back to the wrong
 * control.
 */
export function useCloseOnEscape(openKey: string | null, close: () => void) {
  const latest = useRef(close);
  latest.current = close;
  // Captured while the menu is open, so the trigger is still the element that
  // had focus before the reader stepped into the popup.
  const opener = useRef<Element | null>(null);
  useEffect(() => {
    if (openKey === null) {
      return;
    }
    opener.current = document.activeElement;
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== "Escape") {
        return;
      }
      latest.current();
      const back = opener.current;
      if (back instanceof HTMLElement && back.isConnected) {
        back.focus();
      }
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [openKey]);
}

export function useCloseOnOutsideClick(close: () => void) {
  const latest = useRef(close);
  latest.current = close;
  useEffect(() => {
    const onClick = (event: MouseEvent) => {
      // An event target is not necessarily an element — a click can land on a
      // text node — and only an element can be asked what it sits inside.
      const target = event.target;
      if (!(target instanceof Element)) {
        return;
      }
      // A click that re-rendered the menu — picking an attribute swaps the
      // attribute list for that attribute's values — leaves its own target
      // detached by the time this listener runs, and a detached node has no
      // ancestors to find. That is a click INSIDE the menu, not outside it, so
      // treating it as outside closed the menu on the very step that should
      // have advanced it.
      if (!target.isConnected) {
        return;
      }
      if (!target.closest(".lt-menu-wrap, .lt-menu")) {
        latest.current();
      }
    };
    document.addEventListener("click", onClick);
    return () => document.removeEventListener("click", onClick);
  }, []);
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
  narrowed = false,
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
  /**
   * A dial is cutting the set down, so an empty list is empty BECAUSE of one.
   *
   * It only changes the zero, and it has to. "No companies yet" over a search
   * that matched nothing is a claim about the workspace rather than about the
   * search — and the table's own empty row says "no companies match these
   * filters" directly underneath it, so the reader got both sentences at once
   * and one of them was false. The narrowed zero belongs to the body, which is
   * where the reader is looking and where the way back to everything is, so
   * this line says nothing about the count and goes on saying what the order
   * is.
   */
  narrowed?: boolean;
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
  const counted =
    total === 0
      ? narrowed
        ? // The narrowed zero belongs to the body, which is where the reader is
          // looking and where the way back to everything is.
          ""
        : t("table.none", { unit })
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

/**
 * The controls row, read as two halves. What narrows the list — search, chips,
 * the body's own filter pickers — is on the left; what changes how the list is
 * displayed is on the right. A reader looking to cut the set down never has to
 * scan the whole row to find out which controls do that.
 */
function Toolbar({
  search,
  sort,
  sortOptions,
  chips,
  chosen,
  onChipChange,
  archived,
  tools,
  openMenu,
  setOpenMenu,
}: Readonly<{
  search?: { value: string; onChange: (next: string) => void };
  sort?: SortControl;
  sortOptions: readonly SortOption[];
  chips: readonly ListChip[];
  chosen: Readonly<Record<string, string>>;
  onChipChange?: (key: string, value: string) => void;
  archived?: { checked: boolean; onChange: (next: boolean) => void };
  tools?: ReactNode;
  openMenu: string | null;
  setOpenMenu: (next: string | null) => void;
}>) {
  const t = useT();
  const applied = chips.filter((chip) => chosen[chip.key]);
  // The label a searched value was picked under, kept only while that value is
  // still the one applied — the filter can also be changed by a saved view, by
  // Clear filters or by the reader deleting the row, and a label held past that
  // would name the wrong record.
  const [picked, setPicked] = useState<Record<string, [string, string]>>({});
  const labelFor = (key: string, value: string) => {
    const remembered = picked[key];
    return remembered && remembered[0] === value ? remembered[1] : undefined;
  };
  const remember = (key: string, value: string, label: string) =>
    setPicked((prev) => ({ ...prev, [key]: [value, label] }));
  return (
    <div className="lt-tools">
      {search && (
        <label className="lt-search">
          <Search size={13} strokeWidth={1.6} aria-hidden="true" />
          <span className="sr-only">{t("list.search")}</span>
          <input
            // A search field, not a plain text box: it is what the control is,
            // and it is how a reader's assistive technology announces it.
            type="search"
            className="lt-search-input"
            value={search.value}
            placeholder={t("list.search")}
            onChange={(event) => search.onChange(event.target.value)}
          />
        </label>
      )}

      {applied.map((chip) => (
        <FilterRow
          key={chip.key}
          chip={chip}
          value={chosen[chip.key] ?? ""}
          pickedLabel={labelFor(chip.key, chosen[chip.key] ?? "")}
          openSegment={openRowSegment(openMenu, chip.key)}
          onToggleSegment={(next) => {
            const key = `row:${chip.key}:${next}`;
            setOpenMenu(openMenu === key ? null : key);
          }}
          onPick={(value, label) => {
            remember(chip.key, value, label);
            onChipChange?.(chip.key, value);
            setOpenMenu(null);
          }}
          onDelete={() => {
            onChipChange?.(chip.key, "");
            setOpenMenu(null);
          }}
        />
      ))}

      {chips.length > 0 && (
        <FilterMenu
          chips={chips}
          chosen={chosen}
          onChipChange={onChipChange}
          onRemember={remember}
          open={openMenu === "filter"}
          onToggle={() => setOpenMenu(openMenu === "filter" ? null : "filter")}
          hasApplied={applied.length > 0}
        />
      )}

      {archived && (
        <Checkbox
          checked={archived.checked}
          label={t("list.showArchived")}
          onChange={(event) => archived.onChange(event.target.checked)}
        />
      )}

      <span className="lt-spacer" />

      {sort && sortOptions.length > 0 && (
        <SortMenu
          sort={sort}
          options={sortOptions}
          open={openMenu === "sort"}
          onToggle={() => setOpenMenu(openMenu === "sort" ? null : "sort")}
        />
      )}

      {tools}
    </div>
  );
}
