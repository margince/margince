// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// verdictNow is 15:00 on 10 March in Ho Chi Minh City, 08:00 UTC. The zone is
// seven hours ahead, so a promise due that local day began on 9 March in UTC.
var verdictNow = time.Date(2026, time.March, 10, 8, 0, 0, 0, time.UTC)

// pricedDeal is a deal worth EUR 48,000.00 on the watched contact's account.
func (w dealWatch) pricedDeal(t *testing.T) ids.UUID {
	t.Helper()
	amount, currency := int64(4_800_000), "EUR"
	deal, err := w.Deals.CreateDeal(w.Admin(), deals.CreateDealInput{
		Name: "Platform licence", PipelineID: w.pipeline, StageID: w.stage,
		AmountMinor: &amount, Currency: &currency,
	})
	if err != nil {
		t.Fatalf("creating the priced deal: %v", err)
	}
	return ids.UUID(deal.Id)
}

// promiseDue files a message on deal and the customer's commitment read out of
// it, due at due (nil for none).
func (w dealWatch) promiseDue(t *testing.T, deal ids.UUID, quote string, due *time.Time) ids.UUID {
	t.Helper()
	subject := "From the customer"
	message, _, err := w.Activities.LogActivity(w.Admin(), activities.LogActivityInput{
		Kind: "note", Subject: &subject, Body: &quote, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: w.contact}, {EntityType: "deal", EntityID: deal}},
	})
	if err != nil {
		t.Fatalf("logging the message: %v", err)
	}
	claim, err := w.Contacts.RecordConversationClaim(w.Admin(), contacts.ClaimInput{
		ContactID: ids.From[ids.ContactKind](w.contact), Kind: claimKindTheirs,
		Body: quote, ActivityID: ids.UUID(message.Id), Quote: quote, Source: "extraction", DueAt: due,
	})
	if err != nil {
		t.Fatalf("filing the commitment: %v", err)
	}
	return ids.UUID(claim.Id)
}

func (w dealWatch) verdicts(t *testing.T, deal ids.UUID) map[ids.UUID]crmcontracts.DealCommitment {
	t.Helper()
	reader := w.As(w.Rep1, []ids.UUID{w.Team1}, dealWatchPerms)
	got, err := newDealCommitmentHandlers(w.Pool, func() time.Time { return verdictNow }).read(reader, ids.From[ids.DealKind](deal))
	if err != nil {
		t.Fatalf("reading the deal's commitments: %v", err)
	}
	rows := make(map[ids.UUID]crmcontracts.DealCommitment, len(got.Data))
	for _, row := range got.Data {
		rows[ids.UUID(row.Id)] = row
	}
	return rows
}

// A promise is overdue once its deadline has passed, the rule the company
// card and the deal's own ordering use. A day deadline is stored as the end of
// that local day, so a promise due today is not late until the day is over.
func TestADealCommitmentIsOverdueOnceItsDeadlineHasPassed(t *testing.T) {
	w := seedDealWatch(t)
	zone := setInstallationZone(t, w.Env, "Asia/Ho_Chi_Minh")
	yesterday := time.Date(2026, time.March, 9, 23, 59, 59, 0, zone)
	endOfToday := time.Date(2026, time.March, 10, 23, 59, 59, 0, zone)
	thisMorning := time.Date(2026, time.March, 10, 9, 0, 0, 0, zone)
	late := w.promiseDue(t, w.deal, "We will send the signed NDA by Monday.", &yesterday)
	dueToday := w.promiseDue(t, w.deal, "We will confirm the kick-off date today.", &endOfToday)
	dueThisMorning := w.promiseDue(t, w.deal, "We will call you at nine.", &thisMorning)
	undated := w.promiseDue(t, w.deal, "We will introduce you to our CFO.", nil)

	rows := w.verdicts(t, w.deal)
	for claim, want := range map[ids.UUID]bool{late: true, dueToday: false, dueThisMorning: true, undated: false} {
		row, ok := rows[claim]
		if !ok {
			t.Fatalf("commitment %s is missing from the deal", claim)
		}
		if row.Overdue != want {
			t.Errorf("%q: overdue = %v, want %v", row.SourceQuote, row.Overdue, want)
		}
	}
}

// A total the customer names in the deal's currency that is not the deal's
// value is marked with the stated sum. A deposit, a monthly price and the
// deal's own value are not.
func TestADealCommitmentNamingAnotherTotalIsMarked(t *testing.T) {
	w := seedDealWatch(t)
	deal := w.pricedDeal(t)
	lower := w.promiseDue(t, deal, "We committed to proceeding with the Standard package at €42,000.", nil)
	deposit := w.promiseDue(t, deal, "We will pay a deposit of €10,000 next week.", nil)
	monthly := w.promiseDue(t, deal, "We agreed to €4,000 per month.", nil)
	same := w.promiseDue(t, deal, "We confirm the order at 48.000 EUR.", nil)

	rows := w.verdicts(t, deal)
	got := rows[lower].DiffersFromDeal
	if got == nil || got.AmountMinor != 4_200_000 || got.Currency != "EUR" {
		t.Errorf("a lower total reads differs_from_deal = %+v, want EUR 4200000", got)
	}
	for _, claim := range []ids.UUID{deposit, monthly, same} {
		if row := rows[claim]; row.DiffersFromDeal != nil {
			t.Errorf("%q is marked as differing: %+v", row.SourceQuote, row.DiffersFromDeal)
		}
	}
}

// A deal with no value is compared with nothing, whatever the words state.
func TestADealWithNoValueMarksNoCommitment(t *testing.T) {
	w := seedDealWatch(t)
	claim := w.promiseDue(t, w.deal, "We committed to the package at €42,000.", nil)
	if row := w.verdicts(t, w.deal)[claim]; row.DiffersFromDeal != nil {
		t.Errorf("a deal with no value marks %+v", row.DiffersFromDeal)
	}
}
