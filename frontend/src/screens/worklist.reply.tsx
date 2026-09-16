// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// ANSWERING THE BUYER, from the row that named the wait.
//
// ONE control, and that is the whole point of this file. The move a rule
// prepared and the reply a rep presses are the SAME ACT on a waiting message —
// write back to them — so the row draws one button: the agent's face and mark,
// because a rule worked the move out, over the composer the reply has always
// opened. Drawn apart they were two buttons asking for one answer, and the
// prepared one kept a promise its route could not: a link away from the page,
// or a read of the very message it offered to draft.
//
// Beside the row rather than in it, because the row's file is at the length
// the tree allows and this is one idea that reads whole on its own.

import { useQueryClient } from "@tanstack/react-query";
import { viewerZone } from "../format/timezone";
import { type Locale, type Translator, useLocale, useT } from "../i18n";
import { ChannelReplyAction, RELINK_KINDS, type RelinkKind } from "./compose";
import { intentAbout } from "./compose.intent";
import { phrasedReasons, reasonText } from "./worklist.copy";
import { type WorklistItem, worklistKey } from "./worklist.queries";

/**
 * The record a reply would be filed against, or nothing.
 *
 * Both halves must hold. The verb says the server judged this wait answerable —
 * it is mail, not a channel message the mail composer would answer in the wrong
 * place. The subject says WHICH record the sent message links to, and its type
 * has to be one the composer can file against: the row's own vocabulary is
 * wider than RELINK_KINDS, so an `activity` subject would type-check as a
 * string and fail at the composer.
 */
export function replyTarget(
  item: WorklistItem,
): { type: RelinkKind; id: string } | undefined {
  if (!item.actions.includes("reply") || !item.subject) {
    return undefined;
  }
  const type = item.subject.type;
  if (!RELINK_KINDS.includes(type as RelinkKind)) {
    return undefined;
  }
  return { type: type as RelinkKind, id: item.subject.id };
}

/**
 * Whether this row's prepared move IS its reply.
 *
 * Asked from both sides of one invariant, which is why it is a function rather
 * than a condition written twice: the verb line asks it to WITHHOLD its own
 * copy of the move, and the control below asks it to wear the agent's face.
 * Answered apart, the row would draw the move twice or draw it in neither
 * place.
 */
export function replyIsTheMove(item: WorklistItem): boolean {
  return item.move?.action === "draft_reply" && replyTarget(item) !== undefined;
}

/**
 * Answering the buyer, over the row that named the wait.
 *
 * Its own component so it can hold the hook that refreshes the queue. The
 * composer invalidates the RECORD timelines it knows about, and the worklist is
 * not one of them — nor does the queue poll — so without the callback the row
 * keeps saying nobody has replied, and keeps offering to reply again, over a
 * message the reader has already answered.
 */
export function WaitingReply({
  item,
  to,
}: Readonly<{
  item: WorklistItem;
  to: { type: RelinkKind; id: string };
}>) {
  const t = useT();
  const { locale } = useLocale();
  const queryClient = useQueryClient();
  return (
    <ChannelReplyAction
      activityId={item.id}
      kind="email"
      entityType={to.type}
      entityId={to.id}
      prepared={
        replyIsTheMove(item)
          ? {
              // THE WORD IS THE ACT, always. `moveLabel` says "Read and reply"
              // where the move is only a LINK that may land on the record
              // rather than the composer; this control mounts the composer
              // itself, on every row it is drawn on, so the weaker word would
              // under-promise what pressing it does.
              label: t("worklist.verb.draft_reply_now"),
              intent: replyIntent(item, t, locale),
            }
          : undefined
      }
      // `worklistKey`, not `[worklistKey]`. The key IS the segment array, so
      // wrapping it once more asks for a query whose first segment is itself
      // an array — which nothing in the cache is, so the invalidation matched
      // nothing and the row a rep had just answered stayed in the waiting
      // lane until they reloaded the page.
      onSent={() =>
        void queryClient.invalidateQueries({ queryKey: worklistKey })
      }
    />
  );
}

/**
 * What the draft should be about, from the row's own reason for standing here.
 *
 * The composer is already anchored to the message, so the thread needs no
 * saying; what it cannot read is WHY this one was ranked — "waiting 13 days" is
 * the queue's judgement and not a fact in the mail. That reason is the steer,
 * which is what the field is for.
 *
 * The FIRST reason the row phrases, because the ranking put it first. A reason
 * this build has no sentence for yields none (`reasonText`), so the steer falls
 * back to the bare phrase rather than to a raw key.
 */
function replyIntent(
  item: WorklistItem,
  t: Translator,
  locale: Locale,
): string {
  const why = phrasedReasons(item, false).flatMap(
    (reason) => reasonText(reason, t, locale, viewerZone()) ?? [],
  )[0];
  // The composer's own phrase for answering a message, not a second spelling
  // of it: one sentence reaches the model whichever surface opened the drawer.
  return intentAbout(t("contact.composer.intentReply"), why);
}
