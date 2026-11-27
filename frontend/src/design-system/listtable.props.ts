// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode, RefObject } from "react";
import type { ListColumn, ListSelection } from "./listcolumn";
import type {
  ListChip,
  ListRefusal,
  ListView,
  SortControl,
  ViewRow,
} from "./listsurface";
import type { JumpedRows } from "./listtable.paging";

/** What a screen hands `ListTable`: its rows, its columns and its dials. */
export type ListTableProps<Row> = Readonly<{
  rows: readonly Row[];
  columns: readonly ListColumn<Row>[];
  rowKey: (row: Row) => string;
  /**
   * Row selection for a bulk action. The checkbox sits in the identity cell,
   * which stays put while the rest scrolls, so no second column is needed.
   * `bar` renders above the grid while anything is selected; the screen owns
   * its verbs and their per-row writes, versions and failures.
   */
  selection?: ListSelection<Row>;
  /**
   * Renders in place of the grid, such as a board over the fetched rows. The
   * header, search, chips, views and page-size dial stay where they are, so a
   * reader can see what narrowed the answer.
   */
  body?: ReactNode;
  /** The alternate body carries its own count and continuation control. */
  bodyOwnsPaging?: boolean;
  /** A settled body with nothing in it is headed by the grid's empty line. */
  bodyEmpty?: boolean;
  /**
   * The count sentence for a body that owns its own paging, in the same slot
   * beside the page's name. Ignored unless `bodyOwnsPaging`.
   */
  bodyCount?: ReactNode;
  /** What the table's count names its rows as part of; see `CountLine`'s `scope`. */
  countScope?: string;
  onRowClick?: (row: Row) => void;
  /**
   * Where this row lives, as a URL. It turns the identity cell into a link, so
   * a row opens in a new tab or from the keyboard, which a click handler cannot.
   */
  rowHref?: (row: Row) => string;
  /**
   * The page's own name, when this table is the page; see `ListSurface`. A
   * screen that passes it heads itself and belongs in `SELF_HEADED_SCREENS`.
   */
  title?: ReactNode;
  /** Plural noun for the count and the empty state ("contacts", "leads"). */
  unit: string;
  /** Singular noun for a count of one ("contact", "lead"). Defaults to `unit`. */
  unitOne?: string;
  /**
   * A likelier cause than "there is nothing here", under the empty line
   * whether it is narrowed or not. A caller whose note would blame the data
   * source for what the reader's own dial did passes none.
   */
  emptyNote?: ReactNode;
  /** Replaces the empty line and its Clear filters; see `EmptyRow`. */
  emptyLine?: ReactNode;
  /**
   * Omit for a list whose GET has no `q` param; the box is then not rendered.
   * `answered` is the search the rows answer, which `value` runs ahead of while
   * the typing settles. A `body` reads it as `AnsweredSearch`.
   */
  search?: {
    value: string;
    answered: string;
    onChange: (next: string) => void;
  };
  /**
   * Omit when the data source refuses to sort: the headers then render inert,
   * so the table never offers a control the server would reject.
   */
  sort?: SortControl;
  chips?: readonly ListChip[];
  chosen?: Readonly<Record<string, string>>;
  /**
   * What the list is narrowed by, serialized: the page reset's trigger. It is
   * apart from `chosen`, which the dials show and which must stay current. The
   * reset fires only when the answer changes, so a late option keeps the page.
   * Defaults to `chosen`, for dials all declared up front.
   */
  narrowKey?: string;
  /** Called with "" to clear. */
  onChipChange?: (key: string, value: string) => void;
  archived?: { checked: boolean; onChange: (next: boolean) => void };
  views?: readonly ListView[];
  activeView?: number;
  /**
   * What this list reads when the screen narrows it by something other than a
   * chip or filter, such as the deals pipeline picker. A change lands the
   * reader on page 1, since page 2 of one pipeline is not page 2 of another.
   */
  scopeKey?: string;
  onViewChange?: (index: number) => void;
  /** The one primary action for this surface, e.g. "New contact". */
  action?: ReactNode;
  /** The head's refusal line: ListSurface's `refusal`. */
  refusal?: ListRefusal;
  /**
   * A standing note about what this list is, when the list needs one. The
   * screen's name is not it: the shell already says which screen you are on,
   * and repeating it here would title the surface twice.
   */
  caption?: ReactNode;
  /** An aggregate row under the table, e.g. a count and a total value. */
  footer?: ReactNode;
  /**
   * Whether the server holds rows beyond the ones passed in. Paging is a keyset
   * cursor: the pager walks the pages it has, and past the last one it fetches.
   */
  hasMore?: boolean;
  onLoadMore?: () => void;
  /**
   * No count, dials or pager while every row fits the smallest page and nothing
   * narrows the list. A dial the reader set can then always be undone.
   */
  hideShortChrome?: boolean;
  /**
   * How many rows match on the server, when it counts them: "1-25 of 8,372"
   * against "1-25 of 200 loaded so far". Undefined means not counted, not zero.
   */
  total?: number;
  /**
   * Rows read at an offset for a page past `rows`, for a list whose endpoint
   * reads one. Present while the page is past `rows`; see `JumpedRows`.
   */
  jumped?: JumpedRows<Row>;
  /**
   * Rows per rendered page. The caller fetches a whole multiple of it, so pages
   * split where the fetch does. Read only with `onPerPage`.
   */
  perPage?: number;
  /**
   * The reader picked a different page size; re-ask the server with it. Omit
   * it and the table keeps the size itself, slicing the rows in hand.
   */
  onPerPage?: (next: number) => void;
  /**
   * Which rendered page is on screen, for a caller that keeps it in the address
   * so a reader returns to the page they left. Omit it and the table holds it.
   */
  page?: number;
  /** The rendered page changed, whether by the pager or by a reset. */
  onPage?: (next: number) => void;
  /** True when `page` arrived with the narrowing, as a link names both. */
  keepsPage?: () => boolean;
  /**
   * The element the rows scroll in, for a caller that remembers where the
   * reader was. A full-height list takes the overflow in its rows, so the page
   * column stays at zero. A second reference, not a second scroller.
   */
  bodyRef?: RefObject<HTMLDivElement | null>;
  /**
   * The rows are still loading. The header, dials and primary action stay, as
   * they belong to the screen, and the body shows placeholders.
   */
  pending?: boolean;
  /** Why the rows could not be read, with whatever retry the caller offers. */
  problem?: ReactNode;
  /** Names this table for the column widths it remembers between visits. */
  widthsKey?: string;
  /** A verb that is not a dial, at the toolbar's end; see `ListSurface`. */
  tools?: ReactNode;
  /** Table or Board, at the top of the View panel; see `ListSurface`. */
  layout?: ReactNode;
  /** The screen's own dials in the View panel; see `ListSurface`. */
  viewRows?: readonly ViewRow[];
  /** The list departs from every view it offers; see `ListSurface`. */
  changed?: boolean;
  /** Save view and Manage views, in the View panel's footer. */
  saveView?: ReactNode;
  /** A line under the search; see `ListSurface`. */
  notice?: ReactNode;
}>;
