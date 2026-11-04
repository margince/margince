// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type QueryKey,
  useIsMutating,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { api } from "../api/client";
import { ifMatch, requireVersion } from "../api/version";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { problemCodeOf, unwrap } from "../lib/problem";
import { worklistKey } from "../lib/workkeys";
import { useCompleteTask } from "./taskcomplete";
import { completeFailureKey, refetchAfterWrite } from "./taskwritefollowup";

type TaskPatch = {
  id: string;
  // The version the press was decided against. Every verb on a task goes
  // through this one mutation, so pinning it here pins all four at once — and
  // an unpinned tick is a task two contacts can complete, each told it worked.
  version: number | undefined;
  body: { is_done?: boolean; due_at?: string; remind_at?: string | null };
};

// Every task write carries this key, so any control can see a write in flight.
const TASK_WRITE = ["task-write"] as const;

export function useTaskUpdate(invalidateKeys: readonly QueryKey[]) {
  const t = useT();
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: TASK_WRITE,
    mutationFn: async (input: TaskPatch) => {
      const data = unwrap(
        await api.PATCH("/activities/{id}", {
          params: {
            path: { id: input.id },
            ...ifMatch(requireVersion(input.version)),
          },
          body: input.body,
        }),
        t,
      );
      // The version the write PRODUCED, answered so a follow-on press has one.
      // An undo re-sending the version the row was drawn at would be refused as
      // skew by the very write it is undoing.
      return data?.version;
    },
    // Returned, so the mutation stays pending until the reads have landed. The
    // row the press came from stays drawn at the version it was fetched at
    // until then, and a second press in that window re-sends it and is
    // refused as skew by the write that has just succeeded.
    onSuccess: (_data, input) =>
      refetchAfterWrite(queryClient, invalidateKeys, input.id),
    // A stale press is not a failed one: the task moved on under it, so the
    // reads are refreshed to show where it is now.
    onError: (error, input) => {
      if (problemCodeOf(error) === "version_skew") {
        return refetchAfterWrite(queryClient, invalidateKeys, input.id);
      }
    },
  });
}

/** A task's Done write, shared by every control that offers it. */
export function useTaskDone(id: string, version: number | undefined) {
  const t = useT();
  const toast = useToast();
  const update = useTaskUpdate([worklistKey]);
  // A rejected PATCH otherwise leaves the control idle with nothing on screen
  // to say so, and the reader has no reason to try again.
  const complete = useCompleteTask(update, (error) =>
    toast.show(t(completeFailureKey(error)), { tone: "danger" }),
  );
  // Two Done controls on one card send one write: the second press would
  // re-send the version the first one has just spent.
  const writing =
    useIsMutating({
      mutationKey: TASK_WRITE,
      predicate: (mutation) => writes(mutation.state.variables, id),
    }) > 0;
  return {
    pending: writing,
    complete: () => (writing ? Promise.resolve() : complete(id, version)),
  };
}

function writes(variables: unknown, id: string): boolean {
  return (
    typeof variables === "object" &&
    variables !== null &&
    "id" in variables &&
    variables.id === id
  );
}
