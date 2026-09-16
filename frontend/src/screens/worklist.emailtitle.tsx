// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// How a waiting row names the message somebody is waiting on.
//
// The queue's renderers each composed their own title line from `itemTitle`,
// and a waiting email came out as a bare sentence: no subject beside a
// preview, no access badge, and no way to read the message without leaving the
// queue for the record behind it. This is the one place that decides what a
// waiting email's line looks like, so they cannot drift.
//
// A COMPACT list names its rows with `itemTitle` and not from here. The server
// puts a waiting message's subject in the row's own `title` (classify.go), so
// that is already the subject — and a second helper reading it back off
// `email_summary` was one more place drawing a part of a message outside the
// canonical row. `brief.feed.test.tsx` holds that the column names a waiting
// row by its subject, which is where a claim like that belongs.
//
// It is NOT a fourth email row. `EmailEntry` is the row, and this returns it —
// what lives here is only the question of when a queue line is one.

import { EmailEntry } from "../design-system/emailentry";
import { formatDateTime } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale } from "../i18n";
import type { WorklistItem } from "./worklist.queries";

/**
 * The canonical email row for a waiting message, or null where the line is not
 * one.
 *
 * Null on every source but a waiting customer, null on a channel message, and
 * null on an email whose content is not this reader's — though such a message
 * produces no waiting row at all. The caller draws its own title line when this
 * answers null, which is what keeps a chat, a task and a drifting deal reading
 * exactly as they did.
 */
export function WaitingEmailLine({
  item,
  onOpen,
}: Readonly<{
  item: WorklistItem;
  /**
   * Opens the message. REQUIRED: this line draws the whole message — sender,
   * subject, preview, access badge — so a surface that draws it and cannot
   * open it shows a reader something and then refuses it. Both surfaces that
   * draw a waiting row mount a drawer; a surface that cannot should not draw
   * this line at all.
   */
  onOpen: (activityId: string) => void;
}>) {
  const { locale } = useLocale();
  const summary = item.email_summary;
  if (!summary) {
    return null;
  }
  return (
    <EmailEntry
      summary={summary}
      timestamp={formatDateTime(summary.occurred_at, locale, viewerZone())}
      onOpen={() => onOpen(summary.activity_id)}
    />
  );
}
