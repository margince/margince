// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package policy

// The seeded system-role matrix: the grant rows every default is built from,
// the five role documents themselves, and the positional zip that turns one
// line per role into a map. Split from policy.go, which owns the document
// SHAPE and the read paths (Parse, Merge) — this file owns what the server
// writes at workspace creation, and the two change for different reasons.

import (
	"encoding/json"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

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
)

// The objects whose posture is not visible from any one role's overrides,
// because every role treats them the same way or because the reason lives in a
// relationship no grant expresses.
//
// finance — the ingested accounting mirror (ADR-0083/A128). READ is broad:
// every role that opens a company page sees whether the customer pays on time.
// Connecting or disconnecting the source is destructive workspace-wide config,
// so create/update/delete are admin/ops-only, exactly like overlay_connection.
// No role holds a write on a finance RECORD, because there is no such action:
// the mirror's read-only posture is the ABSENCE of the grant (FIN-DDL-N-1),
// not a runtime refusal.
//
// contract (ADR-0109/A160) and commission carry no owner column of their own —
// visibility is inherited from the deal they hang off, so the row scope that
// reaches them is the DEAL's. A rep records and maintains the agreements they
// close and sees what their partner-sourced deal earned; approving and paying
// are the business's decisions, and an entry is written by the won-deal accrual
// rather than by hand.
//
// integrations (ADR-0101/A152) — a rep reads whether a provider is connected,
// so a dated value on a person record has an explanation; connecting one spends
// money and is admin/ops.
//
// offer_template follows product and offer rather than the pipeline-config
// posture: it is the offer's own branding input, not a locked-down schema
// surface, so reps create and work templates like any other offer-adjacent
// record. Delete stays manager/admin/ops — archiveOfferTemplate carries no
// x-agent-access gate, so any role holding delete may call it directly.
//
// overlay_connection, channel_connection and webhook_subscription share one
// posture: each binds the whole workspace to something outside it — an
// incumbent CRM, a chat bot carrying every seat's inbound traffic, an outbound
// egress of governed events — so create/update/delete are admin/ops-only while
// every role reads the binding's status. A rep needs to know whether overlay
// mode is live, or whether the channel is up, before expecting a reply to
// arrive there. (UC-E10-04 narrates a Rep registering a subscription; that
// posture question is tracked upstream, not settled here.)
//
// weekly_plan — a rep plans their own week and settles it. Their own only: the
// store resolves the plan from the caller and takes no owner argument, so the
// grant admits them to their week and to nobody else's. A lead's read of a
// rep's plan is a separate path gated on the shared-team question, not on a
// wider grant here. No delete, for the reason introduction carries: a week that
// was planned is a thing that happened, and a commitment is dropped rather than
// erased.

// managerObjects is the grid a team lead (`manager`) and the whole-organization
// `management` seat share; only their row scope differs.
var managerObjects = grid(crud, map[string]grant{
	objAiModelRate:          none,
	objAiRouting:            none,
	"automation":            readOnly,
	objCaptureSettings:      createRead,
	objCaptureTrace:         readOnly,
	"channel_connection":    readOnly,
	objComputedField:        readOnly,
	"custom_field":          readOnly,
	objDataCoverage:         none,
	objEmbeddingReindex:     none,
	"finance":               readOnly,
	objForecast:             createRead,
	objFxRate:               none,
	objImportRun:            none,
	objInstallationSettings: readOnly,
	"integrations":          readOnly,
	objIntroduction:         writeNoDelete,
	"knowledge_corpus":      readOnly,
	"knowledge_document":    readOnly,
	objLicense:              none,
	"overlay_connection":    readOnly,
	"pipeline":              readOnly,
	objRetentionPolicy:      none,
	"tag":                   readOnly,
	"webhook_subscription":  readOnly,
})

var defaults = map[string]Document{
	// The installation's administrator: every object, every verb, except where
	// the PRODUCT offers no such verb. computed_field has no authoring surface
	// (RD-AC-7); license is issued rather than edited; capture_trace is written
	// by the pipeline and swept by its own job; the rate sheets and
	// capture_settings are append-forward because a past-dated row prices a
	// historical rollup; forecast supersedes rather than being rewritten.
	"admin": {
		Objects: grid(crud, map[string]grant{
			objAiModelRate:          writeNoDelete,
			objAiRouting:            readUpdate,
			objCaptureSettings:      writeNoDelete,
			objCaptureTrace:         readOnly,
			objComputedField:        readOnly,
			objDataCoverage:         readOnly,
			objEmbeddingReindex:     readUpdate,
			objForecast:             createRead,
			objFxRate:               writeNoDelete,
			objInstallationSettings: readUpdate,
			objIntroduction:         writeNoDelete,
			objLicense:              readOnly,
		}),
		RowScope: principal.RowScopeAll,
	},
	// management is the sales leader's seat (ADR-0110): the manager grid over
	// EVERY row in the organization, and no admin power. Governance actions
	// (inviting users, changing roles, editing role policy, binding another
	// user's passport, issuing password links) key on the literal admin role,
	// so an unbounded row scope here widens what management sees, never what
	// it administers. The two grids are one variable so they cannot drift.
	"management": {
		Objects:  managerObjects,
		RowScope: principal.RowScopeAll,
	},
	"manager": {
		Objects: managerObjects,
		// Team scope: a Team Lead manages their team, so they read and work the
		// records of everyone sharing a live team with them without a share
		// being arranged first. This is the manager grid above, bounded to the
		// team rather than the organization — `management` is the same grid
		// unbounded.
		//
		// Membership resolves through team_membership and live teams only, so a
		// Team Lead who belongs to no team reaches exactly their own rows. A
		// record_grant naming a user or a team stays the mechanism for sharing
		// ACROSS teams, and for every seat that is not this one.
		RowScope: principal.RowScopeTeam,
	},
	// The rep's baseline is READ. Sixteen objects take it and sixteen take
	// writeNoDelete, so the tie is broken deliberately rather than by the
	// count: a base is what an object added later inherits before anybody
	// revisits this file, and inheriting "may read" is a smaller mistake than
	// inheriting "may create and edit". The overrides below are the records
	// they actually work. Those take writeNoDelete — create, read, same-day-correct, never
	// delete — because archiving a commercial record is a manager's call.
	//
	// The three departures from that rule each have a reason. `lead` is crud
	// because disqualifying IS the delete and is routine rep work.
	// `saved_view` is crud because it is the rep's own per-user view state
	// (owner-scoped in the store), so full self-service is correct.
	//
	// Nine objects take the zero grant, and not all for one reason. Most are
	// surfaces with no rep consumer: the workspace's entitlement (UC-ADMIN-03
	// F1), the retention ladder, the two rate sheets, the reindex trigger, the
	// import run, source coverage, and which vendor receives the installation's
	// text. The shared-channel capture trace is a different and stronger claim
	// — a member's own capture traffic is personal data that no grant widens —
	// and its reason is written beside its constant rather than folded in here.
	//
	// A custom field is read-only even for the rep who fills it in: the field
	// itself is admin/ops-set config, and only the VALUE on a record is theirs.
	// knowledge_corpus and knowledge_document are reads because the ask itself
	// is that read — an answer whose source is unreadable is not a citation —
	// while uploading the prose every seat then asks stays with admin/ops.
	// `introduction` and `weekly_plan` carry no delete for a shared reason: an
	// ask that was made and a week that was planned are things that happened,
	// so they are withdrawn rather than erased, and EVERY seat carries that
	// property, admin included.
	"rep": {
		Objects: grid(readOnly, map[string]grant{
			"activity":          writeNoDelete,
			objAiModelRate:      none,
			objAiRouting:        none,
			objCaptureSettings:  createRead,
			objCaptureTrace:     none,
			"contract":          writeNoDelete,
			objDataCoverage:     none,
			"deal":              writeNoDelete,
			"deal_room":         writeNoDelete,
			objEmbeddingReindex: none,
			objFxRate:           none,
			objImportRun:        none,
			objIntroduction:     writeNoDelete,
			"lead":              crud,
			objLicense:          none,
			"list":              writeNoDelete,
			"offer":             writeNoDelete,
			"offer_template":    writeNoDelete,
			"organization":      writeNoDelete,
			"person":            writeNoDelete,
			"product":           writeNoDelete,
			"project":           writeNoDelete,
			"relationship":      writeNoDelete,
			objRetentionPolicy:  none,
			objSavedView:        crud,
			"signal":            writeNoDelete,
			"voice_profile":     writeNoDelete,
			"weekly_plan":       writeNoDelete,
		}),
		RowScope: principal.RowScopeOwn,
	},
	"read_only": {
		// A read-only role still owns its personal view state: saved views
		// are P1-exempt per-user prefs (runtime-config-surface.md §3), not
		// shared records, so full self-service is correct even here. The zero
		// grants are the same set the rep holds nothing on, for the same
		// reasons.
		Objects: grid(readOnly, map[string]grant{
			objAiModelRate:      none,
			objAiRouting:        none,
			objCaptureTrace:     none,
			objDataCoverage:     none,
			objEmbeddingReindex: none,
			objFxRate:           none,
			objImportRun:        none,
			objLicense:          none,
			objRetentionPolicy:  none,
			objSavedView:        crud,
		}),
		RowScope: principal.RowScopeAll,
	},
	// ops administers the installation's wiring on the admin grid, and differs
	// from admin in row scope alone at this layer: what actually separates them
	// is the literal-admin gate on identity and governance routes, which no
	// grant here expresses.
	"ops": {
		Objects: grid(crud, map[string]grant{
			objAiModelRate:          writeNoDelete,
			objAiRouting:            readUpdate,
			objCaptureSettings:      writeNoDelete,
			objCaptureTrace:         readOnly,
			objComputedField:        readOnly,
			objDataCoverage:         readOnly,
			objEmbeddingReindex:     readUpdate,
			objForecast:             createRead,
			objFxRate:               writeNoDelete,
			objInstallationSettings: readUpdate,
			objIntroduction:         writeNoDelete,
			objLicense:              readOnly,
		}),
		RowScope: principal.RowScopeAll,
	},
}

// MustDefaultJSON returns the seeded policy document for a system role key,
// ready for the role.permissions column. Unknown keys panic: the caller
// iterates the compiled-in role list, so a miss is a programming error.
func MustDefaultJSON(roleKey string) []byte {
	doc, ok := defaults[roleKey]
	if !ok {
		panic(fmt.Sprintf("policy: no default document for role %q", roleKey))
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		panic(err) // a compiled-in document always marshals
	}
	return raw
}
