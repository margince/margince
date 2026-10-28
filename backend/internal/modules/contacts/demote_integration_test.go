// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// Demotion is the audited reverse of promotion (formulas §26). Its two
// branches are decided from the promote AUDIT ROW, and its hard block reads
// the relationship graph — both live in Postgres, so a unit test would only
// prove that a fake said what the test told it to.

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// contactState reads the two columns a demotion touches on the contact side.
func (e *promoteConsentEnv) contactState(t *testing.T, id ids.UUID) (archived bool, convertedFrom *ids.UUID) {
	t.Helper()
	if err := e.owner.QueryRow(context.Background(),
		`SELECT archived_at IS NOT NULL, converted_from_lead_id FROM contact WHERE id = $1`, id,
	).Scan(&archived, &convertedFrom); err != nil {
		t.Fatal(err)
	}
	return archived, convertedFrom
}

// A promotion that CREATED the contact unwinds fully: the lead is back at
// work, the contact it minted is archived, and the lineage is gone both ways.
func TestDemoteReversesAPromotionThatCreatedTheContact(t *testing.T) {
	e := setupPromoteConsent(t)
	lead := e.seedLead(t, "fresh@example.test")
	contact, merged, err := e.store.PromoteLead(e.ctx, lead, PromoteLeadInput{Trigger: "human_qualify"})
	if err != nil || merged {
		t.Fatalf("promote: err=%v merged=%t, want a created contact", err, merged)
	}

	preview, err := e.store.PreviewLeadDemotion(e.ctx, lead)
	if err != nil {
		t.Fatal(err)
	}
	out, err := e.store.DemoteLead(e.ctx, lead, "promoted the wrong prospect")
	if err != nil {
		t.Fatalf("demote: %v", err)
	}
	if preview.Unwind != out.Unwind || out.ContactId == nil || preview.ContactID.UUID != ids.UUID(*out.ContactId) {
		t.Fatalf("preview %+v differs from demotion %+v", preview, out)
	}
	if out.Unwind != crmcontracts.DemoteLeadResponseUnwindDemoteUnwindReversed {
		t.Errorf("unwind = %q, want reversed: the promotion created this contact", out.Unwind)
	}
	if out.Lead.Status != crmcontracts.LeadStatusEngaged || out.Lead.ArchivedAt != nil || out.Lead.PromotedContactId != nil {
		t.Errorf("restored lead = status %q archived=%v promoted_contact=%v; want engaged, live, unlinked",
			out.Lead.Status, out.Lead.ArchivedAt, out.Lead.PromotedContactId)
	}
	archived, from := e.contactState(t, ids.UUID(contact.Id))
	if !archived || from != nil {
		t.Errorf("created contact archived=%t converted_from=%v; want archived with the lineage nulled", archived, from)
	}

	// The undo of an undo is not a thing: a second demote finds nothing to reverse.
	var notPromoted *NotPromotedError
	if _, err := e.store.DemoteLead(e.ctx, lead, "again"); !errors.As(err, &notPromoted) {
		t.Errorf("second demote err = %v, want NotPromotedError (409)", err)
	}
}

// A promotion merged into an incumbent is not undone. Restoring its lead would
// leave both records live, so nothing moves.
func TestDemoteOfAMergeIntoAnExistingContactIsRefused(t *testing.T) {
	e := setupPromoteConsent(t)
	const shared = "known@example.test"
	existing, err := e.store.CreateContact(e.ctx, CreateContactInput{
		FullName: "Known Contact",
		Emails:   []ContactEmailInput{{Email: shared, EmailType: "work", IsPrimary: true}},
		Source:   "manual",
	})
	if err != nil {
		t.Fatalf("seed the incumbent contact: %v", err)
	}
	lead := e.seedLead(t, shared)
	if _, merged, err := e.store.PromoteLead(e.ctx, lead, PromoteLeadInput{Trigger: "human_qualify"}); err != nil || !merged {
		t.Fatalf("promote: err=%v merged=%t, want a merge", err, merged)
	}
	if _, from := e.contactState(t, ids.UUID(existing.Id)); from == nil {
		t.Fatal("promotion did not record the lineage on the incumbent; the test cannot see it removed")
	}

	if _, err := e.store.PreviewLeadDemotion(e.ctx, lead); !isContactStays(err, true) {
		t.Fatalf("preview of a merged demote = %v, want the merged refusal", err)
	}
	if _, err := e.store.DemoteLead(e.ctx, lead, "not the same contact after all"); !isContactStays(err, true) {
		t.Fatalf("demote of a merge = %v, want the merged refusal", err)
	}
	if archived, from := e.contactState(t, ids.UUID(existing.Id)); archived || from == nil {
		t.Errorf("incumbent archived=%t converted_from=%v; want it live and still converted", archived, from)
	}
	e.requirePromoted(t, lead)
}

// isContactStays reports a refused demote whose contact stays live.
func isContactStays(err error, merged bool) bool {
	stays, ok := errors.AsType[*ContactStaysError](err)
	return ok && stays.Merged == merged
}

// requirePromoted fails unless the lead is still promoted.
func (e *promoteConsentEnv) requirePromoted(t *testing.T, lead ids.LeadID) {
	t.Helper()
	var status string
	if err := e.owner.QueryRow(context.Background(), `SELECT status FROM lead WHERE id = $1`, lead.UUID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "promoted" {
		t.Errorf("lead status after a refused demote = %q, want promoted", status)
	}
}

// A contact merge repoints the promotion outcome to its survivor. Archiving
// that survivor would destroy merged history, so the demotion is refused.
func TestDemoteOfAMergeSurvivorIsRefused(t *testing.T) {
	e := setupPromoteConsent(t)
	// Owned by the rep who also owns the duplicate, so the merge keeps no
	// former owner's access: that needs identity's grant writer, unwired here.
	lead := e.seedLeadCreatedAt(t, "created@example.test", time.Now())
	created, merged, err := e.store.PromoteLead(e.ctx, lead, PromoteLeadInput{Trigger: "human_qualify"})
	if err != nil || merged {
		t.Fatalf("promote: err=%v merged=%t, want a created contact", err, merged)
	}
	other, err := e.store.CreateContact(e.ctx, CreateContactInput{
		FullName: "Other Contact",
		Emails:   []ContactEmailInput{{Email: "other@example.test", EmailType: "work", IsPrimary: true}},
		Source:   "manual",
	})
	if err != nil {
		t.Fatalf("seed the other contact: %v", err)
	}
	// The created contact survives the merge; the other's history now lives on it.
	if _, err := e.store.MergeContact(e.ctx, ids.From[ids.ContactKind](ids.UUID(other.Id)), ids.From[ids.ContactKind](ids.UUID(created.Id)), nil); err != nil {
		t.Fatalf("merge: %v", err)
	}

	if _, err := e.store.PreviewLeadDemotion(e.ctx, lead); !isContactStays(err, false) {
		t.Fatalf("preview of a survivor's demote = %v, want the refusal", err)
	}
	if _, err := e.store.DemoteLead(e.ctx, lead, "wrong prospect"); !isContactStays(err, false) {
		t.Fatalf("demote of a survivor = %v, want the refusal", err)
	}
	if archived, _ := e.contactState(t, ids.UUID(created.Id)); archived {
		t.Error("the survivor was archived")
	}
	e.requirePromoted(t, lead)
}

// The hard block: a contact who is a stakeholder on a live deal keeps their
// commercial state. Nothing moves, and the refusal names the rule.
func TestDemoteRefusesWhenTheContactIsOnALiveDeal(t *testing.T) {
	e := setupPromoteConsent(t)
	lead := e.seedLead(t, "buyer@example.test")
	contact, _, err := e.store.PromoteLead(e.ctx, lead, PromoteLeadInput{Trigger: "meeting_held"})
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	ctx := context.Background()
	pipeline, stage, deal := ids.NewV7(), ids.NewV7(), ids.NewV7()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO pipeline (id, name, is_default, position) VALUES ($1, 'Sales', true, 0)`, []any{pipeline}},
		{`INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability) VALUES ($1, $2, 'Qualify', 0, 'open', 10)`, []any{stage, pipeline}},
		{`INSERT INTO deal (id, owner_id, name, pipeline_id, stage_id, source, captured_by) VALUES ($1, $2, 'Rollout', $3, $4, 'manual', 'human:x')`, []any{deal, e.user, pipeline, stage}},
		{`INSERT INTO relationship (kind, contact_id, deal_id, source, captured_by) VALUES ('deal_stakeholder', $1, $2, 'manual', 'human:x')`, []any{ids.UUID(contact.Id), deal}},
	} {
		if _, err := e.owner.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatalf("seed deal graph: %v", err)
		}
	}

	var hasDeal *ContactHasDealError
	if _, err := e.store.PreviewLeadDemotion(e.ctx, lead); !errors.As(err, &hasDeal) {
		t.Fatalf("preview err = %v, want ContactHasDealError", err)
	}
	if _, err := e.store.DemoteLead(e.ctx, lead, "oops"); !errors.As(err, &hasDeal) {
		t.Fatalf("demote err = %v, want ContactHasDealError (422 contact_has_deal)", err)
	}
	archived, from := e.contactState(t, ids.UUID(contact.Id))
	if archived || from == nil {
		t.Errorf("a refused demote must change nothing: contact archived=%t converted_from=%v", archived, from)
	}
	var status string
	if err := e.owner.QueryRow(ctx, `SELECT status FROM lead WHERE id = $1`, lead).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "promoted" {
		t.Errorf("lead status = %q after a refused demote, want promoted", status)
	}
}

// The preview runs the promotion's own ladder without writing, so it names
// the outcome the promotion will take — and leaves no trace of having asked.
func TestPromotePreviewNamesTheOutcomeWithoutWriting(t *testing.T) {
	e := setupPromoteConsent(t)
	const shared = "known@example.test"
	existing, err := e.store.CreateContact(e.ctx, CreateContactInput{
		FullName: "Known Contact",
		Emails:   []ContactEmailInput{{Email: shared, EmailType: "work", IsPrimary: true}},
		Source:   "manual",
	})
	if err != nil {
		t.Fatalf("seed the incumbent contact: %v", err)
	}
	fresh := e.seedLead(t, "nobody@example.test")
	known := e.seedLead(t, shared)

	preview, err := e.store.PreviewLeadPromotion(e.ctx, fresh)
	if err != nil {
		t.Fatalf("preview fresh: %v", err)
	}
	if preview.Outcome != crmcontracts.PromoteLeadPreviewOutcomePromoteLeadPreviewOutcomeCreate || preview.Contact != nil {
		t.Errorf("fresh lead preview = %+v, want create with no contact", preview)
	}
	preview, err = e.store.PreviewLeadPromotion(e.ctx, known)
	if err != nil {
		t.Fatalf("preview known: %v", err)
	}
	if preview.Outcome != crmcontracts.PromoteLeadPreviewOutcomePromoteLeadPreviewOutcomeMerge || preview.Contact == nil || preview.Contact.Id != existing.Id {
		t.Errorf("known lead preview = %+v, want merge into %v", preview, existing.Id)
	}

	var status string
	var promotedContact *ids.UUID
	if err := e.owner.QueryRow(context.Background(),
		`SELECT status, promoted_contact_id FROM lead WHERE id = $1`, known).Scan(&status, &promotedContact); err != nil {
		t.Fatal(err)
	}
	if status != "contacted" || promotedContact != nil {
		t.Errorf("preview wrote to the lead: status=%q promoted_contact=%v", status, promotedContact)
	}
	// After the real promotion the preview has nothing to say.
	if _, _, err := e.store.PromoteLead(e.ctx, known, PromoteLeadInput{Trigger: "human_qualify"}); err != nil {
		t.Fatalf("promote: %v", err)
	}
	var already *AlreadyPromotedError
	if _, err := e.store.PreviewLeadPromotion(e.ctx, known); !errors.As(err, &already) {
		t.Errorf("preview of a promoted lead err = %v, want AlreadyPromotedError (409)", err)
	}
}

// withGrants answers a context for the same user with exactly the object
// grants given — how a role narrower than admin reaches the store.
func (e *promoteConsentEnv) withGrants(objects map[string]principal.ObjectGrant) context.Context {
	opCtx := principal.WithWorkspaceID(context.Background(), e.ws)
	opCtx = principal.WithCorrelationID(opCtx, ids.NewV7())
	return principal.WithActor(opCtx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.user.String(), UserID: e.user,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects:  objects,
			RowScope: principal.RowScopeAll,
		},
	})
}

// asStranger answers a context for a DIFFERENT user, scoped to their own rows.
// Every lead this suite seeds carries no owner, so none of them is theirs —
// they hold the grants to demote a lead and the scope to reach none of these.
func (e *promoteConsentEnv) asStranger() context.Context {
	opCtx := principal.WithWorkspaceID(context.Background(), e.ws)
	opCtx = principal.WithCorrelationID(opCtx, ids.NewV7())
	other := ids.NewV7()
	return principal.WithActor(opCtx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + other.String(), UserID: other,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects: map[string]principal.ObjectGrant{
				"lead":    {Create: true, Read: true, Update: true, Delete: true},
				"contact": {Create: true, Read: true, Update: true},
			},
			RowScope: principal.RowScopeOwn,
		},
	})
}

// A caller outside a lead's row scope learns NOTHING about it — including
// whether it was ever promoted.
//
// The demote reads the lead before it holds any lock, because the lead names
// the contact its lock order requires it to take first. That read is therefore
// in front of everything, so the row-scope probe has to be in front of IT.
// Behind it, the two ids below answer differently: a promoted lead refuses at
// the scope check and an unpromoted one refuses earlier, at the read, with
// "never promoted" — which is the promotion state of somebody else's record,
// one call at a time. The assertion is that they are the SAME refusal.
func TestDemoteTellsAStrangerNothingAboutALeadTheyCannotSee(t *testing.T) {
	e := setupPromoteConsent(t)
	promoted := e.seedLead(t, "reachable@example.test")
	if _, _, err := e.store.PromoteLead(e.ctx, promoted, PromoteLeadInput{Trigger: "human_qualify"}); err != nil {
		t.Fatalf("promote the lead this test then hides: %v", err)
	}
	neverPromoted := e.seedLead(t, "untouched@example.test")

	stranger := e.asStranger()
	refusals := map[string]error{}
	for _, lead := range []struct {
		what string
		id   ids.LeadID
	}{{"promoted", promoted}, {"never promoted", neverPromoted}} {
		_, err := e.store.DemoteLead(stranger, lead.id, "not mine to undo")
		if err == nil {
			t.Fatalf("a caller outside the row scope demoted a %s lead", lead.what)
		}
		if _, ok := errors.AsType[*NotPromotedError](err); ok {
			t.Errorf("demoting a %s lead outside the caller's scope answered %v — the read that names the "+
				"contact ran ahead of the row-scope probe, so the refusal reports the lead's state to "+
				"somebody who may not see the lead", lead.what, err)
		}
		refusals[lead.what] = err
	}
	// The refusal is not asserted to be one particular sentinel — which one the
	// scope layer answers is its own decision. What must hold is that BOTH ids
	// get the same one: a caller who can tell them apart has read the promotion
	// state of a record they cannot see.
	if got, want := fmt.Sprint(refusals["never promoted"]), fmt.Sprint(refusals["promoted"]); got != want {
		t.Errorf("a promoted lead refuses with %q and an unpromoted one with %q; two answers to the same "+
			"question is the leak", want, got)
	}
}

// A rep who may work leads and create contacts — the grants promotion itself
// runs under — can also undo a promotion, without holding contact.delete.
func TestDemoteNeedsNoArchiveGrantToUndoACreatedContact(t *testing.T) {
	e := setupPromoteConsent(t)
	rep := e.withGrants(map[string]principal.ObjectGrant{
		"lead":    {Create: true, Read: true, Update: true, Delete: true},
		"contact": {Create: true, Read: true, Update: true},
	})
	lead := e.seedLead(t, "undo@example.test")
	contact, _, err := e.store.PromoteLead(rep, lead, PromoteLeadInput{Trigger: "human_qualify"})
	if err != nil {
		t.Fatalf("promote as rep: %v", err)
	}
	out, err := e.store.DemoteLead(rep, lead, "clicked the wrong row")
	if err != nil {
		t.Fatalf("demote as rep: %v — the reversal must run under the same authority as the promotion", err)
	}
	if out.Unwind != crmcontracts.DemoteLeadResponseUnwindDemoteUnwindReversed {
		t.Errorf("unwind = %q, want reversed", out.Unwind)
	}
	if archived, _ := e.contactState(t, ids.UUID(contact.Id)); !archived {
		t.Error("the created contact must be archived by the reversal")
	}
}

// A contact that another lead has joined no longer belongs only to the first
// promotion, so the first lead's demotion is refused.
func TestDemoteOfAContactAnotherLeadPromotedIntoIsRefused(t *testing.T) {
	e := setupPromoteConsent(t)
	const shared = "twice@example.test"
	first := e.seedLead(t, shared)
	contact, merged, err := e.store.PromoteLead(e.ctx, first, PromoteLeadInput{Trigger: "human_qualify"})
	if err != nil || merged {
		t.Fatalf("first promote: err=%v merged=%t, want a created contact", err, merged)
	}
	second := e.seedLead(t, shared)
	if _, merged, err := e.store.PromoteLead(e.ctx, second, PromoteLeadInput{Trigger: "inbound_reply"}); err != nil || !merged {
		t.Fatalf("second promote: err=%v merged=%t, want a merge into the first's contact", err, merged)
	}

	if _, err := e.store.DemoteLead(e.ctx, first, "the first capture was wrong"); !isContactStays(err, false) {
		t.Fatalf("demote first = %v, want the refusal: another lead depends on this contact", err)
	}
	if archived, _ := e.contactState(t, ids.UUID(contact.Id)); archived {
		t.Error("the contact the second lead promoted into must stay live")
	}
	e.requirePromoted(t, first)
	e.requirePromoted(t, second)
}

// The preview returns a contact, so it needs the contact READ grant as well as
// the lead's: a role that may work leads but not read contacts is told the
// outcome (merge) and nothing about who.
func TestPromotePreviewWithholdsTheContactWithoutTheContactReadGrant(t *testing.T) {
	e := setupPromoteConsent(t)
	const shared = "known@example.test"
	if _, err := e.store.CreateContact(e.ctx, CreateContactInput{
		FullName: "Known Contact",
		Emails:   []ContactEmailInput{{Email: shared, EmailType: "work", IsPrimary: true}},
		Source:   "manual",
	}); err != nil {
		t.Fatalf("seed the incumbent contact: %v", err)
	}
	lead := e.seedLead(t, shared)
	leadsOnly := e.withGrants(map[string]principal.ObjectGrant{
		"lead": {Create: true, Read: true, Update: true, Delete: true},
	})
	preview, err := e.store.PreviewLeadPromotion(leadsOnly, lead)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if preview.Outcome != crmcontracts.PromoteLeadPreviewOutcomePromoteLeadPreviewOutcomeMerge {
		t.Errorf("outcome = %q, want merge — the outcome is a fact about the lead", preview.Outcome)
	}
	if preview.Contact != nil || preview.ContactWithheld == nil || !*preview.ContactWithheld {
		t.Errorf("contact=%v withheld=%v; a caller without contact.read must get no contact and be told so",
			preview.Contact, preview.ContactWithheld)
	}
}

// The version pin an agent's approval was released against is applied in the
// transaction that writes, under the locks the reversal already takes.
//
// A reversal is where a stale pin costs most. Redemption commits its own
// transaction and the handler then opens a fresh one, so a check anywhere
// earlier proves the row was right when the approval was CONSUMED, not when the
// unwind lands — and the agent controls both sides of that window, since its own
// auto-execute writes can commit inside it. Unpinned, the demotion reverses a
// promotion the approval was no longer describing.
func TestDemoteRefusesAVersionThePinNoLongerNames(t *testing.T) {
	e := setupPromoteConsent(t)
	lead := e.seedLead(t, "demotepin@example.test")
	if _, _, err := e.store.PromoteLead(e.ctx, lead, PromoteLeadInput{Trigger: "human_qualify"}); err != nil {
		t.Fatalf("promote: %v", err)
	}

	promoted, err := e.store.GetLead(e.ctx, lead, storekit.IncludeArchived)
	if err != nil {
		t.Fatal(err)
	}
	if promoted.Version == nil {
		t.Fatal("the promoted lead carries no version, so this case can pin nothing")
	}
	at := *promoted.Version

	stale := at - 1
	if _, err := e.store.PreviewLeadDemotion(e.ctx, lead, OnlyAtVersion(&stale)); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("stale preview err = %v", err)
	}
	if _, err := e.store.DemoteLead(e.ctx, lead, "stale", OnlyAtVersion(&stale)); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("a stale pin err = %v, want ErrVersionSkew — the reversal ran on a row the approval "+
			"does not describe", err)
	}
	out, err := e.store.DemoteLead(e.ctx, lead, "current", OnlyAtVersion(&at))
	if err != nil {
		t.Fatalf("the current version was refused: %v", err)
	}
	if out.Lead.Status != crmcontracts.LeadStatusEngaged {
		t.Errorf("status = %v, want engaged", out.Lead.Status)
	}
}

// No pin attaches no precondition, so the REST caller and an agent whose
// approval carried no version spell the call the same way.
func TestDemoteWithNoPinIsUnconditioned(t *testing.T) {
	e := setupPromoteConsent(t)
	lead := e.seedLead(t, "demoteunpinned@example.test")
	if _, _, err := e.store.PromoteLead(e.ctx, lead, PromoteLeadInput{Trigger: "human_qualify"}); err != nil {
		t.Fatalf("promote: %v", err)
	}

	out, err := e.store.DemoteLead(e.ctx, lead, "no pin", OnlyAtVersion(nil))
	if err != nil {
		t.Fatalf("a nil pin refused the write: %v", err)
	}
	if out.Lead.Status != crmcontracts.LeadStatusEngaged {
		t.Errorf("status = %v, want engaged", out.Lead.Status)
	}
}

// And the promotion's own pin, on the lead it graduates.
//
// The cost this closes is the opposite direction from the reversal's: unpinned,
// the promotion mints a contact from lead fields a concurrent edit may have
// changed since the human approved — content nobody released.
func TestPromoteRefusesAVersionThePinNoLongerNames(t *testing.T) {
	e := setupPromoteConsent(t)
	lead := e.seedLead(t, "promotepin@example.test")

	born, err := e.store.GetLead(e.ctx, lead, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if born.Version == nil {
		t.Fatal("the seeded lead carries no version, so this case can pin nothing")
	}
	at := *born.Version

	stale := at - 1
	if _, _, err := e.store.PromoteLead(e.ctx, lead, PromoteLeadInput{
		Trigger: "human_qualify", IfVersion: &stale,
	}); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("a stale pin err = %v, want ErrVersionSkew — the promotion read lead fields the "+
			"approval does not describe", err)
	}
	if _, _, err := e.store.PromoteLead(e.ctx, lead, PromoteLeadInput{
		Trigger: "human_qualify", IfVersion: &at,
	}); err != nil {
		t.Fatalf("the current version was refused: %v", err)
	}
}

// A promoted lead's address can be taken by a live lead while it is away, and
// demoting it back onto the ladder is then the same duplicate-email collision
// create and update answer.
func TestDemoteOfALeadWhoseEmailALiveLeadNowHoldsIsADuplicate(t *testing.T) {
	e := setupPromoteConsent(t)
	lead := e.seedLead(t, "promoted-then-taken@example.test")
	if _, _, err := e.store.PromoteLead(e.ctx, lead, PromoteLeadInput{Trigger: "human_qualify"}); err != nil {
		t.Fatalf("promote: %v", err)
	}
	e.seedLead(t, "promoted-then-taken@example.test")

	_, err := e.store.DemoteLead(e.ctx, lead, "promoted the wrong prospect")

	if _, ok := errors.AsType[*DuplicateLeadError](err); !ok {
		t.Fatalf("demote answered %v, want a duplicate-email conflict", err)
	}
}
