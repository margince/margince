// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { paragraphsFrom } from "../../design-system/richtext";
import { localDateTimeValue } from "../../format/calendarday";
import type { CommunicationContext } from "../../lib/compose-context";
import { RELINK_KINDS, type RelinkKind } from "../../lib/recordtargets";
import type { ScheduledSend } from "./scheduledqueue";

/** What the composer opens with when a scheduled message is edited. */
export type EditSeed = Readonly<{
  anchor: Readonly<{ type: RelinkKind; id: string }>;
  to: string[];
  cc: string[];
  bcc: string[];
  subject: string;
  body: string;
  html: string;
  sendAt: string;
  context: CommunicationContext | "";
  recipientId: string;
  dealId: string;
  projectId: string;
}>;

const isRelinkKind = (type: string): type is RelinkKind =>
  (RELINK_KINDS as readonly string[]).includes(type);

/**
 * The composer's opening state for a scheduled message, or nothing when the
 * message cannot be written again faithfully.
 *
 * Editing is cancel-and-recompose, because the approval is bound to the
 * content. So only a message the composer itself could have written qualifies:
 * a new conversation filed under a record, with none of the marketing purpose,
 * evidence or legacy consent key the composer has no field for. A reply names
 * a thread and not the record behind it, so it is left out as well.
 */
export function editSeedOf(send: ScheduledSend): EditSeed | undefined {
  const [anchor, ...rest] = send.links ?? [];
  const editable =
    (send.status === "scheduled" || send.status === "held") &&
    !send.anchor_activity_id &&
    !send.marketing_purpose &&
    !send.consent_purpose &&
    !send.evidence &&
    anchor !== undefined &&
    isRelinkKind(anchor.entity_type);
  if (!editable || !isRelinkKind(anchor.entity_type)) {
    return undefined;
  }
  const idOf = (kind: RelinkKind) =>
    rest.find((link) => link.entity_type === kind)?.entity_id ?? "";
  return {
    anchor: { type: anchor.entity_type, id: anchor.entity_id },
    to: send.to,
    cc: send.cc ?? [],
    bcc: send.bcc ?? [],
    subject: send.subject,
    body: send.body ?? "",
    html: paragraphsFrom(send.body ?? ""),
    sendAt: localDateTimeValue(send.scheduled_at),
    context: send.communication_context ?? "",
    recipientId: idOf("contact"),
    dealId: idOf("deal"),
    projectId: idOf("project"),
  };
}
