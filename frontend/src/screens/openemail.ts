// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useRef, useState } from "react";

/**
 * Which message a record page's drawer is showing.
 *
 * One drawer per record page, owned by the page: the timeline's rows and the
 * rail's citations open into the same one, and the record stays legible behind
 * it because that is what the reader is working on.
 *
 * `recordId` is the identity, and a change in it CLOSES what was open. React
 * keeps this state across a move from one record to the next — the router does
 * not key these pages by the record they show — so a drawer left open would
 * reopen the previous record's mail over the new record. A reader would see
 * somebody else's message filed under a contact it was never on.
 */
export function useOpenEmail(recordId: string) {
  const [openEmail, setOpenEmail] = useState<string | null>(null);
  const shownFor = useRef(recordId);
  if (shownFor.current !== recordId) {
    shownFor.current = recordId;
    if (openEmail) {
      setOpenEmail(null);
    }
  }
  return [openEmail, setOpenEmail] as const;
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
