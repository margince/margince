// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery } from "@tanstack/react-query";
import { useRef, useState } from "react";
import type { components } from "../../api/schema";
import { problemFieldErrorsOf } from "../../lib/problem";
import { refusalOf } from "./composerefusal";
import type { ComposeSend } from "./composesend";
import { refusalIsCurrent } from "./composesendrequest";
import { fetchReview } from "./directsendaction";
import { type DirectSendCommand, directSend } from "./directsendmodal";
import {
  embeddedReviewOf,
  type SendReviewAction,
  sendReviewOf,
} from "./sendreview";

type CommunicationReview = components["schemas"]["CommunicationReview"];

/**
 * The composer's short warning. A decision may name only a version the review
 * was served with. This build knows the words of this one alone, so any other
 * version leaves the finding with no Send anyway.
 */
export const INLINE_WARNING_VERSION = "inline-v1";

// Review states with nothing left to decide; the server refuses a decision on them.
const SETTLED_STATES = new Set(["resolved", "superseded", "cancelled"]);

type Reviewed = Readonly<{
  review: CommunicationReview;
  actions: readonly SendReviewAction[];
}>;

// A newer review a refused decision points at: embedded or named by id. A
// stale decision reads the same review again.
async function refreshedReview(
  error: unknown,
  current: Reviewed,
): Promise<Reviewed | null> {
  const embedded = embeddedReviewOf(error);
  const named = sendReviewOf(error);
  if (embedded) return { review: embedded, actions: named?.actions ?? [] };
  const stale = problemFieldErrorsOf(error).some(
    ({ code }) => code === "instruction_stale",
  );
  const id = named?.reviewId ?? (stale ? current.review.id : undefined);
  if (!id) return null;
  try {
    const review = await fetchReview(id);
    return { review, actions: named?.actions ?? current.actions };
  } catch {
    return null;
  }
}

type Decision = Readonly<{ from: unknown; command: DirectSendCommand }>;

// Send anyway is drawn only where the server offers this caller the decision,
// serves the composer's warning, and still waits for one.
function mayDecideInline(reviewed: Reviewed | null): boolean {
  if (!reviewed) return false;
  const { review, actions } = reviewed;
  return (
    actions.includes("direct_send") &&
    !SETTLED_STATES.has(review.state) &&
    (review.warnings ?? []).some(
      (warning) => warning.version === INLINE_WARNING_VERSION,
    )
  );
}

// What a refused Send names: its review, embedded or by id. Nothing for any
// other failure.
function refusedReview(error: unknown) {
  const consent = refusalOf(error) === "consent";
  const named = consent ? sendReviewOf(error) : null;
  const embedded = consent ? embeddedReviewOf(error) : null;
  return {
    consent,
    named,
    embedded,
    reviewId: embedded?.id ?? named?.reviewId,
  };
}

/**
 * The review a refused Send left behind, while it still describes the message
 * on screen, and the one decision the composer may make on it.
 *
 * Once any part of the message changed since the refused Send, the review
 * detaches, and the next press is a normal Send.
 */
export function useInlineReview(send: ComposeSend) {
  // The refused Send's error, whose identity marks one attempt.
  const { error } = send.send;
  const current = refusalIsCurrent(
    send.send.variables,
    send.request,
    undefined,
    true,
  );
  const { consent, named, embedded, reviewId } = refusedReview(error);
  const shown = consent && current;
  // An older server names the review without embedding it: one scoped read.
  const fetched = useQuery({
    queryKey: ["communication-review", reviewId ?? ""],
    queryFn: () => fetchReview(reviewId ?? ""),
    enabled: shown && embedded === null && reviewId !== undefined,
    retry: false,
    staleTime: Number.POSITIVE_INFINITY,
  });
  const [refreshed, setRefreshed] = useState<
    (Reviewed & { from: unknown }) | null
  >(null);
  const inHand = embedded ?? fetched.data ?? null;
  const reviewed: Reviewed | null =
    refreshed?.from === error
      ? refreshed
      : inHand && { review: inHand, actions: named?.actions ?? [] };
  const deciding = useRef(false);
  const decide = useMutation({
    mutationFn: ({ command }: Decision) => directSend(command),
    onSuccess: send.directSent,
    onError: async (failure, { from }) => {
      if (!reviewed) return;
      const next = await refreshedReview(failure, reviewed);
      if (next) setRefreshed({ ...next, from });
    },
    onSettled: () => {
      deciding.current = false;
    },
  });
  const review = reviewed?.review ?? null;
  const canSendAnyway = mayDecideInline(reviewed);
  const ownDecision = decide.variables?.from === error;
  const confirm = (explanation: string) => {
    if (!review || !canSendAnyway || deciding.current) return;
    deciding.current = true;
    decide.mutate({
      from: error,
      command: {
        reviewId: review.id,
        reasonCode: "other",
        explanation: explanation.trim(),
        warningVersion: INLINE_WARNING_VERSION,
        assessmentRevision: review.assessment_revision,
        shownReasons: review.refusals.map((refusal) => refusal.reason_code),
      },
    });
  };
  return {
    /** The send's own refusal outside the review, such as a mailbox fault. */
    refusal: send.refusal,
    shown,
    review,
    /** A refusal that opened no review this build can read. */
    unreviewed: shown && reviewId === undefined,
    loading: shown && fetched.isFetching,
    loadFailed: shown && embedded === null && fetched.isError,
    retryLoad: () => {
      void fetched.refetch();
    },
    canSendAnyway,
    confirm,
    pending: decide.isPending,
    /** The message is on its way: the Send or the Send anyway decision. It
     *  holds the draft, the words and close as a Send does. */
    sending: send.send.isPending || decide.isPending,
    changed: refreshed?.from === error,
    failed: ownDecision && decide.isError && refreshed?.from !== error,
  };
}

export type InlineReview = ReturnType<typeof useInlineReview>;

/**
 * The reason typed for Send anyway, which the saved draft keeps. A composer
 * that closes drops it, so the next composition starts empty.
 */
export function useOverrideExplanation(open: boolean) {
  const [text, setText] = useState("");
  const [wasOpen, setWasOpen] = useState(open);
  if (wasOpen !== open) {
    setWasOpen(open);
    if (!open) setText("");
  }
  return { text, setText };
}
