import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { dealStatusFilter, forecastCategoryFilter } from "../format/dealstatus";
import type { MessageKey } from "../i18n/en";
import { throwProblem, unwrap } from "./problem";
import { parseTagIDs, parseTagMode, tagQueryParams } from "./tagfilter";

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
  };
}

/**
 * The deals the board and the table share.
 *
 * A keyset walk rather than one fixed page. The board draws a column per stage
 * out of whatever this holds. A single capped read would show a busy stage only
 * a fraction of its cards. Its header comes from the deals-by-stage report over
 * every matching deal, so it names the true count. A column saying "40 deals"
 * above six cards is what a pipeline view must not do.
 */
export function useDeals(f: DealFilters) {
  return useInfiniteQuery({
    queryKey: ["deals", f],
    // `as` steers useInfiniteQuery's TPageParam generic to the cursor type: a
    // bare `undefined` infers TPageParam=undefined and the string cursor no
    // longer type-checks.
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      return unwrap(
        await api.GET("/deals", {
          params: { query: { ...dealsQueryParams(f), cursor: pageParam } },
        }),
      );
    },
    getNextPageParam: (last) =>
      last.page?.has_more ? (last.page.next_cursor ?? undefined) : undefined,
  });
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
// report: a full aggregate over every matching deal, not the capped page
// useDeals fetches. The server converts each deal before grouping by stage.
// Reports exclude archived deals, so that broader board population withholds
// its totals through the same rule as every other unsupported filter.
export function useStageTotals(f: DealFilters) {
  return useQuery({
    // Under ["deals"], so the one invalidation every deal mutation fires
    // refreshes the column headers along with the cards. Keyed apart, a moved
    // card would sit under a header still counting it in the stage it left.
    queryKey: ["deals", "by-stage-totals", f],
    // Asked only when the report would count the deals the board is drawing.
    // totalsWithheldBecause owns that decision and the words for it both, so
    // the column cannot explain a missing total by the wrong rule.
    enabled: totalsWithheldBecause(f) === undefined,
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
