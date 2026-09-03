// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// How a waiting row names the message somebody is waiting on.
//
// The queue's three renderers — the row, the focus card and next-up — each
// composed their own title line from `itemTitle`, and a waiting email came out
// as a bare sentence: no subject beside a preview, no access badge, and no way
// to read the message without leaving the queue for the record behind it. This
// is the one place that decides what a waiting email's line looks like, so the
// three cannot drift.
//
// It is NOT a fourth email row. `EmailEntry` is the row, and this returns it —
// what lives here is only the question of when a queue line is one.

import { EmailEntry } from "../design-system/emailentry";
import { formatDateTime } from "../format/format";
import { viewerZone } from "../format/timezone";
import type { Locale, useT } from "../i18n";
import { useLocale } from "../i18n";
import { itemTitle } from "./worklist.copy";
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
   * Opens the message. Omitted on a surface with no drawer to open it into —
   * the row then shows the message and does not pretend to open it, which is
   * better than a control that answers nothing.
   */
  onOpen?: (activityId: string) => void;
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
      onOpen={onOpen ? () => onOpen(summary.activity_id) : undefined}
    />
  );
}

/**
 * The one-line name for a queue entry, using a waiting email's own SUBJECT
 * where it has one.
 *
 * For the compact lists, which have room for a line and not for a row. The
 * subject is what a rep recognizes a thread by, and `itemTitle` already
 * returns it for a waiting row — this exists so the compact surfaces say out
 * loud that they take the subject deliberately rather than by coincidence, and
 * so a later change to `itemTitle` cannot silently take it away.
 */
export function nextUpLine(
  item: WorklistItem,
  t: ReturnType<typeof useT>,
  locale: Locale,
): string {
  return item.email_summary?.subject?.trim() || itemTitle(item, t, locale);
}
