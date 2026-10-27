// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { useT } from "../../i18n";
import {
  problemCodeOf,
  problemMessageOf,
  throwProblem,
} from "../../lib/problem";
import type { ComposeThread } from "./composeanchor";
import type { DraftAsk, FollowUp } from "./composedraftcall";

import { fillFromDraft, keptTag, useDraftMutation } from "./composedraftcall";
import type { DraftNeed, PendingAction } from "./composedraftcontext";
import type { ComposeFields } from "./composefields";
import type { RelinkKind } from "./composerelink";

type Activity = components["schemas"]["Activity"];
type Rejection = { profileId: string; draftRef: string };

// What rejecting a draft needs: the reference to name and the profile that
// served it. Non-null when the judgment has a subject.
function rejectionTarget(
  draftRef: string | null,
  voiceProfileId: string | null,
): Rejection | null {
  if (draftRef === null || voiceProfileId === null) {
    return null;
  }
  return { profileId: voiceProfileId, draftRef };
}

// Whether a precondition the reader could meet holds the draft button back.
// A draft in flight keeps the button live with a turning mark.
function draftBlocked(
  state: Readonly<{
    busy: boolean;
    answering: string | undefined;
    intent: string;
    anchorActivity: Activity | undefined;
    needsRecipient: boolean;
  }>,
): boolean {
  if (state.busy) {
    return true;
  }
  if (!state.answering) {
    return !state.intent.trim() || state.needsRecipient;
  }
  return (
    !state.anchorActivity ||
    state.anchorActivity.content_state === "withheld" ||
    state.needsRecipient
  );
}

// An account draft is written for one of the account's contacts, so it waits
// for one in To. An account with none has nobody to write it for.
function draftNeed(
  groundable: boolean,
  recipient: Readonly<{ recipientId: string; noContacts: boolean }>,
): DraftNeed {
  if (!groundable || recipient.recipientId) {
    return null;
  }
  return recipient.noContacts ? "contacts" : "recipient";
}

// A 404 on a reply is the anchor being gone: the activity read still answers
// 200 for an archived row. Without an anchor, a 404 means the record is gone.
function draftErrorOf(
  draft: Readonly<{ isError: boolean; error: unknown }>,
  anchorFailed: boolean,
  answering: string | undefined,
  t: ReturnType<typeof useT>,
): string | null {
  if (anchorFailed) {
    return t("compose.threadFailed");
  }
  if (!draft.isError) {
    return null;
  }
  if (answering && problemCodeOf(draft.error) === "not_found") {
    return t("compose.anchorGone");
  }
  return problemMessageOf(draft.error, t);
}

// The voice draft: asking the model for words, rewriting them, and rejecting
// a served draft so the voice profile learns it missed.
export function useVoiceDraft({
  open,
  fields,
  answering,
  entityType,
  entityId,
  followUp,
  projectId,
  recipient,
  groundable,
  thread,
  bodyId,
  voiceProfileId,
  sendPending,
}: Readonly<{
  open: boolean;
  fields: ComposeFields;
  answering: string | undefined;
  entityType: RelinkKind;
  entityId: string;
  followUp?: FollowUp;
  /** The project the picker holds, which grounds the draft. */
  projectId: string;
  /** Whom an account draft is written for: the account contact first in To. */
  recipient: Readonly<{ recipientId: string; noContacts: boolean }>;
  groundable: boolean;
  thread: ComposeThread;
  bodyId: string;
  voiceProfileId: string | null;
  sendPending: boolean;
}>) {
  const t = useT();
  const { account, draftEpoch, body, html, intent } = fields;
  const { recipientId } = recipient;

  // A draft raises the provenance band above the body and pushes the words
  // past the fold. After the paint, the body scrolls back into view.
  // "nearest" moves nothing when the body already fits, as on a rewrite.
  const revealDraftedBody = () => {
    globalThis.requestAnimationFrame(() => {
      // jsdom has no scrollIntoView; the browser has one.
      document.getElementById(bodyId)?.scrollIntoView?.({ block: "nearest" });
    });
  };

  const draft = useDraftMutation({
    entityId,
    onUnavailable: (reason) => fields.setDraftUnavailable(reason),
    onDrafted: (result, ask) => {
      const written = fillFromDraft(result, {
        subject: fields.subject,
        body,
        toEmpty: fields.to.length === 0,
        rewrite: ask.instruction !== undefined || result.revised === true,
        setSubject: fields.setSubject,
        setBody: fields.adoptDraftedBody,
        setServedBody: fields.setServedBody,
        setTo: fields.setTo,
        setDraftRef: fields.setDraftRef,
        setProvenance: fields.setProvenance,
        setReasoning: account.setReasoning,
        setScope: account.setScope,
      });
      // The rep's words stood in the way, so say the new draft was dropped.
      fields.setDraftKept(!written);
      revealDraftedBody();
    },
    resetUnavailable: () => {
      fields.setDraftUnavailable(null);
      fields.setDraftKept(false);
    },
    onSkipped: (ask) => {
      if (open && ask.activityId === answering && ask.entityId === entityId)
        fields.setDraftKept(true);
    },
    isCurrent: (ask) =>
      open &&
      ask.epoch === draftEpoch.current &&
      ask.body === body &&
      ask.html === html &&
      ask.intent === intent &&
      ask.activityId === answering &&
      ask.entityId === entityId &&
      ask.entityType === entityType &&
      ask.grounding.recipientId === recipientId &&
      ask.grounding.dealId === account.dealId &&
      ask.grounding.projectId === projectId,
    t,
  });

  // Rejecting is a judgment with its own control, never a side effect of
  // closing. The reference is deterministic, so a guessed rejection would
  // stand in for a later identical draft that is sent.
  const rejectable = rejectionTarget(fields.draftRef, voiceProfileId);
  const discard = useMutation({
    mutationFn: async (rejected: Rejection) => {
      const { error, response } = await api.POST(
        "/voice-profiles/{id}/draft-rejections",
        {
          params: { path: { id: rejected.profileId } },
          body: { draft_ref: rejected.draftRef },
        },
      );
      // Only a real 2xx landed. A bodiless gateway 5xx has a falsy `error`,
      // and the server's signal would still be open.
      if (!response.ok) {
        throwProblem(error || { title: t("compose.actionFailed") });
      }
    },
    onMutate: () => {
      // A rejection and a send are rival verdicts on one draft. Dropping the
      // reference now means a racing send carries no draft at all.
      fields.setDraftRef(null);
    },
    onError: (_error, rejected) => {
      // The signal is still open, so the reference returns and stays retryable.
      fields.setDraftRef(rejected.draftRef);
    },
    onSuccess: () => {
      // The rejected words go; the recipients and the project tag are the
      // rep's and stay.
      fields.setSubject((current) => keptTag(current));
      fields.clearRejected();
    },
  });

  // While a rejection is in flight, nothing else may act on its draft. The
  // text stays frozen, so a returned reference names the words on screen.
  const rejectionInFlight = discard.isPending;
  const ask = (instruction?: string): DraftAsk => ({
    activityId: answering,
    entityType,
    entityId,
    followUp,
    intent,
    epoch: draftEpoch.current,
    body,
    html,
    servedBody: fields.servedBody,
    grounding: { ...account.grounding, recipientId, projectId },
    ...(instruction === undefined ? {} : { instruction }),
  });
  const control: PendingAction = {
    run: () => draft.mutate(ask()),
    pending: draft.isPending,
    disabled: draftBlocked({
      busy: rejectionInFlight || sendPending,
      answering,
      intent,
      anchorActivity: thread.anchorActivity,
      needsRecipient: groundable && !recipientId,
    }),
    error: draftErrorOf(draft, thread.anchorRead.failed, answering, t),
  };
  const discardControl = rejectable
    ? {
        run: () => discard.mutate(rejectable),
        pending: discard.isPending,
        disabled: sendPending,
        error: discard.isError ? problemMessageOf(discard.error, t) : null,
      }
    : null;
  return {
    draft,
    control,
    discardControl,
    rejectionInFlight,
    need: draftNeed(groundable, recipient),
    // A rewrite replaces the body, so it waits for any draft in flight.
    rewrite: (instruction: string) => draft.mutate(ask(instruction)),
  };
}

export type VoiceDraft = ReturnType<typeof useVoiceDraft>;

// The verb that opened the composer promised a draft. So the draft starts once
// it can be asked for, once per opening, and never over words on screen.
export function useDraftOnOpen({
  open,
  asked,
  mail,
  settled,
  body,
  control,
}: Readonly<{
  open: boolean;
  asked: boolean | undefined;
  /** A channel reply has no drafting to start. */
  mail: boolean;
  /** A saved draft has had its chance to come back. */
  settled: boolean;
  body: string;
  control: PendingAction;
}>) {
  const started = useRef(false);
  const wanted = open && asked === true && mail;
  const ready = settled && body === "";
  const { run, disabled, pending } = control;
  useEffect(() => {
    if (!wanted) {
      started.current = false;
      return;
    }
    if (started.current || !ready || disabled || pending) return;
    started.current = true;
    run();
  }, [wanted, ready, disabled, pending, run]);
}
