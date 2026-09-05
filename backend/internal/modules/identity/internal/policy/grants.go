// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package policy

// The vocabulary the seeded role documents are written in: the grant rows a
// default is built from, the keyed builder that turns one baseline plus a
// role's departures from it into a map, and the object-name constants those
// departures repeat. defaults.go holds the six documents themselves.
//
// Split from policy.go, which owns the document SHAPE and the read paths
// (Parse, Merge). These two files own what the server writes at workspace
// creation, and the two concerns change for different reasons.

import "fmt"

// crud/read are the two grant rows every default builds from.
var (
	crud       = grant{Create: true, Read: true, Update: true, Delete: true}
	readOnly   = grant{Read: true}
	readUpdate = grant{Read: true, Update: true}
	// writeNoDelete is the append-forward config posture: create + read +
	// same-day-correct (update), never delete. The rate sheets (fx_rate,
	// ai_model_rate) have no delete surface at all — a past-dated row prices
	// historical rollups and must never disappear — so no role holds delete.
	writeNoDelete = grant{Create: true, Read: true, Update: true}
	// createRead is the posture of a record nobody edits: a forecast reading is
	// derived, and a current call SUPERSEDES rather than being rewritten, so
	// neither update nor delete has a surface to gate. A grant for a verb the
	// product does not offer reads as an oversight the next author has to
	// research.
	createRead = grant{Create: true, Read: true}
	// none is the zero grant, named so a role's override map says WHY a line is
	// there. Inside a `map[string]grant` the literal simplifies to a bare `{}`,
	// which reads as an oversight rather than as "this role holds nothing on
	// this object" — and holding nothing is a decision every bit as deliberate
	// as holding crud.
	none = grant{}
	// deleteOnly is the posture of an action whose only surface destroys. The
	// installation reset is the one: POST /admin/reset-data is the single
	// operation on it, and whether the reset is ARMED is a deployment-file fact
	// served to every seat as /me's data_reset_available, gated by no object at
	// all. Granting a read here would name a surface the product does not have.
	//
	// This is the one exception TestNoSeededRoleGrantsAWriteWithoutRead carries,
	// and the exception is written there rather than papered over by inventing
	// the read.
	deleteOnly = grant{Delete: true}
)

// defaults are the seeded system-role policies (they encode
// the choices: reps work team-scoped without delete; managers are
// team-scoped with delete; pipeline, automation and custom-field config
// are admin/ops-owned — each reshapes what the system does
// (or stores) on everyone's records, so they follow the pipeline-config
// posture: everyone reads the catalog, only admin/ops change it.
// computed_field is read-only for every role, admin/ops included —
// RD-AC-7: no runtime formula-authoring surface exists, so there is no
// write to grant). offer_template follows the SAME posture as product/
// offer, not the pipeline-config posture: it's the offer's own branding
// input, not a locked-down schema surface, so reps create and work
// templates like any other offer-adjacent record; delete stays manager/
// admin/ops (archiveOfferTemplate carries no x-agent-access gate — any
// role holding delete may call it directly). overlay_connection follows
// the SAME posture as custom_field, and for the same reason:
// connecting/disconnecting the workspace's incumbent binding is
// destructive workspace-wide config (it purges the mirror and flips
// sor_mode for everyone), so create/update/delete are admin/ops-only;
// every role may read the connection status (a rep needs to see whether
// overlay mode is live, the same as a custom_field catalog read).
// embedding_reindex has no create/delete surface at all — it is a single
// deployment-level trigger, not a record kind — so only read and update
// are ever granted, and both are admin/ops-only: admin/ops may update
// (trigger a reindex; the confirm route itself carries x-agent-access:
// human-only in the contract, so this grant only ever fires from a
// human session, never an agent), and admin/ops alone may read it — the
// banner/card that consumes the read is itself ops-gated in the SPA, so
// manager/rep/read_only have no legitimate consumer of this object and
// get the zero grant, unlike the custom_field catalog or
// overlay_connection's status which every role legitimately reads.
// webhook_subscription follows the SAME admin/ops-owned posture: a
// subscription registers outbound egress of governed events, so managing
// the fan-out surface is workspace integration config (create/update/
// delete admin/ops-only); every role may read subscriptions and their
// delivery health. (UC-E10-04 narrates a Rep registering one; that
// posture question is tracked upstream against the spec, not settled
// here.)
// channel_connection follows overlay_connection's posture exactly: a bot
// bound at the workspace level carries every seat's inbound channel traffic,
// so create/update/delete are admin/ops-only, while every role may read the
// binding's status (a rep needs to know whether the channel is live before
// expecting a reply to arrive there).
// import_run is admin/ops-only on EVERY verb, read included: a
// migration run is a workspace-wide bulk mutation of the estate (the
// overlay→native flip executes through it), and unlike custom_field or
// overlay_connection there is no per-rep read surface — the mode-flip
// and migrate-in screens are admin surfaces.
// grid builds one role's document: every core object at `base`, then the
// objects that differ. It replaces a 44-argument positional zip in which the
// object a grant belonged to was decided by COUNTING — a transposed pair
// typechecked, read identically in review, and would have been caught only by
// the replay fixture. Here each grant is written beside the object it governs,
// and a role states only where it departs from its own baseline, which is the
// sentence the reader actually wants: not "what does rep hold on all 44" but
// "where is rep NOT the record posture".
//
// An override naming an object outside coreObjects panics at package init: a
// typo has to be a build failure rather than a grant that silently governs
// nothing, which is the rule `Parse` already applies to a stored document.
func grid(base grant, overrides map[string]grant) map[string]grant {
	out := make(map[string]grant, len(coreObjects))
	for _, object := range coreObjects {
		out[object] = base
	}
	for object, g := range overrides {
		if _, ok := out[object]; !ok {
			//craft:ignore panic-in-domain runs only during package initialization (the `defaults` var) — a role naming an object the vocabulary does not have is a typo in this file, and booting with it would seed that role a grant short forever
			panic(fmt.Sprintf("policy: a default grant names %q, which is not a core object", object))
		}
		out[object] = g
	}
	return out
}

// The object names a role's overrides repeat. Constants rather than repeated
// literals so a typo is a compile error here too, not only inside `grid` at
// init: these fourteen appear in three or more role documents, and a misspelt
// one in a single map would otherwise take that role's grant with it.
const (
	// The rate sheets. Append-forward on every role that holds them —
	// create, read, same-day-correct, never delete — because a past-dated row
	// prices a historical rollup and must not disappear. No delete surface
	// exists at all, so no role holds one.
	objAiModelRate = "ai_model_rate"
	// Which vendor this installation's text is sent to (ai-operational-spec
	// §1.4). Deliberately NOT folded into installation_settings: whoever may
	// rename the organization has no business re-pointing where its people's
	// correspondence is processed, and those become one grant the moment the
	// two share an object.
	//
	// Narrow on BOTH verbs, unlike most read-broad objects here. Nobody needs
	// this grant to see which models are bound — the AI profile surface answers
	// that from the running config, not from a settings read — so a read grant
	// would buy a rep nothing and widen the reach of the object governing egress.
	objAiRouting = "ai_routing"
	// Everyone reads (a rep sees whether auto-enrich is on); only admin/ops
	// toggle it or carve a domain back out of the consumer-mail baseline.
	// `create` is the one write a rep holds: contributing a consumer domain the
	// shipped baseline missed (CAP-PARAM-5) is everyday judgment about the mail
	// they read, not workspace config.
	objCaptureSettings = "capture_settings"
	// The workspace's SHARED-CHANNEL capture trace (a bot binding's inbound
	// traffic). Read-only for everyone who holds it, because nothing writes it
	// but the pipeline and nothing deletes it but its 24h sweep — there is no
	// create/update/delete anyone could hold.
	//
	// It reaches ONLY rows with no member behind them. A member's own capture
	// traffic is personal data and no grant widens it, which is why rep and
	// read_only hold nothing here rather than holding read: there is no
	// shared-channel debugging in their job, and a grant that looked harmless
	// would be the one somebody later widened.
	objCaptureTrace = "capture_trace"
	// Read-only for every role, admin/ops included — RD-AC-7: no runtime
	// formula-authoring surface exists, so there is no write to grant.
	objComputedField = "computed_field"
	// Nothing, below admin/ops. Source health is connector freshness,
	// permission-limited checks and import backlog: an OPERATOR's view. A rep
	// shown their installation's connector health has been handed somebody
	// else's job, and the screen that would tell them is one they cannot act on.
	objDataCoverage = "data_coverage"
	// No create/delete surface at all — it is a single deployment-level
	// trigger, not a record kind — so only read and update are ever granted,
	// and both are admin/ops-only. Admin/ops may update (trigger a reindex; the
	// confirm route carries x-agent-access: human-only in the contract, so the
	// grant only ever fires from a human session, never an agent), and admin/ops
	// alone may read it: the banner that consumes the read is itself ops-gated,
	// so manager/rep/read_only have no legitimate consumer and get nothing —
	// unlike the custom_field catalog or overlay_connection's status, which
	// every role legitimately reads.
	objEmbeddingReindex = "embedding_reindex"
	// createRead everywhere it is held: a reading is derived, and a current
	// call SUPERSEDES rather than being rewritten, so neither update nor delete
	// has a surface to gate. A grant for a verb the product does not offer
	// reads as an oversight the next author has to research.
	objForecast = "forecast"
	// Append-forward like ai_model_rate, and for the same reason: a past-dated
	// row prices a historical rollup.
	objFxRate = "fx_rate"
	// Admin/ops-only on EVERY verb, read included: a migration run is a
	// workspace-wide bulk mutation of the estate (the overlay→native flip
	// executes through it), and unlike custom_field or overlay_connection there
	// is no per-rep read surface — the mode-flip and migrate-in screens are
	// admin surfaces.
	objImportRun = "import_run"
	// The organization's own identity and reporting calendar (ADR-0090/A135).
	// Read is broad — the base currency and the business timezone shape what
	// every seat sees — and only admin/ops change it.
	objInstallationSettings = "installation_settings"
	// Asking a colleague to open a door, and answering an ask made of you, are
	// both the job, so every seat holds it. WHICH of the two parties a caller is
	// on a given row is the row's own check, and no grant stands in for it.
	//
	// No delete on ANY role, admin included: an ask that was made is a thing
	// that happened, so it is withdrawn rather than erased. That is a property
	// of the object rather than a posture per role, which is why the backfill
	// migration grants the same — a database that upgraded into this object and
	// one created after it answer alike.
	objIntroduction = "introduction"
	// READ IS THE ONLY VERB, for admin as much as anyone: the token is resolved
	// from the deployment file at boot, so there is no write path for a grant to
	// govern. Admin/ops-only — a rep reads their own seat elsewhere; the
	// workspace's entitlement is not theirs to see (UC-ADMIN-03 F1).
	objLicense = "license"
	// Admin/ops-only, read included (UC-GDPR-09, GCS-WIRE-1..5). The retain-only
	// posture setting is gated by this same object, so whoever may author a
	// policy may also suspend every destructive one.
	objRetentionPolicy = "retention_policy"
	// The rep's own per-user view state, owner-scoped in the store, so full
	// self-service is correct for every seat — read_only included.
	objSavedView = "saved_view"

	// The administration objects. Each replaces a literal `admin` role check on
	// a settings surface, so a custom role can carry the authority without the
	// server learning a new role name. Every one of them is `none` on rep and
	// read_only, which is why they are constants: a misspelt name in a single
	// role's map would silently hand that role the base grant instead.

	// Who may invite a member, widen the roster view, change a role, issue a
	// password link and deactivate an account. Admin only: it is the authority
	// that can reach every other authority, so delegating it is a decision an
	// installation makes deliberately by editing a custom role.
	objUserAdmin = "user_admin"
	// The role directory and the role editor. Ops READS it — an operator
	// answering "why can this person not see that" needs the policy in front of
	// them — while changing a role stays with admin, because a holder of the
	// editor can grant themselves anything the editor can express.
	objRoleAdmin = "role_admin"
	// Creating a team and moving members between teams. No delete: a team that
	// existed is what a past record's visibility was resolved against, so it is
	// archived rather than erased.
	objTeamAdmin = "team_admin"
	// The privacy inbox: subject requests arrive, get worked and get answered.
	// Read and update only — a request is raised by the subject, never by an
	// operator, and answering it is the update.
	objPrivacyRequest = "privacy_request"
	// The audit trail. Read is the only verb it will ever carry: an audit log
	// that anyone may write is not an audit log. Admin only by default — Ops
	// administers the wiring, not the record of who did what.
	objAuditLog = "audit_log"
	// Queue depth, retry ladders and what capture's judgement queues hold.
	// Operational, not commercial, so Ops reads it: this is the question an
	// operator is on call for.
	objJobHealth = "job_health"
	// Which extension units this installation composed. Read-only because
	// presence under `extensions/` IS the enablement — there is no runtime
	// toggle for a grant to gate.
	objExtensionAccess = "extension_access"
	// Erasing the installation's data. Admin only, and never Ops: the reset is
	// the one action no operator performs on someone else's behalf.
	objSystemReset = "system_reset"
	// Model calls, AI health and spend. Management reads it because the spend
	// is theirs to answer for; the routing that CHANGES it is objAiRouting and
	// stays admin/ops.
	objAiDiagnostics = "ai_diagnostics"
	// The consent-purpose vocabulary every capture and outreach decision is
	// judged against. Ops holds it in full because the purposes are wiring, and
	// Management reads what its team is bound by.
	objConsentConfig = "consent_config"
	// The sign-in policy: which providers are offered and whether a password is
	// one of them. Management and Ops read the posture; changing who may enter
	// the installation is admin.
	objAuthenticationPolicy = "authentication_policy"
	// The workspace's OAuth applications — the client credentials a mailbox
	// connector is issued against. Split from objCaptureSettings, which every
	// sales role holds, because a client secret is not a capture setting.
	objOauthApplication = "oauth_application"
	// How many seats are used against how many are held. Non-commercial by
	// construction: the licensee, the entitlement and the contract terms stay
	// on objLicense, so Management can plan headcount without reading them.
	objSeatUsage = "seat_usage"
)
