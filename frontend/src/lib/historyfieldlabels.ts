import { profileFieldLabelKey } from "../format/companyprofilelabels";
import type { MessageKey } from "../i18n/en";
import { ADDRESS_COLUMN_LABELS } from "./addresscolumns";

// What a changed field is CALLED, for a reader who never sees a column name
// anywhere else on the record.
//
// The audit spine projects WIRE names — `amount_minor`, `owner_id` — because a
// contract shape carries the name the API binds, not a display label. The
// record page beside it names the same fields in the reader's own language, so
// a history that printed the column left one surface speaking two vocabularies
// about one field.
//
// It is its own map rather than a reuse of the three that already exist,
// because none of them is about this subject. `COLD_FIELD_LABELS`
// (lib/coldfieldlabels.ts) names the enrichment vocabulary.
// `PROFILE_FIELD_LABELS` (features/companies/companies.profile.tsx) names company profile facts,
// and today.merge's map names the fields a merge compares. This one names the
// columns a record's UPDATE writes. `historyfieldlabels.test.ts` derives that
// set from the contract, so a field added upstream fails the gate instead of
// reaching a reader raw.
//
// A Map, not an object: `get` answers `MessageKey | undefined` with no cast,
// and no prototype key can answer for a field the server never sent.
const HISTORY_FIELD_LABELS = new Map<string, MessageKey>([
  ["address", "history.field.address"],
  ["amount_minor", "history.field.amount_minor"],
  ["expected_arr_minor", "history.field.expected_arr_minor"],
  [
    "extension_relationship_types",
    "history.field.extension_relationship_types",
  ],
  ["assignee_id", "history.field.assignee_id"],
  ["body", "history.field.body"],
  ["meeting_notes", "history.field.meeting_notes"],
  ["candidate_company_key", "history.field.candidate_company_key"],
  ["company_name", "history.field.company_name"],
  ["currency", "history.field.currency"],
  ["description", "history.field.description"],
  ["display_name", "history.field.display_name"],
  ["domains", "history.field.domains"],
  ["due_at", "history.field.due_at"],
  ["due_intent", "history.field.due_intent"],
  ["email", "history.field.email"],
  ["emails", "history.field.emails"],
  // A lead's one number, beside the contact's `phones` list below.
  ["phone", "history.field.phone"],
  ["ended_at", "history.field.ended_at"],
  ["expected_close_date", "history.field.expected_close_date"],
  ["first_name", "history.field.first_name"],
  ["full_name", "history.field.full_name"],
  ["fx_rate_date", "history.field.fx_rate_date"],
  ["fx_rate_to_base", "history.field.fx_rate_to_base"],
  ["greeting_register", "history.field.greeting_register"],
  ["industry", "history.field.industry"],
  ["assess_against_entity_id", "history.field.assess_against_entity_id"],
  ["is_done", "history.field.is_done"],
  ["last_name", "history.field.last_name"],
  ["legal_name", "history.field.legal_name"],
  ["lifecycle", "history.field.lifecycle"],
  ["linkedin_url", "history.field.linkedin_url"],
  ["lost_reason", "history.field.lost_reason"],
  ["meeting_status", "history.field.meeting_status"],
  ["meeting_intent", "history.field.meeting_intent"],
  ["call_outcome", "history.field.call_outcome"],
  ["name", "history.field.name"],
  ["occurred_at", "history.field.occurred_at"],
  ["occurred_precision", "history.field.occurred_precision"],
  ["company_id", "history.field.company_id"],
  ["owner_id", "history.field.owner_id"],
  ["parent_company_id", "history.field.parent_company_id"],
  ["relationship_mark", "history.field.relationship_mark"],
  ["salutation", "history.field.salutation"],
  ["partner_attribution", "history.field.partner_attribution"],
  ["partner_company_id", "history.field.partner_company_id"],
  ["phones", "history.field.phones"],
  ["project_id", "history.field.project_id"],
  ["product_id", "history.field.product_id"],
  ["relationship_types", "history.field.relationship_types"],
  ["due_has_time", "history.field.due_has_time"],
  ["task_type", "history.field.task_type"],
  ["ai_assisted", "history.field.ai_assisted"],
  ["remind_at", "history.field.remind_at"],
  ["remind_intent", "history.field.remind_intent"],
  ["score", "history.field.score"],
  ["score_override_reason", "history.field.score_override_reason"],
  ["size_band", "history.field.size_band"],
  ["social", "history.field.social"],
  ["source", "history.field.source"],
  ["origin", "history.field.origin"],
  ["origin_detail", "history.field.origin_detail"],
  ["started_at", "history.field.started_at"],
  ["status", "history.field.status"],
  ["subject", "history.field.subject"],
  ["target_end_date", "history.field.target_end_date"],
  ["title", "history.field.title"],
  ["visibility", "history.field.visibility"],
  ["wait_until", "history.field.wait_until"],
  ["description", "history.field.description"],
  ["commercial_motion", "history.field.commercial_motion"],
  ["priority", "history.field.priority"],
  ["acquisition_source", "history.field.acquisition_source"],
  ["win_probability", "history.field.win_probability"],
]);

// The columns an update stores under names of their own: the deal's
// `win_probability` is recorded as the figure set on the deal and who set it.
// Apart from both maps above, because no `Update<Type>Request` names these and
// no hand-built image writes them, so each census would call them orphans.
// The address parts are the record's columns; the request nests them under
// `address`, and they read the words the Address dialog reads.
const STORED_COLUMN_LABELS = new Map<string, MessageKey>([
  ...ADDRESS_COLUMN_LABELS,
  ["stage_id", "history.field.stage_id"],
  ["disqualify_reason_id", "history.field.disqualify_reason_id"],
  ["trigger", "history.field.trigger"],
  ["role", "history.field.role"],
  ["is_current_primary", "history.field.is_current_primary"],
  ["employment_status", "history.field.employment_status"],
  ["started_precision", "history.field.started_precision"],
  ["ended_precision", "history.field.ended_precision"],
  ["win_probability_override", "history.field.win_probability_override"],
  ["win_probability_set_by", "history.field.win_probability_set_by"],
]);

// Fields a HAND-BUILT audit image names — a write described by its own
// free-form keys rather than by the before/after image of a contract column.
// They come from all over the tree: a consent withdrawal, a VAT consultation, a
// bounce report, a cohort repair, a reply verdict.
//
// It is a SEPARATE map from HISTORY_FIELD_LABELS on purpose. The census below
// derives that one from what an `Update<Type>Request` actually writes, and its
// own "no word for an unwritten field" direction would fail the moment one of
// these appeared there.
//
// Held by TestEveryProjectedAuditKeyHasALabel
// (backend/gates/historyfieldlabels_test.go), which walks the audit doors,
// resolves the images they carry, and fails in both directions: a writer's key
// with no word here, and a word here no writer emits. It also refuses a key
// this map and the contract map both claim unless that collision is ratified —
// the shape `kind` took, which already belongs to every activity's own audited
// create and would have mislabelled every activity in history as a suppression.
// That is why a key here is not the bare word a writer's struct field suggests:
// this lookup carries no entity context, so `suppression_kind` rather than
// `kind`.
const SYNTHETIC_AUDIT_FIELD_LABELS = new Map<string, MessageKey>([
  ["source_changed", "history.field.chat_source_changed"],
  ["source_deleted", "history.field.chat_source_deleted"],
  ["kind", "history.field.kind"],
  ["capability", "co.factField.capability"],
  ["certification", "co.factField.certification"],
  ["company_size", "history.field.size_band"],
  ["contact_email", "co.factField.contact_email"],
  ["email_security", "history.field.email_security"],
  ["employee_range", "history.field.employee_range"],
  ["founded_year", "history.field.founded_year"],
  ["geography", "co.factField.geography"],
  ["hosting_provider", "history.field.hosting_provider"],
  ["location", "co.factField.location"],
  ["mail_provider", "history.field.mail_provider"],
  ["named_customer", "history.field.named_customer"],
  ["operated_service", "co.factField.operated_service"],
  ["partner", "history.field.partner_company_id"],
  ["product", "history.field.product_id"],
  ["quantified_outcome", "history.field.quantified_outcome"],
  ["served_industry", "history.field.served_industry"],
  ["service", "co.factField.service"],
  ["technology", "co.tech.title"],
  ["employment", "rel.kind.employment"],
  ["close_date_provisional", "history.field.close_date_provisional"],
  ["commercial_mode", "history.field.commercial_mode"],
  ["commercial_revision", "history.field.commercial_revision"],
  ["converted_from_lead_id", "history.field.converted_from_lead_id"],
  [
    "deals.maintenance_writes_enabled",
    "history.field.deals.maintenance_writes_enabled",
  ],
  ["deals.silent_close_days", "history.field.deals.silent_close_days"],
  ["linkedin", "history.field.social.linkedin"],
  ["website", "about.website"],
  ["CompanyName", "history.field.company_name"],
  ["Email", "history.field.email"],
  ["FullName", "history.field.full_name"],
  ["Origin", "history.field.origin"],
  ["OriginDetail", "history.field.origin_detail"],
  ["Title", "history.field.title"],
  ["Unowned", "history.field.Unowned"],
  ["activated_at", "history.field.activated_at"],
  ["activation_basis", "history.field.activation_basis"],
  ["agenda_fingerprint", "history.field.agenda_fingerprint"],
  ["amount_minor_base", "history.field.amount_minor_base"],
  ["applied_change", "history.field.applied_change"],
  ["archived_at", "history.field.archived_at"],
  ["arr_source_offer_id", "history.field.arr_source_offer_id"],
  ["audience", "history.field.audience"],
  ["audience_reason", "history.field.audience_reason"],
  ["auto_kept", "history.field.auto_kept"],
  ["auto_reopened", "history.field.auto_reopened"],
  ["body_completeness", "history.field.body_completeness"],
  ["body_edited_at_provider", "history.field.body_edited_at_provider"],
  ["body_restored", "history.field.body_restored"],
  ["calendar_method", "history.field.calendar_method"],
  ["captured_by", "history.field.captured_by"],
  ["cert_status", "history.field.cert_status"],
  ["certified_staff", "history.field.certified_staff"],
  ["claim_erased", "history.field.claim_erased"],
  ["claim_id", "history.field.claim_id"],
  ["claim_kind", "history.field.claim_kind"],
  ["claim_status", "history.field.claim_status"],
  ["closed_at", "history.field.closed_at"],
  ["commitment_admission", "history.field.commitment_admission"],
  ["commitment_admission_erased", "history.field.commitment_admission_erased"],
  [
    "commitment_admission_redacted",
    "history.field.commitment_admission_redacted",
  ],
  ["commitment_locator", "history.field.commitment_locator"],
  ["confidence", "lead.signalConfidence"],
  ["corroboration", "history.field.corroboration"],
  ["counterparty_email", "history.field.counterparty_email"],
  [
    "counterparty_outbound_attested",
    "history.field.counterparty_outbound_attested",
  ],
  ["credited_seats", "history.field.credited_seats"],
  ["decided_by", "history.field.decided_by_level"],
  ["description_rendered", "history.field.description_rendered"],
  ["direction", "analytics.field.direction"],
  ["dismissed_until", "history.field.dismissed_until"],
  ["done_at", "history.field.done_at"],
  ["duration_seconds", "history.field.duration_seconds"],
  ["employment_import", "history.field.employment_import"],
  ["evidence", "stat.evidence"],
  ["facts", "ob.conv.triage.factsLabel"],
  ["first_response_at", "history.field.first_response_at"],
  ["first_response_marked_by", "history.field.first_response_marked_by"],
  ["from_contact_id", "history.field.from_contact_id"],
  ["handling_automation_id", "history.field.handling_automation_id"],
  ["host_user_id", "history.field.host_user_id"],
  ["internal_mail_authorization", "history.field.internal_mail_authorization"],
  ["invitation_response", "history.field.invitation_response"],
  ["invitation_sender", "history.field.invitation_sender"],
  ["judged_evidence", "history.field.judged_evidence"],
  ["judged_through_activity_id", "history.field.judged_through_activity_id"],
  ["language", "offer.language"],
  ["lead_id", "history.field.lead_id"],
  ["lift_basis", "history.field.lift_basis"],
  ["logo", "legalEntities.sectionLogo"],
  ["logo_icon", "history.field.logo_icon"],
  ["lost_reason_basis", "history.field.lost_reason_basis"],
  ["lost_reason_code", "history.field.lost_reason_code"],
  ["mailbox_seats", "history.field.mailbox_seats"],
  ["manager_request_reasons", "history.field.manager_request_reasons"],
  ["manager_requested_at", "history.field.manager_requested_at"],
  ["manager_requested_by", "history.field.manager_requested_by"],
  ["manual_signal", "history.field.manual_signal"],
  ["manual_signals", "history.field.manual_signals"],
  ["margin_tier", "partner.marginTier"],
  ["meeting_documents_removed", "history.field.meeting_documents_removed"],
  ["meeting_documents_restored", "history.field.meeting_documents_restored"],
  ["members", "users.membersTitle"],
  ["moved_by_owner", "history.field.moved_by_owner"],
  ["name_source", "history.field.name_source"],
  ["narrowing_reason", "history.field.narrowing_reason"],
  ["next_step", "lead.nextTask"],
  ["next_step_due_at", "history.field.next_step_due_at"],
  ["owed_verdict", "history.field.owed_verdict"],
  ["owed_verdict_ruleset", "history.field.owed_verdict_ruleset"],
  [
    "participant_addresses_not_lowercase",
    "history.field.participant_addresses_not_lowercase",
  ],
  ["participant_seats", "history.field.participant_seats"],
  ["participant_users", "history.field.participant_users"],
  ["partner_accepted_at", "history.field.partner_accepted_at"],
  ["partner_accepted_by", "history.field.partner_accepted_by"],
  ["partner_archived_at", "history.field.partner_archived_at"],
  ["partner_kinds", "partner.kinds"],
  ["pending_reason", "history.field.pending_reason"],
  ["permanent", "history.field.permanent"],
  ["preferred_slots", "history.field.preferred_slots"],
  ["products", "product.title"],
  ["proven_sender", "history.field.proven_sender"],
  ["purge_reason", "history.field.purge_reason"],
  ["qualified_deal_id", "history.field.qualified_deal_id"],
  ["relationship_id", "history.field.relationship_id"],
  ["relationship_stage", "partner.stage"],
  ["released_by", "history.field.released_by"],
  ["relinked_contracts", "history.field.relinked_contracts"],
  ["relinked_offers", "history.field.relinked_offers"],
  ["relinked_stakeholders", "history.field.relinked_stakeholders"],
  ["remaining", "reporting.remaining"],
  ["reopen_on", "history.field.reopen_on"],
  ["retention_rate", "history.field.retention_rate"],
  ["revalidation_requested_at", "history.field.revalidation_requested_at"],
  ["rule_version", "history.field.rule_version"],
  ["score_computed", "history.field.score_computed"],
  ["sent_by_colleague", "history.field.sent_by_colleague"],
  ["served_segments", "partner.servedSegments"],
  ["sla_breached_at", "history.field.sla_breached_at"],
  ["snoozed_intent", "history.field.snoozed_intent"],
  ["snoozed_until", "history.field.snoozed_until"],
  ["social.linkedin", "history.field.social.linkedin"],
  ["source_activity_id", "history.field.source_activity_id"],
  ["source_id", "history.field.source_id"],
  ["source_revision", "history.field.source_revision"],
  ["source_system", "history.field.source_system"],
  ["status_set_by", "history.field.status_set_by"],
  ["subject_redecoded", "history.field.subject_redecoded"],
  ["system_text", "history.field.system_text"],
  ["taken_at", "history.field.taken_at"],
  ["taken_by_user_id", "history.field.taken_by_user_id"],
  ["task_activity_id", "history.field.task_activity_id"],
  ["updated_at", "history.field.updated_at"],
  ["verdict", "history.field.verdict"],
  ["version", "about.version"],
  [
    "commitment_evidence_unavailable",
    "history.field.commitment_evidence_unavailable",
  ],
  ["proposal_status", "history.field.proposal_status"],
  ["reminder_status", "history.field.reminder_status"],
  ["invitation_status", "history.field.invitation_status"],
  ["attendees", "history.field.attendees"],
  ["admission", "history.field.admission"],
  ["admission_reason", "history.field.admission_reason"],
  ["admission_reason_code", "history.field.admission_reason_code"],
  ["admission_sender_kind", "history.field.admission_sender_kind"],
  ["admission_source", "history.field.admission_source"],
  ["bounce", "history.field.bounce"],
  ["capture_question", "history.field.capture_question"],
  ["channel_identity", "history.field.channel_identity"],
  ["channel_username", "history.field.channel_username"],
  ["cohort_linked", "history.field.cohort_linked"],
  ["cohort_promoted", "history.field.cohort_promoted"],
  ["communication_basis", "history.field.communication_basis"],
  ["confirm_submission", "history.field.confirm_submission"],
  ["corrected", "history.field.corrected"],
  ["decided_by_level", "history.field.decided_by_level"],
  ["disposition", "history.field.disposition"],
  ["link_suggestion", "history.field.link_suggestion"],
  ["record_link", "history.field.record_link"],
  ["domain", "history.field.domain"],
  ["merged_from_id", "history.field.merged_from_id"],
  ["merged_into_id", "history.field.merged_into_id"],
  ["filled", "history.field.filled"],
  ["guests_added", "history.field.guests_added"],
  ["guest_links_added", "history.field.guest_links_added"],
  ["meeting_documents_added", "history.field.meeting_documents_added"],
  ["guests_retired", "history.field.guests_retired"],
  ["meeting_guests_removed", "history.field.meeting_guests_removed"],
  ["guest_answers_changed", "history.field.guest_answers_changed"],
  ["guests_left_out", "history.field.guests_left_out"],
  ["relinked_emails", "history.field.relinked_emails"],
  ["relinked_phones", "history.field.relinked_phones"],
  ["relinked_relationships", "history.field.relinked_relationships"],
  ["relinked_activity_links", "history.field.relinked_activity_links"],
  ["legal_entity_id", "history.field.legal_entity_id"],
  ["lifted_by", "history.field.lifted_by"],
  ["lifted_by_level", "history.field.lifted_by_level"],
  ["lifted_suppression", "history.field.lifted_suppression"],
  ["restriction_lift_requested", "history.field.restriction_lift_requested"],
  ["note", "history.field.note"],
  ["nudge_dismissal", "history.field.nudge_dismissal"],
  ["overrides_carried", "history.field.overrides_carried"],
  ["provider_claims_received", "history.field.provider_claims_received"],
  ["about_deal", "history.field.about_deal"],
  ["qualifying_event", "history.field.qualifying_event"],
  ["qualifying_event_retracted", "history.field.qualifying_event_retracted"],
  [
    "acquisition_request_retracted",
    "history.field.acquisition_request_retracted",
  ],
  [
    "impersonation_warning_carried",
    "history.field.impersonation_warning_carried",
  ],
  [
    "impersonation_warning_raised",
    "history.field.impersonation_warning_raised",
  ],
  [
    "impersonation_warning_cleared",
    "history.field.impersonation_warning_cleared",
  ],
  ["reachability", "history.field.reachability"],
  ["readiness", "history.field.readiness"],
  ["reason", "history.field.reason"],
  ["recorded_at_level", "history.field.recorded_at_level"],
  ["reply_verdict", "history.field.reply_verdict"],
  ["reply_verdict_by", "history.field.reply_verdict_by"],
  ["research_claims_accepted", "history.field.research_claims_accepted"],
  ["resolved_category", "history.field.resolved_category"],
  ["revoked_by", "history.field.revoked_by"],
  ["revoked_by_level", "history.field.revoked_by_level"],
  ["revoked_override", "history.field.revoked_override"],
  ["revoked_rows", "history.field.revoked_rows"],
  ["scope", "history.field.scope"],
  ["stopped", "history.field.stopped"],
  ["stops_carried", "history.field.stops_carried"],
  ["consent_records_carried", "history.field.consent_records_carried"],
  ["submission_decision", "history.field.submission_decision"],
  ["submission_id", "history.field.submission_id"],
  ["suppression_kind", "history.field.suppression_kind"],
  ["vat_checked_at", "history.field.vat_checked_at"],
  ["vat_consultation_number", "history.field.vat_consultation_number"],
  ["vat_number", "history.field.vat_number"],
  ["vat_registered_address", "history.field.vat_registered_address"],
  ["vat_registered_name", "history.field.vat_registered_name"],
  ["vat_requested", "history.field.vat_requested"],
  ["vat_status", "history.field.vat_status"],
]);

// The label a history row shows for one field.
//
// A company's profile fields (VAT ID, registered address and the rest) are
// shown in its history under the names the profile tab gives them.
//
// A field with no key falls back to its own name with the underscores spaced
// out — the same contract `coldFieldLabel` keeps, and the only honest answer
// for a workspace's own `cf_` column, whose label lives in the custom-field
// catalog rather than in any catalog this build ships.
export function historyFieldLabel(
  field: string,
  t: (key: MessageKey) => string,
): string {
  const key = fieldWordKey(field);
  return key ? t(key) : field.replaceAll("_", " ");
}

function fieldWordKey(field: string): MessageKey | undefined {
  return (
    HISTORY_FIELD_LABELS.get(field) ??
    STORED_COLUMN_LABELS.get(field) ??
    SYNTHETIC_AUDIT_FIELD_LABELS.get(field) ??
    profileFieldLabelKey(field)
  );
}

/** Whether a column has a word of its own, rather than its stored name with spaces. */
export function hasFieldWord(field: string): boolean {
  return fieldWordKey(field) !== undefined;
}

// Field pickers and History share labels for request and stored columns.
export function historyFieldLabelKey(field: string): MessageKey | undefined {
  return fieldWordKey(field);
}

// The synthetic map's own keys, for the test that holds each one to an i18n
// key that actually exists — the one direction a Go-literal vocabulary can
// still be checked from this side of the contract.
export function syntheticAuditFieldLabelled(): string[] {
  return [...SYNTHETIC_AUDIT_FIELD_LABELS.keys()];
}

// Every field this map claims a word for — the census reads it to hold the
// map against the contract in BOTH directions.
export function historyFieldLabelled(): string[] {
  return [...HISTORY_FIELD_LABELS.keys()];
}
