// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

type Provider = components["schemas"]["CaptureConnection"]["provider"];

/** The providers that connect a MAILBOX.
 *
 *  The other two — `gcal` and `graphcal` — connect a calendar, and on this
 *  screen the difference decides what a row may say about itself. A calendar
 *  has no mail history to import backward from a date, no posture to take
 *  towards mail it does not carry, and no signature block to read a contact
 *  out of. Every one of those rows was drawn against a calendar anyway, under
 *  an envelope icon and beside the member's own email address, and the import
 *  card answered "this mailbox type can't be backfilled" — which a member read
 *  as the product refusing to import their mail.
 *
 *  A DELIBERATE MIRROR of the server's `compose.MailProviders`, which refuses
 *  those same operations for a calendar, held equal by
 *  backend/gates/frontendmailproviders_test.go. Two answers to "is this a
 *  mailbox" is how a screen offers what a server refuses. */
export const MAIL_PROVIDERS: ReadonlySet<Provider> = new Set<Provider>([
  "gmail",
  "graph",
  "imap",
]);

/** Whether this connection is a mailbox rather than a calendar. */
export function isMailbox(provider: Provider): boolean {
  return MAIL_PROVIDERS.has(provider);
}
