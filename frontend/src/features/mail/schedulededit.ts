// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { paragraphsFrom } from "../../design-system/richtext";
import { localDateTimeValue } from "../../format/calendarday";
import type { CommunicationContext } from "../../lib/compose-context";
import { RELINK_KINDS, type RelinkKind } from "../../lib/recordtargets";
import type { ChosenFile } from "./composeattachments";
import type { ScheduledSend } from "./scheduledqueue";

/** What the composer opens with when a scheduled message is edited. */
export type EditSeed = Readonly<{
  anchor: Readonly<{ type: RelinkKind; id: string }>;
  /** The email this message answers; empty for a new conversation. */
  replyTo: string;
  to: string[];
  cc: string[];
  bcc: string[];
  subject: string;
  body: string;
  html: string;
  files: readonly ChosenFile[];
  /** The voice draft the words came from; empty when none. */
  draftRef: string;
  sendAt: string;
  context: CommunicationContext | "";
  recipientId: string;
  dealId: string;
  projectId: string;
}>;

const isRelinkKind = (type: string): type is RelinkKind =>
  (RELINK_KINDS as readonly string[]).includes(type);

// The composer has no field for a marketing purpose, evidence or a legacy
// consent key. A message that has gone or been withdrawn has nothing left.
function composerCanCarry(send: ScheduledSend): boolean {
  return (
    (send.status === "scheduled" || send.status === "held") &&
    !send.marketing_purpose &&
    !send.consent_purpose &&
    !send.evidence &&
    (send.anchor_activity_id !== undefined ||
      isRelinkKind(send.links?.[0]?.entity_type ?? ""))
  );
}

/**
 * Whether the composer can write this message again faithfully.
 *
 * The approval is bound to the content, so an edit is a new send, and an
 * operator reason has no field to travel in.
 */
export function isEditable(send: ScheduledSend): boolean {
  return composerCanCarry(send) && !send.operator_reason;
}

/** An otherwise editable message whose operator reason an edit would drop. */
export function dropsOperatorReason(send: ScheduledSend): boolean {
  return composerCanCarry(send) && Boolean(send.operator_reason);
}

/** How many frozen files the send would refuse; only the detail read says. */
export function unavailableFilesOf(send: ScheduledSend): number {
  return send.attachments_unavailable ?? 0;
}

/**
 * The composer's opening state for a scheduled message, or nothing when it
 * cannot be written again yet.
 *
 * A reply names a thread and not the record behind it. The caller passes the
 * links of the thread's email, because a reply inherits its links from there.
 */
export function editSeedOf(
  send: ScheduledSend,
  threadLinks?: ScheduledSend["links"],
): EditSeed | undefined {
  const [anchor, ...rest] =
    (send.anchor_activity_id ? threadLinks : send.links) ?? [];
  if (
    !isEditable(send) ||
    unavailableFilesOf(send) > 0 ||
    !anchor ||
    !isRelinkKind(anchor.entity_type)
  ) {
    return undefined;
  }
  const idOf = (kind: RelinkKind) =>
    rest.find((link) => link.entity_type === kind)?.entity_id ?? "";
  return {
    anchor: { type: anchor.entity_type, id: anchor.entity_id },
    replyTo: send.anchor_activity_id ?? "",
    to: send.to,
    cc: send.cc ?? [],
    bcc: send.bcc ?? [],
    subject: send.subject,
    body: send.body ?? "",
    html: send.html_body ?? paragraphsFrom(send.body ?? ""),
    files: (send.attachments ?? []).map((file) => ({
      id: file.id,
      filename: file.filename,
      byteSize: file.byte_size,
    })),
    draftRef: send.draft_ref ?? "",
    sendAt: localDateTimeValue(send.scheduled_at),
    context: send.communication_context ?? "",
    recipientId: idOf("contact"),
    dealId: idOf("deal"),
    projectId: idOf("project"),
  };
}
