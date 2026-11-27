import { listFetchLimit, listPageOf } from "../../components/listquery";
import { useJumpedList } from "../../components/listquery.jump";
import { CLOSE_DAY_DIAL } from "../../lib/dealranges";
import {
  type DealFilters,
  type DealListPage,
  dealsListKey,
  readDealsAt,
  useCloseDay,
  useDeals,
  useStageTotals,
} from "../../lib/deals.queries";
import { useHealthScope } from "../../lib/healthscope";
import type { usePipelines } from "../../lib/pipelines.queries";
import { useUrlParams } from "../../lib/urlstate";
import { useStageColumns } from "./dealboard.queries";
import type { Stage } from "./deals";

// Each surface reads only while it shows, so neither pays for the other.
// Their pipeline scope arrives with the pipelines read. Asking before it lands
// asks for every deal and throws the answer away; no pipelines answers `[]`,
// which is knowing. The table waits on that read, so it shows its failure.
export function useSurfaceReads(
  f: DealFilters,
  stages: readonly Stage[],
  view: "board" | "table",
  refused: boolean,
  pipelinesQuery: ReturnType<typeof usePipelines>,
  perPage: number,
) {
  const scopeKnown = pipelinesQuery.data !== undefined;
  const tableReads = view === "table" && !refused && scopeKnown;
  const dealsQuery = useDeals(f, tableReads, listFetchLimit(perPage));
  const stageColumns = useStageColumns(
    f,
    stages.map((stage) => stage.id),
    view === "board" && !refused,
  );
  // The board's column totals: a per-stage server aggregate over every
  // matching deal, not only the pages the columns have read. Built from the
  // same filter dials, so cards and totals agree on which deals are in view.
  // The table draws no column header to put one in.
  const stageTotalsQuery = useStageTotals(
    f,
    view === "board" && !refused && scopeKnown,
  );
  const readError = view === "board" ? stageColumns.error : dealsQuery.error;
  const listRead = scopeKnown ? dealsQuery : pipelinesQuery;
  const tableRead = useDealsTable(f, perPage, dealsQuery, listRead, tableReads);
  return { stageColumns, stageTotalsQuery, readError, tableRead };
}

// The table's list state: its cursor read, and the page the address names read
// at its offset beside it. Deals carry no `paramScope`, so the page is the
// unscoped one the table writes.
function useDealsTable(
  f: DealFilters,
  perPage: number,
  dealsQuery: ReturnType<typeof useDeals>,
  listRead: ReturnType<typeof useDeals> | ReturnType<typeof usePipelines>,
  enabled: boolean,
) {
  const [params] = useUrlParams();
  const closeDay = useCloseDay(f.filters[CLOSE_DAY_DIAL]);
  const readerScope = useHealthScope();
  const pages = dealsQuery.data?.pages ?? [];
  const rows = pages.flatMap((page) => page.data);
  const list = useJumpedList({
    prefixKey: dealsListKey(
      readerScope ?? "",
      f,
      closeDay,
      listFetchLimit(perPage),
    ),
    read: async (cursor, reach, signal) =>
      listPageOfDeals(
        await readDealsAt(
          f,
          closeDay,
          {
            cursor: cursor ?? undefined,
            offset: reach.offset,
            limit: listFetchLimit(perPage, reach),
          },
          signal,
        ),
      ),
    enabled: enabled && readerScope !== undefined,
    page: listPageOf(params),
    perPage,
    rowKey: (deal) => deal.id,
    cursor: {
      rows,
      hasMore: dealsQuery.hasNextPage,
      pending: dealsQuery.isPending,
      total: pages.findLast((page) => page.page?.total !== undefined)?.page
        ?.total,
      isError: listRead.isError,
      error: listRead.error,
      refetch: () => listRead.refetch(),
    },
  });
  return {
    rows,
    held: list.held,
    jumped: list.jumped,
    total: list.total,
    isPending: listRead.isPending,
    isError: list.isError,
    error: list.error,
    refetch: list.refetch,
    hasMore: dealsQuery.hasNextPage,
    loadMore: () => {
      dealsQuery.fetchNextPage();
    },
  };
}

function listPageOfDeals(page: DealListPage) {
  return {
    data: page.data,
    page: {
      next_cursor: page.page?.next_cursor ?? null,
      has_more: page.page?.has_more ?? false,
      total: page.page?.total,
    },
  };
}
