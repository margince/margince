// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

import "strings"

// syntheticAuditFields mirrors the keys of SYNTHETIC_AUDIT_FIELD_LABELS
// (frontend/src/lib/historyfieldlabels.ts): keys a hand-built audit image
// writes that the History tab has a word for.
// Held by TestTheServerShowsExactlyTheSyntheticKeysTheHistoryTabLabels.
var syntheticAuditFields = setOf(
	"about_deal", "acquisition_request_retracted", "admission", "admission_reason",
	"admission_reason_code", "admission_sender_kind", "admission_source", "attendees", "bounce",
	"capture_question", "channel_identity", "channel_username", "cohort_linked", "cohort_promoted",
	"commitment_evidence_unavailable", "communication_basis", "confirm_submission",
	"consent_records_carried", "corrected", "decided_by_level", "disposition", "domain", "filled",
	"guest_answers_changed", "guest_links_added", "guests_added", "guests_left_out", "guests_retired",
	"invitation_status", "lifted_by", "lifted_by_level", "lifted_suppression", "link_suggestion",
	"meeting_documents_added", "meeting_guests_removed", "merged_from_id", "merged_into_id", "note",
	"nudge_dismissal", "override_category", "overrides_carried", "proposal_status",
	"provider_claims_received", "qualifying_event", "qualifying_event_retracted", "reachability",
	"readiness", "reason", "recorded_at_level", "record_link", "relinked_activity_links", "relinked_emails",
	"relinked_phones", "relinked_relationships", "reminder_status", "reply_verdict", "reply_verdict_by",
	"research_claims_accepted", "resolved_category", "revoked_by", "revoked_by_level",
	"revoked_override", "revoked_rows", "scope", "stopped", "stops_carried", "submission_decision",
	"submission_id", "suppression_kind", "vat_checked_at", "vat_consultation_number", "vat_number",
	"vat_registered_address", "vat_registered_name", "vat_requested", "vat_status",
)

// sharedInternalColumns are columns every record table carries for the
// machinery: identity, provenance, search and lifecycle stamps. A reader meets a
// create or an archive as its own line, never as a changed field. The address
// columns travel in an image as the contract's one `address` object.
var sharedInternalColumns = setOf(append(lifecycleStamps,
	"id", "captured_by", "search_tsv", "legal_hold",
	"source_author_id", "source_author_name", "source_id", "source_system",
	"address_city", "address_country", "address_line1", "address_line2",
	"address_postal_code", "address_region",
)...)

// lifecycleStamps is one string because the entered_at writer census reads a
// bare column literal as a patch field. A list of names classifies; it writes
// nothing.
var lifecycleStamps = strings.Fields("created_at archived_at last_activity_at entered_at quarantined_at")

// internalColumns are each record table's own internal columns: derived
// values, stamps, and the state of the jobs that sort and protect a record.
var internalColumns = map[string]map[string]bool{
	typeDeal: setOf(
		"amount_minor_base", "arr_source_offer_id", "close_date_provisional", "closed_at",
		"commercial_mode", "commercial_revision", "lost_reason_basis", "lost_reason_code",
		"pipeline_id", "priority_rank", "win_probability_set_by",
		"won_without_contract_detail", "won_without_contract_reason",
	),
	typeContact: setOf(
		"anonymized_address_hashes", "anonymized_address_key_id", "converted_from_lead_id", "created_from_activity_id",
		"photo_object_key", "photo_origin", "relationship_mark_set_by",
	),
	typeCompany: setOf(
		"geocode_input_hash", "geocode_lat", "geocode_lon", "geocode_provider", "geocode_status",
		"geocoded_at", "is_anchor", "logo_icon_object_key", "logo_icon_origin", "logo_object_key",
		"logo_origin", "name_source",
	),
	typeLead: setOf(
		"arrival_owner_id", "company_id", "disqualify_note", "disqualify_reason_id",
		"email_domain_folded", "first_response_at", "from_contact_id", "partner_accepted_at",
		"partner_accepted_by", "promoted_at", "promoted_contact_id", "qualified_deal_id", "routed_at",
		"score_computed", "sla_breached_at", "status_set_by",
	),
	typeProject: setOf("closed_reason", "company_id", "key", "phase"),
	typeActivity: setOf(
		"agenda_fingerprint", "audience", "body_completeness", "booking_interval_exact",
		"bulk_mail_attested", "calendar_method", "capture_label", "capture_label_declined_at",
		"capture_label_declined_ruleset", "capture_labeled_at", "channel_provider", "claims_host_slot",
		"counterparty_email", "counterparty_outbound_attested", "deal_topic_checked_at", "deal_topic_digest",
		"direction", "domain_evidence_version",
		"done_at", "duration_seconds", "foreign_link", "has_calendar_part", "host_user_id",
		"inbound_sender_authenticated", "internal_mail_authorization", "invitation_sender", "kind",
		"language", "origin", "owed_verdict_at", "owed_verdict_declined_at",
		"owed_verdict_declined_ruleset", "raw", "raw_capture_id", "redacted_fields", "reply_verdict_at",
		"restricted_at", "restricted_reason", "restricted_until", "retention_class",
		"retention_class_at", "sender_is_machine", "sender_was_stranger", "sent_by_colleague",
		"source_activity_id", "thread_key",
	),
}

func setOf(keys ...string) map[string]bool {
	out := make(map[string]bool, len(keys))
	for _, key := range keys {
		out[key] = true
	}
	return out
}
