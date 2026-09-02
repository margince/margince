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

// The forward half of the weekly. `home.queries.ts` reads the week that closed;
// this writes the week running.

export type WeeklyPlan = components["schemas"]["WeeklyPlan"];
export type WeeklyPlanCommitment =
  components["schemas"]["WeeklyPlanCommitment"];
export type NewWeeklyPlanCommitment =
  components["schemas"]["NewWeeklyPlanCommitment"];

/** What a rep may settle their own commitment to. `missed` is the close's word,
 *  not a rep's, so the contract does not offer it here and neither does this. */
export type SettableState = "open" | "done" | "dropped";

export const weeklyPlanKey = ["weekly-plan", "current"] as const;

/**
 * This week's plan, or null when the rep has not started one.
 *
 * 404 is not a failure: it is a week nobody has planned yet, and the screen
 * offers to open one. Reading it as an error would put a refusal plate over the
 * empty state that is the whole point of the first visit.
 */
export function useWeeklyPlan(): UseQueryResult<WeeklyPlan | null> {
  return useQuery({
    queryKey: weeklyPlanKey,
    queryFn: async (): Promise<WeeklyPlan | null> => {
      const { data, error, response } = await api.GET("/weekly-plans/current");
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
 * A teammate's week, for their lead.
 *
 * Enabled only when an owner is named, so the hook can sit unconditionally in a
 * component that does not always have one. A 404 here means either no plan or
 * no standing to ask — the server deliberately does not distinguish them, and
 * neither does this.
 */
export function useTeammateWeeklyPlan(
  ownerId: string | undefined,
): UseQueryResult<WeeklyPlan | null> {
  return useQuery({
    queryKey: ["weekly-plan", "teammate", ownerId],
    enabled: Boolean(ownerId),
    queryFn: async (): Promise<WeeklyPlan | null> => {
      const { data, error, response } = await api.GET(
        "/weekly-plans/{owner_id}/current",
        { params: { path: { owner_id: ownerId ?? "" } } },
      );
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

/** Open this week's plan. Idempotent on the server, so a second press answers
 *  the plan the first opened rather than reporting a race. */
export function useStartWeeklyPlan() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async () => {
      const { data, error } = await api.POST("/weekly-plans/current");
      if (error) {
        throwProblem(error);
      }
      return data ?? null;
    },
    onSuccess: (plan) => {
      queryClient.setQueryData(weeklyPlanKey, plan);
    },
  });
}

export function useAddCommitment() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (commitment: NewWeeklyPlanCommitment) => {
      const { data, error } = await api.POST("/weekly-plans/commitments", {
        body: commitment,
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: weeklyPlanKey });
    },
  });
}

/**
 * Settle one commitment.
 *
 * One call per commitment rather than one call carrying the whole staged set:
 * the endpoint settles a single id, so a batched write would have to be several
 * requests anyway, and doing it here keeps a refusal attributable to the row
 * that caused it.
 */
export function useSetCommitmentState() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      id,
      state,
    }: Readonly<{ id: string; state: SettableState }>) => {
      const { error } = await api.PUT("/weekly-plans/commitments/{id}/state", {
        params: { path: { id } },
        body: { state },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: weeklyPlanKey });
    },
  });
}

/** Say what you need from your lead, or withdraw a standing ask by sending an
 *  empty one — the server treats the empty string as the withdrawal. */
export function useAskForHelp() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      id,
      helpRequested,
    }: Readonly<{ id: string; helpRequested: string }>) => {
      const { error } = await api.PUT("/weekly-plans/commitments/{id}/help", {
        params: { path: { id } },
        body: { help_requested: helpRequested },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: weeklyPlanKey });
    },
  });
}

/**
 * The lead's answer to one request.
 *
 * It invalidates the teammate read rather than the caller's own plan: a lead
 * answering is looking at somebody else's week, and refreshing their own would
 * leave the row they just wrote to showing the answer it had before.
 */
export function useAnswerCommitment(ownerId: string | undefined) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      id,
      managerResponse,
    }: Readonly<{ id: string; managerResponse: string }>) => {
      const { error } = await api.PUT(
        "/weekly-plans/commitments/{id}/response",
        {
          params: { path: { id } },
          body: { manager_response: managerResponse },
        },
      );
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: ["weekly-plan", "teammate", ownerId],
      });
    },
  });
}
