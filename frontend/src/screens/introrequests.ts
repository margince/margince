// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The asks about one contact: reading them, making one, and answering one.
//
// Every write sends the version it read and takes the row back, so the next
// write on the same screen carries a version the server just minted rather
// than the one this tab loaded. A surface that wrote from its loaded version
// twice would earn a conflict on its own second click.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./common";

export type IntroRequest = components["schemas"]["IntroRequest"];
export type IntroRequestInput = components["schemas"]["IntroRequestInput"];
export type IntroDecision =
  components["schemas"]["IntroRequestDecisionInput"]["decision"];

/** The cached read of one contact's asks. */
export function introRequestsKey(personId: string) {
  return ["introRequests", personId] as const;
}

/** useIntroRequests reads the asks about this contact the viewer is party to. */
export function useIntroRequests(personId: string, enabled = true) {
  return useQuery({
    queryKey: introRequestsKey(personId),
    enabled,
    queryFn: async () => {
      const { data, error } = await api.GET("/people/{id}/intro-requests", {
        params: { path: { id: personId } },
      });
      if (error) {
        throwProblem(error);
      }
      return data?.data ?? [];
    },
  });
}

/**
 * useCreateIntroRequest asks a colleague to open a door.
 *
 * The whole body is the mutation's variable rather than read off a closure: the
 * drawer's route can change while a request is in flight, and a closure would
 * send the reason for one route with the colleague from another.
 */
export function useCreateIntroRequest(personId: string) {
  const invalidate = useAskInvalidation(personId);
  return useMutation({
    mutationKey: ["introRequest.create", personId],
    mutationFn: async (body: IntroRequestInput) => {
      const { data, error } = await api.POST("/people/{id}/intro-requests", {
        params: { path: { id: personId } },
        body,
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: invalidate,
  });
}

/** The colleague's answer, with the version they read it at. */
export type DecisionVariables = Readonly<{
  id: string;
  decision: IntroDecision;
  version: number;
  reason?: string;
  suggestedUserId?: string;
}>;

/** useDecideIntroRequest records one of the four answers. */
export function useDecideIntroRequest(personId: string) {
  const invalidate = useAskInvalidation(personId);
  return useMutation({
    mutationKey: ["introRequest.decide", personId],
    mutationFn: async (v: DecisionVariables) => {
      const { data, error } = await api.POST("/intro-requests/{id}/decision", {
        params: { path: { id: v.id } },
        body: {
          decision: v.decision,
          version: v.version,
          ...(v.reason ? { reason: v.reason } : {}),
          ...(v.suggestedUserId
            ? { suggested_user_id: v.suggestedUserId }
            : {}),
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: invalidate,
  });
}

/**
 * useCompleteIntroRequest records what came of the ask.
 *
 * There is no field for WHICH outcome: the server reads that from the ask's own
 * state, so an accepted ask completes as an introduction and a name-drop-approved
 * one as a name-drop. This surface cannot claim a handshake that did not happen
 * even if it wanted to.
 */
export function useCompleteIntroRequest(personId: string) {
  const invalidate = useAskInvalidation(personId);
  return useMutation({
    mutationKey: ["introRequest.complete", personId],
    mutationFn: async (
      v: Readonly<{ id: string; version: number; sourceActivityId?: string }>,
    ) => {
      const { data, error } = await api.POST("/intro-requests/{id}/complete", {
        params: { path: { id: v.id } },
        body: {
          version: v.version,
          ...(v.sourceActivityId
            ? { source_activity_id: v.sourceActivityId }
            : {}),
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: invalidate,
  });
}

/** useCancelIntroRequest withdraws an ask the viewer made. */
export function useCancelIntroRequest(personId: string) {
  const invalidate = useAskInvalidation(personId);
  return useMutation({
    mutationKey: ["introRequest.cancel", personId],
    mutationFn: async (
      v: Readonly<{ id: string; version: number; reason?: string }>,
    ) => {
      const { data, error } = await api.POST("/intro-requests/{id}/cancel", {
        params: { path: { id: v.id } },
        body: { version: v.version, ...(v.reason ? { reason: v.reason } : {}) },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: invalidate,
  });
}

/**
 * useAskInvalidation refreshes what a written ask makes stale.
 *
 * The graph as well as the list: a route's `availability` is derived from
 * whether an open ask exists, so a tab that refreshed only the list would keep
 * offering "Ask" on a route it had just asked for.
 */
function useAskInvalidation(personId: string) {
  const qc = useQueryClient();
  return () => {
    void qc.invalidateQueries({ queryKey: introRequestsKey(personId) });
    void qc.invalidateQueries({ queryKey: ["person-graph", personId] });
  };
}
