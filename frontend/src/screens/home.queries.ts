// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type UseQueryResult,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./common";
import { worklistKey } from "./worklist.queries";

// Home's reads, in one place. The screen fans out to five of them and each is
// gated on its own, deliberately: a transient failure in the decisions queue
// must never hide a healthy brief, and one combined "my day" payload does not
// exist on the server.

export type MorningBrief = components["schemas"]["MorningBrief"];
export type WeeklyReview = components["schemas"]["WeeklyReview"];
export type MorningBriefItem = components["schemas"]["MorningBriefItem"];
export type MorningDigest = components["schemas"]["MorningDigest"];
export type Deal = components["schemas"]["Deal"];

export function useMorningBrief(): UseQueryResult<MorningBrief | null> {
  return useQuery({
    queryKey: ["brief"],
    // On the same schedule as the attention feed, because the Worklist's
    // briefing lane is BOTH of them: the feed says which entries are still
    // waiting and this says what each one is. Refreshing one without the other
    // leaves the lane intersecting a fresh list of ids against a stale set of
    // items, and an intersection that finds nothing draws the quiet-morning
    // plate over a morning that has work in it.
    refetchOnWindowFocus: true,
    queryFn: async (): Promise<MorningBrief | null> => {
      const { data, error, response } = await api.GET("/brief");
      // No run yet is not a failure: it is a page that offers to make one.
      if (response.status === 404) {
        return null;
      }
      if (error) {
        throwProblem(error);
      }
      return data ?? null;
    },
  });
}

/**
 * The overnight digest (CAP-WIRE-6): the nightly build's stored counts — what
 * capture landed and what awaits review.
 *
 * `404 no_digest_yet` and `501` are the same fact to a reader (there is no
 * digest to show) and both return null. Reading 501 as an error is what once
 * put a permanent loading block on this page: a 501 is a 5xx, so the query
 * client retried it, and React Query PAUSES between retry attempts while the
 * document is hidden — the query sat at fetchStatus "paused" and never settled,
 * and the pending skeleton stood in for the refusal indefinitely.
 */
export function useMorningDigest(): UseQueryResult<MorningDigest | null> {
  return useQuery({
    queryKey: ["digest"],
    queryFn: async (): Promise<MorningDigest | null> => {
      const { data, error, response } = await api.GET("/digest");
      if (response.status === 404 || response.status === 501) {
        return null;
      }
      if (error) {
        throwProblem(error);
      }
      return data ?? null;
    },
  });
}

/** One currency's open pipeline: what it is worth, and what it is worth once
 *  each deal is weighted by its stage's probability. */
export type PipelineValue = {
  currency: string;
  rawMinor: number;
  weightedMinor: number;
  deals: number;
};

/** What the report answered: the per-currency lines, and whether a field mask
 *  kept rows out of them. */
export type PipelineReading = {
  rows: PipelineValue[];
  /** Rows a mask withheld from every total here. Non-zero means the figures
   *  understate the pipeline, and saying so is the difference between a
   *  partial answer and a wrong one. */
  excluded: number;
};

/**
 * The open pipeline, per currency.
 *
 * Grouped by currency and rendered one line each rather than summed: adding
 * native minor units across currencies produces a number that is not money,
 * which is the rule the board's mixed-currency columns already follow.
 *
 * The report never includes archived deals, and this asks only for open ones —
 * a won deal is revenue, not pipeline, and counting it here would make the
 * headline grow every time somebody closed something.
 */
export function usePipelineValue(): UseQueryResult<PipelineReading> {
  return useQuery({
    // Under ["deals"] so the invalidation every deal mutation already fires
    // reaches this too. Keyed apart, the headline went on naming yesterday's
    // pipeline after a rep won something and came back to Home.
    queryKey: ["deals", "home-pipeline-value"],
    queryFn: async (): Promise<PipelineReading> => {
      const { data, error } = await api.POST("/reports/{report}", {
        params: { path: { report: "deals-by-stage" } },
        body: {
          group_by: ["currency"],
          aggregates: [
            { fn: "count", as: "deals" },
            { fn: "sum", field: "amount_minor", as: "raw_minor" },
            { fn: "sum", field: "weighted_amount_minor", as: "weighted_minor" },
          ],
          filters: { status: "open" },
        },
      });
      if (error) {
        throwProblem(error);
      }
      return {
        rows: data.rows.flatMap((row) => {
          const currency = row.currency;
          // A SUM over deals nobody priced is absent, not zero, and a row with
          // no currency cannot be rendered as money at all.
          if (
            typeof currency !== "string" ||
            typeof row.raw_minor !== "number"
          ) {
            return [];
          }
          return [
            {
              currency,
              rawMinor: row.raw_minor,
              weightedMinor:
                typeof row.weighted_minor === "number" ? row.weighted_minor : 0,
              deals: typeof row.deals === "number" ? row.deals : 0,
            },
          ];
        }),
        excluded: data.excluded_by_permission ?? 0,
      };
    },
  });
}

/** One page of deals, and whether the list ended there. */
export type HomeDeals = Readonly<{ rows: Deal[]; more: boolean }>;

/** How many deals Home reads in one go. */
const HOME_DEALS_PAGE = 100;

/**
 * The deals page Home reads twice over: the quiet ones it lists, and the count
 * of open ones its readings strip reports.
 *
 * One query rather than two because there is no server-side "stalled" filter to
 * ask for — the flag arrives on the row and the filtering is ours.
 *
 * ONE page, and the page's own `has_more` travels with it. Following the cursor
 * would cost an unbounded fan-out on the one screen that opens every morning,
 * so the honest answer is the other one: every reading taken from these rows is
 * a FLOOR past the page, and says so where it is drawn. A count that quietly
 * stopped rising is the failure this repo cares about most — the same words, a
 * smaller number, and nothing failing.
 */
export function useHomeDeals(): UseQueryResult<HomeDeals> {
  return useQuery({
    queryKey: ["deals"],
    queryFn: async (): Promise<HomeDeals> => {
      const { data, error } = await api.GET("/deals", {
        params: { query: { limit: HOME_DEALS_PAGE } },
      });
      if (error) {
        throwProblem(error);
      }
      return { rows: data.data, more: data.page?.has_more ?? false };
    },
  });
}

/** The open deals that have gone quiet, in the order the wire sent them. */
export function quietDeals(deals: readonly Deal[]): Deal[] {
  return deals.filter((deal) => deal.stalled && deal.status === "open");
}

/** Every open deal, quiet or not — what the readings strip counts. */
export function openDeals(deals: readonly Deal[]): Deal[] {
  return deals.filter((deal) => deal.status === "open");
}

/**
 * Ask for today's brief now. The overnight pass owns generation and a rep has
 * one run per local day, so this is a catch-up rather than a re-rank: it
 * returns the day's run, assembling it only if the night could not.
 */
export function useBriefRefresh() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async () => {
      const { data, error } = await api.POST("/brief");
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: (data) => {
      queryClient.setQueryData(["brief"], data ?? null);
    },
  });
}

/**
 * What a reader can do to one brief item.
 *
 * A union rather than a mark plus an optional instant: `snoozed_until` is
 * REQUIRED by the contract and must be in the future, so a shape that lets a
 * caller ask for a snooze without naming when it comes back is a 422 waiting to
 * be written.
 */
export type BriefMarkRequest =
  | { itemId: string; mark: "act" | "dismiss" }
  | { itemId: string; mark: "snooze"; snoozedUntil: string };

/**
 * Act on, dismiss or snooze one item.
 *
 * The item id travels as a mutation VARIABLE rather than in the closure: the
 * click handler belongs to the committed render, so a variable it passes cannot
 * be older than the control that carried it (frontend/CLAUDE.md, and the gate
 * in `mutation-variable-coverage.test.ts`).
 */
export function useBriefItemMark() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (variables: BriefMarkRequest) => {
      if (variables.mark === "snooze") {
        const { data, error } = await api.POST("/brief/items/{itemId}/snooze", {
          params: { path: { itemId: variables.itemId } },
          body: { snoozed_until: variables.snoozedUntil },
        });
        if (error) {
          throwProblem(error);
        }
        return data;
      }
      const path =
        variables.mark === "act"
          ? "/brief/items/{itemId}/act"
          : "/brief/items/{itemId}/dismiss";
      const { data, error } = await api.POST(path, {
        params: { path: { itemId: variables.itemId } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: (updated) => {
      queryClient.setQueryData<MorningBrief | null>(["brief"], (current) =>
        current
          ? {
              ...current,
              items: current.items.map((item) =>
                item.id === updated.id ? updated : item,
              ),
            }
          : current,
      );
      // The Worklist draws the same queue as its own lane, and an answered item
      // leaves that lane rather than settling in place. Patching only the brief
      // cache would leave the row on screen until the next refetch, which reads
      // exactly like a click that did nothing.
      void queryClient.invalidateQueries({ queryKey: worklistKey });
    },
  });
}

/**
 * The rep's weekly retrospective — the most recent, or a named week.
 *
 * `404` is not a failure: a rep whose first Monday has not come round yet has
 * no review, and that is a page saying so rather than an error.
 */
export function useWeeklyReview(
  week?: string,
): UseQueryResult<WeeklyReview | null> {
  return useQuery({
    queryKey: ["weekly-review", week ?? "latest"],
    queryFn: async (): Promise<WeeklyReview | null> => {
      const { data, error, response } = await api.GET(
        "/weekly-reviews/latest",
        {
          params: { query: week === undefined ? {} : { week } },
        },
      );
      if (response.status === 404) {
        return null;
      }
      if (error) {
        throwProblem(error);
      }
      // A payload that is not a review reads as no review, not as one with
      // undefined fields. The panel formats local_week_start straight away, so
      // a half-shaped answer would take Home's whole render down rather than
      // drawing the honest "no review yet" state.
      return data?.local_week_start === undefined ? null : data;
    },
  });
}

/** The weeks this rep has a review for — the archive's index. */
export function useWeeklyReviewIndex(): UseQueryResult<readonly string[]> {
  return useQuery({
    queryKey: ["weekly-review-index"],
    queryFn: async (): Promise<readonly string[]> => {
      const { data, error } = await api.GET("/weekly-reviews");
      if (error) {
        throwProblem(error);
      }
      // Never undefined out of this hook. React Query refuses an undefined
      // result, and a payload without the field is a server that answered
      // something else — which must read as "no weeks", not as a crash that
      // takes Home's whole render with it.
      return data?.weeks ?? [];
    },
  });
}
