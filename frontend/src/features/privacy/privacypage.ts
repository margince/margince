// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";

type Controller = components["schemas"]["PrivacyInformationController"];

/**
 * Where a subscription question points its reader for their rights, first
 * stated wins: the privacy page, then the privacy contact, then nothing. The
 * server fills the published question in the same order (privacyPageFields in
 * backend/internal/modules/consent/boundquestion.go), and
 * backend/gates/privacypagefallback_test.go holds the two equal.
 */
export const PRIVACY_PAGE_FIELDS = ["privacy_page", "privacy_contact"] as const;

export function privacyPageOf(controller: Controller | undefined): string {
  for (const field of PRIVACY_PAGE_FIELDS) {
    const value = controller?.[field]?.trim();
    if (value) {
      return value;
    }
  }
  return "";
}

// Stands in for an unstated page while the sentence is filled, so the sentence
// that holds it can be found and dropped. No copy contains it.
const NO_PAGE = "\u0000";

/**
 * The question with its rights link filled, or with the sentence that points to
 * it left out when the controller states no page. The server drops it the same
 * way (withoutRightsSentence in backend/internal/modules/consent/boundquestion.go).
 */
export function withRightsLink(
  fill: (privacyPage: string) => string,
  privacyPage: string,
): string {
  if (privacyPage) {
    return fill(privacyPage);
  }
  const text = fill(NO_PAGE);
  const at = text.indexOf(NO_PAGE);
  if (at < 0) {
    return text;
  }
  const end = text.startsWith(".", at + 1) ? at + 2 : at + 1;
  const start = text.lastIndexOf(". ", at);
  return start < 0
    ? text.slice(end).trim()
    : text.slice(0, start + 1) + text.slice(end);
}
