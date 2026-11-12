// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/extraction"
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

	review, err := e.Deals.AppliedChangeReview(e.Admin(), deal, correction.AuditLogID)
	if err != nil || review.Kind != "deal_open" {
		t.Fatalf("the opened deal reviews as %q (%v), want deal_open", review.Kind, err)
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
	e.WsExec(t, `UPDATE deal SET archived_at = $2 WHERE id = $1`, deal, e.daysAgo(5))
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
	// A colleague adds a task after the warning: the confirmation named less.
	task := "Send the offer"
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &task, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: deal.UUID}},
	}); err != nil {
		t.Fatal(err)
	}
	_, err = e.Deals.RevertCorrectionConfirmed(e.Admin(), correction.ID, nil, nil, asks.Work)
	var again *deals.DealOpenUndoNeedsConfirmError
	if !errors.As(err, &again) || again.Notes != 1 || again.Tasks != 1 || again.Work == asks.Work {
		t.Fatalf("a confirmation over stale work answered %v, want the question with the new task", err)
	}
	if _, err := e.Deals.RevertCorrectionConfirmed(e.Admin(), correction.ID, nil, nil, again.Work); err != nil {
		t.Fatalf("the confirmed Undo: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal WHERE id = $1 AND archived_at IS NOT NULL`, deal); n != 1 {
		t.Error("the confirmed Undo left the deal live")
	}
}

// A note written before Margince opened the deal but filed on it afterwards
// is the team's work too.
func TestAnOlderNoteFiledOnTheOpenedDealNeedsConfirming(t *testing.T) {
	e := setupScout(t)
	e.GrantRole(t, e.AdminUser, "admin")
	acme := e.SeedCompany(t, "Acme GmbH", &e.AdminUser)
	subject := "Earlier call notes"
	note, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "note", Subject: &subject, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	e.buyerPair(t, acme, e.daysAgo(10))
	e.openScouted(t)
	correction, deal := e.openedCorrection(t, acme)
	e.WsExec(t, `UPDATE activity SET created_at = $2 WHERE id = $1`, note.Id, correction.AppliedAt.Add(-time.Hour))
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, deal_id) VALUES ($1, 'deal', $2)`, note.Id, deal)

	_, err = e.Deals.RevertCorrection(e.Admin(), correction.ID, nil, nil)
	var asks *deals.DealOpenUndoNeedsConfirmError
	if !errors.As(err, &asks) || asks.Notes != 1 {
		t.Fatalf("Undo over an older note filed since answered %v, want the question naming it", err)
	}
}

// Evidence the workspace can no longer read by the time the deal would open
// opens no deal.
func TestEvidenceNoLongerWorkspaceReadableOpensNoDeal(t *testing.T) {
	e := setupScout(t)
	e.GrantRole(t, e.AdminUser, "admin")
	acme := e.SeedCompany(t, "Acme GmbH", &e.AdminUser)
	mail, buyer := e.buyingEmail(t, acme, e.daysAgo(10))
	e.signal(t, "new_opportunity", acme, mail, e.now, ids.Nil, buyer)
	e.signal(t, "commitment_made", acme, mail, e.now, ids.Nil)
	e.pass(t)
	suggestion := e.suggestions(e.signalReader(), t, acme)
	if len(suggestion) != 1 {
		t.Fatalf("the pass raised %d suggestions, want one", len(suggestion))
	}
	evidence := []deals.SuggestionEvidence{}
	for _, item := range suggestion[0].Evidence {
		evidence = append(evidence, item.SuggestionEvidence)
	}
	holds := func() bool {
		var held bool
		if err := database.WithWorkspaceTx(e.system(), e.Pool, func(tx pgx.Tx) error {
			var err error
			held, err = scoutEvidenceHolds(e.system(), tx, evidence)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return held
	}
	if !holds() {
		t.Fatal("the evidence does not hold before anything changed")
	}
	e.WsExec(t, `UPDATE activity SET audience = 'participants' WHERE id = $1`, mail)
	if holds() {
		t.Error("evidence citing mail only its participants may read still holds")
	}
	opened, err := newScoutDealOpener(e.Pool, slog.New(slog.DiscardHandler)).OpenWorkspace(e.system())
	if err != nil || opened != 0 {
		t.Fatalf("opened %d deals (%v) on mail only its participants may read, want none", opened, err)
	}
}

// An amount reaches the opened deal only when the document's own words state
// it; a reading with no quote opens the deal with no amount.
func TestAnAmountNeedsTheDocumentsOwnWords(t *testing.T) {
	e := setupScout(t)
	e.GrantRole(t, e.AdminUser, "admin")
	for _, c := range []struct {
		name, quote string
		want        *int64
	}{
		{"Quoted GmbH", "Total: 12.500,00 EUR", new(int64(1250000))},
		{"Unquoted GmbH", "", nil},
		{"Other GmbH", "Total: 9.900,00 EUR", nil},
	} {
		company := e.SeedCompany(t, c.name, &e.AdminUser)
		doc := e.document(t, e.email(t, "Angebot", "outbound", company, e.daysAgo(2)), "Angebot.pdf", "application/pdf")
		e.reading(t, doc,
			extraction.ExtractedField{Field: "amount_minor", Value: "1250000", Confidence: "high", SourceQuote: c.quote},
			extraction.ExtractedField{Field: "currency", Value: "EUR", Confidence: "high"})
		e.buyerPair(t, company, e.daysAgo(6))
		e.openScouted(t)
		_, deal := e.openedCorrection(t, company)
		got := e.WsScalar(t, `SELECT coalesce(amount_minor::text, '') FROM deal WHERE id = $1`, deal)
		want := ""
		if c.want != nil {
			want = strconv.FormatInt(*c.want, 10)
		}
		if got != want {
			t.Errorf("%s: the opened deal's amount is %q, want %q", c.name, got, want)
		}
	}
}

// archive archives the deal as the principal in ctx and dates the archive at.
func (e *scoutEnv) archive(ctx context.Context, t *testing.T, deal ids.DealID, at time.Time) {
	t.Helper()
	if _, err := e.Deals.ArchiveDeal(ctx, deal, nil); err != nil {
		t.Fatalf("archiving the deal: %v", err)
	}
	e.WsExec(t, `UPDATE deal SET archived_at = $2 WHERE id = $1`, deal, at)
}

// Archiving a deal by hand works like Undo: evidence from before the archive
// opens nothing, and newer evidence opens a deal again.
func TestArchivingADealKeepsItsOlderEvidenceFromOpeningAnother(t *testing.T) {
	e := setupScout(t)
	e.GrantRole(t, e.AdminUser, "admin")
	acme := e.SeedCompany(t, "Acme GmbH", &e.AdminUser)
	company := ids.From[ids.CompanyKind](acme)
	created, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Acme platform", PipelineID: e.pipeline, StageID: e.open, CompanyID: &company, Source: "manual",
	})
	if err != nil {
		t.Fatalf("opening the deal: %v", err)
	}
	e.buyerPair(t, acme, e.daysAgo(5))
	e.archive(e.Admin(), t, ids.From[ids.DealKind](ids.UUID(created.Id)), e.daysAgo(3))

	if opened := e.openScouted(t); opened != 0 {
		t.Fatalf("evidence from before the archive opened %d deals, want none", opened)
	}
	if n := e.stored(t, acme, deals.SuggestionOpen); n != 0 {
		t.Fatalf("evidence from before the archive raised %d suggestions, want none", n)
	}

	e.buyerPair(t, acme, e.daysAgo(1))
	if opened := e.openScouted(t); opened != 1 {
		t.Errorf("evidence from after the archive opened %d deals, want one", opened)
	}
}

// A lost deal archived since is not reopened: evidence from before the archive
// opens nothing, and newer evidence opens a new deal beside it.
func TestALostDealArchivedSinceSetsTheFloorAndStaysArchived(t *testing.T) {
	e := setupScout(t)
	e.GrantRole(t, e.AdminUser, "admin")
	acme := e.SeedCompany(t, "Acme GmbH", &e.AdminUser)
	lost := e.lostDeal(t, acme, e.AdminUser, e.daysAgo(40))
	e.buyerPair(t, acme, e.daysAgo(5))
	e.archive(e.Admin(), t, lost, e.daysAgo(3))

	if opened := e.openScouted(t); opened != 0 {
		t.Fatalf("evidence from before the archive opened %d deals, want none", opened)
	}

	e.buyerPair(t, acme, e.daysAgo(1))
	if opened := e.openScouted(t); opened != 1 {
		t.Fatalf("evidence from after the archive opened %d deals, want one", opened)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal WHERE id = $1 AND archived_at IS NOT NULL AND status = 'lost'`, lost); n != 1 {
		t.Error("the archived lost deal was brought back")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal WHERE company_id = $1 AND id <> $2 AND status = 'open' AND archived_at IS NULL`, acme, lost); n != 1 {
		t.Error("no new open deal beside the archived lost one")
	}
}

// A system archive, as retention and an import undo make, judges nothing:
// evidence from before it still opens a deal.
func TestASystemArchiveSetsNoFloor(t *testing.T) {
	e := setupScout(t)
	e.GrantRole(t, e.AdminUser, "admin")
	acme := e.SeedCompany(t, "Acme GmbH", &e.AdminUser)
	lost := e.lostDeal(t, acme, e.AdminUser, e.daysAgo(40))
	e.buyerPair(t, acme, e.daysAgo(5))
	e.archive(e.system(), t, lost, e.daysAgo(3))

	if opened := e.openScouted(t); opened != 1 {
		t.Errorf("evidence from before a system archive opened %d deals, want one", opened)
	}
}

// A suggestion raised before a person archives a deal no longer stands: the
// opener supersedes it and opens nothing.
func TestASuggestionRaisedBeforeAnArchiveOpensNothing(t *testing.T) {
	e := setupScout(t)
	e.GrantRole(t, e.AdminUser, "admin")
	acme := e.SeedCompany(t, "Acme GmbH", &e.AdminUser)
	lost := e.lostDeal(t, acme, e.AdminUser, e.daysAgo(40))
	e.buyerPair(t, acme, e.daysAgo(5))
	e.pass(t)
	if n := e.stored(t, acme, deals.SuggestionOpen); n != 1 {
		t.Fatalf("the pass raised %d suggestions, want one", n)
	}
	e.archive(e.Admin(), t, lost, e.daysAgo(3))

	opened, err := newScoutDealOpener(e.Pool, slog.New(slog.DiscardHandler)).OpenWorkspace(e.system())
	if err != nil || opened != 0 {
		t.Fatalf("opened %d deals (%v) from a suggestion older than the archive, want none", opened, err)
	}
	if n := e.stored(t, acme, deals.SuggestionSuperseded); n != 1 {
		t.Errorf("%d superseded suggestions, want the one the archive outdated", n)
	}
}
