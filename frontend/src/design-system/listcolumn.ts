import type { ReactNode } from "react";

/** A phone-only column needs a phone place, or it would never be drawn. */
type PhonePlacement =
  | {
      /**
       * Its place in a phone row: "end" closes the name's line (the first declared wins),
       * "caption" is a fact under the name. A list that places no column keeps the labelled card.
       * Not DataTable's `fold`: a fold keeps a column without one on line two, a phone row drops it.
       */
      phone?: "end" | "caption";
      phoneOnly?: undefined;
    }
  | {
      phone: "end" | "caption";
      /** Drawn in phone rows alone: no desktop column, tick, width or place in the order. */
      phoneOnly: true;
    };

export type ListColumn<Row> = PhonePlacement & {
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
   * What the column holds, where that sets its width. Width past a date's or
   * a name's length is wasted, and an email tells rows apart, so it grows first.
   */
  holds?: "date" | "colleague" | "score" | "email";
  /** Starts unticked in the Display menu: offered rather than drawn. */
  initiallyHidden?: boolean;
  /**
   * What the cell says in a phone row, the identity's included: `null` says
   * nothing, so the fact and its separator go; `undefined` keeps the cell.
   */
  folded?: (row: Row) => ReactNode | undefined;
};

export type ListSelection<Row> = {
  /** Keys (rowKey) of the selected rows. */
  selected: ReadonlySet<string>;
  /** Tick or untick these rows. `useRowSelection` holds a selection that keeps the rules. */
  onChange: (rows: readonly Row[], selected: boolean) => void;
  /** The checkbox's accessible name for a row, e.g. "Select Anna Weber". */
  label: (row: Row) => string;
  /** Rows the verbs cannot act on carry no checkbox. Default: every row. */
  selectable?: (row: Row) => boolean;
  /** The bulk bar: the count and the verbs. Rendered while anything is selected. */
  bar: ReactNode;
  /**
   * The most rows one selection holds: what one bulk change takes. "Select
   * all" reads the list's next pages until it holds every row or this many.
   */
  limit?: number;
};
