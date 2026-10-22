import type { ReactNode } from "react";

export type ListColumn<Row> = {
  key: string;
  header: string;
  cell: (row: Row) => ReactNode;
  /**
   * The server sort field behind this column. Its presence makes the header
   * clickable. A column the API cannot order by stays inert, rather than
   * offering a control that would silently do nothing.
   */
  sort?: string;
  /** Right-aligns, and makes the first sort click descending. */
  numeric?: boolean;
  /**
   * Exempt from the Display menu's column list, and the card heading on a
   * phone. The identity column has to stay: it is what makes a row
   * recognisable.
   */
  fixed?: boolean;
  /**
   * This column holds the row's verbs, sized by its buttons rather than by a
   * share of the width. A verb read in half cannot be used. "menu" is one "…"
   * trigger, whose header only assistive technology reads.
   */
  verbs?: boolean | "menu";
  /**
   * A value whose length hardly changes: a date, or a colleague's name beside
   * their avatar. The column stops growing at a preset width.
   */
  holds?: "date" | "colleague";
  /** Starts unticked in the Display menu: offered rather than drawn. */
  initiallyHidden?: boolean;
};

export type ListSelection<Row> = {
  /** Keys (rowKey) of the selected rows. */
  selected: ReadonlySet<string>;
  onToggle: (row: Row) => void;
  /** The checkbox's accessible name for a row, e.g. "Select Anna Weber". */
  label: (row: Row) => string;
  /** Rows the verbs cannot act on carry no checkbox. Default: every row. */
  selectable?: (row: Row) => boolean;
  /** The bulk bar: the count and the verbs. Rendered while anything is selected. */
  bar: ReactNode;
};
