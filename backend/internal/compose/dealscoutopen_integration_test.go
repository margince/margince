// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
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

// A suggestion raised before a user archives a deal no longer stands: the
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

// request records an explicit request for a quotation from the buyer, read
// from the mail, with the facts the extractor files under the cite.
func (e *scoutEnv) request(t *testing.T, company, mail, buyer ids.UUID, kind, budget string) {
	t.Helper()
	facts := map[string]any{requestKindKey: kind, decisionDateKey: e.now.AddDate(0, 1, 0).Format(time.DateOnly)}
	if budget != "" {
		facts[budgetQuoteKey] = budget
	}
	ctx := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), signalScanActor)
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := signals.RecordDerived(ctx, tx, signals.DerivedSignal{
			Kind: extractKindOpportunity, CompanyID: company, Summary: requestSummary, Severity: "info",
			Fingerprint: "request:" + mail.String(),
			Evidence: []signals.DerivedEvidence{{
				Snippet: requestSummary, ActivityID: mail, BuyerContactID: &buyer, CommercialRole: commercialRoleBuyer, Fields: facts,
			}},
		}, e.now)
		return err
	}); err != nil {
		t.Fatalf("recording the request: %v", err)
	}
}

const requestSummary = "They ask for a quote for 40 seats of the platform."

// openedDeal is the company's one opened deal as the reader in ctx reads it.
func (e *scoutEnv) openedDeal(ctx context.Context, t *testing.T, company ids.UUID) crmcontracts.Deal {
	t.Helper()
	id, err := ids.Parse(e.WsScalar(t, `SELECT id::text FROM deal WHERE company_id = $1 AND archived_at IS NULL`, company))
	if err != nil {
		t.Fatalf("no deal opened on the company: %v", err)
	}
	deal, err := e.Deals.GetDeal(ctx, ids.From[ids.DealKind](id), storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the opened deal: %v", err)
	}
	return deal
}

// One explicit request opens a deal on its own. The budget is the amount, the
// decision day a provisional close date, and the request the brief. Each is
// marked as Margince's until a rep edits it. A need alone opens nothing.
func TestAnExplicitRequestOpensADealPrefilledFromTheMail(t *testing.T) {
	e := setupScout(t)
	e.GrantRole(t, e.AdminUser, "admin")
	acme := e.SeedCompany(t, "Acme GmbH", &e.AdminUser)
	mail, buyer := e.buyingEmail(t, acme, e.daysAgo(2))
	e.request(t, acme, mail, buyer, requestKindExplicit, "our budget is 10,000 per year")
	needy := e.SeedCompany(t, "Needy GmbH", &e.AdminUser)
	needMail, needBuyer := e.buyingEmail(t, needy, e.daysAgo(2))
	e.request(t, needy, needMail, needBuyer, requestKindNeed, "")

	if opened := e.openScouted(t); opened != 1 {
		t.Fatalf("opened %d deals, want one for the explicit request", opened)
	}
	deal := e.openedDeal(e.Admin(), t, acme)
	var base string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) (err error) {
		base, err = identity.BaseCurrencyAppliedTx(e.Admin(), tx)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if deal.AmountMinor == nil || *deal.AmountMinor != 1_000_000 || deal.Currency == nil || *deal.Currency != base {
		t.Errorf("amount = %v %v, want 10,000 in the base currency %s", deal.AmountMinor, deal.Currency, base)
	}
	if deal.Description == nil || *deal.Description != requestSummary {
		t.Errorf("description = %v, want the request", deal.Description)
	}
	if deal.ExpectedCloseDate == nil || deal.CloseDateProvisional == nil || !*deal.CloseDateProvisional {
		t.Errorf("close date = %v provisional %v, want the decision day, provisional", deal.ExpectedCloseDate, deal.CloseDateProvisional)
	}
	if got := deal.AiWrittenFields; got == nil || !slices.Equal(*got, []string{"amount_minor", "currency", "description"}) {
		t.Errorf("ai_written_fields = %v, want the amount, the guessed currency and the description", got)
	}

	edited := "Forty seats, confirmed on the phone."
	if _, err := e.Deals.UpdateDeal(e.Admin(), ids.From[ids.DealKind](ids.UUID(deal.Id)), deals.UpdateDealInput{Description: &edited}); err != nil {
		t.Fatalf("editing the brief: %v", err)
	}
	if got := e.openedDeal(e.Admin(), t, acme).AiWrittenFields; got == nil || slices.Contains(*got, "description") {
		t.Errorf("after the rep's edit ai_written_fields = %v, want the description off it", got)
	}
	correction, _ := e.openedCorrection(t, acme)
	if _, err := e.Deals.RevertCorrection(e.Admin(), correction.ID, nil, nil); !errors.As(err, new(*deals.DealOpenUndoNeedsConfirmError)) {
		t.Fatalf("Undo after the rep's edit = %v, want a confirmation naming that work", err)
	}
}

// A budget that names its currency is used as stated, and no guess is marked.
func TestARequestBudgetInItsOwnCurrencyIsNotMarkedAsAGuess(t *testing.T) {
	e := setupScout(t)
	e.GrantRole(t, e.AdminUser, "admin")
	acme := e.SeedCompany(t, "Acme GmbH", &e.AdminUser)
	mail, buyer := e.buyingEmail(t, acme, e.daysAgo(2))
	e.request(t, acme, mail, buyer, requestKindExplicit, "USD 40,000")

	if opened := e.openScouted(t); opened != 1 {
		t.Fatalf("opened %d deals, want one", opened)
	}
	deal := e.openedDeal(e.Admin(), t, acme)
	if deal.AmountMinor == nil || *deal.AmountMinor != 4_000_000 || deal.Currency == nil || *deal.Currency != "USD" {
		t.Errorf("amount = %v %v, want 40,000 USD", deal.AmountMinor, deal.Currency)
	}
	if got := deal.AiWrittenFields; got == nil || slices.Contains(*got, "currency") || !slices.Contains(*got, "amount_minor") {
		t.Errorf("ai_written_fields = %v, want the amount without a guessed currency", got)
	}
}

// A request in mail someone protected opens the deal and gives it nothing of
// that mail. It has no brief, amount or close date. Its timeline note names no
// content. A colleague who may not read the mail sees none of it.
func TestAProtectedRequestOpensADealThatShowsNothingOfTheMail(t *testing.T) {
	e := setupScout(t)
	e.GrantRole(t, e.AdminUser, "admin")
	acme := e.SeedCompany(t, "Acme GmbH", &e.AdminUser)
	mail, buyer := e.buyingEmail(t, acme, e.daysAgo(2))
	e.request(t, acme, mail, buyer, requestKindExplicit, "our budget is 10,000")
	if _, err := e.Activities.SetAudience(e.Admin(), ids.From[ids.ActivityKind](mail),
		activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatalf("protecting the mail: %v", err)
	}

	if opened := e.openScouted(t); opened != 1 {
		t.Fatalf("opened %d deals from a protected request, want one", opened)
	}
	colleague := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminWithSignals)
	deal := e.openedDeal(colleague, t, acme)
	if deal.Description != nil || deal.AmountMinor != nil || deal.ExpectedCloseDate != nil || deal.AiWrittenFields != nil {
		t.Errorf("the deal carries %v / %v / %v / %v from the protected mail, want nothing",
			deal.Description, deal.AmountMinor, deal.ExpectedCloseDate, deal.AiWrittenFields)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity a JOIN activity_link l ON l.activity_id = a.id
		 WHERE l.deal_id = $1 AND (a.body LIKE '%40 seats%' OR a.subject LIKE '%40 seats%')`, deal.Id); n != 0 {
		t.Error("the deal's timeline repeats the protected request")
	}
	if shown := e.suggestions(colleague, t, acme); len(shown) != 0 {
		t.Errorf("a colleague who may not read the mail is shown %d suggestions, want none", len(shown))
	}
}

// Mail private only by a mailbox setting counts in full while the switch lets
// a model read it. The deal then shows what the customer asked for. With the
// switch off it is treated as protected.
func TestARoutinePrivateRequestIsSharedOnlyWhileTheSwitchIsOn(t *testing.T) {
	for _, on := range []bool{true, false} {
		t.Run(strconv.FormatBool(on), func(t *testing.T) {
			e := setupScout(t)
			e.GrantRole(t, e.AdminUser, "admin")
			acme := e.SeedCompany(t, "Acme GmbH", &e.AdminUser)
			mail, buyer := e.buyingEmail(t, acme, e.daysAgo(2))
			e.request(t, acme, mail, buyer, requestKindExplicit, "")
			e.WsExec(t, `UPDATE activity SET audience = 'participants', audience_reason = $2 WHERE id = $1`,
				mail, activities.ReasonPosture)
			if !on {
				e.WsExec(t, `INSERT INTO setting (key, value) VALUES ($1, 'false'::jsonb)
					ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, activities.PrivateMailRequests.Key())
				t.Cleanup(func() { privateMailSwitchBack(t) })
			}

			if opened := e.openScouted(t); opened != 1 {
				t.Fatalf("opened %d deals, want one", opened)
			}
			deal := e.openedDeal(e.Admin(), t, acme)
			if shared := deal.Description != nil && *deal.Description == requestSummary; shared != on {
				t.Errorf("with the switch %v the brief is %v", on, deal.Description)
			}
		})
	}
}
