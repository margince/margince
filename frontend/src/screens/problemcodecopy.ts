// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MessageKey } from "../i18n/en";

// Wire codes whose server detail is not a sentence for this screen's reader,
// and the catalog copy problemDetail (common.tsx) shows in its place.
export const PROBLEM_CODE_COPY = new Map<string, MessageKey>([
  ["gateway_unavailable", "common.gatewayUnavailable"],
  ["assistant_unavailable", "common.assistantUnavailable"],
  ["provider_out_of_credit", "common.providerOutOfCredit"],
  ["provider_unauthorized", "common.providerUnauthorized"],
  ["provider_unavailable", "common.providerUnavailable"],
  ["permission_denied", "common.permissionDenied"],
  ["seat_tier_insufficient", "common.seatReadOnly"],
  // The server's skew detail is English, worded for API and agent readers.
  ["version_skew", "common.versionSkew"],
  // Every code httperr.Duplicate sends: its detail is one fixed API sentence
  // for all of them. backend/gates/frontendproblemcodecopy_test.go holds the set.
  ["duplicate_email", "common.duplicate.email"],
  ["duplicate_domain", "common.duplicate.domain"],
  ["duplicate_linkedin_url", "common.duplicate.linkedinUrl"],
  ["duplicate_contact_lead", "common.duplicate.contactLead"],
  ["slot_taken", "common.duplicate.slotTaken"],
]);
