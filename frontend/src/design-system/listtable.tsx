import { useCallback, useMemo, useRef, useState } from "react";
import { AnsweredSearch } from "./answeredsearch";
import { EmptyState, useScrollRegion } from "./atoms";
import { ListSurface, rangeLine } from "./listsurface";
import { useColumnWidths, useFrozenEdge } from "./listtable.layout";
import {
  orderedColumns,
  useColumnDisplay,
  useColumnOrder,
} from "./listtable.order";
import { fitsOnePage, usePaging } from "./listtable.paging";
import {
  applyView,
  clearAll,
  filteredBy,
  holdsCount,
  narrowedBy,
  type QueryDials,
  sortedColumn,
  sortOptionsOf,
} from "./listtable.query";
import { Pager } from "./pager";
import "./listtable.css";
import { ListBulkBar, useSelectAll } from "./listselection";

export type {
  ListChip,
  ListRefusal,
  ListView,
  SortControl,
  SortOption,
  ViewRow,
} from "./listsurface";
export { useListRefusal } from "./listsurface";
export { widthWorthAdopting } from "./listtable.layout";
export { narrowingSignature } from "./listtable.paging";

// The list surface: one component owning the header, the controls, the rows and
// the footer of a record list, so the dials read as belonging to the data they
// act on rather than floating above it.
//
// The query dials are CONTROLLED and server-backed — search, sort and filters
// are reported upward and the caller re-reads the list. Only presentation is
// local state: which columns are shown, how tight the rows are, and which saved
// view is selected. That split is the whole design. A table that quietly sorted
// its own page would be lying about the other pages, and this list is a keyset
// cursor over a set larger than what is loaded.
//
// Generic on purpose: nothing here knows what a CRM record is, which is why
// contacts, companies, leads, deals, products and partners share it.
import { useDisplayRows } from "./listdisplaymenu";
import { EmptyRow, GridBody, ListGrid } from "./listgrid";
import { useDrawnColumns } from "./listgrid.phone";
import type { ListTableProps } from "./listtable.props";

export type { ListColumn, ListSelection } from "./listcolumn";
export { CellStrip } from "./listgrid";

const EMPTY_FILTERS: Readonly<Record<string, string>> = {};

export function ListTable<Row>({
  title,
  bodyCount,
  countScope,
  rows,
  columns: declared,
  rowKey,
  onRowClick,
  rowHref,
  unit,
  unitOne,
  emptyNote,
  emptyLine,
  search,
  sort,
  chips = [],
  chosen = EMPTY_FILTERS,
  narrowKey,
  onChipChange,
  archived,
  views = [],
  activeView = 0,
  onViewChange,
  scopeKey = "",
  action,
  refusal,
  caption,
  footer,
  hasMore = false,
  onLoadMore,
  hideShortChrome = false,
  total: serverTotal,
  perPage: controlledPerPage,
  onPerPage,
  page: controlledPage,
  onPage,
  keepsPage,
  bodyRef,
  pending = false,
  problem,
  widthsKey,
  tools,
  layout: viewLayout,
  viewRows,
  changed,
  saveView,
  notice,
  body,
  bodyOwnsPaging = false,
  bodyEmpty = false,
  selection,
}: ListTableProps<Row>) {
  // Each derived list keeps its identity across a render that changed only the
  // widths, so the rows below skip that render (`gridBody`).
  const columns = useMemo(
    () => declared.filter((column) => !column.phoneOnly),
    [declared],
  );
  const display = useColumnDisplay(columns, widthsKey);
  const order = useColumnOrder(widthsKey);
  const placed = useMemo(
    () => orderedColumns(columns, order.order),
    [columns, order.order],
  );
  const scroller = useRef<HTMLDivElement>(null);
  // The node as state too: the grid mounts after the empty state or a board,
  // and the frozen edge has to measure it then.
  const [scrollerNode, setScrollerNode] = useState<HTMLDivElement | null>(null);
  const scrollerRef = useCallback(
    (node: HTMLDivElement | null) => {
      scroller.current = node;
      if (bodyRef) bodyRef.current = node;
      setScrollerNode(node);
    },
    [bodyRef],
  );
  const head = useRef<HTMLTableElement>(null);
  const { phone, shown } = useDrawnColumns(declared, placed, display.hidden);
  // The width observer re-attaches when the scroller comes or goes.
  const drawsOwnTable = body === undefined || body === null;
  const layout = useColumnWidths(
    shown,
    widthsKey,
    { scroller, head },
    drawsOwnTable,
  );
  // The body is where this surface hides columns, so it is the body that has
  // to be reachable. Named by the noun the count line uses.
  const region = useScrollRegion(scroller, unit);
  const dials: QueryDials = useMemo(
    () => ({
      search,
      sort,
      chips,
      chosen,
      onChipChange,
      views,
      onViewChange,
      scopeKey,
    }),
    [search, sort, chips, chosen, onChipChange, views, onViewChange, scopeKey],
  );
  const paging = usePaging(
    {
      rows,
      page: controlledPage,
      onPage,
      perPage: controlledPerPage,
      onPerPage,
      hasMore,
      onLoadMore,
      total: serverTotal,
      reachAsked: !bodyOwnsPaging,
      keepsPage,
    },
    {
      search: search?.value,
      narrowKey,
      chosen,
      sort: sort?.value,
      archived: archived?.checked,
      scopeKey,
    },
    scroller,
  );
  const all = useSelectAll({
    rows,
    pageRows: paging.pageRows,
    rowKey,
    selection,
    hasMore,
    onLoadMore,
    failed: problem != null,
    total: serverTotal,
  });
  const frozen = useFrozenEdge(scrollerNode, shown.length, display.dense);
  const displayDials = useDisplayRows({
    optional: phone ? [] : placed.filter((column) => !column.fixed),
    declared: columns.flatMap((column) => (column.fixed ? [] : [column.key])),
    onReorder: order.onReorder,
    ...display,
  });
  const short =
    hideShortChrome &&
    !pending &&
    !narrowedBy(dials) &&
    archived?.checked !== true &&
    fitsOnePage(rows, serverTotal, hasMore);
  const emptyRow = useMemo(
    () => (
      <EmptyRow
        unit={unit}
        narrowed={narrowedBy(dials)}
        filtered={filteredBy(dials)}
        onClear={() => clearAll(dials)}
        line={emptyLine}
        emptyNote={emptyNote}
      />
    ),
    [unit, dials, emptyLine, emptyNote],
  );
  // Memoized so a width change re-renders the colgroup and header, not the rows.
  const gridBody = useMemo(
    () => (
      <GridBody
        rows={rows}
        pageRows={paging.pageRows}
        shown={shown}
        rowKey={rowKey}
        onRowClick={onRowClick}
        rowHref={rowHref}
        selection={selection}
        phone={phone}
        pending={pending}
        problem={problem}
        empty={emptyRow}
      />
    ),
    [
      rows,
      paging.pageRows,
      shown,
      rowKey,
      onRowClick,
      rowHref,
      selection,
      phone,
      pending,
      problem,
      emptyRow,
    ],
  );
  const emptyBlock = (
    <div className="lt-emptyline">
      <EmptyState>{emptyRow}</EmptyState>
    </div>
  );

  return (
    <ListSurface
      title={title}
      views={views}
      activeView={activeView}
      onViewChange={(index) => applyView(dials, index)}
      bare={short}
      count={
        bodyOwnsPaging
          ? bodyCount
          : rangeLine(pending, {
              unit,
              unitOne,
              first: paging.from + 1,
              last: paging.from + paging.pageRows.length,
              total: serverTotal ?? rows.length,
              // "Loaded so far" is the caveat for a number the client
              // counted itself; an exact total needs none.
              more: hasMore && serverTotal === undefined,
              scope: countScope,
              sortedBy: sortedColumn(columns, sort)?.header,
            })
      }
      holdCount={holdsCount(bodyOwnsPaging, pending, dials, rows.length)}
      action={action}
      refusal={refusal}
      caption={caption}
      search={search}
      sort={sort}
      sortOptions={sortOptionsOf(columns)}
      chips={chips}
      chosen={chosen}
      onChipChange={onChipChange}
      archived={archived}
      layout={viewLayout}
      viewRows={viewRows}
      // Both dials describe the grid, and a body that draws its own
      // presentation would show controls that do nothing.
      displayRows={bodyOwnsPaging ? undefined : displayDials.rows}
      displayToggles={bodyOwnsPaging ? undefined : displayDials.toggles}
      changed={changed}
      tools={tools}
      saveView={saveView}
      notice={notice}
      footer={
        <>
          {footer && <div className="lt-agg">{footer}</div>}
          {!bodyOwnsPaging && !short && (
            <Pager
              current={paging.current}
              lastPage={paging.lastPage}
              endKnown={paging.endKnown}
              asked={paging.asked}
              hasMore={hasMore}
              perPage={paging.perPage}
              onGoto={paging.goto}
              onPerPage={paging.setPerPage}
            />
          )}
        </>
      }
    >
      <ListBulkBar selection={selection} all={all} hidden={!drawsOwnTable} />
      {short && !problem && rows.length === 0 ? (
        emptyBlock
      ) : body != null ? (
        <AnsweredSearch.Provider value={search?.answered}>
          {bodyEmpty && !problem && emptyBlock}
          {body}
        </AnsweredSearch.Provider>
      ) : (
        <ListGrid
          scrollerRef={scrollerRef}
          region={region}
          frozen={frozen}
          head={head}
          dense={display.dense}
          phone={phone}
          layout={layout}
          shown={shown}
          sort={sort}
          all={all}
        >
          {gridBody}
        </ListGrid>
      )}
    </ListSurface>
  );
}
