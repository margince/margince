// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { navigate } from "../app/router";
import { useToast } from "../design-system/toast";
import { viewerZone } from "../format/timezone";
import { useT } from "../i18n";
import { entityTimelineKeys } from "./activitykeys";
import {
  isConsentNotGranted,
  ProblemError,
  problemFieldErrorsOf,
  problemMessageOf,
  throwProblem,
} from "./common";
import {
  asksWhy,
  type CommunicationContext,
  contextFor,
} from "./compose-context";
import type { ChosenFile } from "./composeattachments";
import type { Grounding } from "./composedraftcall";
import type { ComposeFields } from "./composefields";
import type { RelinkKind } from "./composerelink";
import type { useSavedDraft } from "./composesaveddraft";
import { SCHEDULED_SCREEN } from "./scheduledsends";
import { sendReviewOf } from "./sendreview";
import { useSendPermission } from "./usesendpermission";

type Activity = components["schemas"]["Activity"];
type Link = { entity_type: RelinkKind; entity_id: string };

// The refusals a rep can act on, as opposed to failures. Anything else keeps
// the server's own message, so this surface invents no copy for it.
export type Refusal = "consent" | "mailbox" | "sharedUnsubscribe" | null;

// Consent is a sentinel-mapped 409 that names itself. The two pre-flight
// refusals are 422s, told apart by the field and code the server asserted.
export function refusalOf(error: unknown): Refusal {
  if (error instanceof ProblemError && isConsentNotGranted(error.problem)) {
    return "consent";
  }
  for (const { field, code } of problemFieldErrorsOf(error)) {
    if (field === "from" && code === "mailbox_not_send_capable") {
      return "mailbox";
    }
    if (field === "recipients" && code === "shared_unsubscribe_token") {
      return "sharedUnsubscribe";
    }
  }
  return null;
}

/**
 * What a sent message files under: the page it was written from, and every
 * record the rep named while writing it.
 *
 * The grounding choices are the attribution, so they travel with the send. A
 * link is matched on kind and id, the way the server identifies it, and a
 * repeat is dropped because the link table refuses one.
 *
 * `chosen` is null on a reply, whose links belong to its thread.
 */
export function composedLinks(
  anchor: { entityType: RelinkKind; entityId: string },
  chosen: Grounding | null,
  // The derived filing when the rep chose nothing. Empty means no project.
  derivedProjectId = "",
): Link[] {
  const links: Link[] = [
    { entity_type: anchor.entityType, entity_id: anchor.entityId },
  ];
  const add = (kind: RelinkKind, id: string) => {
    const already = links.some(
      (l) => l.entity_type === kind && l.entity_id === id,
    );
    if (!id || already) {
      return;
    }
    links.push({ entity_type: kind, entity_id: id });
  };
  if (!chosen) {
    // An anchored send: the record it started from, plus the filing it states.
    add("project", derivedProjectId);
    return links;
  }
  add("contact", chosen.recipientId);
  add("deal", chosen.dealId);
  add("project", chosen.projectId);
  return links;
}

type SendRequest = {
  activityId?: string;
  isChannelReply: boolean;
  mail: {
    subject: string;
    body: string;
    // The markup beside the plain part, omitted when nothing was formatted.
    html_body?: string;
    to: string[];
    cc?: string[];
    bcc?: string[];
    // Files already on the record, by id. The send snapshots each one.
    attachment_ids?: string[];
    draft_ref?: string;
    mail_draft_id?: string;
    // Left out on a reply: the engine resolves it from the anchor, and a
    // contradicting claim is recorded as unsupported.
    communication_context?: CommunicationContext;
    scheduled_at?: string;
    scheduled_tz?: string;
  };
  channelBody: {
    body: string;
    communication_context?: CommunicationContext;
    attachment_ids?: string[];
  };
  links: Link[];
};

// The one place the send's origin is chosen. An anchor sends a reply; no
// anchor sends the account-started message with its links. A channel reply
// is anchored by nature.
async function sendFrom(args: SendRequest) {
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
  if (args.activityId) {
    return api.POST("/activities/{id}/send-email", {
      params: { path: { id: args.activityId } },
      body: args.mail,
    });
  }
  return api.POST("/emails", { body: { ...args.mail, links: args.links } });
}

/**
 * The picker's wall-clock text as the wire wants it: an instant plus the IANA
 * zone it was chosen in.
 *
 * `new Date(...)` resolves the text in the rep's own zone. The zone name
 * travels rather than an offset, which would freeze one day's DST rules. Empty
 * means send now.
 */
export function scheduleFields(local: string): {
  scheduled_at?: string;
  scheduled_tz?: string;
} {
  if (local === "") return {};
  const at = new Date(local);
  if (Number.isNaN(at.getTime())) return {};
  return {
    scheduled_at: at.toISOString(),
    scheduled_tz: viewerZone(),
  };
}

// What the send is waiting for, in reading order. A pressed Send names each
// field on the field, so the button never refuses by going grey.
export type MissingField = "to" | "subject" | "body" | "context";

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
  return missing;
}

// The request for the message as it stands, built from one render's values.
function sendRequest(
  fields: ComposeFields,
  where: Readonly<{
    answering: string | undefined;
    isChannelReply: boolean;
    files: readonly ChosenFile[];
    heldDraftId: string | undefined;
    context: CommunicationContext | undefined;
    links: Link[];
  }>,
): SendRequest {
  const { to, cc, bcc, subject, body, html } = fields;
  const attachmentIds = where.files.length
    ? where.files.map((file) => file.id)
    : undefined;
  return {
    activityId: where.answering,
    isChannelReply: where.isChannelReply,
    mail: {
      subject,
      body,
      to,
      // Keep the editor's markup; omit only an empty alternative.
      html_body: html.trim() === "" ? undefined : html,
      cc: cc.length ? cc : undefined,
      bcc: bcc.length ? bcc : undefined,
      attachment_ids: attachmentIds,
      draft_ref: fields.draftRef ?? undefined,
      mail_draft_id: where.heldDraftId,
      communication_context: where.context,
      ...scheduleFields(fields.sendAt),
    },
    channelBody: {
      body,
      communication_context: where.context,
      attachment_ids: attachmentIds,
    },
    links: where.links,
  };
}

// 201 is a message that goes later; 202 is one that has gone.
type SendOutcome =
  | { sent: false }
  | { sent: true; scheduled: boolean; activity: unknown };

// The send: the request, its refusals, the engine's permission answer as the
// rep writes, and the press that names what is still missing.
export function useComposeSend({
  open,
  isChannelReply,
  answering,
  entityType,
  entityId,
  fields,
  files,
  carriageBlocked,
  anchorActivity,
  grounding,
  projectId,
  savedDraft,
  onSent,
  onClose,
}: Readonly<{
  open: boolean;
  isChannelReply: boolean;
  answering: string | undefined;
  entityType: RelinkKind;
  entityId: string;
  fields: ComposeFields;
  files: readonly ChosenFile[];
  carriageBlocked: boolean;
  anchorActivity: Activity | undefined;
  /** The account path's choices; null on an anchored send. */
  grounding: Grounding | null;
  projectId: string;
  savedDraft: ReturnType<typeof useSavedDraft>;
  onSent?: () => void;
  onClose: () => void;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const toast = useToast();
  const { setSendUnavailable } = fields;
  const send = useMutation({
    mutationKey: ["email", entityId],
    // The grounding is the variable, so a stale closure cannot file the mail
    // under a project the picker no longer shows.
    mutationFn: async (request: SendRequest): Promise<SendOutcome> => {
      setSendUnavailable(false);
      const { data, error, response } = await sendFrom(request);
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
      savedDraft.sent();
      for (const queryKey of entityTimelineKeys(entityType, entityId)) {
        queryClient.invalidateQueries({ queryKey });
      }
      // A scheduled send may need undoing, so the toast opens the queue. An
      // `open` verb keeps it until dismissed, door and all.
      if (result.scheduled) {
        toast.show(t("compose.scheduledQueued"), {
          action: {
            kind: "open",
            label: t("compose.scheduledOpenQueue"),
            onAct: () => navigate({ screen: SCHEDULED_SCREEN }),
          },
        });
      }
      onSent?.();
      onClose();
    },
  });

  // A refusal keeps the form open under copy naming the next move, without
  // the raw server detail beside it.
  const refusal = refusalOf(send.error);
  // The undecided work a consent refusal left behind; null for any other.
  const sendReview = sendReviewOf(send.error);
  const sendError =
    send.isError && refusal === null ? problemMessageOf(send.error, t) : null;
  // The one place the claim is decided, so the mail and channel arms agree.
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
  });
  // Spelled once for the send and for the question asked ahead of it.
  const links = composedLinks({ entityType, entityId }, grounding, projectId);
  // The engine's answer, asked while the rep writes. Bcc is in the question,
  // because consent is owed to everyone who receives the message.
  const permission = useSendPermission({
    recipients: [...fields.to, ...fields.cc, ...fields.bcc],
    anchorActivityId: answering,
    links,
    context: claimedContext,
    enabled: open && !isChannelReply,
  });
  // Whether the reader has pressed Send. A form that reports gaps before then
  // scolds a reader for not having finished typing.
  const [attempted, setAttempted] = useState(false);
  // Each opening starts clean; a refusal belongs to the send it answered.
  useEffect(() => {
    if (!open) {
      setAttempted(false);
    }
  }, [open]);
  const flagged: ReadonlySet<MissingField> = attempted
    ? new Set(missing)
    : new Set<MissingField>();
  // The first unanswered field, read off the DOM at the press. The controls
  // share no ref shape, and `aria-invalid` is the mark they share.
  const fieldsRef = useRef<HTMLDivElement | null>(null);
  const focusFirstMissing = () => {
    fieldsRef.current
      ?.querySelector<HTMLElement>('[aria-invalid="true"]')
      ?.focus();
  };
  const confirm = () => {
    // Marking and focusing is harmless when only a carriage block is present.
    if (missing.length > 0 || carriageBlocked) {
      setAttempted(true);
      globalThis.requestAnimationFrame(focusFirstMissing);
      return;
    }
    send.mutate(
      sendRequest(fields, {
        answering,
        isChannelReply,
        files,
        heldDraftId: savedDraft.held?.id,
        context: claimedContext,
        links,
      }),
    );
  };
  return {
    send,
    refusal,
    sendReview,
    sendError,
    claimedContext,
    permission,
    flagged,
    fieldsRef,
    confirm,
    clearAttempt: () => setAttempted(false),
  };
}

export type ComposeSend = ReturnType<typeof useComposeSend>;
