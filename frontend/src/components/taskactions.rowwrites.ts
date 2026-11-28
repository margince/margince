import {
  type QueryKey,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch, requireVersion } from "../api/version";
import { useT } from "../i18n";
import { CLAIM_SETTLED_KEYS } from "../lib/activitykeys";
import { isVersionSkewOf, unwrap } from "../lib/problem";

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
 * Sending a bounced message again to a corrected address.
 *
 * The server replaces the bounced address on the contact and sends the same
 * message through the one send. A refusal throws with the server's own
 * sentence: consent withdrawn, a suppression, or a seat that may not send. The
 * reader sees it on the address field.
 */
export function useBounceSendAgain(invalidateKeys: readonly QueryKey[]) {
  const t = useT();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (send: { id: string; address: string }) =>
      unwrap(
        await api.POST("/attention/bounces/{id}/send-again", {
          params: { path: { id: send.id } },
          body: { address: send.address },
        }),
        t,
      ),
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
/** One answer to a meeting: its outcome, and the fields the writer changes. */
export type MeetingOutcomeWrite = {
  id: string;
  // The version the answer was decided against, as the task verb carries.
  // Without it, two readers answering one meeting both succeed, and the
  // later one wins with no warning.
  version: number | undefined;
  // `null` takes the status back off, which a meeting that never had one needs.
  status: "held" | "no_show" | "canceled" | "booked" | null;
  followUpTask?: components["schemas"]["ActivityFollowUpTask"];
  dealFiling?: components["schemas"]["ActivityDealFiling"];
  // Each sent only when given, so an answer alone never touches the notes.
  subject?: string;
  body?: string | null;
  meeting_notes?: string | null;
  // A model polished the notes this write carries.
  aiAssisted?: boolean;
  start?: Readonly<{
    occurred_at: string;
    occurred_precision: components["schemas"]["OccurredPrecision"];
  }>;
  // The custom-field values the writer changes, by their cf_ keys.
  custom?: Record<string, unknown>;
};

/**
 * The one write of a meeting's outcome, for the outcome hook, the outcome
 * dialog and the Log form.
 */
export async function writeMeetingOutcome(
  input: MeetingOutcomeWrite,
  t: ReturnType<typeof useT>,
): Promise<components["schemas"]["Activity"]> {
  return unwrap(
    await api.PATCH("/activities/{id}", {
      params: {
        path: { id: input.id },
        ...ifMatch(requireVersion(input.version)),
      },
      body: {
        meeting_status: input.status,
        ...(input.followUpTask ? { follow_up_task: input.followUpTask } : {}),
        ...(input.dealFiling ? { deal_filing: input.dealFiling } : {}),
        ...(input.subject === undefined ? {} : { subject: input.subject }),
        ...(input.body === undefined ? {} : { body: input.body }),
        ...(input.meeting_notes === undefined
          ? {}
          : { meeting_notes: input.meeting_notes }),
        ...(input.aiAssisted ? { ai_assisted: true } : {}),
        ...input.start,
        ...input.custom,
      },
    }),
    t,
  );
}

/** The status a meeting holds now, so an Undo can put it back. */
export async function readMeetingStatus(id: string): Promise<"booked" | null> {
  const { data } = await api.GET("/activities/{id}", {
    params: { path: { id } },
  });
  return data?.meeting_status === "booked" ? "booked" : null;
}

/**
 * The notes of a meeting whose outcome already stands. No `meeting_status`
 * goes with them, so writing the notes the next day never answers it again.
 */
export async function writeMeetingNotes(
  input: Readonly<{
    id: string;
    version: number | undefined;
    meeting_notes: string | null;
  }>,
  t: ReturnType<typeof useT>,
): Promise<components["schemas"]["Activity"]> {
  return unwrap(
    await api.PATCH("/activities/{id}", {
      params: {
        path: { id: input.id },
        ...ifMatch(requireVersion(input.version)),
      },
      body: { meeting_notes: input.meeting_notes ?? "" },
    }),
    t,
  );
}

export function useMeetingOutcome(invalidateKeys: readonly QueryKey[]) {
  const t = useT();
  const queryClient = useQueryClient();
  const reread = (id: string) => {
    for (const queryKey of invalidateKeys) {
      queryClient.invalidateQueries({ queryKey });
    }
    // The meeting's own detail read too: a drawer open on it would otherwise
    // keep showing the row as unanswered after it has been answered.
    queryClient.invalidateQueries({ queryKey: ["activity", id] });
  };
  return useMutation({
    mutationFn: (input: MeetingOutcomeWrite) => writeMeetingOutcome(input, t),
    onSuccess: (_data, input) => reread(input.id),
    // A retry must carry the version the meeting holds now, not the one refused.
    onError: (error, input) => {
      if (isVersionSkewOf(error)) reread(input.id);
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
  const rereadClaims = useClaimReread(invalidateKeys);
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
    onSuccess: rereadClaims,
  });
}

/**
 * Undoing Margince's own close of a promise. The server reopens only the claim,
 * the one thing the automatic close changed.
 */
export function useClaimReopen(invalidateKeys: readonly QueryKey[]) {
  const t = useT();
  const rereadClaims = useClaimReread(invalidateKeys);
  return useMutation({
    mutationFn: async (input: { id: string }) => {
      unwrap(
        await api.POST("/claims/{id}/reopen", {
          params: { path: { id: input.id } },
        }),
        t,
      );
    },
    onSuccess: rereadClaims,
  });
}

function useClaimReread(invalidateKeys: readonly QueryKey[]) {
  const queryClient = useQueryClient();
  return () => {
    for (const queryKey of [...invalidateKeys, ...CLAIM_SETTLED_KEYS]) {
      queryClient.invalidateQueries({ queryKey });
    }
  };
}
