// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package dealscout_test

// An explicit request opening a deal, through the product's own pass and
// opener, against a real database.

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/dealscout"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type requestEnv struct {
	*integration.Env
	now time.Time
}

func setupRequests(t *testing.T) *requestEnv {
	t.Helper()
	e := integration.Setup(t)
	integration.DealFixture(t, e)
	return &requestEnv{Env: e, now: clocktest.Now(t).Truncate(time.Microsecond)}
}

func (e *requestEnv) daysAgo(n int) time.Time { return e.now.AddDate(0, 0, -n) }

// openScouted runs the scout pass and then the opener, as the job does.
func (e *requestEnv) openScouted(t *testing.T) int {
	t.Helper()
	ctx := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), "agent:deal-scout")
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := compose.RunDealScout(ctx, tx, e.now)
		return err
	}); err != nil {
		t.Fatalf("the scout pass: %v", err)
	}
	opened, err := compose.OpenScoutedDeals(ctx, e.Pool, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("opening the suggested deals: %v", err)
	}
	return opened
}

// buyingEmail logs a buyer's inbound request, the buyer working at company.
func (e *requestEnv) buyingEmail(t *testing.T, company ids.UUID, at time.Time) (ids.UUID, ids.UUID) {
	t.Helper()
	buyer := e.SeedContact(t, "Mira Buyer", nil)
	contact, employer := ids.From[ids.ContactKind](buyer), ids.From[ids.CompanyKind](company)
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &contact, CompanyID: &employer, Source: "manual",
	}); err != nil {
		t.Fatalf("employing the buyer: %v", err)
	}
	subject, body, direction := "Platform request", "We need a platform migration. Please send a proposal.", "inbound"
	logged, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: &direction, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: buyer}},
	})
	if err != nil {
		t.Fatalf("logging the request: %v", err)
	}
	return ids.UUID(logged.Id), buyer
}

// openedCorrection is the deal_open correction on the company's deal.
func (e *requestEnv) openedCorrection(t *testing.T, company ids.UUID) deals.DealCorrection {
	t.Helper()
	auditID, err := ids.Parse(e.WsScalar(t, `SELECT c.audit_log_id::text FROM deal_correction c JOIN deal d ON d.id = c.deal_id
		 WHERE d.company_id = $1 AND c.correction = 'deal_open'`, company))
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
	return correction
}

// switchOff turns off reading private mail until the test ends. The setting
// is installation-wide.
func switchOff(t *testing.T) {
	t.Helper()
	owner := integration.OwnerConn(t)
	if _, err := owner.Exec(context.Background(), `INSERT INTO setting (key, value) VALUES ($1, 'false'::jsonb)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, activities.PrivateMailRequests.Key()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := owner.Exec(context.Background(), `DELETE FROM setting WHERE key = $1`,
			activities.PrivateMailRequests.Key()); err != nil {
			t.Errorf("restoring the private-mail switch: %v", err)
		}
	})
}

// request records an explicit request for a quotation from the buyer, read
// from the mail, with the facts the extractor files under the cite.
func (e *requestEnv) request(t *testing.T, company, mail, buyer ids.UUID, kind, budget string) {
	t.Helper()
	facts := map[string]any{dealscout.RequestKindKey: kind, dealscout.DecisionDateKey: e.now.AddDate(0, 1, 0).Format(time.DateOnly)}
	if budget != "" {
		facts[dealscout.BudgetQuoteKey] = budget
	}
	ctx := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), "agent:signal-scan")
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := signals.RecordDerived(ctx, tx, signals.DerivedSignal{
			Kind: "new_opportunity", CompanyID: company, Summary: requestSummary, Severity: "info",
			Fingerprint: "request:" + mail.String(),
			Evidence: []signals.DerivedEvidence{{
				Snippet: requestSummary, ActivityID: mail, BuyerContactID: &buyer, CommercialRole: "buyer", Fields: facts,
			}},
		}, e.now)
		return err
	}); err != nil {
		t.Fatalf("recording the request: %v", err)
	}
}

const requestSummary = "They ask for a quote for 40 seats of the platform."

// openedDeal is the company's one opened deal as the reader in ctx reads it.
func (e *requestEnv) openedDeal(ctx context.Context, t *testing.T, company ids.UUID) crmcontracts.Deal {
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
	e := setupRequests(t)
	e.GrantRole(t, e.AdminUser, "admin")
	acme := e.SeedCompany(t, "Acme GmbH", &e.AdminUser)
	mail, buyer := e.buyingEmail(t, acme, e.daysAgo(2))
	e.request(t, acme, mail, buyer, dealscout.RequestKindExplicit, "our budget is 10,000 per year")
	needy := e.SeedCompany(t, "Needy GmbH", &e.AdminUser)
	needMail, needBuyer := e.buyingEmail(t, needy, e.daysAgo(2))
	e.request(t, needy, needMail, needBuyer, dealscout.RequestKindNeed, "")

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
	correction := e.openedCorrection(t, acme)
	if _, err := e.Deals.RevertCorrection(e.Admin(), correction.ID, nil, nil); !errors.As(err, new(*deals.DealOpenUndoNeedsConfirmError)) {
		t.Fatalf("Undo after the rep's edit = %v, want a confirmation naming that work", err)
	}
}

// A budget that names its currency is used as stated, and no guess is marked.
func TestARequestBudgetInItsOwnCurrencyIsNotMarkedAsAGuess(t *testing.T) {
	e := setupRequests(t)
	e.GrantRole(t, e.AdminUser, "admin")
	acme := e.SeedCompany(t, "Acme GmbH", &e.AdminUser)
	mail, buyer := e.buyingEmail(t, acme, e.daysAgo(2))
	e.request(t, acme, mail, buyer, dealscout.RequestKindExplicit, "USD 40,000")

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
	e := setupRequests(t)
	e.GrantRole(t, e.AdminUser, "admin")
	acme := e.SeedCompany(t, "Acme GmbH", &e.AdminUser)
	mail, buyer := e.buyingEmail(t, acme, e.daysAgo(2))
	e.request(t, acme, mail, buyer, dealscout.RequestKindExplicit, "our budget is 10,000")
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
	if shown, _, err := e.Deals.ListSuggestions(colleague, deals.SuggestionQuery{CompanyID: &acme}); err != nil || len(shown) != 0 {
		t.Errorf("a colleague who may not read the mail is shown %d suggestions, want none", len(shown))
	}
}

// Mail private only by a mailbox setting counts in full while the switch lets
// a model read it. The deal then shows what the customer asked for. With the
// switch off it is treated as protected.
func TestARoutinePrivateRequestIsSharedOnlyWhileTheSwitchIsOn(t *testing.T) {
	for _, on := range []bool{true, false} {
		t.Run(strconv.FormatBool(on), func(t *testing.T) {
			e := setupRequests(t)
			e.GrantRole(t, e.AdminUser, "admin")
			acme := e.SeedCompany(t, "Acme GmbH", &e.AdminUser)
			mail, buyer := e.buyingEmail(t, acme, e.daysAgo(2))
			e.request(t, acme, mail, buyer, dealscout.RequestKindExplicit, "")
			e.WsExec(t, `UPDATE activity SET audience = 'participants', audience_reason = $2 WHERE id = $1`,
				mail, activities.ReasonPosture)
			if !on {
				switchOff(t)
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
