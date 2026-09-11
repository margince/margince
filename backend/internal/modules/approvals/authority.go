// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The decision-authority predicate: who may see and decide a staged
// approval. One predicate (decidable) backs List, Get and Decide alike
// (C3/ADR-0036) — what you cannot see you cannot decide. Its target half —
// whether the record a staged row points at is one this human may see — is
// targetvisibility.go.

package approvals

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// objectActivity is the RBAC object every timeline write is governed by, spelled
// once: three staged kinds and the target-visibility switch below all name it,
// and a typo in any of them would silently ask for a grant nobody holds.
const objectActivity = "activity"

// objectDeal is the RBAC object a stage move is governed by. Spelled here
// rather than imported, for targetImportRun's reason: approvals may not import
// a module it governs.
const objectDeal = "deal"

// targetImportRun is the staged target a migrate-in commit names, and the RBAC
// object the migration module admits on (migration.ImportRunObject). One word
// for both, spelled here rather than imported: approvals may not import a
// module it governs.
const targetImportRun = "import_run"

// KindImportCommit is the approval a migrate-in commit stages. The kind IS the
// tool verb, which is how every other confirm-first verb is keyed here.
const KindImportCommit = "commit_import"

// grantRequirement is one RBAC pair approving a staged kind requires. Named
// rather than anonymous because the derivation below appends to the set and the
// composition layer's satisfiability gate reads the objects back out of it.
type grantRequirement struct {
	Object string
	Action principal.Action
}

// kindLinkedInMatch is the staged kind for "this imported connection is this
// contact". This module makes three separate statements about it — the grants
// deciding it needs, that only the member it was staged for may decide it, and
// that it declines the version pin — and a typo across them would leave the
// kind half-governed with nothing saying so. Compose owns the registration
// spelling; the compose-side waiver fitness tests bind the two together.
const kindLinkedInMatch = "linkedin_match"

// kindHeldDraft is an automation-composed reply held for the rep it was written
// for. Named rather than spelled: this module makes three separate statements
// about it — the grants deciding it needs, that approving it SENDS, and what
// the release files the message under — and a typo across them would leave the
// kind half-governed with nothing saying so.
const kindHeldDraft = "held_draft"

// kindDealFollowUp is the nightly reconciliation's "this conversation left no
// next step" card, and kindTranscriptProposal the next step a transcript
// recorded somebody committing to. Named rather than spelled: this module makes
// three separate statements about each — the grants deciding it needs, that the
// rep it was staged for decides it alone, and what filing it means — and a typo
// across them would leave the kind half-governed with nothing saying so.
const (
	kindDealFollowUp       = "deal_follow_up"
	kindTranscriptProposal = "transcript_proposal"
)

// KindScheduledSendHeld is the card a stopped scheduled message raises for the
// rep who scheduled it (ADR-0104 §5). Exported because compose stages it and
// registers both its effects: the message lives in activities and the inbox
// lives here, so the edge is injected there and both halves name one kind.
const KindScheduledSendHeld = "scheduled_send_held"

// KindCommunicationReview is a send the engine refused, put in front of
// somebody who may decide it goes anyway.
//
// NOT SELF-ONLY, unlike the held-message kind above, and that is the whole
// point of routing one. A rep refused at the keyboard may not hold the
// authority to override the engine; this card exists so they can ask somebody
// who does. A kind only its initiator could decide would put the question back
// in front of the person who could not answer it.
const KindCommunicationReview = "communication_review"

// objectCommunicationException is the RBAC object directing a send answers to,
// spelled here as identity's policy spells it. Deciding one of these cards IS
// directing a send, so it takes the same grant the direct-send door takes —
// anything less would make the card a way around that door.
const objectCommunicationException = "communication_exception"

// decisionGrants maps each stageable kind onto the RBAC its effect needs given
// the KIND ALONE; approving requires every one of them. A kind whose grant also
// depends on what the staging points at carries that half in
// targetResolvedGrants, and approving then requires the UNION — no kind needs
// both today, and decisionGrantsFor combines them so one that grows a second half
// gains authority rather than silently losing the first.
var decisionGrants = map[string][]grantRequirement{
	// A held message asks its rep to try again or give up. Both answers move the
	// message's own state — reschedule and cancel — which the store gates on
	// activity.UPDATE, so that is what deciding requires. It is the effects'
	// grant rather than the send's, because a gate that admitted a decision its
	// effect then refuses would commit the decision and fail the work: the card
	// gone, the message still held. selfOnlyKinds narrows it from "anyone
	// holding that grant" to the one contact whose message it is.
	KindScheduledSendHeld: {{objectActivity, principal.ActionUpdate}},
	// Committing an import writes the estate in bulk, so deciding one requires
	// the same grant creating a run does. It is the CREATE grant rather than an
	// update: an import creates records, and the contact releasing it is
	// authorising those creations, not editing a run.
	KindImportCommit: {{targetImportRun, principal.ActionCreate}},
	// A step-up requires NO object grant, and the empty slice is the decision
	// rather than an omission. Releasing a volume window touches no record: it
	// does not widen what the agent may read, only how much of what it may
	// already read it may be handed. There is no object to name, and naming one
	// would be a fiction that decided the wrong question — a human holding
	// deal.update is not thereby the contact who lent this passport.
	//
	// What bounds it instead is selfOnlyKinds below: the lender, and nobody
	// else. Without that entry this empty set would make a step-up decidable by
	// everyone, which is the failure decisionGrantsFor's own comment names — so
	// the two entries are one decision and TestAStepUpIsDecidedByTheLenderAlone
	// holds them together.
	KindVolumeRelease: {},
	// Deciding a refused send is directing it, so it takes the grant the
	// direct-send door takes. The card is a route to that act, not a second
	// authority for it — a reviewer who could approve here but not direct
	// would be releasing work they could not perform, and the effect would
	// refuse after the decision had already committed.
	KindCommunicationReview: {{objectCommunicationException, principal.ActionCreate}},

	"advance_deal": {{tableDeal, principal.ActionUpdate}},
	// progress_deal is advance_deal plus a timeline note; the gated effect
	// is the deal move, so deciding it needs the same grant.
	"progress_deal": {{tableDeal, principal.ActionUpdate}},
	"promote_lead":  {{tableLead, principal.ActionUpdate}, {tableContact, principal.ActionCreate}},
	// Disqualifying retires the lead in place, and the store gates it on
	// `lead:delete` (contacts/lead.go DisqualifyLead) as the REST twin's DELETE
	// implies. Deciding takes the grant PERFORMING it takes: anything less and
	// the confirm-first control point sits with someone who could not do the
	// thing they are releasing.
	"disqualify_lead": {{tableLead, principal.ActionDelete}},
	// Demotion reverses a promotion: the lead returns to the open ladder and the
	// contact the promotion created is archived. Both halves are gated where they
	// are performed (contacts/demote.go), so deciding takes both — the lead's
	// update and the contact's delete. The contact grant is the one that matters:
	// releasing a demotion is releasing the archival of a contact somebody may
	// have been working, and an approver who could not archive that contact is
	// not the contact to authorise it.
	"demote_lead": {{tableLead, principal.ActionUpdate}, {tableContact, principal.ActionDelete}},
	// A tag merge releases the source's NAME and no later act restores it, which
	// is why it confirms where a record merge does not: mergeContact archives the
	// source with `merged_into_id` and audit walks it back, and a tag keeps no
	// such pointer. The store gates it on `tag.update` (collections/tagvocab.go
	// MergeTags), so deciding takes that grant, for disqualify_lead's reason.
	"merge_tags": {{targetTag, principal.ActionUpdate}},
	// A phase transition writes the project row and one phase-history row in
	// one transaction (deals/project_advance.go); the gated effect is the
	// project move, so the approver needs the project's update grant.
	"advance_project_phase": {{tableProject, principal.ActionUpdate}},
	// A send is an activity write plus consent enforcement at redemption
	// time; the approver needs the write grant, the consent gate runs in
	// the handler regardless of who approved.
	"send_email": {{objectActivity, principal.ActionCreate}},
	// send_account_email is the same send from the other origin (ADR-0087 §6):
	// it starts a conversation instead of continuing one, and the effect it
	// stages is the identical activity write. The records the message is filed
	// under are named rather than inherited, and those are row-scope probed at
	// staging and again at insert — they are a question about the STAGER's
	// reach, not about the approver's, so they add nothing here. Governed
	// identically to the reply means exactly this grant and no other.
	"send_account_email": {{objectActivity, principal.ActionCreate}},
	// send_message is the same effect on a messaging channel: an activity
	// write, with the consent gate running in the handler whoever approved it.
	"send_message": {{objectActivity, principal.ActionCreate}},
	"book_meeting": {{objectActivity, principal.ActionCreate}},
	// A relink moves an activity onto another record, which the store gates on
	// activity.UPDATE — an association change, not a re-capture. It reaches a
	// human at all only for one destination: filing under a PROJECT classifies
	// the correspondence as a Handelsbrief, and that classification is
	// write-once in the database and is not lifted by relinking away. Every
	// other destination auto-executes, so a card here is always the six-year
	// decision rather than an ordinary move.
	//
	// The grant is the one PERFORMING it takes, for the reason disqualify_lead
	// states: anything less puts the control point with somebody who could not
	// do the thing they are releasing.
	"relink_activity": {{objectActivity, principal.ActionUpdate}},
	// The thread and named-set forms are the same decision at scale — every
	// row they move is gated on activity.UPDATE in the store — so deciding
	// them takes the same grant.
	"relink_thread":     {{objectActivity, principal.ActionUpdate}},
	"relink_activities": {{objectActivity, principal.ActionUpdate}},
	// Accepting a cold-start read-back writes enrichment fields onto an
	// company; "enrich" is the same effect staged through the
	// transport gate by an agent caller.
	"coldstart": {{tableCompany, principal.ActionUpdate}},
	"enrich":    {{tableCompany, principal.ActionUpdate}},
	// A rate refresh proposes an effective-dated row on a workspace-shared price
	// sheet, and deciding it requires BOTH write verbs on that sheet.
	//
	// The release is an upsert: it inserts a new (currency, day) or replaces an
	// existing rate, and which one it will be is not knowable when the decision
	// is made — the sheet can change between the decision and the apply. The
	// apply also runs as the system principal, so the store's in-transaction
	// check on the specific verb never fires here; this is the only grant
	// standing between an approver and the row the release replaces.
	//
	// Either verb alone would authorize the operation it does not name: a
	// create-only approver could release an overwrite, precisely the
	// substitution the store's second check exists to refuse. Requiring both is
	// the conservative reading — approve an upsert only if you could have
	// performed either half yourself. Every seeded role holding one holds the
	// other (writeNoDelete for admin and ops, the zero grant for everyone
	// else), so this constrains edited roles only, and constrains them right.
	"fx_rate_proposal": {
		{targetFxRate, principal.ActionCreate},
		{targetFxRate, principal.ActionUpdate},
	},
	"ai_model_rate_proposal": {
		{targetAIModelRate, principal.ActionCreate},
		{targetAIModelRate, principal.ActionUpdate},
	},
	// Accepting a deep site read writes profile fields and category facts
	// onto the target company — the same update authority enrich needs.
	"deepread": {{tableCompany, principal.ActionUpdate}},
	// Accepting a site_lead proposal (a published contact from a deep read's
	// team page) captures them as a LEAD through the capture sink — the
	// effect is a lead create, so deciding it needs that grant.
	"site_lead": {{tableLead, principal.ActionCreate}},
	// Approving a LinkedIn match links an imported connection to a contact and
	// writes that contact's LinkedIn address — a contact write, so deciding it
	// needs the grant the write itself takes.
	kindLinkedInMatch: {{tableContact, principal.ActionUpdate}},
	// Accepting a capture_counterparty proposal (ADR-0072/A118: a first-time
	// sender the verdict engine could not judge) creates the contact and, unless
	// the domain is free-mail, the company behind them — so deciding it
	// needs both create grants, exactly as if the approver had typed them in.
	"capture_counterparty": {{tableContact, principal.ActionCreate}, {tableCompany, principal.ActionCreate}},
	// Accepting a vcard_create proposal (an imported card the dedupe pass
	// refused to create beside its near-match) creates the contact; when the
	// card names an employer, also the employment edge (relationship create
	// plus the contact-anchor update that edge takes), and when nobody holds
	// that employer yet, the company behind them. Deciding needs every
	// grant the release can spend, exactly as if the approver had typed the
	// card in — a shorter list would show the card to an approver whose
	// approval then fails partway.
	"vcard_create": {
		{tableContact, principal.ActionCreate},
		{tableContact, principal.ActionUpdate},
		{tableCompany, principal.ActionCreate},
		{targetRelationship, principal.ActionCreate},
	},
	// Accepting a company_name_promotion proposal (PO-F-2a: one employee's
	// signature naming their company, with nothing corroborating it) renames
	// the company — the same update authority the name editor needs.
	"company_name_promotion": {{tableCompany, principal.ActionUpdate}},
	// Accepting a lifecycle_change proposal (the account-intelligence arc: the
	// correspondence says the contract ended while the record still reads as
	// live) moves the account's stage — the same update authority the header's
	// own stage picker needs.
	//
	// It also carries the signal's own summary in its payload and settles that
	// signal on accept, so it needs the signal grant too. Without it an
	// company editor could read model-derived correspondence and close a
	// signal they have no standing to see — the ordinary triage path takes
	// signal:update and EnsureSignalVisible for exactly that reason.
	"lifecycle_change": {{tableCompany, principal.ActionUpdate}, {targetSignal, principal.ActionUpdate}},
	// Confirming a nightly close-date correction (formulas §11 🟡 tier)
	// releases an expected_close_date write onto the deal.
	closeDateCorrectionKind: {{tableDeal, principal.ActionUpdate}},
	// Confirming an overnight follow-up proposal (features/07 §8a) creates
	// the drafted task activity; the target deal's visibility gates who
	// may see and decide it (targetVisible), the create grant gates the
	// write the confirm performs.
	kindDealFollowUp: {{objectActivity, principal.ActionCreate}},
	// Confirming a next step read out of a meeting transcript (S-E04.3)
	// creates the task activity it proposed. The transcript activity it is
	// filed against gates who may see and decide it (targetVisible); the
	// create grant gates the write the confirm performs. Read is not enough:
	// somebody who may read the transcript but not add to the timeline could
	// otherwise release a task they could not have logged themselves.
	kindTranscriptProposal: {{objectActivity, principal.ActionCreate}},
	// A proposed stage move is decided by whoever may MOVE the deal. Approving
	// it performs the advance, so read is not enough: somebody who can see a
	// deal but not steer it must not be able to release a move they could not
	// have made by hand.
	"stage_progression": {{objectDeal, principal.ActionUpdate}},
	// An automation's request_approval stages under emit_flow_event, and that
	// action IS the confirm-first act: approving it performs no downstream
	// write, it finishes the asking. The grant is the one the action catalog
	// already pins for request_approval (PermissionPinned activity:create,
	// automation/catalog_actions.go) — the same "create something for a human
	// to act on" shape send_email, book_meeting and deal_follow_up carry, which
	// is the precedent that catalog entry cites by name.
	//
	// Not an empty set, which would make it decidable by everyone: the
	// automation's owner needed activity:create to raise the card, and somebody
	// who could not have raised it must not be the one who answers it.
	"emit_flow_event": {{objectActivity, principal.ActionCreate}},
	// Releasing a held draft SENDS it, so the approver needs exactly what
	// sending takes — the same grant send_email carries, for the same reason.
	// The consent gate runs inside the release whoever approved it, and the
	// seat check runs on the decision, so this grant is the object half only.
	// Deliberately identical to send_email rather than lighter: a draft an
	// automation composed is still a message this human is putting their name
	// on, and "an automation wrote it" is not a reason to release it on
	// weaker authority than typing it would have taken.
	kindHeldDraft: {{objectActivity, principal.ActionCreate}},
}

// targetResolvedGrants are the kinds whose decision grant is not fixed by the
// kind but read off the staged target's own entity type, mapped to the action
// the release performs on it:
//
//   - archive_record deletes the target.
//   - merge_records rewrites where records point, and the store maps the merge
//     verb to update.
//   - update_record releases a field patch (human-edit precedence,
//     interfaces.md §2.1) — the grant the patch itself would need.
//   - create_record releases a new record of the type (a 🟡 create staged at the
//     transport gate, e.g. createCustomField).
//
// ONE table, because two readers derive from it: requireDecisionGrants enforces
// the pair against a principal, and DecisionGrantObjects reports the objects to
// the composition layer's satisfiability gate. A second copy would let that gate
// certify a route whose real decision demands a different object.
var targetResolvedGrants = map[string]principal.Action{
	// A reassignment at scale (AUTO-T07's 🟡 branch) writes owner_id onto
	// whatever the firing named, so the grant is resolved from the target the
	// way the action catalog resolves it: PermissionTargetScoped with verb
	// update (automation/catalog_actions.go). Pinning it to one object would
	// gate the wrong entity every time the trigger fired on another.
	//
	// It releases the same write the 🟢 branch performs, against the same record
	// through the same provider, so deciding it takes exactly the authority
	// performing it takes — the rule disqualify_lead states above. (What the
	// release adds beyond that write — its own provenance and the redeemed
	// version pin — is enumerated in compose/assignownerrelease.go and changes
	// nothing about the grant.)
	"assign_owner":   principal.ActionUpdate,
	"archive_record": principal.ActionDelete,
	"merge_records":  principal.ActionUpdate,
	"update_record":  principal.ActionUpdate,
	"create_record":  principal.ActionCreate,
}

// decidable is the ONE visibility-and-authority predicate for the inbox
// and the decision: true when p holds every grant approving a would
// require AND could read the staged target itself — the object-read grant
// on its type, then that record's own row rule (targetvisibility.go). It
// backs List, Get and Decide alike, so triage visibility and the decision
// gate can never drift apart — you see exactly what you could act on, and
// what you cannot see you cannot decide (in either direction). Two shapes
// narrow that to ONE seat, the member the row was staged for: a self-only
// kind, a staged create against a table whose rows belong to one human
// each, and a kind that is one rep's own morning work
// (decidedByTheSeatStagedFor). An unknown kind (no mapping) or unknown target type is not
// decidable: fail-closed.
func decidable(ctx context.Context, tx pgx.Tx, p principal.Principal, a row) (bool, error) {
	if requireDecisionGrants(p, a) != nil {
		return false, nil
	}
	if withheldFromOtherSeats(p, a) {
		return false, nil
	}
	return targetDecidable(ctx, tx, a.TargetType, a.TargetID)
}

func requireDecisionGrants(p principal.Principal, a row) error {
	grants, err := decisionGrantsFor(a.Kind, a.TargetType)
	if err != nil {
		return err
	}
	for _, g := range grants {
		if !p.Permissions.Allows(g.Object, g.Action) {
			return fmt.Errorf("approving %s needs %s.%s: %w", a.Kind, g.Object, g.Action, apperrors.ErrPermissionDenied)
		}
	}
	return nil
}

// decisionGrantsFor derives every grant approving this staging requires: the
// kind's own, plus — for the kinds whose authority is the target's entity type —
// that type's. An unmapped kind, and a target-resolved kind staged with no
// target type, both error: neither can be decided by anyone, and answering an
// empty grant set for either would make them decidable by everyone.
func decisionGrantsFor(kind string, targetType *string) ([]grantRequirement, error) {
	fixed, hasFixed := decisionGrants[kind]
	action, resolvedFromTarget := targetResolvedGrants[kind]
	if !hasFixed && !resolvedFromTarget {
		// A verb a composed extension set registered at boot (extensionkinds.go).
		// Its grant is the one the operation itself gates on, which is the same
		// rule every entry above follows: deciding takes the grant performing
		// it takes. Consulted only AFTER the static maps, so a unit can never
		// answer for a core kind.
		if ext, registered := extensionKind(kind); registered {
			return []grantRequirement{{ext.RbacObject, ext.RbacAction}}, nil
		}
		return nil, fmt.Errorf("crmapprovals: kind %q has no decision-grant mapping", kind)
	}
	if !resolvedFromTarget {
		return fixed, nil
	}
	if targetType == nil {
		return nil, fmt.Errorf("crmapprovals: %s staged without a target type", kind)
	}
	// Cloned, not appended in place: fixed is the map's own slice, and appending
	// into its spare capacity would rewrite what the next caller reads.
	return append(slices.Clone(fixed), grantRequirement{*targetType, action}), nil
}

// DecisionGrantObjects reports the RBAC objects approving a staging of this kind
// against this target type requires.
//
// Exported for the composition layer's decidability gate, which has to prove
// more than that the staged target has a visibility rule: the derived grants must
// also be SATISFIABLE. An object outside the vocabulary a role document may name
// is allowed by no principal that can exist, so a staged row demanding it is
// permanently undecidable — the same dead row as a missing visibility rule,
// reached from the authority side. This package cannot ask identity for that
// vocabulary itself (a module never imports a sibling), so it answers what it
// demands and the composition layer, which imports both, checks it.
func DecisionGrantObjects(kind, targetType string) ([]string, error) {
	grants, err := decisionGrantsFor(kind, &targetType)
	if err != nil {
		return nil, err
	}
	objects := make([]string, 0, len(grants))
	for _, g := range grants {
		objects = append(objects, g.Object)
	}
	return objects, nil
}
