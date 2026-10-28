// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";

export type WebForm = components["schemas"]["WebForm"];
export type WebFormCreated = components["schemas"]["WebFormCreated"];
export type WebFormSender = components["schemas"]["WebFormSender"];

export const WEB_FORMS_KEY = "web-forms";

// In the order a reader meets them: the ones that need no developer first.
export const WEB_FORM_SENDERS: readonly WebFormSender[] = [
  "browser",
  "typeform",
  "tally",
  "server",
  "webflow",
];

// The guide a reader setting a form up is pointed at.
export const WEB_FORMS_GUIDE =
  "https://github.com/margince/margince/blob/main/docs/handbook/connect-your-website.md";

// Webflow issues the secret its API-made webhook signs with, so it is pasted.
export function pastesSecret(sender: WebFormSender): boolean {
  return sender === "webflow";
}

// The senders whose secret Margince mints, shows once and may rotate.
// gates/frontendwebformsigning_test.go holds them to the server's MintsSecret.
const MINTED_SECRET_SENDERS: readonly WebFormSender[] = [
  "server",
  "typeform",
  "tally",
];

export function mintsSecret(sender: WebFormSender): boolean {
  return MINTED_SECRET_SENDERS.includes(sender);
}
