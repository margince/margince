import { ArrowDown, ArrowUp } from "lucide-react";
import { useId } from "react";
import { type Translator, useT } from "../i18n";
import { Radio } from "./atoms";
import {
  nextSortValue,
  type SortControl,
  type SortOption,
  sortDirection,
} from "./listsurface.dials";

/** Every order the server can give, hidden columns included, and its own
 * default, which a saved view naming no sort asks for. */
export function SortStep({
  sort,
  options,
}: Readonly<{ sort: SortControl; options: readonly SortOption[] }>) {
  const t = useT();
  const group = useId();
  return (
    <>
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
      <SortItem
        group={group}
        label={t("table.sortDefault")}
        direction={sort.value ? null : "none"}
        onPick={() => sort.onChange("")}
      />
    </>
  );
}

/** The order in force, in words. A sort also arrives from a saved view, a
 * header or an address, so the row names it without being opened. */
export function sortSummary(
  sort: SortControl,
  options: readonly SortOption[],
  t: Translator,
): string {
  for (const option of options) {
    const direction = sortDirection(option.field, sort.value);
    if (direction !== null) {
      return t("table.sortValue", {
        column: option.label,
        direction:
          direction === "asc"
            ? t("table.sortAscending")
            : t("table.sortDescending"),
      });
    }
  }
  return t("table.sortDefault");
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
