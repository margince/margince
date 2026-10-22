import type { QueryClient, QueryKey } from "@tanstack/react-query";
import type { components } from "../api/schema";
import { showsAMessage, TASK_QUEUE_KEY } from "../lib/activitykeys";
import { ENTITY_KINDS } from "../lib/entity";
import { problemCodeOf } from "../lib/problem";
import { recordWriteKeys } from "../lib/recordwritekeys";
import { worklistKey } from "../lib/workkeys";

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

export function refetchAfterAssignment(
  client: QueryClient,
  taskId: string,
  activity: components["schemas"]["Activity"] | undefined,
) {
  // Conflicts carry no links, and may follow a link or audience change.
  // Derive the broader record families from the same key catalog.
  const unknownId = "\u0000unknown-record";
  const linked = activity
    ? (activity.links ?? []).flatMap((link) =>
        recordWriteKeys(link.entity_type, link.entity_id),
      )
    : ENTITY_KINDS.flatMap((kind) =>
        recordWriteKeys(kind, unknownId).map((key) => {
          const idAt = key.indexOf(unknownId);
          return idAt < 0 ? key : key.slice(0, idAt);
        }),
      );
  return Promise.all([
    refetchAfterWrite(client, [worklistKey, TASK_QUEUE_KEY, ...linked], taskId),
    // A contact's employer can also show this task without being a direct link.
    client.invalidateQueries({ predicate: showsAMessage }),
  ]);
}
