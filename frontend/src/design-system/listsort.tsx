import { ArrowDown, ArrowDownUp, ArrowUp, ChevronDown } from "lucide-react";
import { useId } from "react";
import { useT } from "../i18n";
import { Button, Radio } from "./atoms";
import { Menu } from "./listmenu";
import {
  nextSortValue,
  type SortControl,
  type SortOption,
  sortDirection,
} from "./listsurface.dials";

/**
 * The sort dial: every attribute the server can order this list by, in one menu.
 *
 * A column header is the other route to the same state, and it is not enough on
 * its own. A header can only offer the columns currently shown, at a width a
 * phone does not have. A sort restored by a saved view arrives with no header
 * pressed at all. Then there is nowhere for the reader to see what else they
 * could have asked for. This is that one place.
 *
 * Pressing the active attribute flips its direction. Pressing another takes
 * its own opening direction. Clicking a header does the same, because both
 * ends read `nextSortValue`. "Default order" is offered because a reader can
 * reach that state: a saved view that names no sort asks for the server's own
 * order. A state they can reach is one they must be able to ask for.
 */
export function SortMenu({
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
  // What the list is ordered by, on the control that changes it. A dial labelled
  // only "Sort" leaves the reader to open it to find out. A sort can arrive from
  // three places they did not press: a saved view, a column header, the address
  // they pasted. Naming it here is the one place all three are visible.
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
        {/* The glyph is the sighted reader's half. Which way the list runs has
            to be said as well, or the dial names a column and not an order. */}
        {activeDirection && (
          // The space is the separator: a name concatenates what is inside it.
          <span className="sr-only">
            {" "}
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
        {/* Offered because a reader can reach this state: a saved view that
            names no sort asks for the server's own order. A state they can
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

/** Which way the list runs, or the pair of arrows that offers an order. */
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
 * One order at a time, so it is a radio. The server takes one field plus the
 * house tie-breaker. A column of boxes would promise a combination this list
 * cannot be in. The direction arrow sits against the label, because it
 * qualifies that name rather than being a second column of its own.
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
      // Pressing the order already in force flips its direction. A radio that
      // is already checked reports no change, so the press is read here too.
      // It is guarded on the direction this entry rendered with. The guard
      // keeps a press from being answered twice. The click runs before the
      // change, so on an entry that was not standing this does nothing. The
      // handler above takes that press.
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
                {" "}
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
