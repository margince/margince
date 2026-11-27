import { listFetchLimit, listPageOf } from "../../components/listquery";
import { jumpTarget, useListJump } from "../../components/listquery.jump";
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
  const dealJump = useDealJump(f, perPage, dealsQuery, tableReads);
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
  return {
    dealsQuery,
    stageColumns,
    stageTotalsQuery,
    readError,
    tableRead: tableReadOf(listRead, dealsQuery, dealJump),
  };
}

// The table's list state, read off its cursor read and the page read beside it.
function tableReadOf(
  listRead: ReturnType<typeof useDeals> | ReturnType<typeof usePipelines>,
  dealsQuery: ReturnType<typeof useDeals>,
  dealJump: ReturnType<typeof useDealJump>,
) {
  return {
    isPending: listRead.isPending,
    isError: listRead.isError || dealJump.isError,
    error: listRead.error ?? dealJump.error,
    refetch: () => {
      listRead.refetch();
      dealJump.refetch();
    },
    hasMore: dealsQuery.hasNextPage,
    loadMore: () => {
      dealsQuery.fetchNextPage();
    },
    jumped: dealJump.jumped,
    total:
      dealJump.total ??
      dealsQuery.data?.pages.findLast((page) => page.page?.total !== undefined)
        ?.page?.total,
  };
}

// The table's page past the cursor's deals, read at its offset.
function useDealJump(
  f: DealFilters,
  perPage: number,
  dealsQuery: ReturnType<typeof useDeals>,
  enabled: boolean,
) {
  const [params] = useUrlParams();
  const closeDay = useCloseDay(f.filters[CLOSE_DAY_DIAL]);
  const readerScope = useHealthScope();
  const pages = dealsQuery.data?.pages ?? [];
  return useListJump({
    prefixKey: dealsListKey(
      readerScope ?? "",
      f,
      closeDay,
      listFetchLimit(perPage),
    ),
    target: jumpTarget(listPageOf(params), perPage, {
      rows: pages.reduce((count, page) => count + page.data.length, 0),
      hasMore: dealsQuery.hasNextPage,
      pending: dealsQuery.isPending,
    }),
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
  });
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
