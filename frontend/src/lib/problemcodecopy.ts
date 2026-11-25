// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MessageKey } from "../i18n/en";
import { problemCode, problemDetails, problemFieldErrors } from "./problembody";

// Wire codes whose server detail is not a sentence for this screen's reader,
// and the catalog copy problemDetail (problem.ts) shows in its place.
export const PROBLEM_CODE_COPY = new Map<string, MessageKey>([
  ["deal_contracts_cross_company", "deals.contractCompanyBlocked"],
  ["company_merge_contract_mismatch", "companies.contractMergeBlocked"],
  ["booking_horizon", "scheduling.windowHorizon"],
  ["booking_notice", "scheduling.windowNotice"],
  ["booking_limits", "scheduling.windowLimits"],
  ["length_not_offered", "book.lengthNotOffered"],
  ["approval_preview_incomplete", "approval.preview.incomplete"],
  ["approval_preview_unavailable", "approval.preview.unavailable"],
  ["approval_preview_conflict", "approval.preview.resolveConflict"],
  ["approval_needs_wider_sight", "approval.preview.needsWiderSight"],
  // An edit that moves an entity id; the detail names JSON paths.
  ["retargeted", "approval.editRetargeted"],
  ["gateway_unavailable", "common.gatewayUnavailable"],
  ["request_timed_out", "common.requestTimedOut"],
  ["assistant_unavailable", "common.assistantUnavailable"],
  // No model is connected; the screens with a button for model work add the link.
  ["ai_not_configured", "connectAi.first"],
  ["ai_allowance_used_up", "common.aiAllowanceUsedUp"],
  ["provider_out_of_credit", "common.providerOutOfCredit"],
  ["provider_unauthorized", "common.providerUnauthorized"],
  ["provider_unavailable", "common.providerUnavailable"],
  ["calendar_unavailable", "scheduling.calendarUnavailable"],
  ["permission_denied", "common.permissionDenied"],
  ["consumer_mail_domain", "captureValue.consumerMail"],
  ["seat_tier_insufficient", "common.seatReadOnly"],
  ["trial_ended", "common.trialEnded"],
  ["license_lapsed", "common.licenseLapsed"],
  ["role_cannot_capture", "connectors.roleCannotCapture"],
  // The installation switched off individual performance; the detail is English.
  ["individual_performance_off", "common.individualPerformanceOff"],
  // An owner filter naming someone the reader may not measure; the detail is English.
  ["owner_not_measurable", "common.ownerNotMeasurable"],
  // The server's skew detail is English, worded for API and agent readers.
  ["version_skew", "common.versionSkew"],
  // The detail names a stage or a kind of stage in English.
  ["stage_not_in_pipeline", "deal.stageOtherPipeline"],
  ["pipeline_retired", "deal.pipelineRetired"],
  ["last_closing_stage", "stage.lastClosing"],
  // Every code httperr.Duplicate sends: its detail is one fixed API sentence
  // for all of them. backend/gates/frontendproblemcodecopy_test.go holds the set.
  ["duplicate_email", "common.duplicate.email"],
  ["duplicate_domain", "common.duplicate.domain"],
  ["duplicate_linkedin_url", "common.duplicate.linkedinUrl"],
  ["lead_holds_email", "common.duplicate.leadHolds"],
  ["contact_holds_email", "common.duplicate.contactHolds"],
  ["slot_taken", "common.duplicate.slotTaken"],
  ["import_source_busy", "import.sourceBusy"],
  // The team chat link refusals: each detail is an English API sentence.
  ["channel_bot_not_connected", "channelLink.refused.botNotConnected"],
  ["channel_not_found", "channelLink.refused.channelNotFound"],
  ["channel_name_refused", "channelLink.refused.nameRefused"],
  ["channel_not_private", "channelLink.refused.channelNotPrivate"],
  ["channel_bot_not_member", "channelLink.refused.botNotMember"],
  ["channel_already_linked", "channelLink.refused.channelAlreadyLinked"],
  ["record_already_linked", "channelLink.refused.recordAlreadyLinked"],
  ["channel_provider_unreachable", "channelLink.refused.providerUnreachable"],
  ["website_form_busy", "webForms.handlingBusy"],
  // The server's detail lists the closed vocabulary by its wire spellings.
  ["win_evidence_required", "deals.winEvidenceRequired"],
  // The detail is one English sentence per review state; legalReviewRefusalCopy
  // picks the state's own sentence when the fault names it.
  ["legal_review_required", "legalReview.advanceRefused"],
  ["run_in_flight", "import.reason.run_in_flight"],
  // A date-only meeting cannot hold its host's calendar hour.
  ["day_holds_host_slot", "activity.dayHoldsHostSlot"],
  // The member invite's refusals: the detail is English and names no screen.
  ["already_invited", "users.alreadyInvited"],
  ["email_taken", "users.emailTaken"],
  ["removed_member_email", "users.removedEmail"],
  // A legal entity's refusals: the detail is English and names no screen.
  ["legal_entity_has_members", "legalEntities.hasMembers"],
  ["legal_entity_is_assessed_against", "legalEntities.assessedAgainst"],
  ["legal_entity_name_taken", "legalEntities.nameTaken"],
  ["invalid_iban", "legalEntities.invalidIban"],
  ["invalid_bic", "legalEntities.invalidBic"],
  ["invalid_country", "legalEntities.invalidCountry"],
  ["invalid_language", "legalEntities.invalidLanguage"],
  // The phone parser's details are English and speak of E.164.
  ["phone_empty", "common.phone.empty"],
  ["phone_needs_country_code", "common.phone.needsCountryCode"],
  ["phone_malformed", "common.phone.malformed"],
  // The detail speaks of a promotion; the screen calls it a qualification.
  ["contact_has_deal", "lead.demoteContactHasDeal"],
  // A contact's address, profile, country and date refusals: the detail names
  // wire keys and the request schema.
  ["email_names_no_one", "create.emailNamesNoOne"],
  ["address_erased", "create.addressErased"],
  ["linkedin_url_malformed", "create.linkedinMalformed"],
  ["linkedin_url_not_linkedin", "create.linkedinNotLinkedin"],
  ["linkedin_url_not_profile", "create.linkedinNotProfile"],
  ["invalid_date_range", "create.dateRangeReversed"],
  ["domain_malformed", "create.domainMalformed"],
  // params.name names the chosen parent, which already sits below the company.
  ["parent_company_below", "create.parentCompanyBelow"],
  ["parent_company_self", "create.parentCompanySelf"],
  ["delivery_team_gone", "delivery.teamGone"],
  ["invalid_country_code", "legalEntities.invalidCountry"],
  // A Deal Room refusal: the detail is an English API sentence.
  ["deal_room_document_already_present", "room.docs.alreadyPresent"],
  // The detail says to regenerate, which a draft cannot; one sentence serves both.
  ["offer_lapsed", "offer.lapsed"],
  // A name or membership clash a screen words as its own field's refusal.
  ["pipeline_name_taken", "pipeline.nameTaken"],
  ["pipeline_default_taken", "pipeline.defaultTaken"],
  ["custom_field_label_taken", "cf.labelTaken"],
  ["already_on_list", "lists.alreadyOnShortlist"],
  // The currency rate refusals name wire fields and the server's digit limits.
  ["fx_rate_base_self", "settings.rates.fromIsBase"],
  ["fx_rate_currency", "settings.rates.fromNotCurrency"],
  ["fx_rate_positive", "settings.rates.rateNotPositive"],
  ["fx_rate_past", "settings.rates.pastDate"],
  // A member's own profile that is not a LinkedIn profile.
  ["linkedin_profile_url_invalid", "common.linkedinProfileInvalid"],
]);

// The held refusal names no record; its words depend only on whether the
// colleague who holds the contact was asked to share it.
export function heldByColleagueCopy(problem: unknown): MessageKey | undefined {
  if (problemCode(problem) !== "contact_held_by_colleague") return undefined;
  return problemDetails(problem)?.share_requested === true
    ? "common.heldByColleague.asked"
    : "common.heldByColleague";
}

// The next step a legal refusal asks for, by the review state it names. An
// older server and the create refusal name none, and keep the generic sentence.
const LEGAL_REVIEW_STATE_COPY: Readonly<Record<string, MessageKey>> = {
  none: "legalReview.advanceRefused",
  pending: "legalReview.advanceRefused.pending",
  changes_needed: "legalReview.advanceRefused.changesNeeded",
  terms_changed: "legalReview.advanceRefused.termsChanged",
};

export function legalReviewRefusalCopy(
  problem: unknown,
): MessageKey | undefined {
  const fault = problemFieldErrors(problem).find(
    (f) => f.code === "legal_review_required",
  );
  const state = fault?.params?.review_state;
  return state !== undefined && Object.hasOwn(LEGAL_REVIEW_STATE_COPY, state)
    ? LEGAL_REVIEW_STATE_COPY[state]
    : undefined;
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
