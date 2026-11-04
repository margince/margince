import { ChevronDown, SlidersHorizontal } from "lucide-react";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { Button, Checkbox } from "./atoms";
import type { ListColumn } from "./listcolumn";
import { Menu } from "./listsurface";
import { useSortableRows } from "./sortablelist";

/**
 * Everything that decides how the grid is drawn rather than what is in it.
 * That is how tight the rows are, and which optional columns stand where.
 * It is one menu because these are one question to a reader. Density, a
 * hidden column and a moved one all answer "show me more of this at once". A
 * toolbar that spends a trigger on each spends its right half on dials with
 * nothing grouping them.
 *
 * It goes in the surface's `displayMenu` slot, which hands it the row's own
 * open state. The surface knows nothing of a column or a density. It knows
 * only that one menu on a row is open at a time.
 */
export function DisplayMenu<Row>({
  optional,
  hidden,
  onToggleColumn,
  onReorder,
  dense,
  onDense,
  open,
  onToggle,
}: Readonly<{
  /** The optional columns in the order the grid draws them. */
  optional: readonly ListColumn<Row>[];
  hidden: ReadonlySet<string>;
  onToggleColumn: (key: string) => void;
  /** The reader moved a column; receives every optional key in its new order. */
  onReorder?: (keys: string[]) => void;
  dense: boolean;
  onDense: () => void;
  open: boolean;
  onToggle: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const place = (position: number, count: number) => ({
    position: formatNumber(position, locale),
    total: formatNumber(count, locale),
  });
  // A hidden column keeps its row, so it keeps its place when it is shown again.
  const sortable = useSortableRows(
    optional,
    columnKey,
    onReorder && {
      onReorder: (keys) => onReorder(keys),
      labels: {
        handle: (column, position, count) =>
          t("table.columnHandle", {
            name: column.header,
            ...place(position, count),
          }),
        moved: (column, position, count) =>
          t("table.columnMoved", {
            name: column.header,
            ...place(position, count),
          }),
      },
    },
  );
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
            <div
              className={sortable.sorting ? "lt-mcols is-sorting" : "lt-mcols"}
            >
              {sortable.shown.map((column) => (
                <div
                  key={column.key}
                  ref={sortable.rowRef(column.key)}
                  className={
                    sortable.lifted === column.key
                      ? "lt-mcol is-lifted"
                      : "lt-mcol"
                  }
                >
                  {sortable.grip(column)}
                  <Checkbox
                    className="lt-mi"
                    checked={!hidden.has(column.key)}
                    label={column.header}
                    onChange={() => onToggleColumn(column.key)}
                  />
                </div>
              ))}
            </div>
            {sortable.live}
          </>
        )}
      </Menu>
    </span>
  );
}

const columnKey = <Row,>(column: ListColumn<Row>) => column.key;
