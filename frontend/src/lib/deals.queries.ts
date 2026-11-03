import {
  hashKey,
  type QueryClient,
  type QueryKey,
  type UseQueryResult,
  useInfiniteQuery,
  useQueries,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { dealStatusFilter, forecastCategoryFilter } from "../format/dealstatus";
import { useNow } from "../format/now";
import { dayInZone } from "../format/timezone";
import type { MessageKey } from "../i18n/en";
import {
  CLOSE_DAY_DIAL,
  closeDayRange,
  type DayRange,
  narrowsByRange,
  rangeQueryParams,
} from "./dealranges";
import { throwProblem, unwrap } from "./problem";
import { useRecordZone } from "./recordzone";
import { parseTagIDs, parseTagMode, tagQueryParams } from "./tagfilter";
import { useInstallationSettings } from "./uploadlimit";

/**
 * The pipeline a record belongs to, falling back to the default.
 *
 * `pipelineId` is the deal's own. Without it, every deal would get the default
 * pipeline, and the stepper offers its stages as moves. The stages of a
 * pipeline the deal is not in are targets the server refuses as a pipeline
 * mismatch.
 */
export function usePipeline(pipelineId?: string | null) {
  return useQuery({
    queryKey: ["pipelines", pipelineId ?? "default"],
    queryFn: async () => {
      const data = unwrap(
        await api.GET("/pipelines", {
          params: { query: {} },
        }),
      );
      const pipeline =
        (pipelineId &&
          data.data.find((candidate) => candidate.id === pipelineId)) ||
        data.data.find((candidate) => candidate.is_default) ||
        data.data[0];
      if (!pipeline) {
        throw new Error("no pipeline");
      }
      return pipeline;
    },
  });
}

export type DealFilters = {
  pipelineId: string;
  q: string;
  sort: string;
  includeArchived: boolean;
  filters: Record<string, string>;
  /** The close-day dial read as days, which needs today and the fiscal year. */
  closeDay?: DayRange;
};

// Drawing mode and pipeline selection are URL dials, not deal-list filters.

export const PIPELINE_PARAM = "pipeline_id";

export const VIEW_PARAM = "view";

// This screen's own dials, held out of the list's parameter space by the codec's
// shared pair rather than by copies kept here. A copy can drift, and then a list
// sends its drawing choice to the server as a filter.
export const DEAL_SCREEN_DIALS: readonly string[] = [
  PIPELINE_PARAM,
  VIEW_PARAM,
];

// The board's owner dials as the server reads them. The deals and the
// suggestions drawn beside them both take these, so Mine narrows both alike.
export function ownerDialParams(filters: Record<string, string>) {
  return {
    owner_id: filters.owner_id || undefined,
    owner_team_id: filters.owner_team_id || undefined,
    unassigned: filters.unassigned === "true" ? true : undefined,
  };
}

// dealsQueryParams builds the board's /deals query: the full dial set
// (pipeline/stage/owner/company filters + sort).
export function dealsQueryParams(f: DealFilters) {
  const { filters } = f;
  return {
    limit: 100,
    include_archived: f.includeArchived || undefined,
    pipeline_id: f.pipelineId || undefined,
    q: f.q || undefined,
    sort: f.sort || undefined,
    stage_id: filters.stage_id || undefined,
    ...ownerDialParams(filters),
    company_id: filters.company_id || undefined,
    status: dealStatusFilter(filters.status),
    partner_company_id: filters.partner_company_id || undefined,
    stalled: filters.stalled === "true" ? true : undefined,
    forecast_category: forecastCategoryFilter(filters.forecast_category),
    partner_sourced: filters.partner_sourced === "true" ? true : undefined,
    // Several ids and their mode, out of the one comma-joined string the
    // address carries them in. Spelled by the shared encoder rather than here,
    // so this board and the three lists cannot read one address two ways.
    ...tagQueryParams(
      parseTagIDs(filters.tag_id),
      parseTagMode(filters.tag_mode),
    ),
    ...rangeQueryParams(filters, f.closeDay),
  };
}

// The close-day dial as days. "This quarter" is the fiscal quarter holding
// today in the installation's zone, the one the reports count.
function useCloseDay(value: string | undefined): DayRange | undefined {
  const today = dayInZone(useNow(0), useRecordZone());
  const fiscalStart =
    useInstallationSettings().data?.fiscal_year_start_month ?? 1;
  return closeDayRange(value, today, fiscalStart);
}

type DealListQuery = ReturnType<typeof dealsQueryParams> & {
  cursor?: string;
};

type DealListPage = Awaited<ReturnType<typeof readDealPage>>;

type Deal = components["schemas"]["Deal"];

async function readDealPage(query: DealListQuery) {
  return unwrap(await api.GET("/deals", { params: { query } }));
}

function nextCursorOf(page: DealListPage): string | undefined {
  return page.page?.has_more ? (page.page.next_cursor ?? undefined) : undefined;
}

/**
 * The deals as one keyset walk over every stage, which the table draws. The
 * board reads each column on its own instead (useStageColumns).
 */
export function useDeals(f: DealFilters, enabled = true) {
  const closeDay = useCloseDay(f.filters[CLOSE_DAY_DIAL]);
  return useInfiniteQuery({
    enabled,
    // The days, not the dial's word: a list opened after the quarter turns
    // never reuses the old quarter's cached page. Today is read at mount.
    queryKey: ["deals", f, closeDay],
    // `as` steers useInfiniteQuery's TPageParam generic to the cursor type: a
    // bare `undefined` infers TPageParam=undefined and the string cursor no
    // longer type-checks.
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) =>
      readDealPage({
        ...dealsQueryParams({ ...f, closeDay }),
        cursor: pageParam,
      }),
    getNextPageParam: nextCursorOf,
  });
}

// A board column's page: smaller than the table's, since every column reads at once.
export const STAGE_COLUMN_LIMIT = 25;

/** One stage column of the board, in the shape QueryStates and LoadMoreButton read. */
export type StageColumnRead = Readonly<{
  deals: Deal[];
  isPending: boolean;
  /** The first page failed, so the column holds no cards. */
  isError: boolean;
  error: unknown;
  refetch: () => unknown;
  hasNextPage: boolean;
  isFetchingNextPage: boolean;
  fetchNextPage: () => unknown;
  /** A later page failed; the cards already read stay. */
  loadMoreError: unknown;
}>;

// Each cursor comes from the page this walk just read, never a cached one, so a
// walk's pages meet with no gap or overlap however the column changed.
async function walkStagePages(
  query: DealListQuery,
  depth: number,
  seed: readonly DealListPage[],
): Promise<DealListPage[]> {
  const pages = [...seed];
  const seedEnd = seed.at(-1);
  let cursor = seedEnd ? nextCursorOf(seedEnd) : undefined;
  while (pages.length < depth && (pages.length === 0 || cursor !== undefined)) {
    const page = await readDealPage({ ...query, cursor });
    pages.push(page);
    cursor = nextCursorOf(page);
  }
  return pages;
}

// The shallower depth's pages start a "load more" walk, so it costs one request.
// Never on a refetch or from invalidated pages: a write may have shifted them.
function seedPages(
  client: QueryClient,
  queryKey: QueryKey,
  shallowerKey: QueryKey,
): readonly DealListPage[] {
  if (client.getQueryData(queryKey) !== undefined) return [];
  const shallower = client.getQueryState<DealListPage[]>(shallowerKey);
  return shallower?.data && !shallower.isInvalidated ? shallower.data : [];
}

// The newest copy of each deal, by version. Mid-refetch a moved deal can sit in
// the column it left and the one it joined; it draws once, where it now is.
function newestOfEach(deals: readonly Deal[]): Deal[] {
  const byId = new Map<string, Deal>();
  for (const deal of deals) {
    const held = byId.get(deal.id);
    if (!held || (deal.version ?? 0) > (held.version ?? 0))
      byId.set(deal.id, deal);
  }
  return [...byId.values()];
}

function stageColumnRead(
  query: UseQueryResult<DealListPage[]>,
  shown: DealListPage[] | undefined,
  loadMore: () => void,
): StageColumnRead {
  const last = shown?.at(-1);
  const fetchFailed = query.isError ? query.error : null;
  return {
    deals: shown?.flatMap((page) => page.data) ?? [],
    isPending: shown === undefined && query.isPending,
    isError: shown === undefined && query.isError,
    error: shown === undefined ? fetchFailed : null,
    refetch: () => query.refetch(),
    hasNextPage: last !== undefined && nextCursorOf(last) !== undefined,
    isFetchingNextPage:
      query.data === undefined && shown !== undefined && query.isFetching,
    // A failed deeper read is retried at that depth, from the seed it had.
    fetchNextPage: () => (query.isError ? query.refetch() : loadMore()),
    loadMoreError: shown === undefined ? null : fetchFailed,
  };
}

// Not one walk over every stage: a later stage's column would head a count over
// no cards until the busy stages ran out. The filters reset each column's depth.
export function useStageColumns(
  f: DealFilters,
  stageIds: readonly string[],
  enabled = true,
) {
  const closeDay = useCloseDay(f.filters[CLOSE_DAY_DIAL]);
  const client = useQueryClient();
  const scope = hashKey([f, closeDay]);
  const [paging, setPaging] = useState<{
    scope: string;
    depths: Readonly<Record<string, number>>;
  }>({ scope, depths: {} });
  const depths = paging.scope === scope ? paging.depths : {};
  const readStages = f.filters.stage_id
    ? stageIds.filter((stageId) => stageId === f.filters.stage_id)
    : stageIds;
  const keyOf = (stageId: string, depth: number) =>
    ["deals", "stage-column", f, closeDay, stageId, depth] as const;
  const queries = useQueries({
    queries: readStages.map((stageId) => {
      const depth = depths[stageId] ?? 1;
      return {
        queryKey: keyOf(stageId, depth),
        enabled,
        queryFn: ({ queryKey }: { queryKey: QueryKey }) =>
          walkStagePages(
            {
              ...dealsQueryParams({ ...f, closeDay }),
              limit: STAGE_COLUMN_LIMIT,
              stage_id: stageId,
            },
            depth,
            seedPages(client, queryKey, keyOf(stageId, depth - 1)),
          ),
      };
    }),
  });
  const loadMore = (stageId: string) =>
    setPaging((prev) => {
      const held = prev.scope === scope ? prev.depths : {};
      return {
        scope,
        depths: { ...held, [stageId]: (held[stageId] ?? 1) + 1 },
      };
    });
  const columns = new Map<string, StageColumnRead>();
  readStages.forEach((stageId, index) => {
    const query = queries[index];
    if (!query) return;
    const depth = depths[stageId] ?? 1;
    // While a deeper read loads or after it failed, the column keeps the cards
    // it already drew.
    const shown =
      query.data ??
      (depth > 1
        ? client.getQueryData<DealListPage[]>(keyOf(stageId, depth - 1))
        : undefined);
    columns.set(
      stageId,
      stageColumnRead(query, shown, () => loadMore(stageId)),
    );
  });
  const reads = [...columns.values()];
  return {
    columns,
    deals: newestOfEach(reads.flatMap((column) => column.deals)),
    isPending: reads.some((column) => column.isPending),
    error: queries.find((query) => query.isError)?.error ?? null,
  };
}

// dealsByStageReportFilters translates the board's own filter dials into the
// deals-by-stage report's filter shape. They are the same dials dealsQueryParams
// sends to /deals, so a stage total and its cards agree on which deals are in
// scope. Unsupported population dials withhold totals before this runs.
function dealsByStageReportFilters(f: DealFilters): Record<string, unknown> {
  const { filters } = f;
  const out: Record<string, unknown> = {};
  if (f.pipelineId) out.pipeline_id = f.pipelineId;
  if (filters.owner_id) out.owner_id = filters.owner_id;
  if (filters.company_id) out.company_id = filters.company_id;
  const status = dealStatusFilter(filters.status);
  if (status) out.status = status;
  const forecast = forecastCategoryFilter(filters.forecast_category);
  if (forecast) out.forecast_category = forecast;
  if (filters.partner_company_id)
    out.partner_company_id = filters.partner_company_id;
  if (filters.stalled === "true") out.stalled = true;
  if (filters.partner_sourced === "true") out.partner_sourced = true;
  return out;
}

// Why the board may not state a stage total, or undefined when it may.
//
// One function for both the decision and the words. The query's `enabled` keys
// on this answering undefined, and the column prints what it answers. So a
// reason cannot be added to one without the other. The column can never
// explain a total by a rule other than the one that withheld it.
//
// Derived from the filters, never from the query's own status. A query that
// errored also settles to idle-and-not-successful. Reading status would tell a
// reader whose report answered 422 to press a filter, not show the failure.
//
// Unsupported filters and archived-inclusive lists have a different population
// from the live report. Their counts and money must stay limited to loaded cards.
export function totalsWithheldBecause(f: DealFilters): MessageKey | undefined {
  if (f.includeArchived) return "deals.totalsNoArchived";
  if (f.filters.stage_id) return "deals.totalsNoStageFilter";
  if (parseTagIDs(f.filters.tag_id).length > 0) {
    return "deals.totalsNoTagFilter";
  }
  if (f.q) return "deals.totalsNoSearch";
  if (f.filters.owner_team_id || f.filters.unassigned === "true") {
    return "deals.totalsNoTeamFilter";
  }
  if (narrowsByRange(f.filters)) return "deals.totalsNoRangeFilter";
  return undefined;
}

// The one reason the server decides: an owner dial naming somebody whose
// totals this reader may not measure answers 403. The cards stay, because
// reading a deal and measuring its owner are different permissions.
export function totalsRefusedBecause(
  totals: Map<string, StageTotals> | null | undefined,
): MessageKey | undefined {
  return totals === null ? "deals.totalsOwnerNotMeasurable" : undefined;
}

// useStageTotals reads the board's per-column totals from the deals-by-stage
// report: a full aggregate over every matching deal, not the pages the columns
// have read. The server converts each deal before grouping by stage.
// Reports exclude archived deals, so that broader board population withholds
// its totals through the same rule as every other unsupported filter.
export function useStageTotals(f: DealFilters, enabled = true) {
  return useQuery({
    // Under ["deals"], so the one invalidation every deal mutation fires
    // refreshes the column headers along with the cards. Keyed apart, a moved
    // card would sit under a header still counting it in the stage it left.
    queryKey: ["deals", "by-stage-totals", f],
    // Asked only when the report would count the deals the board is drawing.
    // totalsWithheldBecause owns that decision and the words for it both, so
    // the column cannot explain a missing total by the wrong rule.
    enabled: enabled && totalsWithheldBecause(f) === undefined,
    queryFn: async (): Promise<Map<string, StageTotals> | null> => {
      const { data, error, response } = await api.POST("/reports/{report}", {
        params: { path: { report: "deals-by-stage" } },
        body: {
          group_by: ["stage_id"],
          aggregates: [
            { fn: "count", as: "deals" },
            { fn: "sum", field: "amount_base_minor", as: "base_minor" },
            {
              fn: "sum",
              field: "weighted_base_minor",
              as: "weighted_base_minor",
            },
            { fn: "count", field: "amount_base_minor", as: "priced_deals" },
          ],
          filters: dealsByStageReportFilters(f),
        },
      });
      if (error) {
        if (response.status === 403) {
          return null;
        }
        throwProblem(error);
      }
      return buildStageTotals(
        data.rows,
        data.base_currency ?? null,
        data.as_of ?? null,
        data.timezone ?? null,
      );
    },
  });
}

// Stage totals carry the server's reporting currency and valuation date.
export type StageTotals = {
  count: number;
  baseMinor: number | null;
  weightedBaseMinor: number | null;
  currency: string | null;
  asOf: string | null;
  timezone: string | null;
  unpricedCount: number | null;
};

function reportMinor(value: unknown): number | null {
  if (value == null || value === "") return null;
  const amount = Number(value);
  return Number.isFinite(amount) ? amount : null;
}

export function buildStageTotals(
  rows: Record<string, unknown>[],
  currency: string | null = null,
  asOf: string | null = null,
  timezone: string | null = null,
): Map<string, StageTotals> {
  return new Map(
    rows.map((row) => {
      const count = reportMinor(row.deals) ?? 0;
      const priced = reportMinor(row.priced_deals);
      return [
        String(row.stage_id ?? ""),
        {
          count,
          baseMinor: currency ? reportMinor(row.base_minor) : null,
          weightedBaseMinor: currency
            ? reportMinor(row.weighted_base_minor)
            : null,
          currency,
          asOf,
          timezone,
          unpricedCount: priced == null ? null : Math.max(0, count - priced),
        },
      ];
    }),
  );
}
