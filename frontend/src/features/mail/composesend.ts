// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { api } from "../../api/client";
import { type IntentKeys, intentKeys } from "../../api/idempotencykey";
import type { components } from "../../api/schema";
import { focusFirstRefused } from "../../design-system/savepage";
import { useToast } from "../../design-system/toast";
import { useT } from "../../i18n";
import { entityTimelineKeys } from "../../lib/activitykeys";
import { fileListKeys } from "../../lib/attachmentkeys";
import { asksWhy, contextFor } from "../../lib/compose-context";
import { problemMessageOf, throwProblem } from "../../lib/problem";
import { navigate } from "../../lib/router";
import type { ChosenFile } from "./composeattachments";
import type { ComposeFields } from "./composefields";
import { composedLinks, type SendGrounding } from "./composelinks";
import type { Meeting } from "./composemeeting";
import { refusalOf } from "./composerefusal";
import type { RelinkKind } from "./composerelink";
import type { useSavedDraft } from "./composesaveddraft";
import { inlineSendHold } from "./composesendhold";
import {
  refusalStillShown,
  type SendRequest,
  sendRequest,
} from "./composesendrequest";
import { dealOf, reasonEvidence, useReasonForAnchor } from "./reasonanchor";
import { SCHEDULED_SCREEN } from "./scheduledqueue";

type Activity = components["schemas"]["Activity"];

// The one place the send's origin is chosen. An anchor sends a reply; no
// anchor sends the account-started message with its links. A channel reply
// is anchored by nature.
async function sendFrom(args: SendRequest, keyed: IntentKeys) {
  if (args.isChannelReply) {
    if (!args.activityId) {
      // The mail arm would post a channel message as an email, so a channel
      // reply without its conversation is refused.
      throw new Error(
        "compose: a channel reply needs the conversation it answers",
      );
    }
    return api.POST("/activities/{id}/send-message", {
      params: { path: { id: args.activityId } },
      body: args.channelBody,
    });
  }
  const anchor = args.activityId;
  if (anchor) {
    return keyed([anchor, args.mail], (key) =>
      api.POST("/activities/{id}/send-email", {
        params: { path: { id: anchor }, header: { "Idempotency-Key": key } },
        body: args.mail,
      }),
    );
  }
  const body = { ...args.mail, links: args.links };
  return keyed(body, (key) =>
    api.POST("/emails", {
      params: { header: { "Idempotency-Key": key } },
      body,
    }),
  );
}

// What the send is waiting for, in reading order. A pressed Send names each
// field on the field, so the button never refuses by going grey.
export type MissingField = "to" | "subject" | "body" | "context" | "deal";

export function missingToSend(
  isChannelReply: boolean,
  fields: {
    to: string[];
    subject: string;
    body: string;
    context: string;
    // False where the anchor answers what the message is. A reply derives
    // its category from the thread.
    asksContext: boolean;
    // An Active deal claim with a deal to choose and none chosen.
    needsDeal?: boolean;
  },
): readonly MissingField[] {
  const missing: MissingField[] = [];
  // A channel resolves its own recipient and carries no subject.
  if (!isChannelReply && fields.to.length === 0) {
    missing.push("to");
  }
  if (!isChannelReply && fields.subject.trim() === "") {
    missing.push("subject");
  }
  if (fields.body.trim() === "") {
    missing.push("body");
  }
  if (fields.asksContext && fields.context === "") {
    missing.push("context");
  }
  if (fields.needsDeal) {
    missing.push("deal");
  }
  return missing;
}

// Both responses accept delivery; only 201 names a scheduled release.
type SendOutcome =
  | { sent: false }
  | { sent: true; scheduled: boolean; activity: unknown };

// The send: the request, its refusals, and the press that names what is still
// missing. The press is the check: nothing is asked while the rep writes, and a
// refusal comes back with its review.
export function useComposeSend({
  open,
  isChannelReply,
  answering,
  entityType,
  entityId,
  dealId,
  fields,
  files,
  carriageBlocked,
  sourceBlocked,
  anchorActivity,
  grounding,
  projectId,
  needsDeal,
  savedDraft,
  meeting,
  onSent,
  onClose,
}: Readonly<{
  open: boolean;
  isChannelReply: boolean;
  answering: string | undefined;
  entityType: RelinkKind;
  entityId: string;
  dealId?: string;
  fields: ComposeFields;
  files: readonly ChosenFile[];
  carriageBlocked: boolean;
  sourceBlocked?: boolean;
  anchorActivity: Activity | undefined;
  /** What a new message is about; null on an anchored send. */
  grounding: SendGrounding | null;
  projectId: string;
  needsDeal: boolean;
  savedDraft: ReturnType<typeof useSavedDraft>;
  /** A meeting face to face to note on the contact before the send. */
  meeting?: Meeting;
  onSent?: () => void;
  onClose: () => void;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const toast = useToast();
  const { setSendUnavailable } = fields;
  useReasonForAnchor(answering, fields);
  // What every sent message does next, whichever door sent it: the draft is
  // spent, the timeline reads again, and the composer closes.
  const finishSent = () => {
    savedDraft.sent();
    const changed = [
      ...entityTimelineKeys(entityType, entityId),
      ...(dealId ? entityTimelineKeys("deal", dealId) : []),
      // The sent message carries copies of its files. Its id is not held
      // here, so every message's lists refresh.
      ...(files.length > 0 ? fileListKeys({ entityType: "activity" }) : []),
    ];
    for (const queryKey of changed) {
      queryClient.invalidateQueries({ queryKey });
    }
    onSent?.();
    onClose();
  };
  const sendKeys = useRef(intentKeys());
  const send = useMutation({
    mutationKey: ["email", entityId],
    // The grounding is the variable, so a stale closure cannot file the mail
    // under a project the picker no longer shows.
    mutationFn: async (request: SendRequest): Promise<SendOutcome> => {
      setSendUnavailable(false);
      const { data, error, response } = await sendFrom(
        request,
        sendKeys.current,
      );
      if (response.status === 501) return { sent: false as const };
      // Only a real 2xx is a send. openapi-fetch reports a falsy `error` for a
      // bodiless gateway 5xx, which must not close the modal as sent.
      if (!response.ok) {
        throwProblem(error || { title: t("compose.actionFailed") });
      }
      return {
        sent: true as const,
        scheduled: response.status === 201,
        activity: data,
      };
    },
    onSuccess: (result) => {
      if (!result.sent) {
        setSendUnavailable(true);
        return;
      }
      // A scheduled send may need undoing, so the toast opens the queue. An
      // `open` verb keeps it until dismissed, door and all.
      if (result.scheduled) {
        queryClient.invalidateQueries({ queryKey: ["scheduled-sends"] });
        toast.show(t("compose.scheduledQueued"), {
          action: {
            kind: "open",
            label: t("compose.scheduledOpenQueue"),
            onAct: () => navigate({ screen: SCHEDULED_SCREEN }),
          },
        });
      }
      finishSent();
    },
  });
  // Clearing the refusal and draft after staging stops a resend of the held message.
  const directSent = () => {
    send.reset();
    toast.show(t("directSend.queued"));
    finishSent();
  };

  // A refusal keeps the form open under copy naming the next move, without
  // the raw server detail beside it.
  const refusal = refusalOf(send.error);
  const sendError =
    send.isError && refusal === null ? problemMessageOf(send.error, t) : null;
  const claimedContext = contextFor({
    anchor: anchorActivity,
    chosen: fields.context,
  });
  const missing = missingToSend(isChannelReply, {
    to: fields.to,
    subject: fields.subject,
    body: fields.body,
    context: fields.context,
    asksContext: asksWhy(anchorActivity),
    needsDeal,
  });
  const links = composedLinks(
    { entityType, entityId, dealId },
    grounding,
    projectId,
  );
  const contextDealId = dealOf(answering ? anchorActivity?.links : links);
  const evidence = reasonEvidence(contextDealId, fields.meetingEvidence);
  const request = sendRequest(fields, {
    answering,
    isChannelReply,
    files,
    heldDraftId: savedDraft.onScreenId,
    context: claimedContext,
    evidence,
    links,
    answerAtSend: undefined,
  });
  const shownRefusal = refusalStillShown(
    refusal,
    send.variables,
    request,
    undefined,
  );
  const hold = inlineSendHold(shownRefusal === "legacyFooter");
  const [attempted, setAttempted] = useState(false);
  useEffect(() => {
    if (!open) {
      setAttempted(false);
    }
  }, [open]);
  const flagged: ReadonlySet<MissingField> = attempted
    ? new Set(missing)
    : new Set<MissingField>();
  const fieldsRef = useRef<HTMLDivElement | null>(null);
  const focusFirstMissing = () => focusFirstRefused(fieldsRef.current);
  const confirm = () => {
    // Marking and focusing is harmless when only a carriage block is present.
    if (missing.length > 0 || carriageBlocked || sourceBlocked) {
      setAttempted(true);
      globalThis.requestAnimationFrame(focusFirstMissing);
      return;
    }
    if (hold.blocks) {
      return;
    }
    if (!meeting?.on) {
      send.mutate(request);
      return;
    }
    // The meeting is the evidence the send is judged on, so it is noted
    // first. A failed note sends nothing and says so beside the meeting.
    meeting.record().then(
      () => send.mutate(request),
      () => undefined,
    );
  };
  // What this client already knows will stop the send. The mark beside Send
  // makes no passed claim while any of it stands.
  const blocked =
    Boolean(sourceBlocked) ||
    missing.length > 0 ||
    carriageBlocked ||
    fields.sendUnavailable ||
    refusal === "mailbox" ||
    refusal === "sharedUnsubscribe" ||
    shownRefusal === "legacyFooter";
  return {
    send,
    request,
    blocked,
    refusal: shownRefusal,
    sendError,
    claimedContext,
    contextDealId,
    flagged,
    fieldsRef,
    confirm,
    clearAttempt: () => setAttempted(false),
    directSent,
    hold,
  };
}

export type ComposeSend = ReturnType<typeof useComposeSend>;
