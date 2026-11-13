// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The email a trial installation sends the Margince team. It is written here
// and sent from the reader's own mail app, so the installation never calls home.

/** Where the Margince team reads a trial's request. */
export const MARGINCE_TEAM_ADDRESS = "info@margince.com";

/** The contact form on the Margince website, for a reader with no mail app set up. */
export const MARGINCE_CONTACT_FORM = "https://margince.com/contact/";

/** What the reader asks the team for. */
export type TrialAsk = "time" | "license" | "call";

export const TRIAL_ASKS: readonly TrialAsk[] = ["time", "license", "call"];

/** The `mailto:` link that opens the reader's mail app on this email. */
export function trialMailtoHref(
  mail: Readonly<{ subject: string; body: string }>,
): string {
  const query = new URLSearchParams({ subject: mail.subject, body: mail.body })
    .toString()
    // A mail app reads `+` as a plus sign: RFC 6068 wants a space as %20.
    .replaceAll("+", "%20");
  return `mailto:${MARGINCE_TEAM_ADDRESS}?${query}`;
}
