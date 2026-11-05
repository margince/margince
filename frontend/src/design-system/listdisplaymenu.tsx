import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { Checkbox } from "./atoms";
import type { ListColumn } from "./listcolumn";
import type { ViewRow } from "./listviewmenu";
import { useSortableRows } from "./sortablelist";

type ColumnDials<Row> = Readonly<{
  /** The optional columns in the order the grid draws them. */
  optional: readonly ListColumn<Row>[];
  hidden: ReadonlySet<string>;
  onToggleColumn: (key: string) => void;
  /** The reader moved a column; receives every optional key in its new order. */
  onReorder?: (keys: string[]) => void;
}>;

/** The grid's presentation dials as View rows: which optional columns stand
 * where, and how tight the rows are. */
export function useDisplayRows<Row>({
  dense,
  onDense,
  ...columns
}: ColumnDials<Row> &
  Readonly<{ dense: boolean; onDense: () => void }>): ViewRow[] {
  const t = useT();
  const { locale } = useLocale();
  const { optional, hidden } = columns;
  const density: ViewRow = {
    key: "density",
    label: t("table.density"),
    value: dense ? t("table.compact") : t("table.comfortable"),
    step: () => (
      // A choice the reader may turn back at once, so the step stays open.
      <Checkbox
        className="lt-mi"
        checked={dense}
        label={t("table.compact")}
        onChange={onDense}
      />
    ),
  };
  if (optional.length === 0) {
    return [density];
  }
  const shown = optional.filter((column) => !hidden.has(column.key)).length;
  return [
    {
      key: "columns",
      label: t("table.columns"),
      value: t("table.columnsShown", {
        shown: formatNumber(shown, locale),
        total: formatNumber(optional.length, locale),
      }),
      step: () => <ColumnsStep {...columns} />,
    },
    density,
  ];
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
