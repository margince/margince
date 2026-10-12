import type { QueryClient, QueryKey } from "@tanstack/react-query";
import { problemCodeOf } from "./common";

// The task's own detail read is refreshed too, always: a modal open on the task
// that was just changed would otherwise keep showing the old due date and
// offering the verbs that no longer apply.
export function refetchAfterWrite(
  queryClient: QueryClient,
  invalidateKeys: readonly QueryKey[],
  taskId: string,
) {
  return Promise.all(
    [...invalidateKeys, ["activity", taskId]].map((queryKey) =>
      queryClient.invalidateQueries({ queryKey }),
    ),
  );
}

// What a refused Done says. A stale press means the task moved on under it,
// which is not the same as it not being completed.
export function completeFailureKey(
  error: unknown,
): "worklist.verb.completeStale" | "worklist.verb.completeFailed" {
  return problemCodeOf(error) === "version_skew"
    ? "worklist.verb.completeStale"
    : "worklist.verb.completeFailed";
}
