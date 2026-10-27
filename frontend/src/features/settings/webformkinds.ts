// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";

export type WebForm = components["schemas"]["WebForm"];
export type WebFormCreated = components["schemas"]["WebFormCreated"];
export type WebFormSender = components["schemas"]["WebFormSender"];

export const WEB_FORMS_KEY = "web-forms";

export const WEB_FORM_SENDERS: readonly WebFormSender[] = [
  "browser",
  "server",
  "webflow",
  "typeform",
  "tally",
];

// A builder signs its own webhook with a secret pasted from its settings.
export function isBuilder(sender: WebFormSender): boolean {
  return sender !== "browser" && sender !== "server";
}
