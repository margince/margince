// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  isConsentNotGranted,
  ProblemError,
  problemCodeOf,
  problemFieldErrorsOf,
} from "../../lib/problem";

// The refusals a rep can act on, as opposed to failures. Anything else keeps
// the server's own message, so this surface invents no copy for it.
export type Refusal =
  | "consent"
  | "mailbox"
  | "noMailbox"
  | "sharedUnsubscribe"
  | "legacyFooter"
  | null;

// Consent is a 409 naming itself. A mailbox that cannot send, or none
// connected, is a 422 naming no field. A shared unsubscribe link is a 422 field
// fault on the recipients.
export function refusalOf(error: unknown): Refusal {
  if (error instanceof ProblemError && isConsentNotGranted(error.problem)) {
    return "consent";
  }
  if (problemCodeOf(error) === "mailbox_not_connected") {
    return "noMailbox";
  }
  if (problemCodeOf(error) === "mailbox_not_send_capable") {
    return "mailbox";
  }
  if (
    problemCodeOf(error) === "public_origin_unusable" ||
    problemFieldErrorsOf(error).some(
      ({ code }) => code === "undisclosable_obligation",
    )
  )
    return "legacyFooter";
  const shared = problemFieldErrorsOf(error).some(
    ({ field, code }) =>
      field === "recipients" && code === "shared_unsubscribe_token",
  );
  return shared ? "sharedUnsubscribe" : null;
}
