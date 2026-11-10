// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  keepPreviousData,
  type QueryClient,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ifMatch } from "../../api/version";
import { identifierNumber, monthName } from "../../format/format";
import type { Locale } from "../../i18n";
import { unwrap } from "../../lib/problem";

export type SalesProgress = components["schemas"]["SalesProgress"];
export type SalesProgressAmount = components["schemas"]["SalesProgressAmount"];
export type SalesProgressGoal = components["schemas"]["SalesProgressGoal"];
export type SalesProgressMember = components["schemas"]["SalesProgressMember"];
export type SalesProgressMonth = components["schemas"]["SalesProgressMonth"];
export type SalesGoalPlan = components["schemas"]["SalesGoalPlan"];
export type SalesGoalRoster = components["schemas"]["SalesGoalRoster"];
export type SalesGoalMonths = components["schemas"]["SalesGoalMonths"];
export type SalesGoalDraftInput = components["schemas"]["SalesGoalDraftInput"];

/** The period and population a Progress read asks about. */
export type ProgressAsk = Readonly<{
  period: "month" | "year";
  year: number;
  month: number;
  scope?: Readonly<{ kind: "owner" | "team"; id: string }>;
}>;

// "analytics" heads every key, so turning individual performance off drops
// these figures with the rest (lib/individualperformance.ts).
const PROGRESS_KEY = ["analytics", "progress"] as const;
const GOALS_KEY = ["analytics", "sales-goals"] as const;

export function useSalesProgress(ask: ProgressAsk) {
  return useQuery({
    queryKey: [...PROGRESS_KEY, ask],
    placeholderData: keepPreviousData,
    queryFn: async ({ signal }) =>
      unwrap(
        await api.GET("/analytics/progress", {
          params: {
            query: {
              period: ask.period,
              year: ask.year,
              month: ask.period === "month" ? ask.month : undefined,
              scope_kind: ask.scope?.kind,
              scope_id: ask.scope?.id,
            },
          },
          signal,
        }),
      ),
  });
}

export function useSalesGoalRoster(year: number) {
  return useQuery({
    queryKey: [...GOALS_KEY, "sellers", year],
    queryFn: async ({ signal }) =>
      unwrap(
        await api.GET("/analytics/sales-goals", {
          params: { query: { year } },
          signal,
        }),
      ),
  });
}

export function useSalesGoalPlan(owner: string | undefined, year: number) {
  return useQuery({
    queryKey: [...GOALS_KEY, "plan", owner, year],
    enabled: owner !== undefined,
    queryFn: async ({ signal }) =>
      unwrap(
        await api.GET("/analytics/sales-goals/{owner_id}/{year}", {
          params: { path: { owner_id: owner ?? "", year } },
          signal,
        }),
      ),
  });
}

type PlanPath = Readonly<{ owner: string; year: number; version: number }>;

// A plan write moves the goals every Progress figure is read against.
function refreshGoals(client: QueryClient, plan: SalesGoalPlan) {
  client.setQueryData([...GOALS_KEY, "plan", plan.owner_id, plan.year], plan);
  void client.invalidateQueries({ queryKey: [...GOALS_KEY, "sellers"] });
  void client.invalidateQueries({ queryKey: PROGRESS_KEY });
}

function planPath(path: PlanPath) {
  return { owner_id: path.owner, year: path.year };
}

/** Saves, discards, publishes and withdraws one plan, each under the version the reader saw. */
export function useSalesGoalWrites() {
  const client = useQueryClient();
  const onSuccess = (plan: SalesGoalPlan) => refreshGoals(client, plan);
  const save = useMutation({
    mutationFn: async (
      input: PlanPath & { body: SalesGoalDraftInput; key: string },
    ) =>
      unwrap(
        await api.PUT("/analytics/sales-goals/{owner_id}/{year}/draft", {
          params: {
            path: planPath(input),
            ...ifMatch(input.version, { "Idempotency-Key": input.key }),
          },
          body: input.body,
        }),
      ),
    onSuccess,
  });
  const discard = useMutation({
    mutationFn: async (input: PlanPath) =>
      unwrap(
        await api.DELETE("/analytics/sales-goals/{owner_id}/{year}/draft", {
          params: { path: planPath(input), ...ifMatch(input.version) },
        }),
      ),
    onSuccess,
  });
  const publish = useMutation({
    mutationFn: async (input: PlanPath & { reason: string; key: string }) =>
      unwrap(
        await api.POST("/analytics/sales-goals/{owner_id}/{year}/publish", {
          params: {
            path: planPath(input),
            ...ifMatch(input.version, { "Idempotency-Key": input.key }),
          },
          body: { reason: input.reason, confirm_agreed: true },
        }),
      ),
    onSuccess,
  });
  const withdraw = useMutation({
    mutationFn: async (input: PlanPath & { reason: string }) =>
      unwrap(
        await api.POST("/analytics/sales-goals/{owner_id}/{year}/withdraw", {
          params: { path: planPath(input), ...ifMatch(input.version) },
          body: { reason: input.reason },
        }),
      ),
    onSuccess,
  });
  return { save, discard, publish, withdraw };
}

/**
 * A yearly amount spread over twelve months. Each gets a twelfth, rounded down.
 * The first months get one more minor unit each until the sum is the year.
 */
export function equalMonths(totalMinor: number): number[] {
  const base = Math.floor(totalMinor / 12);
  const rest = totalMinor - base * 12;
  return Array.from({ length: 12 }, (_, index) =>
    index < rest ? base + 1 : base,
  );
}

/** The twelve months added up, or null while one is not set. */
export function monthsTotal(months: readonly (number | null)[]): number | null {
  if (months.length !== 12 || months.some((month) => month === null))
    return null;
  return months.reduce<number>((sum, month) => sum + (month ?? 0), 0);
}

/** The period a reading covers, as its heading names it. */
export function periodLabel(
  period: Pick<SalesProgress["period"], "kind" | "year" | "month">,
  locale: Locale,
): string {
  return period.kind === "month" && period.month
    ? `${monthName(period.month, locale)} ${identifierNumber(period.year)}`
    : identifierNumber(period.year);
}
