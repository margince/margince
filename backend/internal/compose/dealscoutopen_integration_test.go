// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// openScouted runs the pass and the opener, and answers how many deals opened.
func (e *scoutEnv) openScouted(t *testing.T) int {
	t.Helper()
	e.pass(t)
	opened, err := newScoutDealOpener(e.Pool, slog.New(slog.DiscardHandler)).OpenWorkspace(e.system())
	if err != nil {
		t.Fatalf("opening the suggested deals: %v", err)
	}
	return opened
}

// openedCorrection is the deal_open correction on the company's deal.
func (e *scoutEnv) openedCorrection(t *testing.T, company ids.UUID) (deals.DealCorrection, ids.DealID) {
	t.Helper()
	auditID, err := ids.Parse(e.WsScalar(t, `SELECT c.audit_log_id::text FROM deal_correction c JOIN deal d ON d.id = c.deal_id
		 WHERE d.company_id = $1 AND c.correction = 'deal_open' ORDER BY c.applied_at DESC LIMIT 1`, company))
	if err != nil {
		t.Fatalf("no deal_open correction on the company: %v", err)
	}
	var correction deals.DealCorrection
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		correction, err = e.Deals.CorrectionForAudit(e.Admin(), tx, auditID)
		return err
	}); err != nil {
		t.Fatalf("reading the opened deal's correction: %v", err)
	}
	return correction, correction.DealID
}

// Evidence on an owned company opens a deal for the owner, as Margince. The
// reason is on the create's audit row and a note is on the timeline. Undo
// archives it and dismisses the suggestion. The same evidence then opens
// nothing, and newer evidence opens it again.
func TestMarginceOpensADealForTheCompanyOwnerAndUndoArchivesIt(t *testing.T) {
	e := setupScout(t)
	e.GrantRole(t, e.AdminUser, "admin")
	acme := e.SeedCompany(t, "Acme GmbH", &e.AdminUser)
	e.buyerPair(t, acme, e.daysAgo(10))

	if opened := e.openScouted(t); opened != 1 {
		t.Fatalf("opened %d deals, want one", opened)
	}
	correction, deal := e.openedCorrection(t, acme)
	if n := e.WsCount(t, `SELECT count(*) FROM deal WHERE id = $1 AND owner_id = $2 AND status = 'open'`, deal, e.AdminUser); n != 1 {
		t.Fatal("the opened deal is not the company owner's open deal")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE id = $1 AND actor_id = $2 AND on_behalf_of = $3
		 AND evidence->>'basis' <> '' AND evidence->>'basis_owner' = $4`,
		correction.AuditLogID, marginceActorID, e.AdminUser, e.AdminUser.String()); n != 1 {
		t.Error("the create is not Margince's for the owner with the reason on it")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity a JOIN activity_link l ON l.activity_id = a.id
		 WHERE l.deal_id = $1 AND a.subject = 'Margince opened a deal for Acme GmbH'`, deal); n != 1 {
		t.Error("the opening is not on the deal's timeline")
	}
	if n := e.stored(t, acme, deals.SuggestionAccepted); n != 1 {
		t.Errorf("%d accepted suggestions, want the one the deal came from", n)
	}

	if _, err := e.Deals.RevertCorrection(e.Admin(), correction.ID, nil, nil); err != nil {
		t.Fatalf("Undo: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal WHERE id = $1 AND archived_at IS NOT NULL`, deal); n != 1 {
		t.Error("Undo left the opened deal live")
	}
	if n := e.stored(t, acme, deals.SuggestionDismissed); n != 1 {
		t.Errorf("%d dismissed suggestions after Undo, want one", n)
	}
	if opened := e.openScouted(t); opened != 0 {
		t.Errorf("the same evidence opened %d deals after Undo, want none", opened)
	}

	// The Undo happened days ago, and the customer wrote again since.
	e.WsExec(t, `UPDATE deal_suggestion SET decided_at = $2 WHERE company_id = $1 AND state = 'dismissed'`, acme, e.daysAgo(5))
	e.buyerPair(t, acme, e.daysAgo(1))
	if opened := e.openScouted(t); opened != 1 {
		t.Errorf("new evidence opened %d deals, want one", opened)
	}
}

// A company with no owner, or an owner who switched deal_open off, keeps the
// suggestion for a human.
func TestNoOwnerOrASwitchOffKeepsTheSuggestion(t *testing.T) {
	e := setupScout(t)
	unowned := e.SeedCompany(t, "Initech", nil)
	e.WsExec(t, `UPDATE company SET owner_id = NULL WHERE id = $1`, unowned)
	e.buyerPair(t, unowned, e.daysAgo(10))
	e.GrantRole(t, e.AdminUser, "admin")
	owned := e.SeedCompany(t, "Globex", &e.AdminUser)
	e.buyerPair(t, owned, e.daysAgo(10))
	if _, err := approvals.NewService(e.DB()).SetAutoApply(e.Admin(), deals.DealOpenKind, false); err != nil {
		t.Fatalf("switching deal_open off: %v", err)
	}

	if opened := e.openScouted(t); opened != 0 {
		t.Fatalf("opened %d deals, want none", opened)
	}
	for _, company := range []ids.UUID{unowned, owned} {
		if n := e.stored(t, company, deals.SuggestionOpen); n != 1 {
			t.Errorf("company %s has %d open suggestions, want its one", company, n)
		}
	}
}

// Undo of an opened deal the team has worked on since asks first, and the
// confirmed Undo archives it.
func TestUndoOfAnOpenedDealTheTeamWorkedOnNeedsConfirming(t *testing.T) {
	e := setupScout(t)
	e.GrantRole(t, e.AdminUser, "admin")
	acme := e.SeedCompany(t, "Acme GmbH", &e.AdminUser)
	e.buyerPair(t, acme, e.daysAgo(10))
	e.openScouted(t)
	correction, deal := e.openedCorrection(t, acme)
	subject := "Call notes"
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "note", Subject: &subject, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: deal.UUID}},
	}); err != nil {
		t.Fatal(err)
	}

	_, err := e.Deals.RevertCorrection(e.Admin(), correction.ID, nil, nil)
	var asks *deals.DealOpenUndoNeedsConfirmError
	if !errors.As(err, &asks) || asks.Notes != 1 {
		t.Fatalf("an unconfirmed Undo answered %v, want the question naming one note", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal WHERE id = $1 AND archived_at IS NULL`, deal); n != 1 {
		t.Fatal("the unconfirmed Undo archived the deal")
	}
	if _, err := e.Deals.RevertCorrectionConfirmed(e.Admin(), correction.ID, nil, nil, true); err != nil {
		t.Fatalf("the confirmed Undo: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal WHERE id = $1 AND archived_at IS NOT NULL`, deal); n != 1 {
		t.Error("the confirmed Undo left the deal live")
	}
}
