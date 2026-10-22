import {
  type QueryKey,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { api } from "../api/client";
import { ifMatch, requireVersion } from "../api/version";
import { useT } from "../i18n";
import { CLAIM_SETTLED_KEYS } from "../lib/activitykeys";
import { unwrap } from "../lib/problem";

// useNoticeRead settles one notice, which is all the acknowledge verb means.
// It is apart from useTaskUpdate because the two have different owners. A
// task's verbs go to the activity; a notice's one verb goes to its read endpoint.
export function useNoticeRead(invalidateKeys: readonly QueryKey[]) {
  const t = useT();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => {
      unwrap(
        await api.POST("/notices/{id}/read", {
          params: { path: { id } },
        }),
        t,
      );
    },
    onSuccess: () => {
      for (const queryKey of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey });
      }
    },
  });
}

/**
 * Running one failed automation firing again.
 *
 * It resolves with the server's answer instead of throwing on a refusal. A
 * refusal is a fact about the run, not a fault. The firing was blocked,
 * its handler is not known safe to repeat, or its trigger event cannot be
 * rebuilt. The reader is told which, and an error path would have to rebuild it.
 */
export function useAutomationRetry(invalidateKeys: readonly QueryKey[]) {
  const t = useT();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => {
      return unwrap(
        await api.POST("/automations/runs/{id}/retry", {
          params: { path: { id } },
        }),
        t,
      );
    },
    onSuccess: () => {
      for (const queryKey of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey });
      }
    },
  });
}

/**
 * Recording how a meeting went.
 *
 * Its own hook, not a wider `useTaskUpdate`, whose body is task-shaped (is_done,
 * due_at, remind_at). A meeting outcome is a different fact about another kind
 * of activity. Widening it would let a caller send `is_done` for a meeting. The
 * server refuses that with `field_not_valid_for_kind`, and the types prevent it.
 */
export function useMeetingOutcome(invalidateKeys: readonly QueryKey[]) {
  const t = useT();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      id: string;
      // The version the answer was decided against, as the task verb carries.
      // Without it, two readers answering one meeting both succeed, and the
      // later one wins with no warning.
      version: number | undefined;
      status: "held" | "no_show" | "canceled";
    }) => {
      unwrap(
        await api.PATCH("/activities/{id}", {
          params: {
            path: { id: input.id },
            ...ifMatch(requireVersion(input.version)),
          },
          body: { meeting_status: input.status },
        }),
        t,
      );
    },
    onSuccess: (_data, input) => {
      for (const queryKey of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey });
      }
      // The meeting's own detail read too: a drawer open on it would otherwise
      // keep showing the row as unanswered after it has been answered.
      queryClient.invalidateQueries({ queryKey: ["activity", input.id] });
    },
  });
}

/**
 * Settling a promise: it was kept, or it no longer stands.
 *
 * Its own hook, not useTaskUpdate's: a claim is not an activity, and the two
 * settle through different endpoints. The server keeps `done` and `dismissed`
 * apart because they answer different questions later. How many commitments a
 * workspace keeps is a fact about the team. How many extracted claims were never
 * real is a fact about the extractor.
 */
export function useClaimSettle(invalidateKeys: readonly QueryKey[]) {
  const t = useT();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      id: string;
      outcome: "done" | "dismissed";
    }) => {
      unwrap(
        await api.POST("/claims/{id}/settle", {
          params: { path: { id: input.id } },
          body: { outcome: input.outcome },
        }),
        t,
      );
    },
    onSuccess: () => {
      for (const queryKey of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey });
      }
      for (const queryKey of CLAIM_SETTLED_KEYS) {
        queryClient.invalidateQueries({ queryKey });
      }
    },
  });
}
