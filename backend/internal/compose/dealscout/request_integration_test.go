// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package dealscout_test

// An explicit request opening a deal, through the product's own signal pass,
// scout and opener, against a real database.

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/dealscout"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// One explicit request opens a deal on its own. The budget is the amount, the
// decision day a provisional close date, and the request the brief. Each is
// marked as Margince's until a rep edits it. A need alone opens nothing.
func TestAnExplicitRequestOpensADealPrefilledFromTheMail(t *testing.T) {
	e := setupRequests(t)
	acme := e.SeedCompany(t, "Acme GmbH", &e.AdminUser)
	e.request(t, acme, dealscout.RequestKindExplicit, "our budget is 10,000 per year", sent)
	needy := e.SeedCompany(t, "Needy GmbH", &e.AdminUser)
	e.request(t, needy, dealscout.RequestKindNeed, "", sent)

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
	if deal.ExpectedCloseDate == nil || deal.ExpectedCloseDate.Format(time.DateOnly) != e.decisionDay() ||
		deal.CloseDateProvisional == nil || !*deal.CloseDateProvisional {
		t.Errorf("close date = %v provisional %v, want %s, provisional", deal.ExpectedCloseDate, deal.CloseDateProvisional, e.decisionDay())
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
	acme := e.SeedCompany(t, "Acme GmbH", &e.AdminUser)
	e.request(t, acme, dealscout.RequestKindExplicit, "USD 40,000", sent)

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

// A contact who was only copied, or a sender nobody authenticated, opens no
// deal with one request. Anyone can put a buyer on Cc or forge a From.
func TestACopiedOrUnauthenticatedBuyerOpensNoDealAlone(t *testing.T) {
	for name, m := range map[string]mail{
		"copied":          {role: "cc", authenticated: true},
		"unauthenticated": {role: "from"},
	} {
		t.Run(name, func(t *testing.T) {
			e := setupRequests(t)
			acme := e.SeedCompany(t, "Acme GmbH", &e.AdminUser)
			e.request(t, acme, dealscout.RequestKindExplicit, "our budget is 10,000", m)
			if n := e.WsCount(t, `SELECT count(*) FROM signal WHERE kind = 'new_opportunity'`); n != 1 {
				t.Fatalf("%d requests filed, want the one the scout must refuse", n)
			}
			if opened := e.openScouted(t); opened != 0 {
				t.Errorf("a %s buyer opened %d deals, want none", name, opened)
			}
		})
	}
}

// A request in mail protected after it was read still opens the deal. The deal
// gets nothing of that mail: no brief, amount or close date. Its timeline note
// names no content, and a colleague is not shown the suggestion.
func TestAProtectedRequestOpensADealThatShowsNothingOfTheMail(t *testing.T) {
	for _, protect := range []struct {
		name   string
		reason string
	}{
		{"by hand", activities.ReasonManual},
		{"workspace floor", activities.ReasonWorkspaceFloor},
		{"no record", activities.ReasonNoRecord},
		{"held shared mailbox", activities.ReasonSharedMailbox},
		{"counterparty", activities.ReasonCounterparty},
	} {
		t.Run(protect.name, func(t *testing.T) {
			e := setupRequests(t)
			acme := e.SeedCompany(t, "Acme GmbH", &e.AdminUser)
			message := e.request(t, acme, dealscout.RequestKindExplicit, "our budget is 10,000", sent)
			e.WsExec(t, `UPDATE activity SET audience = 'participants', audience_reason = $2 WHERE id = $1`, message, protect.reason)

			if opened := e.openScouted(t); opened != 1 {
				t.Fatalf("opened %d deals from a protected request, want one", opened)
			}
			if n := e.WsCount(t, `SELECT count(*) FROM deal_suggestion WHERE company_id = $1
				 AND (proposed_amount_minor IS NOT NULL OR proposed_close_date IS NOT NULL)`, acme); n != 0 {
				t.Error("the suggestion carries the protected request's budget or decision day")
			}
			deal := e.openedDeal(e.Admin(), t, acme)
			if deal.Description != nil || deal.AmountMinor != nil || deal.ExpectedCloseDate != nil || deal.AiWrittenFields != nil {
				t.Errorf("the deal carries %v / %v / %v / %v from the protected mail, want nothing",
					deal.Description, deal.AmountMinor, deal.ExpectedCloseDate, deal.AiWrittenFields)
			}
			if n := e.WsCount(t, `SELECT count(*) FROM activity a JOIN activity_link l ON l.activity_id = a.id
				 WHERE l.deal_id = $1 AND (a.body LIKE '%40 seats%' OR a.subject LIKE '%40 seats%')`, deal.Id); n != 0 {
				t.Error("the deal's timeline repeats the protected request")
			}
			colleague := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminWithSignals)
			if shown, _, err := e.Deals.ListSuggestions(colleague, deals.SuggestionQuery{CompanyID: &acme}); err != nil || len(shown) != 0 {
				t.Errorf("a colleague who may not read the mail is shown %d suggestions (%v), want none", len(shown), err)
			}
		})
	}
}

// Mail private only by a mailbox setting is shared while the switch is on: the
// deal shows what the customer asked for. Switched off after the reading, the
// deal opens and shows nothing of it.
func TestARoutinePrivateRequestIsSharedOnlyWhileTheSwitchIsOn(t *testing.T) {
	for _, on := range []bool{true, false} {
		t.Run(strconv.FormatBool(on), func(t *testing.T) {
			e := setupRequests(t)
			acme := e.SeedCompany(t, "Acme GmbH", &e.AdminUser)
			routine := sent
			routine.reason = activities.ReasonPosture
			e.request(t, acme, dealscout.RequestKindExplicit, "", routine)
			if !on {
				switchOff(t, shareKey)
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

// One cited signal whose mail may not be shared withholds every title from
// the deal's timeline, even when the newest request is shareable.
func TestOneProtectedSignalWithholdsTheTimelineTitles(t *testing.T) {
	e := setupRequests(t)
	acme := e.SeedCompany(t, "Acme GmbH", &e.AdminUser)
	older := e.request(t, acme, dealscout.RequestKindExplicit, "", sent)
	newer := e.request(t, acme, dealscout.RequestKindExplicit, "", sent)
	signal := func(message ids.UUID) deals.SuggestionEvidence {
		id, err := ids.Parse(e.WsScalar(t, `SELECT id::text FROM signal WHERE evidence->0->>'source_id' = $1`, message.String()))
		if err != nil {
			t.Fatalf("no signal for the request: %v", err)
		}
		return deals.SuggestionEvidence{Kind: deals.EvidenceSignal, SignalID: &id}
	}
	evidence := []deals.SuggestionEvidence{signal(newer), signal(older)}
	prefill := func() deals.DealPrefill {
		var fill deals.DealPrefill
		today := func(context.Context, pgx.Tx, time.Time) (time.Time, error) { return storekit.AsDate(e.now), nil }
		if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) (err error) {
			fill, err = dealscout.Prefill(e.now, today)(e.Admin(), tx, evidence)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return fill
	}
	if fill := prefill(); fill.Withheld || fill.Description == nil {
		t.Fatalf("two shareable requests: withheld=%v description=%v, want shown", fill.Withheld, fill.Description)
	}
	e.WsExec(t, `UPDATE activity SET audience = 'participants', audience_reason = 'manual' WHERE id = $1`, older)
	if fill := prefill(); !fill.Withheld || fill.Description == nil {
		t.Errorf("the older request protected: withheld=%v description=%v, want withheld with the newer brief", fill.Withheld, fill.Description)
	}
}
