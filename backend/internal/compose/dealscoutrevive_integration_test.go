// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// lostDeal opens a deal on the company for owner and closes it as lost with
// a note and a catalog code, dated closedAt.
func (e *scoutEnv) lostDeal(t *testing.T, company ids.UUID, owner ids.UUID, closedAt time.Time) ids.DealID {
	t.Helper()
	companyID, ownerID := ids.From[ids.CompanyKind](company), ids.From[ids.UserKind](owner)
	created, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Acme platform", PipelineID: e.pipeline, StageID: e.open, CompanyID: &companyID, OwnerID: &ownerID, Source: "manual",
	})
	if err != nil {
		t.Fatalf("opening the deal: %v", err)
	}
	deal := ids.From[ids.DealKind](ids.UUID(created.Id))
	lost, err := ids.Parse(e.WsScalar(t, `SELECT id::text FROM stage WHERE pipeline_id = $1 AND semantic = 'lost' ORDER BY position LIMIT 1`, e.pipeline))
	if err != nil {
		t.Fatalf("no lost stage: %v", err)
	}
	note, code := "Budget frozen", deals.LossReasonWentSilent
	if _, err := e.Deals.AdvanceDeal(e.Admin(), deal, deals.AdvanceDealInput{
		ToStageID: ids.From[ids.StageKind](lost), LostReason: &note, LostReasonCode: &code,
	}); err != nil {
		t.Fatalf("losing the deal: %v", err)
	}
	e.WsExec(t, `UPDATE deal SET closed_at = $2 WHERE id = $1`, deal, closedAt)
	return deal
}

// revivedCorrection is the newest deal_revive correction on the deal.
func (e *scoutEnv) revivedCorrection(t *testing.T, deal ids.DealID) deals.DealCorrection {
	t.Helper()
	auditID, err := ids.Parse(e.WsScalar(t, `SELECT audit_log_id::text FROM deal_correction
		 WHERE deal_id = $1 AND correction = 'deal_revive' ORDER BY applied_at DESC LIMIT 1`, deal))
	if err != nil {
		t.Fatalf("no deal_revive correction on the deal: %v", err)
	}
	var correction deals.DealCorrection
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		correction, err = e.Deals.CorrectionForAudit(e.Admin(), tx, auditID)
		return err
	}); err != nil {
		t.Fatalf("reading the reopen's correction: %v", err)
	}
	return correction
}

// backdate stores the company's newest buying evidence as arrived at.
func (e *scoutEnv) backdate(t *testing.T, company ids.UUID, since, at time.Time) {
	t.Helper()
	e.WsExec(t, `UPDATE activity SET created_at = $3 WHERE created_at > $2 AND id IN (
		SELECT (cite->>'source_id')::uuid FROM signal s, jsonb_array_elements(s.evidence) cite
		 WHERE s.resolved_company_id = $1 AND cite->>'source_type' = 'activity')`, company, since, at)
	e.WsExec(t, `UPDATE signal SET created_at = $3 WHERE resolved_company_id = $1 AND created_at > $2`, company, since, at)
}

// reopenReceiptsShowTheReasonToTheOwnerOnly reads the deal's changes as its
// owner and as a colleague: both see the reopen, only the owner its reason.
func (e *scoutEnv) reopenReceiptsShowTheReasonToTheOwnerOnly(t *testing.T, deal ids.DealID) {
	t.Helper()
	owned, _, err := e.Deals.DealChanges(e.Admin(), deal, nil, 10)
	if err != nil || len(owned) != 1 {
		t.Fatalf("the owner reads %d changes (%v), want the reopen", len(owned), err)
	}
	if summary := dealReviveReceipt(owned[0]).Summary; !strings.HasPrefix(summary, `Reopened "Acme platform" in `) ||
		!strings.Contains(summary, "new_opportunity") {
		t.Errorf("the owner's receipt = %q, want the reopen with its reason", summary)
	}
	seen, _, err := e.Deals.DealChanges(e.rep(e.Rep1, e.Team1), deal, nil, 10)
	if err != nil || len(seen) != 1 {
		t.Fatalf("a colleague reads %d changes (%v), want the reopen", len(seen), err)
	}
	if seen[0].Basis != "" {
		t.Errorf("a colleague reads the reason %q, want none", seen[0].Basis)
	}
}

// New buying evidence reopens the company's lost deal at the first stage, as
// Margince for its owner. No second deal is opened. Undo
// closes it as lost again with its note, code and close day. Evidence that
// arrived before that Undo reopens nothing; evidence after it does.
func TestMarginceReopensALostDealAndUndoClosesItAgain(t *testing.T) {
	e := setupScout(t)
	e.GrantRole(t, e.AdminUser, "admin")
	acme := e.SeedCompany(t, "Acme GmbH", &e.AdminUser)
	closedAt := e.daysAgo(40).UTC()
	deal := e.lostDeal(t, acme, e.AdminUser, closedAt)
	e.buyerPair(t, acme, e.daysAgo(10))

	if done := e.openScouted(t); done != 1 {
		t.Fatalf("acted on %d suggestions, want the one reopen", done)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal WHERE company_id = $1`, acme); n != 1 {
		t.Fatalf("the company has %d deals, want only the reopened one", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal WHERE id = $1 AND status = 'open' AND stage_id = $2
		 AND closed_at IS NULL AND lost_reason IS NULL`, deal, e.open); n != 1 {
		t.Fatal("the lost deal is not open at the first stage")
	}
	correction := e.revivedCorrection(t, deal)
	if n := e.WsCount(t, `SELECT count(*) FROM deal_correction c JOIN deal_stage_history h ON h.id = c.stage_history_id
		 WHERE c.id = $1 AND h.to_stage_id = $2`, correction.ID, e.open); n != 1 {
		t.Fatal("the reopen names no stage move to the first stage")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE id = $1 AND actor_id = $2 AND on_behalf_of = $3
		 AND evidence->>'basis' <> '' AND evidence->>'basis_owner' = $4`,
		correction.AuditLogID, marginceActorID, e.AdminUser, e.AdminUser.String()); n != 1 {
		t.Error("the reopen is not Margince's for the owner with the reason on it")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal_suggestion WHERE company_id = $1 AND kind = 'revive'
		 AND state = 'accepted' AND accepted_deal_id = $2`, acme, deal); n != 1 {
		t.Error("the suggestion is not settled as the reopen of the deal")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity a JOIN activity_link l ON l.activity_id = a.id
		 WHERE l.deal_id = $1 AND a.subject = 'Margince reopened Acme platform'`, deal); n != 1 {
		t.Error("the reopen is not on the deal's timeline")
	}
	e.reopenReceiptsShowTheReasonToTheOwnerOnly(t, deal)
	if summary, err := e.Deals.PendingChangesOwnedBy(e.Admin(), nil); err != nil || summary.Revived != 1 {
		t.Errorf("the owner's change summary counts %d reopened deals (%v), want one", summary.Revived, err)
	}
	review, err := e.Deals.AppliedChangeReview(e.Admin(), deal, correction.AuditLogID)
	if err != nil || review.Kind != "deal_revive" {
		t.Fatalf("the reopen reviews as %q (%v), want deal_revive", review.Kind, err)
	}

	if _, err := e.Deals.RevertCorrection(e.Admin(), correction.ID, nil, nil); err != nil {
		t.Fatalf("Undo: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal WHERE id = $1 AND status = 'lost' AND lost_reason = 'Budget frozen'
		 AND lost_reason_code = $2 AND closed_at = $3`, deal, deals.LossReasonWentSilent, closedAt); n != 1 {
		t.Error("Undo did not close the deal as lost with its earlier reason, code and day")
	}
	undoneAt := e.revivedCorrection(t, deal).ReversedAt
	if undoneAt == nil {
		t.Fatal("Undo left the reopen unreversed")
	}

	e.buyerPair(t, acme, e.daysAgo(5))
	e.backdate(t, acme, closedAt, undoneAt.Add(-time.Minute))
	if done := e.openScouted(t); done != 0 {
		t.Errorf("evidence that arrived before the Undo acted %d times, want none", done)
	}
	e.buyerPair(t, acme, e.daysAgo(1))
	if done := e.openScouted(t); done != 1 {
		t.Errorf("evidence after the Undo acted %d times, want the reopen", done)
	}
}

// Evidence that reached Margince before the deal was lost reopens nothing and
// retires its suggestion. The company gets no second deal beside the lost one.
func TestEvidenceThatArrivedBeforeTheLossReopensNothing(t *testing.T) {
	e := setupScout(t)
	e.GrantRole(t, e.AdminUser, "admin")
	acme := e.SeedCompany(t, "Acme GmbH", &e.AdminUser)
	closedAt := e.daysAgo(40)
	deal := e.lostDeal(t, acme, e.AdminUser, closedAt)
	e.buyerPair(t, acme, e.daysAgo(10))
	e.backdate(t, acme, e.daysAgo(50), e.daysAgo(45))

	if done := e.openScouted(t); done != 0 {
		t.Fatalf("acted on %d suggestions, want none", done)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal WHERE company_id = $1`, acme); n != 1 {
		t.Errorf("the company has %d deals, want the lost one alone", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal WHERE id = $1 AND status = 'lost'`, deal); n != 1 {
		t.Error("the deal was reopened on evidence from before its loss")
	}
	if n := e.stored(t, acme, deals.SuggestionSuperseded); n != 1 {
		t.Errorf("%d superseded suggestions, want the one about the lost deal", n)
	}
}

// An owner who left, or who switched deal_revive off, keeps the lost deal
// closed and the suggestion open for a human.
func TestADepartedOwnerOrASwitchOffKeepsTheLostDealClosed(t *testing.T) {
	e := setupScout(t)
	e.GrantRole(t, e.AdminUser, "admin")
	departed := e.SeedCompany(t, "Initech", &e.AdminUser)
	gone := e.lostDeal(t, departed, e.Rep3, e.daysAgo(40))
	e.WsExec(t, `UPDATE app_user SET status = 'deactivated' WHERE id = $1`, e.Rep3)
	e.buyerPair(t, departed, e.daysAgo(10))
	switched := e.SeedCompany(t, "Globex", &e.AdminUser)
	off := e.lostDeal(t, switched, e.AdminUser, e.daysAgo(40))
	e.buyerPair(t, switched, e.daysAgo(10))
	if _, err := approvals.NewService(e.DB()).SetAutoApply(e.Admin(), deals.DealReviveKind, false); err != nil {
		t.Fatalf("switching deal_revive off: %v", err)
	}

	if done := e.openScouted(t); done != 0 {
		t.Fatalf("acted on %d suggestions, want none", done)
	}
	for company, deal := range map[ids.UUID]ids.DealID{departed: gone, switched: off} {
		if n := e.WsCount(t, `SELECT count(*) FROM deal WHERE company_id = $1 AND (id <> $2 OR status <> 'lost')`, company, deal); n != 0 {
			t.Errorf("company %s got a deal or a reopen, want its lost deal alone", company)
		}
		if n := e.stored(t, company, deals.SuggestionOpen); n != 1 {
			t.Errorf("company %s has %d open suggestions, want its one", company, n)
		}
	}
}

// Undo of a reopen is refused once a colleague moved the deal since.
func TestUndoOfAReopenTheTeamMovedSinceIsRefused(t *testing.T) {
	e := setupScout(t)
	e.GrantRole(t, e.AdminUser, "admin")
	acme := e.SeedCompany(t, "Acme GmbH", &e.AdminUser)
	deal := e.lostDeal(t, acme, e.AdminUser, e.daysAgo(40))
	e.buyerPair(t, acme, e.daysAgo(10))
	e.openScouted(t)
	correction := e.revivedCorrection(t, deal)
	later, err := ids.Parse(e.WsScalar(t, `SELECT id::text FROM stage WHERE pipeline_id = $1 AND semantic = 'open' AND id <> $2
		 ORDER BY position LIMIT 1`, e.pipeline, e.open))
	if err != nil {
		t.Fatalf("no second open stage: %v", err)
	}
	if _, err := e.Deals.AdvanceDeal(e.Admin(), deal, deals.AdvanceDealInput{ToStageID: ids.From[ids.StageKind](later)}); err != nil {
		t.Fatalf("moving the reopened deal: %v", err)
	}

	_, err = e.Deals.RevertCorrection(e.Admin(), correction.ID, nil, nil)
	var refused *deals.CorrectionReversalError
	if !errors.As(err, &refused) || !strings.Contains(refused.Reason, "reopened") {
		t.Fatalf("Undo after a colleague's move answered %v, want a refusal", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal WHERE id = $1 AND status = 'open' AND stage_id = $2`, deal, later); n != 1 {
		t.Error("the refused Undo moved the deal")
	}
}
