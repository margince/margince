// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind shape H1

package gates

// Table ownership as a fitness function: the import DAG is enforced three
// ways, but nothing in the import graph stops a package from writing SQL
// against a table it does not own. This test closes that gap — it walks the
// hand-written Go under internal/modules and internal/compose, extracts every
// INSERT/UPDATE/DELETE target from SQL string literals (plus the storekit
// applier and row-lock table arguments), and asserts each module only writes its own
// tables. Cross-store writes exist by design (merge relinks, GDPR erasure,
// ingest materialization); each one is ratified in crossStoreWrites
// (tableownershipwaivers_test.go) with a self-contained
// rationale — an entry without a rationale is a finding, not a pass, and a
// waiver that matches no remaining write is stale and fails too. SELECTs are
// out of scope: reads are governed by each statement's own workspace predicate
// and the platform/auth row-scope clauses, not by ownership.
//
// That last sentence names two of the three halves of a read's admission and
// stops. The OBJECT half — may this caller read this KIND of record at all — is
// enforced at module store entry points by rbacgate_test.go and, in the compose
// tier, by no gate at all for any core object except `relationship`
// (backend/gates/edgereaders_test.go). Nine compose reads of one table drifted inside
// that gap while a correct implementation sat one directory away, so it is
// written here rather than left to be re-derived: a sentence that lists the
// guards a read has is read as the list of guards a read needs.

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// storekitOwned marks the tables written ONLY through
// platform/database/storekit (Audit/Emit) or the migration runner — no
// walked package owns them, so any direct module write needs a waiver.
const storekitOwned = "internal/platform/database/storekit"

// extSecretsStoreDir is the extension tier's secret namespace, which owns
// extension_secret. The constant exists so tableOwners can point at it in one
// spelling; WHICH platform packages this gate walks is answered by
// platformStoreDirs, not by a name here.
const extSecretsStoreDir = "internal/platform/extsecrets"

// keyVaultStoreDir is the local key-vault provider, which owns vault_secret.
// Until this gate walked it the table had no owner entry at all.
//
// What that left open, stated narrowly because the wide version is not true: a
// second writer in internal/modules or internal/compose was already caught, by
// the no-declared-owner arm below. The unguarded case was a second writer
// inside a platform package the gate did not walk — and the hand-kept list of
// those was short by two, which is what the derivation replaced.
const keyVaultStoreDir = "internal/platform/keyvault"

// jobsStoreDir runs the River fleet, and is where river_job is written from —
// discovered by platformStoreDirs rather than named here; the constant exists
// so tableOwners can point at it in one spelling.
const jobsStoreDir = "internal/platform/jobs"

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
	"worklist_snapshot":          "internal/compose/worklistsnap",
	"relationship":               "internal/modules/people",
	"partner":                    "internal/modules/people",
	"lead":                       "internal/modules/people",
	"lead_score_history":         "internal/modules/people",
	"lead_manual_signal":         "internal/modules/people",
	"lead_source":                "internal/modules/people",
	"lead_disqualify_reason":     "internal/modules/people",
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
	"commission_entry":   "internal/modules/commissions",
	"contract":           "internal/modules/contracts",
	"deal":               "internal/modules/deals",
	"pipeline":           "internal/modules/deals",
	"stage":              "internal/modules/deals",
	"deal_stage_history": "internal/modules/deals",
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
	"activity_sales_state":        "internal/modules/activities",
	"activity_reader_state":       "internal/modules/activities",
	"worklist_pin":                "internal/modules/activities",
	// ACT-DDL-3: who was in the interaction. It belongs beside activity and
	// activity_link for the same reason they belong together — it is part of
	// what an activity IS, not a graph artifact derived from one.
	"activity_participant": "internal/modules/activities",
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
	"assurance_run":             "internal/modules/assurance",
	"assurance_source_coverage": "internal/modules/assurance",
	"assurance_exception":       "internal/modules/assurance",
	"assurance_resolution":      "internal/modules/assurance",
	"forecast_call":             "internal/modules/forecasting",
	"forecast_snapshot":         "internal/modules/forecasting",
	"forecast_contribution":     "internal/modules/forecasting",
	"weekly_plan":               "internal/modules/weeklyplan",
	"weekly_plan_commitment":    "internal/modules/weeklyplan",
	"weekly_review_deal":        "internal/compose/weekly",
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
	// The rep's own "not this, not now" on a suggestion: per user, keyed on
	// the evidence it fired on. Same ruling — view state, no audit row.
	"suggestion_dismissal": "internal/compose/org360",
	// The account brief's per-user cache: derived content, regenerable at
	// any time, readable by nobody but its own user. Same ruling.
	"org_brief":      "internal/compose/orgbrief",
	"org_dossier":    "internal/compose/orgdossier",
	"org_growth_fit": "internal/compose/orgdossier",
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

// owningDir normalizes a package dir to its ownership unit: the module root
// under internal/modules (subpackages share their module's ownership), or
// internal/compose.
// isIntegrationTagged reports whether the file builds only under the
// integration tag — the test lane's scaffolding (harnesses, fixtures).
// The ownership and write-shape obligations bind PRODUCTION writes; an
// integration-tagged file can never reach a shipped binary, and its
// seeding writes are the suites' own fixtures.
func isIntegrationTagged(path string) bool {
	f, err := os.Open(path) // #nosec G304 -- path is a *.go file from walking the trusted source tree
	if err != nil {
		return false
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			panic(cerr) // a leaked fd in a test helper is a bug, not a condition
		}
	}()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "//go:build integration" {
			return true
		}
		if line != "" && !strings.HasPrefix(line, "//") {
			return false // past the header — build constraints must precede it
		}
	}
	return false
}

func owningDir(pkgDir string) string {
	if strings.HasPrefix(pkgDir, "internal/modules/") {
		parts := strings.SplitN(pkgDir, "/", 4)
		return strings.Join(parts[:3], "/")
	}
	return pkgDir
}

// storekitTableArg names the storekit calls that carry their table as the third
// argument. The four Patch appliers issue the UPDATE themselves; LockRow and
// LockPair do not write, but they are where ApplyLocked's table comes from —
// the lock carries it in an unexported field, so the lock site is the only
// place the table is legible, and without it every ApplyLocked write is
// invisible here.
var storekitTableArg = map[string]bool{
	"ApplyWithVersion": true,
	"ApplyGuarded":     true,
	"ApplyGuardedIn":   true,
	"LockRow":          true,
	"LockPair":         true,
}

// stringConstsByPackage maps each walked directory to the string constants its
// own files declare. A table name spelled as a package constant is the tree's
// normal style — `entityLead`, `projectObject` — and a walker that reads only
// literals attributes none of those writes to anybody.
func stringConstsByPackage(t *testing.T, fset *token.FileSet, roots []string) map[string]map[string]string {
	t.Helper()
	consts := map[string]map[string]string{}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
				return err
			}
			file, err := parser.ParseFile(fset, filepath.ToSlash(path), nil, 0)
			if err != nil {
				return err
			}
			dir := filepath.ToSlash(filepath.Dir(path))
			for _, decl := range file.Decls {
				gen, ok := decl.(*ast.GenDecl)
				if !ok || gen.Tok != token.CONST {
					continue
				}
				for _, spec := range gen.Specs {
					value, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for i, name := range value.Names {
						if i >= len(value.Values) {
							continue
						}
						lit, ok := value.Values[i].(*ast.BasicLit)
						if !ok || lit.Kind != token.STRING {
							continue
						}
						text, err := strconv.Unquote(lit.Value)
						if err != nil {
							continue
						}
						if consts[dir] == nil {
							consts[dir] = map[string]string{}
						}
						consts[dir][name.Name] = text
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return consts
}

type tableWrite struct {
	pos   string // file:line for the finding
	table string
	// verb is which write this is — "insert", "delete" or "update".
	//
	// cols carries the columns the statement names, where they can be read.
	//
	// Ownership does not care: writing a table you do not own is the same
	// finding whichever verb does it. It is recorded because a SECOND gate
	// asks a question ownership cannot — whether a column classified as
	// having no writer yet has gained one — and "a row was created here" is
	// not the same claim as "privacy deletes from this table", which it
	// already does for both communication tables.
	verb string
	cols []string
	// site is the write's INSTANCE — "path/to/file.go:enclosingFunc" — and it
	// is what a waiver ratifies. The enclosing FUNCTION rather than the line:
	// a line moves whenever anything above it does, so a line-keyed waiver
	// goes stale on edits that never touched the write, and a map that is
	// re-typed on unrelated diffs stops being read.
	site string
}

// indirectTableArg ratifies the storekit call sites whose table arrives through
// a struct field rather than a name this walker can read, each with the tables
// the field can actually hold. Ratified, not discovered: the reason must name
// them, so the exception is re-checkable against the construction sites.
var indirectTableArg = gatekit.Waive(map[string]string{
	"internal/modules/people:w.table": "the evidence writer is one shape over two sidecars; the field is set at four struct literals in this package, to the organization_fact constant and to organization_profile_field, and people owns both",
})

// tableArgText reads a storekit table argument: a string literal, or an
// identifier declared as a string constant in the same package.
func tableArgText(arg ast.Expr, consts map[string]string) (string, bool) {
	switch v := arg.(type) {
	case *ast.BasicLit:
		if v.Kind != token.STRING {
			return "", false
		}
		text, err := strconv.Unquote(v.Value)
		return text, err == nil
	case *ast.Ident:
		text, ok := consts[v.Name]
		return text, ok
	default:
		return "", false
	}
}

// collectTableWrites walks every non-test module/compose source file and
// records each SQL write target (string literals plus the storekit applier and
// row-lock table arguments, see storekitTableArg) under its owning directory.
func collectTableWrites(t *testing.T) map[string][]tableWrite {
	t.Helper()
	writes := map[string][]tableWrite{} // owning dir → writes
	// storekitWrites counts what the CallExpr arm attributes. This gate lost
	// that whole arm once — it matched a method name no Patch has — and a
	// matcher that matches nothing is indistinguishable from a tree with no
	// versioned writes in it. The floor is what tells those two apart.
	storekitWrites := 0
	fset := token.NewFileSet()
	roots := append([]string{"internal/modules", "internal/compose"}, platformStoreDirs(t)...)
	consts := stringConstsByPackage(t, fset, roots)
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") ||
				strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") ||
				isIntegrationTagged(path) {
				return err
			}
			path = filepath.ToSlash(path)
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			dir := filepath.ToSlash(filepath.Dir(path))
			owner := owningDir(dir)
			// enclosing is the declaration the walk is currently inside. Set
			// per top-level declaration below rather than tracked during the
			// walk: ast.Inspect is flat, and a stack maintained by hand here
			// would be a second traversal to keep correct.
			enclosing := unnamedDeclSite
			record := func(pos token.Pos, tables []sqlTarget) {
				for _, target := range tables {
					writes[owner] = append(writes[owner], tableWrite{
						pos:   fset.Position(pos).String(),
						table: target.table,
						verb:  target.verb,
						cols:  target.cols,
						// Relative to the owner, which the key already names:
						// the absolute path would repeat that prefix in every
						// entry and push the part a reader is actually
						// comparing off the end of the line.
						site: strings.TrimPrefix(path, owner+"/") + ":" + enclosing,
					})
				}
			}
			visit := func(n ast.Node) {
				switch node := n.(type) {
				case *ast.BasicLit:
					if node.Kind != token.STRING {
						return
					}
					text, err := strconv.Unquote(node.Value)
					if err != nil {
						return
					}
					record(node.Pos(), sqlWrites(text))
				case *ast.CallExpr:
					sel, ok := node.Fun.(*ast.SelectorExpr)
					if !ok || !storekitTableArg[sel.Sel.Name] || len(node.Args) < 4 {
						return
					}
					table, ok := tableArgText(node.Args[2], consts[dir])
					if !ok {
						if indirectTableArg.Waived(t, owner+":"+exprText(fset, node.Args[2])) {
							return
						}
						// A table this walker cannot read is a table it cannot
						// attribute, and a skip here reads exactly like a module
						// that writes nothing. Reported, so the write names its
						// table where a reader — and this gate — can see it.
						t.Errorf("%s: %s.%s takes its table from an expression this gate cannot read — "+
							"name the table in a string literal or a package-level string constant, "+
							"or the write is attributed to no owner at all",
							fset.Position(node.Pos()), exprText(fset, sel.X), sel.Sel.Name)
						return
					}
					// Not "insert", and that is checkable rather than assumed:
					// storekitTableArg holds Apply* and Lock* only, and none
					// of them creates a row. TestNoPendingWriterHasAWriter
					// leans on that — an INSERT cannot arrive through this arm
					// and be read as an update.
					record(node.Pos(), []sqlTarget{{table: strings.ToLower(table), verb: "storekit"}})
					storekitWrites++
				}
			}
			walkDeclSites(fset, file, func(site string, n ast.Node) {
				enclosing = site
				visit(n)
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if storekitWrites < storekitWriteFloor {
		t.Fatalf("attributed only %d storekit table arguments, expected at least %d — the %v matcher has stopped matching, and every versioned write in the tree is now invisible to this gate",
			storekitWrites, storekitWriteFloor, slices.Sorted(maps.Keys(storekitTableArg)))
	}
	return writes
}

// storekitWriteFloor is set below the live count so ordinary refactoring does
// not trip it; it catches the arm going to zero, not a write being deleted.
const storekitWriteFloor = 25

// waiverKey is the subject crossStoreWrites ratifies: the writing package, the
// table it reaches into, and the INSTANCE that reaches it.
//
// The declaration is the point. Keyed on owner and table alone, one entry
// ratifies the CATEGORY — "people may write activity_link" — and a second,
// differently written copy of that write inside the same package is admitted by
// the entry the first one earned, with no finding to notice. A cross-store
// write is ratified on its own evidence or it is not ratified.
func waiverKey(owner string, w tableWrite) string {
	return owner + ":" + w.table + ":" + w.site
}

// unratifiedCrossStoreWrites returns one finding per write that reaches a table
// its package does not own and that no waiver ratifies.
//
// It RETURNS the findings rather than reporting them, so the gate's decision can
// be exercised against synthetic writes without failing the test that exercises
// it, and it takes the waiver set rather than reaching for the package-level one
// so such a test can pass a throwaway copy. Querying the real set MARKS entries
// matched, and gatekit accumulates that across every test in the package — a
// probe that consulted it would silently satisfy AssertAllMatched for the entry
// it named, which is the one staleness a stale-waiver gate exists to report. A gate whose only input is the tree it happens to be checked out over can
// be tested for what it accepts today and never for what it would refuse — and
// refusing is the half that has to keep working.
func unratifiedCrossStoreWrites(t testing.TB, waivers *gatekit.Waivers[string], writes map[string][]tableWrite) []string {
	t.Helper()
	owners := slices.Sorted(maps.Keys(writes))
	var findings []string
	for _, owner := range owners {
		for _, w := range writes[owner] {
			declared, known := tableOwners[w.table]
			if !known {
				findings = append(findings, fmt.Sprintf(
					"%s: %s writes table %q which has no declared owner — add it to tableOwners in backend/gates/tableownership_test.go",
					w.pos, owner, w.table,
				))
				continue
			}
			if declared == owner {
				continue
			}
			key := waiverKey(owner, w)
			if waivers.Waived(t, key) {
				continue
			}
			findings = append(findings, fmt.Sprintf(
				"%s: %s writes table %q owned by %s — move the write into the owning module, or ratify THIS write in crossStoreWrites[%q] with a self-contained rationale. "+
					"A waiver a sibling write in the same package already holds does not cover this one: the key names the function, so every copy is ratified on its own evidence",
				w.pos, owner, w.table, declared, key,
			))
		}
	}
	return findings
}

func TestEveryPackageOnlyWritesTablesItOwns(t *testing.T) {
	t.Parallel()
	defer crossStoreWrites.AssertAllMatched(t)
	defer indirectTableArg.AssertAllMatched(t)

	for _, finding := range unratifiedCrossStoreWrites(t, crossStoreWrites, collectTableWrites(t)) {
		t.Error(finding)
	}
}

// TestASecondCopyOfARatifiedWriteIsNotCoveredByTheFirst is this gate's own
// defect case, and it is why the key names a declaration rather than a package
// and a table.
//
// Keyed on package and table alone, a waiver ratifies the CATEGORY, and a
// second, differently written copy of the same cross-store write is admitted by
// the entry the first one earned — silently, because a key that already exists
// produces no finding to notice.
//
// The waiver set is a throwaway rather than crossStoreWrites: querying the real
// one marks its entries matched for the whole package, which would quietly
// satisfy the staleness sweep for whichever entry this case names.
func TestASecondCopyOfARatifiedWriteIsNotCoveredByTheFirst(t *testing.T) {
	t.Parallel()
	const (
		owner = "internal/modules/people"
		table = "activity_link"
		first = "ensure.go:Store.linkActivityToPerson"
	)
	ratified := tableWrite{pos: "internal/modules/people/ensure.go:1:1", table: table, site: first}
	// Same package, same table, a different declaration.
	planted := tableWrite{
		pos:   "internal/modules/people/planted.go:1:1",
		table: table,
		site:  "planted.go:aSecondWriterOfARatifiedTable",
	}

	// The plant only tests coverage if it is a write the tree really ratifies
	// and really does not own; both halves can drift out from under it.
	declared, known := tableOwners[table]
	if !known || declared == owner {
		t.Fatalf("%s is no longer a table %s writes without owning, so this case plants nothing", table, owner)
	}
	if !slices.Contains(crossStoreWrites.Subjects(), waiverKey(owner, ratified)) {
		t.Fatalf("crossStoreWrites no longer ratifies %s — repoint this case at a live waiver",
			waiverKey(owner, ratified))
	}
	// And the two must differ ONLY in the part the key added, or the case would
	// pass on a distinction the superseded key already drew.
	if owner+":"+ratified.table != owner+":"+planted.table {
		t.Fatalf("the plant differs from the ratified write in package or table, so a key naming " +
			"neither would already separate them and this case proves nothing about the declaration")
	}

	waivers := gatekit.Waive(map[string]string{
		waiverKey(owner, ratified): "the write this case treats as already ratified",
	})

	t.Run("the second copy is refused", func(t *testing.T) {
		findings := unratifiedCrossStoreWrites(t, waivers, map[string][]tableWrite{owner: {planted}})
		if len(findings) != 1 {
			t.Fatalf("planted one unratified write, got %d findings: %v", len(findings), findings)
		}
		// Named, not merely counted: a finding about some other write would
		// satisfy a bare count while the planted copy went through.
		if !strings.Contains(findings[0], planted.site) {
			t.Errorf("the finding does not name the planted write %q, so this case cannot tell that the "+
				"planted copy was the one refused:\n%s", planted.site, findings[0])
		}
	})

	t.Run("the ratified copy still passes", func(t *testing.T) {
		// The other direction. A gate that refused the planted write by
		// refusing every write would pass the subtest above and fail every
		// ratified write in the tree on the next push.
		if findings := unratifiedCrossStoreWrites(t, waivers, map[string][]tableWrite{owner: {ratified}}); len(findings) != 0 {
			t.Errorf("the ratified write is no longer covered by its own waiver: %v", findings)
		}
	})

	t.Run("two same-named methods on different receivers are two sites", func(t *testing.T) {
		// This tree writes two workers into one file as a matter of course, and
		// a bare method name would collapse both onto one site — the category
		// ratification above, one level down.
		const src = `package p

type aWorker struct{}
type aWorkspaceWorker struct{}

func (w *aWorker) Work() {}
func (w *aWorkspaceWorker) Work() {}
`
		// The type declarations are in the fixture only so the methods have
		// receivers; the sites under test are the two Work methods.
		sites := methodSitesOf(t, "jobs.go", src)
		if len(sites) != 2 {
			t.Fatalf("the fixture no longer declares exactly two methods: %v", sites)
		}
		if sites[0] == sites[1] {
			t.Errorf("both methods name the site %q, so one waiver would ratify both writes — "+
				"the receiver has stopped reaching the key", sites[0])
		}
	})

	t.Run("two package-level statements in one file are two sites", func(t *testing.T) {
		// GROUPED, which is the shape a per-declaration answer gets wrong:
		// `const (...)` is ONE declaration holding two statements. Separate
		// `const` statements are two declarations and pass either way, so a
		// fixture written that way exercises the walk only where it was
		// already right.
		const src = `package p

const (
	blankOne  = ` + "`UPDATE t SET a = NULL`" + `
	deleteOne = ` + "`DELETE FROM t`" + `
)
`
		sites := literalSitesOf(t, "statements.go", src)
		if len(sites) != 2 {
			t.Fatalf("the fixture no longer declares exactly two statements: %v", sites)
		}
		if sites[0] == sites[1] {
			t.Errorf("both statements name the site %q, so one waiver would ratify both — the walk "+
				"is answering a grouped block per declaration rather than per statement", sites[0])
		}
	})

	t.Run("two statements bound by one spec are two sites", func(t *testing.T) {
		// ONE ValueSpec binding two names. The grouped-block arm above answers
		// per spec, which is still one answer for both statements here — the
		// same collapse a third level in, and the reason the walk pairs values
		// with names rather than stopping at the spec.
		const src = `package p

const blankOne, deleteOne = ` + "`UPDATE t SET a = NULL`" + `, ` + "`DELETE FROM t`" + `
`
		sites := literalSitesOf(t, "onespec.go", src)
		if len(sites) != 2 {
			t.Fatalf("the fixture no longer binds exactly two statements: %v", sites)
		}
		if sites[0] == sites[1] {
			t.Errorf("both statements name the site %q, so one waiver would ratify both — the walk "+
				"is answering one spec per spec rather than per value bound in it", sites[0])
		}
	})
}
