// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// WHICH module owns each table's writes — the registry alone.
//
// Apart from the walk that enforces it (tableownership_test.go) because it is
// data rather than logic: 400-odd lines a reader consults one line of, in front
// of a gate a reader reads whole. The two are one gate; only the file is split.

// tableOwners maps every core-migration table to the ONE module whose store
// owns its writes (module doc.go "Tables owned" declarations, kept in sync).
// This map is the hand-maintained artifact: a new table gets an owner here
// before its first write lands.
// gatekit:fixture the owning module path each table's writes are compared
// against — expected data, not a cost anyone is paying.
var tableOwners = map[string]string{
	// identity
	"workspace":          "internal/modules/identity",
	"app_user":           "internal/modules/identity",
	"team":               "internal/modules/identity",
	"team_membership":    "internal/modules/identity",
	"session":            "internal/modules/identity",
	"passport":           "internal/modules/identity",
	"setup_token":        "internal/modules/identity",
	"auth_token":         "internal/modules/identity",
	"role":               "internal/modules/identity",
	"role_assignment":    "internal/modules/identity",
	"federated_identity": "internal/modules/identity",
	// The columns a role reads as withheld; written by administration, read
	// by the grant loader into the principal.
	"field_mask":               "internal/modules/identity",
	"record_grant":             "internal/modules/identity",
	"oauth_client":             "internal/modules/identity",
	"oauth_authorization_code": "internal/modules/identity",
	"oauth_grant":              "internal/modules/identity",
	"oauth_refresh_token":      "internal/modules/identity",
	"onboarding_wizard_state":  "internal/modules/identity",
	// people
	"person":        "internal/modules/people",
	"person_email":  "internal/modules/people",
	"person_social": "internal/modules/people",
	"person_phone":  "internal/modules/people",
	// The channel identity is a resolution key on the person, not connection
	// state: it answers "which Person is this Telegram user", so it lives with
	// the one dedupe implementation that resolves them.
	"person_channel_identity": "internal/modules/people",
	// What was promised, asked and decided in captured conversations
	// (ADR-0097 D1). It lives with people because a claim is an attribute of
	// the PERSON it is about, written through the same store that owns them.
	"conversation_claim":             "internal/modules/people",
	"organization":                   "internal/modules/people",
	"organization_domain":            "internal/modules/people",
	"organization_relationship_type": "internal/modules/people",
	"signal_thread_scan":             "internal/compose",
	// One reader's frozen walk through their worklist. Owned by the compose
	// package that writes it, the way compose/weekly owns team_weekly_review.
	"worklist_snapshot":      "internal/compose/worklistsnap",
	"relationship":           "internal/modules/people",
	"partner":                "internal/modules/people",
	"lead":                   "internal/modules/people",
	"lead_score_history":     "internal/modules/people",
	"lead_manual_signal":     "internal/modules/people",
	"lead_source":            "internal/modules/people",
	"lead_disqualify_reason": "internal/modules/people",
	// A prospect passed from an SDR to an account executive: the row carrying
	// its current state, the append-only transitions behind it, and the
	// administered reason a refusal names. people owns them because the SUBJECT
	// is a lead or a person; the deal an acceptance produces is an outcome, and
	// compose wires the caller that does both.
	"sdr_handoff":                "internal/modules/people",
	"sdr_handoff_event":          "internal/modules/people",
	"sdr_handoff_reason":         "internal/modules/people",
	"organization_profile_field": "internal/modules/people",
	"organization_vat_check":     "internal/modules/people",
	"person_profile_field":       "internal/modules/people",
	// The signature pass's per-person read cursor (PO-F-2a): which mail was
	// already shown to the model, so the same empty signature is not re-read
	// every night.
	"person_signature_enrich_state": "internal/modules/people",
	"organization_fact":             "internal/modules/people",
	"organization_geocode_state":    "internal/modules/people",
	"geocode_cache":                 "internal/modules/people",
	// What a technical lookup last read for one company, per public source,
	// and what those sources answered. The cache is installation-global for
	// the same reason geocode_cache is — a domain's DNS records are the same
	// for every tenant — and people owns both because it owns the company
	// record they describe.
	"organization_technical_state": "internal/modules/people",
	"technical_lookup_cache":       "internal/modules/people",
	// What a mail domain is allowed to create. It governs ORGANIZATION
	// creation, which people owns, so the verdict lives with the records it
	// authorizes rather than with the capture path that asks the question.
	"organization_domain_disposition": "internal/modules/people",
	"site_read":                       "internal/modules/people",
	// DH-DDL-1: the pair verdicts live with the ONE dedupe implementation.
	"dedupe_candidate": "internal/modules/people",
	// deals (incl. the E03 offer engine: rate-card + versioned offers)
	"commission_entry":          "internal/modules/commissions",
	"contract":                  "internal/modules/contracts",
	"deal":                      "internal/modules/deals",
	"pipeline":                  "internal/modules/deals",
	"stage":                     "internal/modules/deals",
	"stage_exit_criterion":      "internal/modules/deals",
	"deal_stage_evidence":       "internal/modules/deals",
	"stage_progression_outcome": "internal/modules/deals",
	"deal_stage_history":        "internal/modules/deals",
	// The Deal Room is its own capability rather than a corner of deals: it
	// owns an external audience, its own credentials and an immutable
	// publication history, none of which the deal spine has a place for.
	"deal_room":             "internal/modules/dealrooms",
	"deal_room_release":     "internal/modules/dealrooms",
	"deal_room_participant": "internal/modules/dealrooms",
	"deal_room_invitation":  "internal/modules/dealrooms",
	"deal_room_session":     "internal/modules/dealrooms",
	"deal_room_document":    "internal/modules/dealrooms",
	"deal_room_thread":      "internal/modules/dealrooms",
	"deal_room_comment":     "internal/modules/dealrooms",
	"deal_room_engagement":  "internal/modules/dealrooms",
	// Kept apart from deal_stage_history rather than folded into it: readers
	// outside this module count that table's rows as stage movements.
	"deal_forecast_history": "internal/modules/deals",
	// The project is its own bounded context, superseding ADR-0073 — see
	// modules/projects/doc.go. This entry is what makes that a rule rather than
	// a layout: a statement writing either table from any other package fails
	// TestEveryPackageOnlyWritesTablesItOwns.
	"project":               "internal/modules/projects",
	"project_phase_history": "internal/modules/projects",
	"fx_rate":               "internal/modules/deals",
	"product":               "internal/modules/deals",
	"offer":                 "internal/modules/deals",
	"offer_line_item":       "internal/modules/deals",
	"offer_template":        "internal/modules/deals",
	// activities
	"activity":              "internal/modules/activities",
	"transcript_read":       "internal/modules/activities",
	"attachment_extraction": "internal/modules/activities",
	"activity_link":         "internal/modules/activities",
	// The evidence that qualified an activity for the statutory retention
	// floor (A165/ADR-0114). It hangs off `activity` and is written by the
	// stamp, so it belongs to the module that owns the row it substantiates.
	"activity_retention_evidence": "internal/modules/activities",
	// How a reply verdict came to be what it is: the classifier's own judgement
	// and every human correction after it. It hangs off `activity` and is
	// written beside the column it explains, so it belongs to the module that
	// owns that row.
	"activity_reply_verdict_history": "internal/modules/activities",
	"activity_sales_state":           "internal/modules/activities",
	"activity_reader_state":          "internal/modules/activities",
	"worklist_pin":                   "internal/modules/activities",
	// ACT-DDL-3: who was in the interaction. It belongs beside activity and
	// activity_link for the same reason they belong together — it is part of
	// what an activity IS, not a graph artifact derived from one.
	"activity_participant": "internal/modules/activities",
	// What a meeting BECAME and when. It hangs off activity.meeting_status and
	// is written in the same transaction by the two doors that set it, so it
	// belongs to the module that owns the column it is the history of.
	"activity_meeting_history": "internal/modules/activities",
	// The named readers of an activity whose audience a human limited to
	// `selected`; it is written by the audience endpoint in the same
	// transaction as the column it qualifies.
	"activity_audience_member": "internal/modules/activities",
	// CG-DDL-1: the interaction projection. Owned by search per the ratified
	// module answer (ADR-0078 §2) — the graph capability lives inside the
	// search module, and a sibling would have to import its traversal
	// primitives, which a module may not do.
	"graph_interaction_edge": "internal/modules/search",
	"graph_contact_edge":     "internal/modules/search",
	// CG-DDL-2: LinkedIn ghosts. Owned by people because the work they exist
	// for is identity matching — the same dedupe rules, the same chokepoint.
	"email_signature":     "internal/modules/people",
	"linkedin_account":    "internal/modules/people",
	"linkedin_connection": "internal/modules/people",
	"attachment":          "internal/modules/activities",
	"deal_document_hide":  "internal/modules/activities",
	"booking_page":        "internal/modules/activities",
	// approvals (signing_key backs the approval-token JWS; the autonomy policy
	// is what each rep has decided about a KIND of proposal, so it belongs to
	// the module that owns the kinds and records the decisions it counts)
	"approval":                 "internal/modules/approvals",
	"signing_key":              "internal/modules/approvals",
	"approval_autonomy_policy": "internal/modules/approvals",
	// consent (the DSR case queue and the retention-policy catalog are
	// consent's; the engines that EXECUTE them live in privacy)
	"consent_purpose":   "internal/modules/consent",
	"person_consent":    "internal/modules/consent",
	"consent_event":     "internal/modules/consent",
	"consent_doi_token": "internal/modules/consent",
	// What made business correspondence lawful, and the §7(3) flag: both are
	// the gate's own evidence (ADR-0098 D2/D4), written where the gate that
	// relies on them lives.
	"person_acquisition_evidence":    "internal/modules/people",
	"privacy_notice_case":            "internal/modules/consent",
	"consent_text_version":           "internal/modules/consent",
	"communication_decision":         "internal/modules/consent",
	"communication_basis":            "internal/modules/consent",
	"communication_suppression":      "internal/modules/consent",
	"consent_qualifying_event":       "internal/modules/consent",
	"consent_existing_customer_flag": "internal/modules/consent",
	"data_subject_request":           "internal/modules/consent",
	"preference_token":               "internal/modules/consent",
	// The emailed link that shows a contact their own record and carries their
	// marketing answer back, and what comes back through it. Consent's, because
	// what the token authorises is a consent decision and the address it was
	// delivered to is the evidence that decision rests on.
	"confirm_token":             "internal/modules/consent",
	"person_confirm_submission": "internal/modules/consent",
	// retention_policy sits in consent's DDL block (DM-DDL-10) but is OWNED by
	// privacy, because ownership here names the module whose store owns the
	// writes: privacy runs the nightly evaluator that reads it and, since the
	// authoring surface landed (GCS-WIRE-1..4), holds the only CRUD path to it.
	// Consent's bootstrap seed is the waiver below, not the owner — a boot-time
	// INSERT of the shipped defaults is not a store.
	"retention_policy": "internal/modules/privacy",
	// capture
	"raw_capture":                  "internal/modules/capture",
	"capture_connection":           "internal/modules/capture",
	"capture_sync_state":           "internal/modules/capture",
	"capture_backfill":             "internal/modules/capture",
	"capture_backfill_creation":    "internal/modules/capture",
	"workspace_email_domain":       "internal/modules/capture",
	"capture_sender_override":      "internal/modules/capture",
	"capture_exclusion":            "internal/modules/capture",
	"capture_owner_identity":       "internal/modules/capture",
	"capture_alias_sighting":       "internal/modules/capture",
	"capture_import":               "internal/modules/capture",
	"capture_thread_verdict":       "internal/modules/capture",
	"capture_counterparty_hold":    "internal/modules/capture",
	"capture_digest":               "internal/modules/capture",
	"capture_auto_enrich_state":    "internal/modules/capture",
	"capture_pending_counterparty": "internal/modules/capture",
	"capture_auto_enrich_budget":   "internal/modules/capture",
	// What the pipeline decided about each message, for 24 hours. Written by
	// the sink alone; compose reads it and sweeps it, and the verdict engine
	// writes nothing here — its answers live in the disposition ledger and are
	// joined at read time.
	"capture_trace": "internal/modules/capture",
	// The workspace's own additions to and carve-outs from the shipped
	// consumer-mail baseline (CAP-PARAM-5).
	"capture_freemail_domain": "internal/modules/capture",
	// The workspace's bot channel: credentials, webhook secret and connection
	// status. It is a connection, so it sits with capture_connection under the
	// ONE connector.Sink rather than with the identities it delivers.
	"channel_connection": "internal/modules/capture",
	// The installation-settings table (ADR-0090/A135). Owned by the platform
	// mechanism rather than by any module, which is the point: a setting's
	// MEANING belongs to the module that declares its entry, but no module
	// owns the row shape — platform/settings is the one writer, and a module
	// reaching this table directly would be re-implementing the governance
	// (validator, freeze probe, per-entry audit verb) the entry already
	// carries. The unusual owner is therefore the invariant, not an exception.
	"setting": "internal/platform/settings",
	// The extension tier's secret namespace (ADR-0069): the mapping from an
	// extension's own key names onto keyvault refs. Owned by the platform
	// mechanism for the same reason `setting` is — no module owns the row
	// shape, and a second writer would be a second namespace wall, which is
	// the one thing this table exists to be. platform/extsecrets is
	// therefore a walked root below, so the ownership really is enforced.
	"extension_secret": "internal/platform/extsecrets",
	// search
	"embedding":           "internal/modules/search",
	"embed_store_binding": "internal/modules/search",
	// ai (voice DNA: the derived profile artifact + corpus manifest;
	// the tracing spine: per-call metadata + opt-in captured payload)
	"ai_usage":              "internal/modules/ai",
	"voice_profile":         "internal/modules/ai",
	"voice_corpus_source":   "internal/modules/ai",
	"voice_build":           "internal/modules/ai",
	"voice_profile_version": "internal/modules/ai",
	"voice_profile_delta":   "internal/modules/ai",
	"voice_learning_signal": "internal/modules/ai",
	"ai_call":               "internal/modules/ai",
	"ai_call_payload":       "internal/modules/ai",
	"ai_call_config":        "internal/modules/ai",
	"ai_feedback":           "internal/modules/ai",
	"ai_model_rate":         "internal/modules/ai",
	// agents (incl. the runner subpackage)
	"agent_run":  "internal/modules/agents",
	"runner_job": "internal/modules/agents",
	// Whether a rep granted an agent standing authority, and which of THEIR
	// OWN passports carries it. The grant records the decision; the passport
	// it names is minted by identity, which is the only writer of that table.
	"agent_standing_grant": "internal/modules/agents",
	// The AI-activity projection. Derived read-model state written by exactly
	// one consumer, so it carries no audit or outbox row of its own — the
	// events that FEED it carry the write shape at their own writers.
	"ai_task_run": "internal/modules/aiactivity",
	// automation (the deterministic trigger-and-action catalog)
	"workflow_run":            "internal/modules/automation",
	"notice":                  "internal/modules/notices",
	"intro_request":           "internal/modules/introductions",
	"automation_effect_claim": "internal/modules/automation",
	"automation":              "internal/modules/automation",
	// signals (the warm-room signal spine + its append-only resolution log)
	"finance_connection":         "internal/modules/finance",
	"provider_connection":        "internal/modules/integrations",
	"provider_connection_budget": "internal/modules/integrations",
	"provider_run":               "internal/modules/integrations",
	"provider_run_reservation":   "internal/modules/integrations",
	// The purchased VALUES, owned by people rather than by integrations
	// (migration 0219 says so in the DDL): the domain decides what a claim
	// means and how it renders, while integrations owns the run that bought
	// it. That split is what lets a person page show a bought email beside a
	// canonical one and say which is which.
	"provider_applied_field":       "internal/modules/people",
	"person_provider_claim":        "internal/modules/people",
	"relationship_nudge_dismissal": "internal/modules/people",
	"finance_external_customer":    "internal/modules/finance",
	"finance_customer_link":        "internal/modules/finance",
	"finance_invoice":              "internal/modules/finance",
	"finance_payment":              "internal/modules/finance",
	"signal":                       "internal/modules/signals",
	"signal_resolution":            "internal/modules/signals",
	// collections
	"list":        "internal/modules/collections",
	"list_member": "internal/modules/collections",
	"tag":         "internal/modules/collections",
	"taggable":    "internal/modules/collections",
	"saved_view":  "internal/modules/collections",
	// privacy (the erasure suppression list is the module's own state;
	// its other writes are ratified waivers below)
	"erasure_suppression": "internal/modules/privacy",
	// customfields (the governed add-field engine's catalog)
	"custom_field": "internal/modules/customfields",
	// knowledge (the asked document corpus; the chunk is a derived artifact of
	// its document and carries no audit identity of its own)
	"knowledge_corpus":     "internal/modules/knowledge",
	"knowledge_document":   "internal/modules/knowledge",
	"knowledge_chunk":      "internal/modules/knowledge",
	"webhook_subscription": "internal/modules/webhooks",
	"webhook_delivery":     "internal/modules/webhooks",
	// comms (outbound delivery machinery; the activity row is the
	// user-visible fact and stays owned by activities)
	"comms_outbound": "internal/modules/comms",
	"scheduled_send": "internal/modules/activities",
	// overlay (the HubSpot mirror cluster, ADR-0017 custom namespace —
	// design.md §4.2)
	"incumbent_connection":        "internal/modules/overlay",
	"overlay_mode":                "internal/modules/overlay",
	"overlay_mirror":              "internal/modules/overlay",
	"overlay_association":         "internal/modules/overlay",
	"mirror_user_map":             "internal/modules/overlay",
	"mirror_user_automap_block":   "internal/modules/overlay",
	"mirror_visibility":           "internal/modules/overlay",
	"overlay_write_ledger":        "internal/modules/overlay",
	"overlay_mirror_halt":         "internal/modules/overlay",
	"overlay_tombstone":           "internal/modules/overlay",
	"overlay_backfill_cursor":     "internal/modules/overlay",
	"overlay_reconcile_watermark": "internal/modules/overlay",
	"overlay_sync_state":          "internal/modules/overlay",
	// migration (the shared importer engine's run records, IEM-DDL-1;
	// native rows land through injected Writers, so the record tables'
	// owners are untouched)
	"import_run":        "internal/modules/migration",
	"import_record_map": "internal/modules/migration",
	// compose (HTTP replay protection is transport plumbing, not domain;
	// the brief read model is the cross-module ranker's own snapshot —
	// deals + people strength + activities compose only here)
	"idempotency_key": "internal/compose",
	// The MCP Tasks handle, beside the claim above and owned for the same
	// reason: it is transport-owned operational state, not a domain record, and
	// modules/agents declares the seam while owning no SQL.
	"agent_task": "internal/compose",
	// The activity-kind and channel-provider registries (DESIGN-SP4 §4):
	// derived from the composed connector/extension set at boot, so no domain
	// module decides "which providers exist" — compose observes it, the same
	// way it owns idempotency_key and agent_task.
	"activity_kind":    "internal/compose",
	"channel_provider": "internal/compose",
	"analytics_share":  "internal/compose",
	"report_run":       "internal/compose",
	"brief_run":        "internal/compose/briefs",
	"brief_item":       "internal/compose/briefs",
	// The weekly retrospective, in its own aggregate rather than the brief's:
	// a weekly row on brief_run would become "the latest brief" to the reader
	// that decides the next morning's overnight window, and weekly content on
	// brief_item would be cascaded away by deleting a deal.
	"weekly_review":             "internal/compose/weekly",
	"team_weekly_review":        "internal/compose/weekly",
	"team_weekly_review_rep":    "internal/compose/weekly",
	"assurance_run_finding":     "internal/modules/assurance",
	"assurance_run":             "internal/modules/assurance",
	"assurance_source_coverage": "internal/modules/assurance",
	// One pass of assurance over a scope, and what each finding contributed to
	// that pass's task. They belong to assurance because a cycle is a window
	// over its own findings; the TASK is an ordinary activity the caller mints
	// through the activities door, so no activity write lives here.
	"assurance_cycle":                 "internal/modules/assurance",
	"assurance_task_item":             "internal/modules/assurance",
	"assurance_exception":             "internal/modules/assurance",
	"assurance_resolution":            "internal/modules/assurance",
	"forecast_call":                   "internal/modules/forecasting",
	"forecast_snapshot":               "internal/modules/forecasting",
	"forecast_contribution":           "internal/modules/forecasting",
	"weekly_plan":                     "internal/modules/weeklyplan",
	"weekly_plan_commitment":          "internal/modules/weeklyplan",
	"weekly_review_deal":              "internal/compose/weekly",
	"weekly_review_outlook":           "internal/compose/weekly",
	"weekly_review_movement":          "internal/compose/weekly",
	"weekly_review_driver":            "internal/compose/weekly",
	"weekly_review_scorecard":         "internal/compose/weekly",
	"weekly_review_learning":          "internal/compose/weekly",
	"weekly_review_learning_citation": "internal/compose/weekly",
	// The company view's per-user visit baseline: view state, not a record
	// fact, so it is written without an audit row — the saved-view ruling.
	// The person view acknowledges visits into the SAME table (one baseline
	// per user per record, whatever kind of record it is), ratified below.
	"user_record_view": "internal/compose/org360",
	// Which activities have had their stored originals re-read for further
	// participants. Job bookkeeping about a background pass rather than a
	// fact about a customer, and the pass is composed here because it spans
	// the mail and calendar parsers that no single module may reach across.
	"activity_participant_replay": "internal/compose",
	// Which captured meetings have had their attendees resolved under the
	// current rule — the same kind of bookkeeping as the replay marker above,
	// owned here for the same reason. A marker of its own because the replay's
	// records a completed PARSE, and every meeting this pass must re-read
	// already carries one.
	"activity_meeting_attendee_repair": "internal/compose",
	// The rep's own "not this, not now" on a suggestion: per user, keyed on
	// the evidence it fired on. Same ruling — view state, no audit row.
	"suggestion_dismissal": "internal/compose/org360",
	// The account brief's per-user cache: derived content, regenerable at
	// any time, readable by nobody but its own user. Same ruling.
	"org_brief":      "internal/compose/orgbrief",
	"org_dossier":    "internal/compose/orgdossier",
	"org_growth_fit": "internal/compose/orgdossier",
	// The account scan's per-user row: the last findings the model read for
	// this reader, and the job carrier of the read in flight. Same ruling.
	"org_scan": "internal/compose/orgscan",
	// The relationship brief's per-user cache — the person-side sibling of
	// org_brief, and the same ruling for the same reasons.
	"person_brief": "internal/compose/personbrief",
	// The deal status card's per-user cache — the deal-side sibling of the
	// two above, and the same ruling: derived content, regenerable from the
	// records at any time, readable by nobody but the user it was written
	// for.
	"deal_status_card": "internal/compose/dealstatus",
	// The reader's own "not this, not now" on the page's one moment, held
	// against the evidence it fired on so it re-arms when that evidence moves
	// (ADR-0096 D3). View state: no audit row, no outbox event, no other
	// viewer's page.
	"person_moment_dismissal": "internal/compose/person360",
	// platform, the key vault: the local provider's ciphertext store. No
	// workspace_id — a deployment credential belongs to the installation, not a
	// tenant.
	"vault_secret": keyVaultStoreDir,

	// River's own queue table. Owned by the package that runs the fleet, which
	// is the only one that writes it directly: the client library writes the
	// rest, and a purge is the one statement this tree issues against it.
	"river_job": jobsStoreDir,

	// platform, storekit: the audit+outbox pair has ONE sanctioned writer, and
	// the shared field-provenance layer (B-E02.12) is spelled once next to it.
	// system_log is the non-entity operational ledger written through
	// storekit.LogSystem, the same storekit-owned posture as audit_log.
	"audit_log":        storekitOwned,
	"event_outbox":     storekitOwned,
	"field_provenance": storekitOwned,
	"system_log":       storekitOwned,
}

// sqlTarget is one write a literal performs: which table, by which verb, and
// which columns it names.
//
// cols is best-effort and only ever a FLOOR: an INSERT's parenthesised column
// list and an UPDATE's SET targets are read, and anything else — a fragment
// assembled at runtime, an INSERT … SELECT with no list — contributes none.
// Every reader of it has to treat an empty list as "unknown", never as "writes
// nothing".
type sqlTarget struct {
	table, verb string
	cols        []string
}
