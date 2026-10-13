// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";

type Provider = components["schemas"]["CaptureConnection"]["provider"];

// Keyed by the contract enum, so a provider added there without a name here
// fails the typecheck.
const PROVIDER_NAME: Readonly<Record<Provider, MessageKey>> = {
  gmail: "connectors.provGmail",
  gcal: "connectors.provGcal",
  graph: "connectors.provGraph",
  graphcal: "connectors.provGraphCal",
  imap: "connectors.provImap",
  test_mailbox: "connectors.provTestMailbox",
};

// Mirrors the server's `compose.MailProviders`, which refuses mail operations
// on a calendar; backend/gates/frontendmailproviders_test.go holds them equal.
export const MAIL_PROVIDERS: ReadonlySet<Provider> = new Set<Provider>([
  "gmail",
  "graph",
  "imap",
]);

export function isMailbox(provider: Provider): boolean {
  return MAIL_PROVIDERS.has(provider);
}

// A test mailbox is mail, though no mailbox setting applies to it.
export function isMailIcon(provider: Provider): boolean {
  return isMailbox(provider) || provider === "test_mailbox";
}

export function isCaptureProvider(id: string): id is Provider {
  return Object.hasOwn(PROVIDER_NAME, id);
}

/** The catalog key that names a capture provider. */
export function providerNameKey(provider: Provider): MessageKey {
  return PROVIDER_NAME[provider];
}

// An id this build cannot name comes back as sent: a newer server's provider is
// still one the reader can quote.
export function providerName(id: string, t: (key: MessageKey) => string) {
  return isCaptureProvider(id) ? t(PROVIDER_NAME[id]) : id;
}
