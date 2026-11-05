import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { Checkbox } from "./atoms";
import type { ListColumn } from "./listcolumn";
import type { ViewRow, ViewToggle } from "./listviewmenu";
import { useSortableRows } from "./sortablelist";

type ColumnDials<Row> = Readonly<{
  /** The optional columns in the order the grid draws them. */
  optional: readonly ListColumn<Row>[];
  /** The optional columns' keys in the order the screen declares them. */
  declared: readonly string[];
  hidden: ReadonlySet<string>;
  onToggleColumn: (key: string) => void;
  /** The reader moved a column; receives every optional key in its new order. */
  onReorder?: (keys: string[]) => void;
}>;

/** The grid's presentation dials in View. Which optional columns stand where
 * is a row with a step; whether the rows are compact is a toggle. */
export function useDisplayRows<Row>({
  dense,
  onDense,
  ...columns
}: ColumnDials<Row> & Readonly<{ dense: boolean; onDense: () => void }>): {
  rows: ViewRow[];
  toggles: ViewToggle[];
} {
  const t = useT();
  const { locale } = useLocale();
  const { optional, hidden } = columns;
  const toggles: ViewToggle[] = [
    {
      key: "compact",
      label: t("table.compact"),
      checked: dense,
      onChange: onDense,
    },
  ];
  if (optional.length === 0) {
    return { rows: [], toggles };
  }
  const shown = optional.filter((column) => !hidden.has(column.key)).length;
  const moved = optional.some(
    (column, index) => column.key !== columns.declared[index],
  );
  const toggled = optional.some(
    (column) => hidden.has(column.key) !== (column.initiallyHidden ?? false),
  );
  const columnsRow: ViewRow = {
    key: "columns",
    label: t("table.columns"),
    value: t("table.columnsShown", {
      shown: formatNumber(shown, locale),
      total: formatNumber(optional.length, locale),
    }),
    changed: moved || toggled,
    step: () => <ColumnsStep {...columns} />,
  };
  return { rows: [columnsRow], toggles };
}

function ColumnsStep<Row>({
  optional,
  hidden,
  onToggleColumn,
  onReorder,
}: ColumnDials<Row>) {
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
      onReorder,
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
    <>
      <div className={sortable.sorting ? "lt-mcols is-sorting" : "lt-mcols"}>
        {sortable.shown.map((column) => (
          <div
            key={column.key}
            ref={sortable.rowRef(column.key)}
            className={
              sortable.lifted === column.key ? "lt-mcol is-lifted" : "lt-mcol"
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
  );
}

const columnKey = <Row,>(column: ListColumn<Row>) => column.key;
