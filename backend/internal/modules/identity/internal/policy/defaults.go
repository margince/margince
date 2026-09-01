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
)

// defaults are the seeded system-role policies (they encode
// the choices: reps work team-scoped without delete; managers are
// team-scoped with delete; pipeline, automation, custom-field config AND
// quota targets are admin/ops-owned — each reshapes what the system does
// (or stores) on everyone's records, so they follow the pipeline-config
// posture: everyone reads the catalog, only admin/ops change it (quota's
// createQuota/updateQuota/archiveQuota carry the matching x-agent-access:
// human-only gate in the contract — a target is never agent-set).
// computed_field is read-only for every role, admin/ops included —
// RD-AC-7: no runtime formula-authoring surface exists, so there is no
// write to grant). offer_template follows the SAME posture as product/
// offer, not the pipeline-config posture: it's the offer's own branding
// input, not a locked-down schema surface, so reps create and work
// templates like any other offer-adjacent record; delete stays manager/
// admin/ops (archiveOfferTemplate carries no x-agent-access gate — any
// role holding delete may call it directly). overlay_connection follows
// the SAME posture as quota, and for the same reason:
// connecting/disconnecting the workspace's incumbent binding is
// destructive workspace-wide config (it purges the mirror and flips
// sor_mode for everyone), so create/update/delete are admin/ops-only;
// every role may read the connection status (a rep needs to see whether
// overlay mode is live, the same as a quota's attainment read).
// embedding_reindex has no create/delete surface at all — it is a single
// deployment-level trigger, not a record kind — so only read and update
// are ever granted, and both are admin/ops-only: admin/ops may update
// (trigger a reindex; the confirm route itself carries x-agent-access:
// human-only in the contract, so this grant only ever fires from a
// human session, never an agent), and admin/ops alone may read it — the
// banner/card that consumes the read is itself ops-gated in the SPA, so
// manager/rep/read_only have no legitimate consumer of this object and
// get the zero grant, unlike quota's attainment or
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
// overlay→native flip executes through it), and unlike quota or
// overlay_connection there is no per-rep read surface — the mode-flip
// and migrate-in screens are admin surfaces.
// managerObjects is the grid a team lead (`manager`) and the whole-organization
// `management` seat share; only their row scope differs.
var managerObjects = objects(crud, crud, crud, crud, crud, readOnly, crud, crud, crud, crud, readOnly, crud, crud, crud, crud, crud, readOnly, readOnly, readOnly, crud, readOnly, grant{}, readOnly, grant{}, grant{}, grant{Create: true, Read: true}, crud, readOnly, grant{}, readOnly, readOnly, readOnly, grant{}, readOnly, grant{}, crud, grant{}, crud, crud, readOnly, readOnly, writeNoDelete)

var defaults = map[string]Document{
	"admin": {
		Objects:  objects(crud, crud, crud, crud, crud, crud, crud, crud, crud, crud, crud, crud, crud, crud, crud, crud, crud, readOnly, crud, crud, crud, readUpdate, crud, writeNoDelete, writeNoDelete, writeNoDelete, crud, crud, crud, readUpdate, crud, crud, crud, readOnly, readOnly, crud, readUpdate, crud, crud, crud, crud, writeNoDelete),
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
	"rep": {
		// Reps create and work records but never delete them — except
		// leads, where disqualify IS the delete and is routine rep work.
		// Lists and tags are everyday organizational surfaces: reps
		// create and use them; archiving stays manager/admin. A voice
		// profile is the rep's own working material: create/maintain
		// yes, delete stays manager/admin. Rate-card products, offers and
		// warm-room signals follow the record posture: reps create and
		// work them, delete stays manager/admin. An offer template
		// follows the same posture (see the comment above defaults). A
		// saved view is the rep's own per-user view state (owner-scoped
		// in the store) — full self-service, including deleting one's own
		// view. A quota is read-only even for its own owner: the target
		// itself is admin/ops-set config, not the rep's working material —
		// only the attainment READ is the rep's to consult.
		Objects: objects(
			grant{Create: true, Read: true, Update: true},
			grant{Create: true, Read: true, Update: true},
			grant{Create: true, Read: true, Update: true},
			crud,
			grant{Create: true, Read: true, Update: true},
			readOnly,
			grant{Create: true, Read: true, Update: true},
			grant{Create: true, Read: true, Update: true},
			grant{Create: true, Read: true, Update: true},
			readOnly,
			readOnly,
			grant{Create: true, Read: true, Update: true},
			grant{Create: true, Read: true, Update: true},
			grant{Create: true, Read: true, Update: true},
			grant{Create: true, Read: true, Update: true},
			crud,
			readOnly,
			readOnly,
			readOnly,
			grant{Create: true, Read: true, Update: true},
			readOnly,
			grant{},
			readOnly,
			grant{}, // fx_rate — admin/ops-only
			grant{}, // ai_model_rate — admin/ops-only
			// capture_settings — everyone reads (a rep sees whether
			// auto-enrich is on), only admin/ops toggle it or carve a
			// domain back out of the consumer-mail baseline. `create` is
			// the one write a rep holds: contributing a consumer domain
			// the shipped baseline missed (CAP-PARAM-5) is everyday
			// judgment about the mail they read, not workspace config.
			grant{Create: true, Read: true},
			// project — the record posture: reps create and work a body
			// of work, archiving stays manager/admin/ops.
			grant{Create: true, Read: true, Update: true},
			// channel_connection — everyone reads (a rep needs to know
			// whether the channel is live), only admin/ops bind one.
			readOnly,
			grant{},  // import_run — admin/ops-only
			readOnly, // installation_settings — the base currency and zone
			readOnly, // finance — a rep reads whether the customer pays on time
			// integrations — a rep reads whether a provider is connected, so
			// a dated value on a person record has an explanation; connecting
			// one spends money and is admin/ops.
			readOnly,
			// retention_policy — admin/ops-only, read included (see objects()).
			grant{},
			// capture_trace — a rep debugs no shared bot binding, and their own
			// capture activity needs no grant.
			grant{},
			// license — admin/ops-only. A rep reads their own seat elsewhere;
			// the workspace's entitlement is not theirs to see (UC-ADMIN-03 F1).
			grant{},
			// A rep records and maintains the agreements they close; archiving
			// one stays with manager/admin, like every other commercial record.
			grant{Create: true, Read: true, Update: true},
			// ai_routing — re-pointing which vendor receives the installation's text
			// is not a rep's decision, and reading the binding is not part of the job.
			grant{},
			// commission — a rep sees what their partner-sourced deal earned, and
			// nothing more: approving and paying are the business's decisions, and
			// an entry is written by the won-deal accrual rather than by hand.
			readOnly,
			// deal_room — a rep opens and runs the room on their own deal:
			// create, publish, invite, revoke. Deleting one stays manager/admin,
			// the same posture every other record the rep works carries.
			grant{Create: true, Read: true, Update: true},
			// knowledge_corpus — the ask itself is this read, so a rep holds it
			// or the help bot is an admin tool. The floor that decides what the
			// product refuses to answer is not theirs to move.
			readOnly,
			// knowledge_document — a rep who receives a cited answer can open
			// what it cited; an answer whose source is unreadable is not a
			// citation. Uploading third-party prose every seat then asks, and
			// deleting it for good, stay with admin/ops.
			readOnly,
			// introduction — asking a colleague to open a door, and answering
			// an ask made of you, are both the job. The grant admits a rep to
			// the surface; WHICH of the two parties they are on a given row is
			// the row's own check, and no grant stands in for it. No delete: an
			// ask that was made is a thing that happened, so it is withdrawn
			// rather than erased — a property of the object, so EVERY seat
			// carries it, admin and ops included. The backfill migration grants
			// the same, so a database that upgraded into this object and one
			// created after it answer alike.
			writeNoDelete),
		// Own scope, not team: membership of a team is not by itself permission
		// to rewrite a teammate's records. Writing somebody else's customer
		// record takes an explicit share — a record_grant naming the user or
		// one of their teams — or an unbounded seat.
		//
		// The team ARM survives in the write predicate and is not dead: a
		// record_grant may name a team, and an operator may still author a
		// custom role at team scope. What changed is only what the seeded
		// roles claim by default.
		RowScope: principal.RowScopeOwn,
	},
	"read_only": {
		// A read-only role still owns its personal view state: saved views
		// are P1-exempt per-user prefs (runtime-config-surface.md §3), not
		// shared records, so full self-service is correct even here.
		Objects:  objects(readOnly, readOnly, readOnly, readOnly, readOnly, readOnly, readOnly, readOnly, readOnly, readOnly, readOnly, readOnly, readOnly, readOnly, readOnly, crud, readOnly, readOnly, readOnly, readOnly, readOnly, grant{}, readOnly, grant{}, grant{}, readOnly, readOnly, readOnly, grant{}, readOnly, readOnly, readOnly, grant{}, grant{}, grant{}, readOnly, grant{}, readOnly, readOnly, readOnly, readOnly, readOnly),
		RowScope: principal.RowScopeAll,
	},
	"ops": {
		Objects:  objects(crud, crud, crud, crud, crud, crud, crud, crud, crud, crud, crud, crud, crud, crud, crud, crud, crud, readOnly, crud, crud, crud, readUpdate, crud, writeNoDelete, writeNoDelete, writeNoDelete, crud, crud, crud, readUpdate, crud, crud, crud, readOnly, readOnly, crud, readUpdate, crud, crud, crud, crud, writeNoDelete),
		RowScope: principal.RowScopeAll,
	},
}

// objects zips grants onto coreObjects in declaration order — one line
// per role instead of twelve repeated map literals.
//
// The parameter list IS the mechanism, which is why it is this long: each role
// states its grants once, positionally, against the object order declared above,
// and rbacfixture_test.go holds the result to the matrix the server seeds. Five
// map literals of every key is what this replaced. Shortening it is a refactor of
// the seed rather than of this call.
func objects(person, organization, deal, lead, activity, pipeline, list, tag, relationship, partner, automation, voiceProfile, product, offer, signal, savedView, customField, computedField, quota, offerTemplate, overlayConnection, embeddingReindex, webhookSubscription, fxRate, aiModelRate, captureSettings, project, channelConnection, importRun, installationSettings, finance, integrations, retentionPolicy, captureTrace, license, contract, aiRouting, commission, dealRoom, knowledgeCorpus, knowledgeDocument, introduction grant) map[string]grant { // NOSONAR(go:S107) -- the parameter list is the mechanism; see above
	return map[string]grant{
		"person": person, "organization": organization, "deal": deal,
		"lead": lead, "activity": activity, "pipeline": pipeline,
		"list": list, "tag": tag, "relationship": relationship, "partner": partner,
		"automation": automation, "voice_profile": voiceProfile,
		"product": product, "offer": offer, "signal": signal,
		"saved_view": savedView, "custom_field": customField,
		"computed_field": computedField, "quota": quota,
		"offer_template": offerTemplate, "overlay_connection": overlayConnection,
		"embedding_reindex":    embeddingReindex,
		"webhook_subscription": webhookSubscription,
		"fx_rate":              fxRate,
		"ai_model_rate":        aiModelRate,
		"capture_settings":     captureSettings,
		"project":              project,
		"channel_connection":   channelConnection,
		"import_run":           importRun,
		// The workspace's SHARED-CHANNEL capture trace (a bot binding's inbound
		// traffic). Read-only for everyone who holds it, because nothing writes
		// it but the pipeline and nothing deletes it but its 24h sweep -- there
		// is no create/update/delete anyone could hold.
		//
		// It reaches ONLY rows with no member behind them. A member's own capture
		// traffic is personal data and no grant widens it, which is why rep and
		// read_only hold nothing here rather than holding read: there is no
		// shared-channel debugging in their job, and a grant that looked
		// harmless would be the one somebody later widened.
		"capture_trace": captureTrace,
		// The agreements an account has signed (ADR-0109/A160). Read is broad
		// because a contract is what makes an account's commercial state legible
		// and every role that opens a company page needs it; writing one stays
		// with the roles that own commercial records, and read_only holds read
		// alone. A contract carries no owner column — visibility is inherited
		// from its deal, falling back to its organization — so the row scope
		// that reaches it is the deal's, not one of its own.
		"contract": contract,
		// What a partner earned on a won deal. Read is broad because a rep
		// working a partner-sourced deal needs to see what it earned; approving
		// and paying stay with the roles that own commercial records, and a rep
		// holds read alone. A commission entry carries no owner column —
		// visibility is inherited from its deal — so the row scope that reaches
		// it is the deal's, not one of its own.
		"commission": commission,
		"deal_room":  dealRoom,
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
		"ai_routing": aiRouting,
		// The installation's own identity and reporting basis (ADR-0090/A135).
		// Read is broad on purpose — a rep reading amounts benefits from knowing
		// which currency they are in — and only admin/ops change it.
		"installation_settings": installationSettings,
		// The ingested finance mirror (ADR-0083/A128). READ is broad — every
		// role that opens a company page sees whether the customer pays on
		// time — and connecting or disconnecting the accounting source is
		// destructive workspace-wide config, so create/update/delete are
		// admin/ops only, exactly like overlay_connection.
		//
		// No role holds a write on a finance RECORD, because there is no such
		// action: the mirror's read-only posture is the absence of the grant
		// (FIN-DDL-N-1), not a runtime refusal.
		"finance": finance,
		// The licensed data-provider connections (ADR-0101/A152). READ is
		// broad for the same reason finance's is: a rep looking at a person
		// record needs to know whether a provider is connected and why a
		// value is dated, and every state the card renders is a fact about
		// the installation rather than about a customer. Connecting one
		// spends the customer's money and sends their contacts' identifiers
		// to a third party, so the writes are admin/ops only.
		"integrations": integrations,
		// The installation's entitlement: what the license grants, and how much of
		// it is used. Admin/ops-only on read — import_run's and retention_policy's
		// posture rather than quota's, because a seat meter is commercial standing
		// and UC-ADMIN-03 F1 gives a rep their own seat and not the workspace's
		// entitlement.
		//
		// READ IS THE ONLY VERB, for admin as much as anyone: the token is resolved
		// from the deployment file at boot, so there is no write path for a grant to
		// govern. Any other verb here would describe an action this product has not
		// got.
		"license": license,
		// The storage-limitation ladder (UC-GDPR-09, GCS-WIRE-1..5). Admin/ops-only
		// on EVERY verb, READ INCLUDED — the import_run precedent, not quota's. A
		// retention policy decides what the installation destroys and when, and the
		// screen showing it is an admin surface: unlike a quota's attainment or an
		// overlay connection's status, no rep has a legitimate consumer for the read.
		// The retain-only posture setting is gated by this same object, so whoever may
		// author a policy may also suspend every destructive one.
		"retention_policy": retentionPolicy,
		// A named body of uploaded documents a person asks free-text questions
		// of. READ IS THE ASK: the ask returns corpus content, and anything that
		// returns a record is a read — so every role that reads records holds it,
		// or the help bot is an admin tool. Defining a corpus, editing its words
		// and moving its grounding floor are workspace config in the
		// installation_settings sense: the floor decides what the product will
		// refuse to answer for everyone, so it is admin/ops.
		"knowledge_corpus": knowledgeCorpus,
		// The uploaded files themselves. Read is broad for the same reason the
		// corpus read is — a person who gets a cited answer must be able to open
		// what it cited, and an answer whose source is unreadable is not a
		// citation. Uploading and deleting are admin/ops: an upload puts third-
		// party prose into the corpus every seat then asks, and the delete is a
		// hard one that takes the chunks, the vectors and the stored file.
		"knowledge_document": knowledgeDocument,
		// One rep asking a colleague to open a door to a contact. Create and
		// update are a rep's: asking is the job, and answering an ask made OF
		// you is answering for yourself — the row's own party check decides
		// which of the two you are, and a grant cannot substitute for it.
		"introduction": introduction,
	}
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
