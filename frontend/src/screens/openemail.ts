// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";

/**
 * Which message a record page's drawer is showing.
 *
 * One drawer per record page, owned by the page: the timeline's rows and the
 * rail's citations open into the same one, and the record stays legible behind
 * it because that is what the reader is working on.
 *
 * It needs no reset when the record changes. `identityOfAddress` keys the
 * routed subtree by the record id (app/router.tsx's IDENTITY_DEPTH, depth 2 for
 * a contact, an account, a deal), so moving from one record to the next throws
 * the page away and this state with it —
 * App.tabkey.test.tsx's "still throws the page away when the record itself
 * changes" is the proof. A tab change deliberately does NOT remount, and does
 * not need to: the same reader is still on the same record.
 */
export function useOpenEmail() {
  return useState<string | null>(null);
}

/**
 * Attaches the opener to the email rows of a timeline, leaving every other
 * kind untouched.
 *
 * The screens that build their entries through `useRecordChronology` get this
 * from the hook; the two that call `activityTimeline` directly — the deal and
 * the lead — spell it here rather than each writing the same conditional.
 */
export function withEmailOpener<
  T extends { id: string; emailSummary?: unknown },
>(entries: T[], onOpen: (activityId: string) => void): T[] {
  return entries.map((entry) =>
    entry.emailSummary
      ? { ...entry, onOpenEmail: () => onOpen(entry.id) }
      : entry,
  );
}
