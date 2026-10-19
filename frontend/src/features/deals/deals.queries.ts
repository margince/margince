import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { MessageKey } from "../../i18n/en";
import { throwProblem, unwrap } from "../../lib/problem";
import { parseTagIDs, parseTagMode, tagQueryParams } from "../../lib/tagfilter";
import { dealStatusFilter, forecastCategoryFilter } from "./dealdials";

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
    owner_id: filters.owner_id || undefined,
    owner_team_id: filters.owner_team_id || undefined,
    unassigned: filters.unassigned === "true" ? true : undefined,
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
// scope. stage_id is absent: the report is grouped by stage_id, which already
// answers "per stage".
function dealsByStageReportFilters(f: DealFilters): Record<string, unknown> {
  const { filters } = f;
  const out: Record<string, unknown> = {};
  if (f.pipelineId) out.pipeline_id = f.pipelineId;
  if (filters.owner_id) out.owner_id = filters.owner_id;
  if (filters.company_id) out.company_id = filters.company_id;
  const status = dealStatusFilter(filters.status);
  if (status) out.status = status;
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
// A tag, a search, a team and the unassigned queue have no filter field on the
// report, and sending one is a 422. The totals would count deals the board is
// not showing. The report takes every other dial, and it measures every deal
// the reader may see, which is the set `GET /deals` draws as cards.
export function totalsWithheldBecause(f: DealFilters): MessageKey | undefined {
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
// useDeals fetches. Grouped by [stage_id, currency], so a mixed-currency stage
// arrives as more than one row, which buildStageTotals reads as "hide the sum".
// Like every report it never includes archived deals. So the totals reflect the
// live pipeline whatever the board's "show archived" toggle says.
export function useStageTotals(f: DealFilters) {
  return useQuery({
    // Under ["deals"], so the one invalidation every deal mutation fires
    // refreshes the column headers along with the cards. Keyed apart, a moved
    // card would sit under a header still counting it in the stage it left.
    // A foreign-currency deal arriving in a single-currency stage would leave
    // the old sum standing, and the mixed-currency refusal would not happen.
    queryKey: ["deals", "by-stage-totals", f],
    // Asked only when the report would count the deals the board is drawing.
    // totalsWithheldBecause owns that decision and the words for it both, so
    // the column cannot explain a missing total by the wrong rule.
    enabled: totalsWithheldBecause(f) === undefined,
    queryFn: async (): Promise<Map<string, StageTotals> | null> => {
      const { data, error, response } = await api.POST("/reports/{report}", {
        params: { path: { report: "deals-by-stage" } },
        body: {
          group_by: ["stage_id", "currency"],
          aggregates: [
            { fn: "count", as: "deals" },
            { fn: "sum", field: "amount_minor", as: "raw_minor" },
            {
              fn: "sum",
              field: "weighted_amount_minor",
              as: "weighted_minor",
            },
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
      return buildStageTotals(data.rows);
    },
  });
}

// StageTotals is one stage's count/raw/weighted total, sourced from the
// deals-by-stage report rather than the board's own (capped) card fetch. A
// pipeline with more deals than that cap would otherwise show a wrong sum.
export type StageTotals = {
  count: number;
  // Null where the report stated no figure. A stage can hold a real count of
  // deals nobody has priced, and its `SUM` then arrives as null with no
  // currency beside it. That is not the zero a naive read makes of it.
  rawMinor: number | null;
  weightedMinor: number | null;
  currency: string | null;
  sumHidden: boolean;
};

// A report cell as a figure, or nothing. An absent or non-numeric cell is the
// report declining to state an amount, and `Number(null)` turns that into a 0
// the server never sent.
function reportMinor(value: unknown): number | null {
  if (value == null || value === "") {
    return null;
  }
  const amount = Number(value);
  return Number.isFinite(amount) ? amount : null;
}

// buildStageTotals shapes a deals-by-stage report grouped by
// `["stage_id","currency"]` into one entry per stage. More than one currency
// row for a stage means the sum is cross-currency. That is decided from the
// report's full row set, not from whichever cards happened to load.
export function buildStageTotals(
  rows: Record<string, unknown>[],
): Map<string, StageTotals> {
  const byStage = new Map<string, Record<string, unknown>[]>();
  for (const row of rows) {
    const stageId = String(row.stage_id ?? "");
    const forStage = byStage.get(stageId) ?? [];
    forStage.push(row);
    byStage.set(stageId, forStage);
  }
  const totals = new Map<string, StageTotals>();
  for (const [stageId, stageRows] of byStage) {
    const count = stageRows.reduce(
      (sum, row) => sum + Number(row.deals ?? 0),
      0,
    );
    const mixed = stageRows.length > 1;
    const [single] = stageRows;
    if (single === undefined) continue;
    // One row is not the same fact as one currency. A stage whose deals are all
    // unpriced groups into a single row whose `currency` is null. The
    // cross-currency test says nothing about it, and its figures belong to no
    // currency at all. Naming EUR there is indistinguishable from a real EUR
    // total, which is the reading a rep would act on.
    const currency =
      !mixed && typeof single.currency === "string" && single.currency !== ""
        ? single.currency
        : null;
    totals.set(stageId, {
      count,
      rawMinor: currency ? reportMinor(single.raw_minor) : null,
      weightedMinor: currency ? reportMinor(single.weighted_minor) : null,
      currency,
      sumHidden: mixed,
    });
  }
  return totals;
}
