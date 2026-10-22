// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MessageKey } from "../i18n/en";

// Wire codes whose server detail is not a sentence for this screen's reader,
// and the catalog copy problemDetail (problem.ts) shows in its place.
export const PROBLEM_CODE_COPY = new Map<string, MessageKey>([
  ["approval_preview_incomplete", "approval.preview.incomplete"],
  ["approval_preview_unavailable", "approval.preview.unavailable"],
  ["approval_preview_conflict", "approval.preview.resolveConflict"],
  ["gateway_unavailable", "common.gatewayUnavailable"],
  ["assistant_unavailable", "common.assistantUnavailable"],
  ["provider_out_of_credit", "common.providerOutOfCredit"],
  ["provider_unauthorized", "common.providerUnauthorized"],
  ["provider_unavailable", "common.providerUnavailable"],
  ["calendar_unavailable", "scheduling.calendarUnavailable"],
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
  ["import_source_busy", "import.sourceBusy"],
  // The server's detail lists the closed vocabulary by its wire spellings.
  ["win_evidence_required", "deals.winEvidenceRequired"],
  ["run_in_flight", "import.reason.run_in_flight"],
]);
