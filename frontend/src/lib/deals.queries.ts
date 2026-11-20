import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { dealStatusFilter } from "../format/dealstatus";
import { useNow } from "../format/now";
import { dayInZone } from "../format/timezone";
import { useLocale } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { useCompanyZone } from "./companyzone";
import { motionFilter, priorityFilter } from "./dealcommercialvalues";
import {
  CLOSE_DAY_DIAL,
  closeDayRange,
  type DayRange,
  narrowsByRange,
  rangeQueryParams,
  WIN_PROBABILITY_DIAL,
} from "./dealranges";
import { useHealthScope } from "./healthscope";
import { translatedPipeline } from "./labeltranslation";
import {
  defaultPipelineOf,
  pipelinesOptions,
  pipelinesWithArchivedOptions,
} from "./pipelines.queries";
import { throwProblem, unwrap } from "./problem";
import { parseTagIDs, parseTagMode, tagQueryParams } from "./tagfilter";
import { useInstallationSettings } from "./uploadlimit";
import { bandOfDial } from "./winprobabilitybands";

/**
 * The pipeline a record belongs to: the deal's own, retired ones included.
 *
 * The active catalog the pickers share answers first. Only a pipeline missing
 * there is looked up among the retired ones. One found in neither is a load
 * error, since another pipeline's stages would offer moves the server refuses.
 * Without an id, the default pipeline answers.
 */
export function usePipeline(pipelineId?: string | null) {
  const { locale } = useLocale();
  const active = useQuery(pipelinesOptions);
  const inActive = pipelineId
    ? active.data?.find((candidate) => candidate.id === pipelineId)
    : defaultPipelineOf(active.data ?? []);
  const lookUpRetired = Boolean(pipelineId) && active.isSuccess && !inActive;
  const retired = useQuery({
    ...pipelinesWithArchivedOptions,
    enabled: lookUpRetired,
  });
  const found =
    inActive ?? retired.data?.find((candidate) => candidate.id === pipelineId);
  const missing =
    active.isSuccess && !found && (!lookUpRetired || retired.isSuccess);
  const failed = active.error ?? retired.error ?? null;
  return {
    data: found ? translatedPipeline(found, locale) : undefined,
    error: failed ?? (missing ? new Error("no pipeline") : null),
    isError: failed !== null || missing,
    isPending: !found && failed === null && !missing,
  };
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
    partner_sourced: filters.partner_sourced === "true" ? true : undefined,
    ...commercialDialParams(filters),
    link_type: filters.link_type || undefined,
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

// The commercial dials, sent to the deal list and the stage totals alike, so a
// column's total counts the cards it draws.
function commercialDialParams(filters: Record<string, string>) {
  return {
    commercial_motion: motionFilter(filters.commercial_motion),
    priority: priorityFilter(filters.priority),
    acquisition_source: filters.acquisition_source || undefined,
  };
}

// The close-day dial as days. "This quarter" is the fiscal quarter holding
// today in the installation's zone, the one the reports count.
export function useCloseDay(value: string | undefined): DayRange | undefined {
  const today = dayInZone(useNow(0), useCompanyZone());
  const fiscalStart =
    useInstallationSettings().data?.fiscal_year_start_month ?? 1;
  return closeDayRange(value, today, fiscalStart);
}

export type DealListQuery = ReturnType<typeof dealsQueryParams> & {
  cursor?: string;
};

export type DealListPage = Awaited<ReturnType<typeof readDealPage>>;

export async function readDealPage(query: DealListQuery, signal?: AbortSignal) {
  return unwrap(await api.GET("/deals", { params: { query }, signal }));
}

export function nextCursorOf(page: DealListPage): string | undefined {
  return page.page?.has_more ? (page.page.next_cursor ?? undefined) : undefined;
}

/**
 * The deals as one keyset walk over every stage, `limit` rows a read. A table
 * passes the size its pager slices (`listFetchLimit`); the default suits a
 * picker that lists them all.
 */
export function useDeals(f: DealFilters, enabled = true, limit = 100) {
  const closeDay = useCloseDay(f.filters[CLOSE_DAY_DIAL]);
  // The rows one reader scope may see; nothing is read before /me names it.
  const readerScope = useHealthScope();
  return useInfiniteQuery({
    enabled: enabled && readerScope !== undefined,
    // The days, not the dial's word: a list opened after the quarter turns
    // never reuses the old quarter's cached page. Today is read at mount.
    queryKey: ["deals", readerScope ?? "", f, closeDay, limit],
    // `as` steers useInfiniteQuery's TPageParam generic to the cursor type: a
    // bare `undefined` infers TPageParam=undefined and the string cursor no
    // longer type-checks.
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      readDealPage(
        { ...dealsQueryParams({ ...f, closeDay }), limit, cursor: pageParam },
        signal,
      ),
    getNextPageParam: nextCursorOf,
  });
}

// dealsByStageReportFilters translates the board's own filter dials into the
// deals-by-stage report's filter shape. They are the same dials dealsQueryParams
// sends to /deals, so a stage total and its cards agree on which deals are in
// scope. Unsupported population dials withhold totals before this runs.
export function dealsByStageReportFilters(
  f: DealFilters,
): Record<string, unknown> {
  const { filters } = f;
  const out: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(commercialDialParams(filters))) {
    if (value) out[key] = value;
  }
  if (f.pipelineId) out.pipeline_id = f.pipelineId;
  if (filters.owner_id) out.owner_id = filters.owner_id;
  if (filters.company_id) out.company_id = filters.company_id;
  const status = dealStatusFilter(filters.status);
  if (status) out.status = status;
  const band = bandOfDial(filters[WIN_PROBABILITY_DIAL]);
  if (band) out.win_probability_band = band.key;
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
  // The report's link_type matches a deal's whole set of link types. The list
  // matches any one of them, so the two would count different deals.
  if (f.filters.link_type) return "deals.totalsNoLinkFilter";
  if (narrowsByRange(f.filters)) return "deals.totalsNoRangeFilter";
  // The report narrows by band only, so a range no band matches has no total.
  const probability = f.filters[WIN_PROBABILITY_DIAL];
  if (probability && !bandOfDial(probability)) {
    return "deals.totalsNoRangeFilter";
  }
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
  // The reader's grants decide what the report counts, so a changed grant must
  // never reuse totals counted under the old one.
  const scope = useHealthScope();
  const reportFilters = dealsByStageReportFilters(f);
  const withheld = totalsWithheldBecause(f);
  return useQuery({
    // Under ["deals"], so the one invalidation every deal mutation fires
    // refreshes the column headers along with the cards. Keyed apart, a moved
    // card would sit under a header still counting it in the stage it left.
    // Keyed by the request body, not the whole filter set. A table sort then
    // reuses the board's fresh totals on return, since sort never reaches it.
    // A withheld key is never read, so it never holds a total.
    queryKey: [
      "deals",
      "by-stage-totals",
      scope,
      reportFilters,
      withheld ?? null,
    ],
    // Asked only when the report would count the deals the board is drawing.
    // totalsWithheldBecause owns that decision and the words for it both, so
    // the column cannot explain a missing total by the wrong rule.
    enabled: enabled && scope !== undefined && withheld === undefined,
    queryFn: async ({ signal }): Promise<Map<string, StageTotals> | null> => {
      const { data, error, response } = await api.POST("/reports/{report}", {
        signal,
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
          filters: reportFilters,
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
