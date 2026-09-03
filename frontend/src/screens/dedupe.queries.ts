import {
  type QueryKey,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { api } from "../api/client";
import { throwProblem } from "./common";

export const dedupeQueueKey = ["dedupe-candidates"];

/**
 * The open duplicate queue, in one spelling.
 *
 * Exported because the screen is no longer the only reader: chrome that reports
 * what is waiting on a person reads the same queue, and two queries against one
 * path are two answers that can disagree on screen.
 */
export function useDedupeQueue() {
  return useQuery({
    queryKey: dedupeQueueKey,
    queryFn: async () => {
      const { data, error } = await api.GET("/dedupe/candidates", {
        params: { query: { status: "open", limit: 50 } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}

/**
 * Deciding one pair: merge into a surviving record, or say they are not the
 * same and never be asked again.
 *
 * ONE writer for both answers, because the server has one: `merge` runs the
 * owning module's own merge server-side, so a client calling the merge endpoint
 * directly would be a second way to do the same thing with a different audit
 * trail.
 *
 * `winner_id` is REQUIRED for `merge` and refused for the other answer. The
 * caller passes it only with a merge, and an absent one serialises away — a
 * key whose value is undefined and a key that was never set are the same two
 * bytes on the wire, so the refusal cannot be tripped by a shape this function
 * can produce. What it CAN produce, and what the caller must not do, is pass a
 * winner alongside `not_a_duplicate`.
 *
 * Which cached reads to drop depends on where the pair was decided — the dedupe
 * screen and the Worklist both draw these — so the caller passes them in.
 */
export function useDedupeDisposition(invalidateKeys: readonly QueryKey[]) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      id: string;
      disposition: "merge" | "not_a_duplicate";
      winnerId?: string;
    }) => {
      const { error } = await api.POST("/dedupe/candidates/{id}/disposition", {
        params: { path: { id: input.id } },
        body: { disposition: input.disposition, winner_id: input.winnerId },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: () => {
      for (const queryKey of [...invalidateKeys, dedupeQueueKey]) {
        queryClient.invalidateQueries({ queryKey });
      }
    },
  });
}
