// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MessageKey } from "../i18n/en";

type LegalLink = Readonly<{ key: string; labelKey: MessageKey; href: string }>;

// Only a document that has an address is listed, so the sign-in screen never
// links to a page that does not exist. Margince publishes its privacy notice
// on its website; no terms page exists yet.
export const LEGAL_LINKS: readonly LegalLink[] = [
  {
    key: "privacy",
    labelKey: "auth.legalPrivacy",
    href: "https://margince.com/privacy/",
  },
];
