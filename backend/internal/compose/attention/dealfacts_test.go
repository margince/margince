// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The deal-figures pass.
//
// The row it exists for is the overnight brief's: a rep opens their day on it,
// and before this it named a deal and said nothing else about it.

import (
	"context"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type stubDealFacts struct {
	figures map[ids.UUID]DealFigures
	asked   []ids.UUID
	err     error
}

func (s *stubDealFacts) Figures(_ context.Context, dealIDs []ids.UUID) (map[ids.UUID]DealFigures, error) {
	s.asked = append(s.asked, dealIDs...)
	return s.figures, s.err
}

func briefRow(dealID ids.UUID) crmcontracts.AttentionItem {
	return crmcontracts.AttentionItem{
		Id:      dealID.String(),
		Source:  "brief_item",
		Actions: []crmcontracts.AttentionItemActions{},
		Subject: &crmcontracts.AttentionSubject{
			Type: subjectDeal,
			Id:   openapi_types.UUID(dealID),
		},
	}
}

// A brief row states the deal's money and its close date, which is the whole
// point: a card naming a deal and nothing else cannot be acted on.
func TestABriefRowGainsItsDealsFigures(t *testing.T) {
	dealID := ids.NewV7()
	amount := int64(160_100_00)
	closes := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	svc := (&Service{}).WithDealFacts(&stubDealFacts{figures: map[ids.UUID]DealFigures{
		dealID: {AmountMinor: &amount, Currency: "EUR", ExpectedCloseDate: &closes},
	}})
	day := crmcontracts.Attention{AsOf: rankInstant, ThisMorning: []crmcontracts.AttentionItem{briefRow(dealID)}}

	if err := svc.nameTheMoney(context.Background(), &day); err != nil {
		t.Fatalf("naming the money: %v", err)
	}

	got := day.ThisMorning[0]
	if got.Deal == nil {
		t.Fatal("the brief row still carries no deal figures")
	}
	if got.Deal.AmountMinor == nil || *got.Deal.AmountMinor != amount {
		t.Fatalf("the row says %v, wanted the deal's amount %d", got.Deal.AmountMinor, amount)
	}
	if got.Deal.Currency == nil || *got.Deal.Currency != "EUR" {
		t.Fatalf("the row says %v currency — a figure whose units are unnamed is dropped by the client", got.Deal.Currency)
	}
	if got.DueAt == nil || !got.DueAt.Equal(closes) {
		t.Fatalf("the row's deadline is %v, wanted the close date %v", got.DueAt, closes)
	}
}

// A deal the reader may not see leaves the row as it was. The refusal is an
// absent answer, not an error, so one withheld deal cannot take the page down.
func TestAWithheldDealLeavesTheRowUnchanged(t *testing.T) {
	dealID := ids.NewV7()
	svc := (&Service{}).WithDealFacts(&stubDealFacts{figures: map[ids.UUID]DealFigures{}})
	day := crmcontracts.Attention{AsOf: rankInstant, ThisMorning: []crmcontracts.AttentionItem{briefRow(dealID)}}

	if err := svc.nameTheMoney(context.Background(), &day); err != nil {
		t.Fatalf("a deal the reader may not see failed the whole read: %v", err)
	}

	if day.ThisMorning[0].Deal != nil {
		t.Fatal("a withheld deal put figures on the row anyway")
	}
}

// A row that already carries figures is left alone, and its deal is never
// asked about. The lane that produced it read the deal under this same reader,
// and a second answer to a settled question is how two numbers come to
// disagree.
func TestARowThatAlreadyHasFiguresIsNotAskedAbout(t *testing.T) {
	dealID := ids.NewV7()
	existing := int64(42_00)
	stub := &stubDealFacts{figures: map[ids.UUID]DealFigures{}}
	row := briefRow(dealID)
	row.Deal = &crmcontracts.AttentionDealFacts{AmountMinor: &existing}
	day := crmcontracts.Attention{AsOf: rankInstant, ThisMorning: []crmcontracts.AttentionItem{row}}

	if err := (&Service{}).WithDealFacts(stub).nameTheMoney(context.Background(), &day); err != nil {
		t.Fatalf("naming the money: %v", err)
	}

	if len(stub.asked) != 0 {
		t.Fatalf("asked about %v, and the row already knew its own figures", stub.asked)
	}
	if day.ThisMorning[0].Deal.AmountMinor == nil || *day.ThisMorning[0].Deal.AmountMinor != existing {
		t.Fatal("the row's own figures were overwritten")
	}
}

// The same deal on several rows is one question. A page of brief items about
// one account would otherwise ask about it once per row.
func TestOneDealNamedTwiceIsAskedAboutOnce(t *testing.T) {
	dealID := ids.NewV7()
	stub := &stubDealFacts{figures: map[ids.UUID]DealFigures{}}
	day := crmcontracts.Attention{AsOf: rankInstant, ThisMorning: []crmcontracts.AttentionItem{
		briefRow(dealID), briefRow(dealID),
	}}

	if err := (&Service{}).WithDealFacts(stub).nameTheMoney(context.Background(), &day); err != nil {
		t.Fatalf("naming the money: %v", err)
	}

	if len(stub.asked) != 1 {
		t.Fatalf("asked %d times about one deal", len(stub.asked))
	}
}

// Unbound, the pass does nothing and says so by not failing: the seam is
// optional, and a feed without it sends rows the way it always did.
func TestAnUnboundReaderLeavesEveryRowAlone(t *testing.T) {
	day := crmcontracts.Attention{AsOf: rankInstant, ThisMorning: []crmcontracts.AttentionItem{
		briefRow(ids.NewV7()),
	}}

	if err := (&Service{}).nameTheMoney(context.Background(), &day); err != nil {
		t.Fatalf("an unbound reader failed the read: %v", err)
	}
	if day.ThisMorning[0].Deal != nil {
		t.Fatal("an unbound reader invented figures")
	}
}
