// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

type Activity = components["schemas"]["Activity"];
type ContactRole = NonNullable<
  components["schemas"]["ActivityVoice"]["contact_role"]
>;

/**
 * Who wrote a row, read from the server's author projection (`voice`). Every
 * surface that names an author reads it from here, never from `direction`:
 * a mail that came in was not necessarily written by the page's contact.
 */
export function authorName(row: Activity): string | undefined {
  const author = row.voice?.author;
  if (!author) {
    return undefined;
  }
  return author.display_name?.trim() || author.address?.trim() || undefined;
}

/** The part the page's contact had on a contact 360 row, when the row says. */
export function contactRole(row: Activity): ContactRole | undefined {
  return row.voice?.contact_role;
}

/** The page's contact wrote this row. */
export function writtenByPageContact(row: Activity): boolean {
  return contactRole(row) === "author";
}

/** The signed-in reader wrote this row. */
export function writtenByReader(row: Activity): boolean {
  return row.voice?.author?.is_reader === true;
}

/**
 * The page's contact may owe or be owed a move on this row. Only a stated part
 * other than author or addressee rules it out; a row that states no part keeps
 * whatever duty it carries.
 */
export function owesOrIsOwed(row: Activity): boolean {
  const role = contactRole(row);
  return role === undefined || role === "author" || role === "addressee";
}

// Mirrors activities.WroteItOurSide: authorship names the side when known;
// direction supplies the answer for older or unresolved projections. On a
// logged call the author is whoever wrote it down, so only direction counts.
export function correspondenceSide(
  row: Activity,
): "ours" | "theirs" | undefined {
  if (!row.direction) return undefined;
  const voiceNamesSender =
    row.kind === "email" || row.kind === "message" || row.kind === "meeting";
  switch (voiceNamesSender ? row.voice?.author?.kind : undefined) {
    case "colleague":
      return "ours";
    case "contact_record":
      return "theirs";
  }
  if (row.direction === "outbound") return "ours";
  if (row.direction === "inbound") return "theirs";
  return undefined;
}
