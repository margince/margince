// The one place the app changes who may read a message.
//
// There are two writes because the backend has two, and they answer different
// questions. A captured message's audience is DERIVED from what every
// importing mailbox asks for, so it is changed by revising this owner's
// contribution to the thread; a hand-logged one carries an audience somebody
// set, so it is changed directly. The server says which applies — the caller
// passes the mode it was given rather than working it out from the row.
//
// Both live here because the thread write was spelled twice before: once in
// the timeline's action cluster and once in the held-threads queue, byte for
// byte, down to the sentence about what a share that changed nothing means.
// Two copies of one decision drift, and the drift shows up as a control that
// reports the wrong outcome.

import { useMutation, useQueryClient } from "@tanstack/react-query";
import type { QueryKey } from "@tanstack/react-query";

import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch, requireVersion } from "../api/version";
import { emailPresentationKey } from "./activitykeys";
import { throwProblem } from "./common";

type ThreadAudienceOutcome = components["schemas"]["ThreadAudienceOutcome"];
type ActivityAudience = components["schemas"]["ActivityAudience"];
type AudienceMember = components["schemas"]["AudienceMember"];

/** What a thread decision reached, for the caller that has to report it. */
export type ThreadAudienceResult = {
  threadKey: string;
  outcome: ThreadAudienceOutcome | null;
};

/**
 * useThreadAudience shares or holds back one THREAD, for a message a mailbox
 * brought in.
 *
 * The decision releases only the CALLER's hold. A thread two colleagues
 * imported opens when both allow it, so the outcome reports how many other
 * seats still hold it — a count and never a name, because whose mail a person
 * keeps private is itself private. A caller that does not say so renders a
 * control that looks broken when it in fact did exactly what it said.
 *
 * `invalidate` is the caller's, because what a decision invalidates depends on
 * where it was made: a timeline row refreshes its record's reads, the
 * held-threads queue refreshes the queue. The WRITE is the same either way,
 * which is why it is here and that is not.
 *
 * What every caller shares is the messages the decision actually reached — the
 * server names them — so those are refreshed here rather than left to each
 * caller to remember. A thread decision changes several messages filed against
 * several records, and refreshing only the one on screen leaves the rest
 * showing the audience they had before the press.
 */
export function useThreadAudience(options: {
  invalidate: (result: ThreadAudienceResult) => QueryKey[];
  onSettled?: (result: ThreadAudienceResult) => void;
}) {
  const queryClient = useQueryClient();
  return useMutation({
    // Both the thread and the decision arrive as variables, so the press
    // belongs to the render the reader saw rather than to whatever the
    // component last held (frontend/AGENTS.md, mutation-variable-coverage).
    mutationFn: async (variables: { threadKey: string; share: boolean }) => {
      const { data, error } = await api.POST(
        "/activities/threads/{thread_key}/audience",
        {
          params: { path: { thread_key: variables.threadKey } },
          body: { share: variables.share },
        },
      );
      if (error) throwProblem(error);
      return { threadKey: variables.threadKey, outcome: data ?? null };
    },
    onSuccess: (result) => {
      for (const queryKey of options.invalidate(result)) {
        queryClient.invalidateQueries({ queryKey });
      }
      for (const activityId of result.outcome?.activity_ids ?? []) {
        queryClient.invalidateQueries({
          queryKey: emailPresentationKey(activityId),
        });
      }
      options.onSettled?.(result);
    },
  });
}

/** The audience write for a message somebody logged by hand. */
export type MessageAudienceVariables = {
  activityId: string;
  version: number | undefined;
  audience: ActivityAudience;
  /** The full replacement set, read only when `audience` is `selected`. */
  members?: AudienceMember[];
};

/**
 * useMessageAudience limits (or re-opens) who may read ONE message.
 *
 * Per message on purpose: a thread is not a unit of trust, and the contact
 * stays visible to everyone either way. It carries If-Match, so a row whose
 * audience somebody else changed first refuses rather than overwriting a
 * decision this reader never saw.
 */
export function useMessageAudience(options: {
  invalidate: () => QueryKey[];
  onSettled?: () => void;
}) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (variables: MessageAudienceVariables) => {
      const { data, error } = await api.PATCH("/activities/{id}/audience", {
        params: {
          path: { id: variables.activityId },
          ...ifMatch(requireVersion(variables.version)),
        },
        body: variables.members
          ? { audience: variables.audience, members: variables.members }
          : { audience: variables.audience },
      });
      if (error) throwProblem(error);
      return data;
    },
    onSuccess: () => {
      for (const queryKey of options.invalidate()) {
        queryClient.invalidateQueries({ queryKey });
      }
      options.onSettled?.();
    },
  });
}
