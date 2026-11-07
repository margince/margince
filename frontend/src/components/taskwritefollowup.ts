import type {
  InfiniteData,
  QueryClient,
  QueryKey,
} from "@tanstack/react-query";
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

type Activity = components["schemas"]["Activity"];
type Worklist = components["schemas"]["Worklist"];
type WorklistItem = components["schemas"]["WorklistItem"];

export async function refreshMovedTask(
  client: QueryClient,
  invalidateKeys: readonly QueryKey[],
  activity: Activity,
) {
  const queryKey = ["activity", activity.id];
  await Promise.all([
    client.cancelQueries({ queryKey }),
    client.cancelQueries({ queryKey: worklistKey }),
  ]);
  client.setQueryData(queryKey, activity);
  const overdue = activity.due_at
    ? Date.parse(activity.due_at) < Date.now()
    : false;
  const moved = (item: WorklistItem): WorklistItem =>
    item.source === "task" && item.id === activity.id
      ? {
          ...item,
          due_at: activity.due_at ?? undefined,
          due_has_time: activity.due_has_time,
          version: activity.version,
          overdue,
          because: overdue
            ? item.because
            : item.because.filter((reason) => reason.kind !== "overdue"),
        }
      : item;
  client.setQueriesData<InfiniteData<Worklist>>(
    {
      predicate: (query) =>
        query.queryKey[0] === "worklist" && query.queryKey.length >= 4,
    },
    (day) =>
      day && Array.isArray(day.pages)
        ? {
            ...day,
            pages: day.pages.map((page) => ({
              ...page,
              queue: page.queue.map(moved),
              focus: page.focus
                ? { ...page.focus, items: page.focus.items.map(moved) }
                : page.focus,
            })),
          }
        : day,
  );
  await Promise.all([
    client.invalidateQueries({ queryKey }),
    ...invalidateKeys.map((key) => {
      const refresh = client.invalidateQueries({ queryKey: key });
      // Home rebuilds the whole day; record lists must land their new versions first.
      return key[0] === worklistKey[0] ? undefined : refresh;
    }),
  ]);
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
