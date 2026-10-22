import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ifMatch, requireVersion } from "../../api/version";
import { refetchAfterAssignment } from "../../components/taskwritefollowup";
import { isEntityKind } from "../../lib/entity";
import { problemCodeOf, throwProblem, unwrap } from "../../lib/problem";
import { invalidateRecord } from "../../lib/recordwritekeys";
import type {
  ReopenCondition,
  TeamException,
  WorklistDisposition,
} from "./worklist.queries";
import { worklistKey } from "./worklist.reads";

// Hand one task to somebody else.
//
// The same endpoint the record surface calls. This page adds no rule of its own
// about who may be assigned what. It puts the existing verb where the reader is
// standing, and the server answers as it would from anywhere else.
export function useReassignTask() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      activityId: string;
      // The version the reassigning lead was looking at. Unpinned, two leads
      // can hand one task to different reps, and the second press overwrites
      // the first. Neither is told: the row sits on one rep's queue while the
      // other believes they delegated it.
      version: number | undefined;
      assigneeId: string;
    }) => {
      return unwrap(
        await api.PATCH("/activities/{id}", {
          params: {
            path: { id: input.activityId },
            ...ifMatch(requireVersion(input.version)),
          },
          body: { assignee_id: input.assigneeId },
        }),
      );
    },
    onSuccess: (activity, input) =>
      refetchAfterAssignment(queryClient, input.activityId, activity),
    onError: (error, input) => {
      if (problemCodeOf(error) === "version_skew") {
        return refetchAfterAssignment(queryClient, input.activityId, undefined);
      }
    },
  });
}

// Give one record a new owner, through the module that owns it.
//
// The exception's subject decides the endpoint, and this table is the whole
// dispatch. A generic worklist writer that set an owner across deals, leads and
// activities would be a second writer of a field five modules already own.
// Each of them audits and events its own writes. The plan's rule for takeover
// is that it reuses the source owner's operation rather than reaching past it.
//
// Exhaustive over AttentionSubject's own union, so a seventh subject type is a
// compile error here rather than a button that does nothing. The field differs
// on activities because a task is assigned rather than owned. The two words are
// the modules' own: collapsing them would write a field that does not exist,
// and fail at runtime on one subject type.
const OWNER_WRITE = {
  deal: { path: "/deals/{id}", field: "owner_id" },
  lead: { path: "/leads/{id}", field: "owner_id" },
  contact: { path: "/contacts/{id}", field: "owner_id" },
  company: { path: "/companies/{id}", field: "owner_id" },
  project: { path: "/projects/{id}", field: "owner_id" },
  activity: { path: "/activities/{id}", field: "assignee_id" },
} as const satisfies Record<
  NonNullable<TeamException["subject"]["type"]>,
  { path: string; field: "owner_id" | "assignee_id" }
>;

/**
 * Whether this subject can be handed to somebody at all.
 *
 * Every type in the union has an owner write today, so this is true for all of
 * them. It is asked rather than assumed: when a source has no owner write, the
 * control is absent with a reason rather than failing when pressed.
 */
export function subjectAcceptsAnOwner(
  subject: TeamException["subject"],
): boolean {
  return subject.type !== undefined && subject.type in OWNER_WRITE;
}

// Take one exception's record for yourself.
//
// Every queue is invalidated, not just the reader's. The record left somebody
// else's day and arrived in this one. Refetching only the caller's view would
// leave the other queue claiming work it no longer holds.
export function useTakeOwnership() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      subject: TeamException["subject"];
      userId: string;
    }) => {
      const write = input.subject.type && OWNER_WRITE[input.subject.type];
      if (!write) {
        // Not reachable through the control, which is drawn only where
        // subjectAcceptsAnOwner agrees. Thrown rather than returned so a future
        // caller that skips that check fails loudly instead of reporting a
        // handover that never happened.
        throw new Error(`no owner write for subject ${input.subject.type}`);
      }
      if (input.subject.type === "activity") {
        const current = unwrap(
          await api.GET("/activities/{id}", {
            params: { path: { id: input.subject.id } },
          }),
        );
        return unwrap(
          await api.PATCH("/activities/{id}", {
            params: {
              path: { id: input.subject.id },
              ...ifMatch(requireVersion(current.version)),
            },
            body: { assignee_id: input.userId },
          }),
        );
      }
      unwrap(
        await api.PATCH(write.path as "/deals/{id}", {
          params: { path: { id: input.subject.id } },
          body: { [write.field]: input.userId },
        }),
      );
    },
    onError: (error, input) => {
      if (
        input.subject.type === "activity" &&
        problemCodeOf(error) === "version_skew"
      ) {
        return refetchAfterAssignment(queryClient, input.subject.id, undefined);
      }
    },
    onSuccess: async (activity, input) => {
      if (input.subject.type === "activity") {
        await refetchAfterAssignment(queryClient, input.subject.id, activity);
        return;
      }
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: worklistKey }),
        input.subject.type && isEntityKind(input.subject.type)
          ? invalidateRecord(queryClient, input.subject.type, input.subject.id)
          : Promise.resolve(),
      ]);
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
      unwrap(
        await api.POST("/notices", {
          body: {
            recipient_user_id: input.recipientUserId,
            kind: input.kind,
            // An empty note is ABSENT rather than an empty string: the coach
            // added none, which the kind's own headline already covers.
            ...(input.note.trim() === "" ? {} : { note: input.note }),
          },
        }),
      );
    },
  });
}

// One staged proposal, whole.
//
// The queue sends a row's worth of each item: a sentence, a category, a
// reason. Deciding one needs the rest: the payload a reader may edit, who
// staged it, the evidence behind it. So the row being decided fetches the
// approval it shows, the same read the record page makes.
export function useApproval(id: string, enabled: boolean) {
  return useQuery({
    enabled,
    queryKey: ["approvals", "one", id],
    queryFn: async () => {
      return unwrap(
        await api.GET("/approvals/{id}", {
          params: { path: { id } },
        }),
      );
    },
  });
}

/**
 * Set a lapsed contact aside, and put them back.
 *
 * Thirty days, and the number lives here rather than being asked of the reader.
 * A duration picker on this verb would ask a rep to predict when a quiet
 * relationship becomes worth chasing again, which nobody knows. The contract
 * caps it at ninety because no value means forever. A relationship worth
 * raising once is worth raising again.
 */
export function useNudgeDismissal() {
  const queryClient = useQueryClient();
  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: worklistKey });
  };
  const dismiss = useMutation({
    mutationFn: async (input: { contactId: string }) => {
      unwrap(
        await api.PUT("/contacts/{id}/nudge-dismissal", {
          params: { path: { id: input.contactId } },
          body: { days: DISMISSAL_DAYS },
        }),
      );
    },
    onSuccess: invalidate,
  });
  const restore = useMutation({
    mutationFn: async (input: { contactId: string }) => {
      unwrap(
        await api.DELETE("/contacts/{id}/nudge-dismissal", {
          params: { path: { id: input.contactId } },
        }),
      );
    },
    onSuccess: invalidate,
  });
  return { dismiss, restore };
}

/**
 * How long a set-aside contact stays off the lane.
 *
 * Thirty rather than the contract's ninety-day ceiling. The cap is what the
 * server will accept, not what a rep means by "not now". A month is long enough
 * for the row to stop being noise. It is short enough that a relationship worth
 * keeping comes back while it still is one.
 */
const DISMISSAL_DAYS = 30;

// Put a waiting message down, or pick it back up.
//
// The Worklist claims to be finite: work it and it empties. A message the rep
// has already judged needs a way to leave, or it returns every morning and the
// count above it stays wrong.
//
// The queue is invalidated rather than edited in place, for the reason the
// reassign mutation states. The rank numbers and the summary above them are
// the server's. A row removed locally leaves both describing a page that is no
// longer on screen.
export function useSetDisposition() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      activityId: string;
      disposition: WorklistDisposition;
      snoozedUntil?: string;
      reopenOn?: ReopenCondition;
      reopenRef?: string;
    }) => {
      unwrap(
        await api.PUT("/activities/{id}/disposition", {
          params: { path: { id: input.activityId } },
          body: {
            disposition: input.disposition,
            // Only a snooze names a moment or a condition. Sending either on the
            // others is a 422, because a hand-off that expires on a Thursday is
            // not a hand-off.
            ...(input.snoozedUntil
              ? { snoozed_until: input.snoozedUntil }
              : {}),
            ...(input.reopenOn ? { reopen_on: input.reopenOn } : {}),
            ...(input.reopenRef ? { reopen_ref: input.reopenRef } : {}),
          },
        }),
      );
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: worklistKey });
    },
  });
}

// The undo behind every set-aside verb.
//
// `scope` decides the reach of the undo, and the default is the narrow one.
// Clearing your own snooze must not also re-admit a thread a colleague ruled
// out for the whole workspace.
export function useClearDisposition() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      activityId: string;
      scope: "mine" | "thread";
    }) => {
      unwrap(
        await api.DELETE("/activities/{id}/disposition", {
          params: {
            path: { id: input.activityId },
            query: { scope: input.scope },
          },
        }),
      );
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: worklistKey });
    },
  });
}

/**
 * The reader's own override: this row leads their day, whatever the ranking
 * chose.
 *
 * The row identity is the pair, not the id. The lanes mint ids independently,
 * so an id alone can name a row in a lane the reader was not looking at. The
 * pin store keys on the same pair for the same reason.
 */
export function usePinRow() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      source: string;
      rowId: string;
      pinned: boolean;
    }) => {
      const { error } = input.pinned
        ? await api.DELETE("/worklist/pins", {
            params: { query: { source: input.source, row_id: input.rowId } },
          })
        : await api.PUT("/worklist/pins", {
            body: { source: input.source, row_id: input.rowId },
          });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: () => {
      // The whole day, because a pin REORDERS it: the row moves to the top and
      // every row it passed moves down. Invalidating the one row would leave
      // the page holding an order the server no longer agrees with.
      queryClient.invalidateQueries({ queryKey: worklistKey });
    },
  });
}

/**
 * Start a new walk over today's rows.
 *
 * `refetch` is not this. An infinite query refetches every page it holds, each
 * with the cursor it was loaded under. Every page past the first carries the
 * snapshot. A reader who has paged once and presses the refresh this notice
 * offers would resume the walk they asked to leave. The work that arrived
 * behind them would stay absent, and the notice would keep asking for a refresh.
 *
 * Resetting drops the loaded pages, so the next fetch is a first page with no
 * cursor. The server then freezes a fresh snapshot over the day as it stands.
 */
export function useRefreshWalk() {
  const queryClient = useQueryClient();
  return () => {
    void queryClient.resetQueries({ queryKey: worklistKey });
  };
}
