// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MessageKey } from "../i18n/en";
import { problemCode, problemDetails } from "./problembody";

// Wire codes whose server detail is not a sentence for this screen's reader,
// and the catalog copy problemDetail (problem.ts) shows in its place.
export const PROBLEM_CODE_COPY = new Map<string, MessageKey>([
  ["approval_preview_incomplete", "approval.preview.incomplete"],
  ["approval_preview_unavailable", "approval.preview.unavailable"],
  ["approval_preview_conflict", "approval.preview.resolveConflict"],
  ["approval_needs_wider_sight", "approval.preview.needsWiderSight"],
  ["gateway_unavailable", "common.gatewayUnavailable"],
  ["assistant_unavailable", "common.assistantUnavailable"],
  ["provider_out_of_credit", "common.providerOutOfCredit"],
  ["provider_unauthorized", "common.providerUnauthorized"],
  ["provider_unavailable", "common.providerUnavailable"],
  ["calendar_unavailable", "scheduling.calendarUnavailable"],
  ["permission_denied", "common.permissionDenied"],
  ["seat_tier_insufficient", "common.seatReadOnly"],
  ["role_cannot_capture", "connectors.roleCannotCapture"],
  // The installation switched off individual performance; the detail is English.
  ["individual_performance_off", "common.individualPerformanceOff"],
  // The server's skew detail is English, worded for API and agent readers.
  ["version_skew", "common.versionSkew"],
  // Every code httperr.Duplicate sends: its detail is one fixed API sentence
  // for all of them. backend/gates/frontendproblemcodecopy_test.go holds the set.
  ["duplicate_email", "common.duplicate.email"],
  ["duplicate_domain", "common.duplicate.domain"],
  ["duplicate_linkedin_url", "common.duplicate.linkedinUrl"],
  ["lead_holds_email", "common.duplicate.leadHolds"],
  ["contact_holds_email", "common.duplicate.contactHolds"],
  ["slot_taken", "common.duplicate.slotTaken"],
  ["import_source_busy", "import.sourceBusy"],
  // The server's detail lists the closed vocabulary by its wire spellings.
  ["win_evidence_required", "deals.winEvidenceRequired"],
  ["run_in_flight", "import.reason.run_in_flight"],
  // A date-only meeting cannot hold its host's calendar hour.
  ["day_holds_host_slot", "activity.dayHoldsHostSlot"],
  // A legal entity's refusals: the detail is English and names no screen.
  ["legal_entity_has_members", "legalEntities.hasMembers"],
  ["legal_entity_is_assessed_against", "legalEntities.assessedAgainst"],
  ["legal_entity_name_taken", "legalEntities.nameTaken"],
  ["invalid_iban", "legalEntities.invalidIban"],
  ["invalid_bic", "legalEntities.invalidBic"],
  ["invalid_country", "legalEntities.invalidCountry"],
  ["invalid_language", "legalEntities.invalidLanguage"],
]);

// The held refusal names no record; its words depend only on whether the
// colleague who holds the contact was asked to share it.
export function heldByColleagueCopy(problem: unknown): MessageKey | undefined {
  if (problemCode(problem) !== "contact_held_by_colleague") return undefined;
  return problemDetails(problem)?.share_requested === true
    ? "common.heldByColleague.asked"
    : "common.heldByColleague";
}

// The grant a capture refusal names; an older server sends none, and the
// sentence then names no grant.
const CAPTURE_REFUSAL_COPY: Readonly<Record<string, MessageKey>> = {
  activity: "connectors.roleCannotCapture.activity",
  contact: "connectors.roleCannotCapture.contact",
  lead: "connectors.roleCannotCapture.lead",
  seat: "connectors.roleCannotCapture.seat",
};

export function captureRefusalCopy(missing: unknown): MessageKey {
  const named =
    typeof missing === "string" && Object.hasOwn(CAPTURE_REFUSAL_COPY, missing)
      ? CAPTURE_REFUSAL_COPY[missing]
      : undefined;
  return named ?? "connectors.roleCannotCapture";
}

// A capture refusal names the grant the member lacks when the server says which.
export function roleCannotCaptureCopy(
  problem: unknown,
): MessageKey | undefined {
  if (problemCode(problem) !== "role_cannot_capture") return undefined;
  return captureRefusalCopy(problemDetails(problem)?.missing);
}
