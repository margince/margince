// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Answering the message that is open, from the drawer that is showing it.
//
// Reply lived only on the timeline row. A reader who opened a mail to read it
// whole — from a record, from the worklist, from a search hit — had to put it
// away and find the row again to answer it, and on the surfaces whose rows are
// citations rather than timeline entries there was no row to find: the message
// could be read in the product and not answered in it.
//
// It is `ChannelReplyAction` and not a second reply. That component exists so
// the surfaces offering this verb cannot come to offer different things — the
// same composer, anchored to the same activity, filing the same way — and a
// drawer that assembled its own would be exactly the divergence it prevents.

import type { components } from "../api/schema";
import { ChannelReplyAction, type RelinkKind } from "./compose";

type EmailPresentation = components["schemas"]["EmailPresentation"];
type ActivityLink = components["schemas"]["ActivityLink"];

/**
 * Which of the message's records the reply is filed under.
 *
 * The composer anchors a send to ONE record — a reply written from a deal
 * timeline files under the deal and a reply written from a contact files under
 * the contact — and the drawer is opened from surfaces that are not a record
 * page at all, so it answers from the message's own filing instead of from
 * where it was opened.
 *
 * A contact leads because a reply is written TO a person and their timeline is
 * where the conversation reads as a conversation. Where the message names no
 * contact, the work it is about; the account last, being the broadest thing
 * the message could be filed under.
 */
const ANCHOR_ORDER: readonly RelinkKind[] = [
  "person",
  "deal",
  "lead",
  "project",
  "organization",
];

/**
 * The message's own filing, read through the order above.
 *
 * Order in the LIST must not decide it: the server sends links in its own
 * order, and an anchor taken from that would move whenever the read's ordering
 * did. Null when the message is filed against nothing at all.
 */
export function replyAnchor(
  links: readonly ActivityLink[],
): ActivityLink | null {
  for (const kind of ANCHOR_ORDER) {
    const found = links.find((link) => link.entity_type === kind);
    if (found) {
      return found;
    }
  }
  return null;
}

/**
 * The reply verb for the message the drawer is showing, or nothing when this
 * message cannot honestly offer one.
 *
 * `can_reply` is the SERVER's answer and the only thing consulted: a withheld
 * presentation carries false, which is what keeps the drawer from offering to
 * answer a message it is at the same time refusing to show.
 *
 * A message filed against nothing also offers none. The composer files the
 * send under the record it was anchored to, and there is no such record here —
 * a reply anchored to nothing would be a message the product sends and then
 * cannot show on any timeline.
 */
export function EmailReplyAction({
  presentation,
  onSent,
}: Readonly<{
  presentation: EmailPresentation;
  /**
   * Told when the message actually went, for a host whose own list the send
   * changes. `ComposeModal` refreshes the RECORD timelines it knows about, so
   * a record page needs nothing here; a surface listing the UNANSWERED — the
   * worklist's waiting lane — is not one of them, and without this its row
   * goes on saying nobody has replied and goes on offering to reply again.
   */
  onSent?: () => void;
}>) {
  const anchor = presentation.can_reply
    ? replyAnchor(presentation.links)
    : null;
  if (!anchor) {
    return null;
  }
  return (
    <ChannelReplyAction
      activityId={presentation.id}
      // The drawer reads delivered correspondence and nothing else, so the
      // reply is a mail. Reading the kind off the row would be reading a field
      // whose only value this surface can ever meet is this one.
      kind="email"
      entityType={anchor.entity_type}
      entityId={anchor.entity_id}
      // Read off the anchor rather than searched for a second time: a contact
      // leads the order above, so a message that names one has it AS its
      // anchor. A second `find` here would be a second answer to the same
      // question, free to disagree with the first the day the order changes.
      personId={anchor.entity_type === "person" ? anchor.entity_id : undefined}
      onSent={onSent}
    />
  );
}
