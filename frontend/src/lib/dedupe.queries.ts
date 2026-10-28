// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type QueryClient,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { api } from "../api/client";
import { problemCodeOf, problemStatus, unwrap } from "./problem";
import { contactCasesKey, worklistKey } from "./workkeys";

export type PairDisposition = "merge" | "not_a_duplicate";

// A decided or re-opened pair re-shapes the Worklist rows and every contact
// case read off the pairs, so both reads go stale together.
function invalidatePairReads(client: QueryClient): Promise<unknown> {
  return Promise.all([
    client.invalidateQueries({ queryKey: worklistKey }),
    client.invalidateQueries({ queryKey: contactCasesKey }),
  ]);
}

/**
 * Deciding one pair: merge into a surviving record, or say they are not the
 * same and never be asked again.
 *
 * One writer for both answers, because the server has one. `merge` runs the
 * owning module's own merge server-side. A client calling the merge endpoint
 * directly would be a second way to do the same thing with a different audit
 * trail.
 *
 * `winner_id` is REQUIRED for `merge` and refused for the other answer. The
 * caller passes it only with a merge, and an absent one serialises away.
 */
export function useDedupeDisposition() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      id: string;
      disposition: PairDisposition;
      winnerId?: string;
    }) => {
      unwrap(
        await api.POST("/dedupe/candidates/{id}/disposition", {
          params: { path: { id: input.id } },
          body: { disposition: input.disposition, winner_id: input.winnerId },
        }),
      );
    },
    onSuccess: () => invalidatePairReads(queryClient),
  });
}

/** Re-opening a dismissed pair, so it is asked about again. */
export function useDedupeUndo() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { id: string }) => {
      unwrap(
        await api.POST("/dedupe/candidates/{id}/undo", {
          params: { path: { id: input.id } },
        }),
      );
    },
    onSuccess: () => invalidatePairReads(queryClient),
  });
}

/**
 * Why a pair decision or undo failed, as the three outcomes a reader acts on.
 * A refusal refuses again however often it is pressed, so the reader needs
 * someone with more authority. A settled pair was decided first by someone
 * else, so the reader needs a fresh read. Only `failed` is worth a retry.
 */
export type PairDecisionFailure = "refused" | "settled" | "failed";

export function pairDecisionFailure(error: unknown): PairDecisionFailure {
  const code = problemCodeOf(error);
  if (code === "permission_denied" || problemStatus(error) === 403) {
    return "refused";
  }
  if (code === "already_disposed" || code === "not_undoable") return "settled";
  return "failed";
}
