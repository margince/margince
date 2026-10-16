import { ChevronDown, SlidersHorizontal } from "lucide-react";
import { useT } from "../i18n";
import { Button, Checkbox } from "./atoms";
import type { ListColumn } from "./listcolumn";
import { Menu } from "./listsurface";

/**
 * Everything that decides how the grid is drawn rather than what is in it:
 * how tight the rows are, and which optional columns stand. It is one menu
 * because the two are one question to a reader. Density and a hidden column
 * both answer "show me more of this at once". A toolbar that spends a trigger
 * on each spends its right half on dials with nothing grouping them.
 *
 * It goes in the surface's `displayMenu` slot, which hands it the row's own
 * open state. The surface knows nothing of a column or a density. It knows
 * only that one menu on a row is open at a time.
 */
export function DisplayMenu<Row>({
  optional,
  hidden,
  onToggleColumn,
  dense,
  onDense,
  open,
  onToggle,
}: Readonly<{
  optional: readonly ListColumn<Row>[];
  hidden: ReadonlySet<string>;
  onToggleColumn: (key: string) => void;
  dense: boolean;
  onDense: () => void;
  open: boolean;
  onToggle: () => void;
}>) {
  const t = useT();
  return (
    <span className="lt-menu-wrap">
      <Button aria-expanded={open} onClick={onToggle}>
        <SlidersHorizontal strokeWidth={1.5} aria-hidden="true" />
        {t("table.display")}
        <ChevronDown className="lt-caret" aria-hidden="true" />
      </Button>
      <Menu open={open} head={t("table.display")} align="right">
        {/* A group is named a rung under the menu's own head: the head says
            which menu this is, a group says which part of it. */}
        <div className="lt-mgroup t-caption">{t("table.density")}</div>
        {/* Density and the column set are both sets the reader builds in one
            visit, so every row is a `Checkbox` and the menu stays open. */}
        <Checkbox
          className="lt-mi"
          checked={dense}
          label={t("table.compact")}
          onChange={onDense}
        />
        {optional.length > 0 && (
          <>
            <div className="lt-mgroup t-caption">{t("table.shownColumns")}</div>
            {optional.map((column) => (
              <Checkbox
                key={column.key}
                className="lt-mi"
                checked={!hidden.has(column.key)}
                label={column.header}
                onChange={() => onToggleColumn(column.key)}
              />
            ))}
          </>
        )}
      </Menu>
    </span>
  );
}
