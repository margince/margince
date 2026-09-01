import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./common";

export type Worklist = components["schemas"]["Worklist"];
export type WorklistItem = components["schemas"]["WorklistItem"];
export type WorklistReason = components["schemas"]["WorklistReason"];
export type WorklistComparison = components["schemas"]["WorklistComparison"];
export type WorklistValue = components["schemas"]["WorklistValue"];
// Derived from the contract's own enums rather than spelled again here: a
// hand-written union goes stale the moment the server adds a value, and it goes
// stale silently — nothing compares the two.
export type WorklistScope = Worklist["scope"];
export type WorklistFilter = NonNullable<Worklist["filter"]>;
export type WorklistCategory = WorklistItem["category"];
// The ways a row can be put down, derived from the item rather than spelled
// again: the contract declares them inline, and a hand-written union would go
// stale the moment the server gained a fourth — silently, because nothing
// compares the two.
export type WorklistDisposition = NonNullable<
  WorklistItem["dispositions"]
>[number];

export const worklistKey = ["worklist"] as const;

// The whole day in one read, at one scope or one named owner, and one filter.
//
// The key carries every dial, so switching any of them is a different query
// rather than a refetch of the same one — which is what lets the reader move
// back to a view they have already loaded without watching it reassemble.
//
// A named owner narrows the day to one person's queue and OUTRANKS the scope
// word: "their queue" is a narrower question than any of mine/team/all, and the
// server answers 422 for the pair rather than guessing which was meant. So the
// scope travels only when nobody is named.
export function useWorklist(
  scope: WorklistScope,
  filter: WorklistFilter,
  owner?: string,
) {
  return useQuery({
    queryKey: [...worklistKey, scope, filter, owner ?? ""],
    refetchOnWindowFocus: true,
    queryFn: async (): Promise<Worklist> => {
      const { data, error } = await api.GET("/worklist", {
        params: {
          query: owner ? { owner, filter } : { scope, filter },
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}

// Hand one task to somebody else.
//
// The SAME endpoint the record surface calls. This page adds no rule of its own
// about who may be assigned what — it puts the existing verb where the reader
// is standing, and the server answers exactly as it would from anywhere else.
export function useReassignTask() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { activityId: string; assigneeId: string }) => {
      const { error } = await api.PATCH("/activities/{id}", {
        params: { path: { id: input.activityId } },
        body: { assignee_id: input.assigneeId },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: (_data, input) => {
      // Every scope and owner of this queue, because the task left one
      // person's day and arrived in another's: refetching only the view in
      // front of the reader would leave the receiving queue stale until
      // something else happened to invalidate it.
      queryClient.invalidateQueries({ queryKey: worklistKey });
      queryClient.invalidateQueries({
        queryKey: ["activity", input.activityId],
      });
    },
  });
}

// Leave a note on a teammate's queue.
//
// Nothing is invalidated here: the notice lands in the RECIPIENT's day, which
// is not a view this reader is holding. Their own page is unchanged, and
// pretending otherwise by refetching it would only make the press feel like it
// had done something local.
export function useCoachTeammate() {
  return useMutation({
    mutationFn: async (input: {
      recipientUserId: string;
      kind: components["schemas"]["NoticeKind"];
      note: string;
    }) => {
      const { error } = await api.POST("/notices", {
        body: {
          recipient_user_id: input.recipientUserId,
          kind: input.kind,
          // An empty note is ABSENT rather than an empty string: the coach
          // added none, which the kind's own headline already covers.
          ...(input.note.trim() === "" ? {} : { note: input.note }),
        },
      });
      if (error) {
        throwProblem(error);
      }
    },
  });
}

// One staged proposal, whole.
//
// The queue sends a ROW's worth of each item — a sentence, a category, a
// reason. Deciding one needs the rest: the payload a reader may edit, who
// staged it, the evidence behind it. So the row that is being decided fetches
// the approval it is showing, which is the same read the record page makes.
export function useApproval(id: string, enabled: boolean) {
  return useQuery({
    enabled,
    queryKey: ["approvals", "one", id],
    queryFn: async () => {
      const { data, error } = await api.GET("/approvals/{id}", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}

// Put a waiting message down, or pick it back up.
//
// The Worklist claims to be finite: work it and it empties. A message the rep
// has already judged had no way to leave, so it returned every morning and the
// count above it stayed wrong.
//
// The queue is invalidated rather than edited in place, for the reason the
// reassign mutation states: the rank numbers and the summary above them are the
// server's, and a row removed locally leaves both describing a page that is no
// longer on screen.
export function useSetDisposition() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      activityId: string;
      disposition: WorklistDisposition;
      snoozedUntil?: string;
    }) => {
      const { error } = await api.PUT("/activities/{id}/disposition", {
        params: { path: { id: input.activityId } },
        body: {
          disposition: input.disposition,
          // Only a snooze names a moment. Sending one on the others is a 422,
          // because a hand-off that expires on a Thursday is not a hand-off.
          ...(input.snoozedUntil ? { snoozed_until: input.snoozedUntil } : {}),
        },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: worklistKey });
    },
  });
}

// The undo behind every set-aside verb.
//
// `scope` decides the REACH of the undo, and the default is deliberately the
// narrow one: clearing your own snooze must not also re-admit a thread a
// colleague ruled out for the whole workspace.
export function useClearDisposition() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      activityId: string;
      scope: "mine" | "thread";
    }) => {
      const { error } = await api.DELETE("/activities/{id}/disposition", {
        params: {
          path: { id: input.activityId },
          query: { scope: input.scope },
        },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: worklistKey });
    },
  });
}
