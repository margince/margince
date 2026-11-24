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
