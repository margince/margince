// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import type { CommunicationContext } from "../../lib/compose-context";
import type { ChosenFile } from "./composeattachments";
import type { ComposeFields } from "./composefields";
import type { Link } from "./composelinks";
import type { Refusal } from "./composerefusal";
import { scheduleFields } from "./composeschedule";

// The message a Send carries, and whether a refusal still describes it.

type Preview = components["schemas"]["SendAuthorizationPreview"];

export type SendRequest = {
  activityId?: string;
  isChannelReply: boolean;
  mail: {
    subject: string;
    body: string;
    // The markup beside the plain part, omitted when nothing was formatted.
    html_body?: string;
    // The sender's mailbox, omitted when the server chooses.
    from_connection_id?: string;
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
    evidence?: components["schemas"]["CommunicationEvidence"];
    scheduled_at?: string;
    scheduled_tz?: string;
    scheduled_intent?: components["schemas"]["IntentTime"];
  };
  channelBody: {
    body: string;
    communication_context?: CommunicationContext;
    attachment_ids?: string[];
  };
  links: Link[];
  // The engine's answer on screen when Send was pressed. A refusal is about
  // that message and that answer, and only while both still hold.
  answerAtSend: Preview | undefined;
};

// The request for the message as it stands, built from one render's values.
export function sendRequest(
  fields: ComposeFields,
  where: Readonly<{
    answering: string | undefined;
    isChannelReply: boolean;
    files: readonly ChosenFile[];
    heldDraftId: string | undefined;
    context: CommunicationContext | undefined;
    evidence?: components["schemas"]["CommunicationEvidence"];
    links: Link[];
    answerAtSend: Preview | undefined;
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
      from_connection_id: fields.from.connectionId || undefined,
      cc: cc.length ? cc : undefined,
      bcc: bcc.length ? bcc : undefined,
      attachment_ids: attachmentIds,
      draft_ref: fields.draftRef ?? undefined,
      mail_draft_id: where.heldDraftId,
      communication_context: where.context,
      evidence: where.evidence,
      ...scheduleFields(fields.sendAt, fields.sendSchedule),
    },
    channelBody: {
      body,
      communication_context: where.context,
      attachment_ids: attachmentIds,
    },
    links: where.links,
    answerAtSend: where.answerAtSend,
  };
}

/**
 * Whether a refused Send still describes the message on screen. It stops when
 * anything the request carries changes: a recipient, the words, a file, a
 * record, the reason. It also stops when the check answers anew.
 */
export function refusalIsCurrent(
  sent: SendRequest | undefined,
  now: SendRequest,
  preview: Preview | undefined,
  directing = false,
): boolean {
  return (
    sent !== undefined &&
    (directing || sent.answerAtSend === preview) &&
    messageOf(sent) === messageOf(now)
  );
}

// An older server's footer refusal holds only the message it refused, so a
// changed message or a new check can be sent again.
export function refusalStillShown(
  refusal: Refusal,
  sent: SendRequest | undefined,
  now: SendRequest,
  preview: Preview | undefined,
): Refusal {
  return refusal === "legacyFooter" && !refusalIsCurrent(sent, now, preview)
    ? null
    : refusal;
}

// The message a request sends, without the bookkeeping that names drafts. A
// draft id can arrive after the press and does not change the message.
function messageOf(request: SendRequest): string {
  // JSON leaves an undefined field out, which is how these are dropped.
  return JSON.stringify({
    ...request,
    answerAtSend: undefined,
    mail: { ...request.mail, draft_ref: undefined, mail_draft_id: undefined },
  });
}
