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

// A deal Figures cannot answer for — gone (archived, deleted) or no longer
// visible to this reader — drops the row entirely, rather than
// leaving an unidentifiable one on the page: no amount, no close date, no
// reason, yet still offering act/set_aside/dismiss over a deal that no longer
// resolves. A row that can name nothing is not a suggestion, and the refusal
// is an absent answer rather than an error, so one unresolved deal cannot take
// the whole read down.
func TestABriefRowWhoseDealCannotBeResolvedIsDropped(t *testing.T) {
	dealID := ids.NewV7()
	svc := (&Service{}).WithDealFacts(&stubDealFacts{figures: map[ids.UUID]DealFigures{}})
	day := crmcontracts.Attention{AsOf: rankInstant, ThisMorning: []crmcontracts.AttentionItem{briefRow(dealID)}}

	if err := svc.nameTheMoney(context.Background(), &day); err != nil {
		t.Fatalf("an unresolved deal failed the whole read: %v", err)
	}

	if len(day.ThisMorning) != 0 {
		t.Fatalf("ThisMorning still carries %d row(s); an unresolved deal's row should have been dropped", len(day.ThisMorning))
	}
}

// A row beside an unresolved one is untouched: dropping one deal's row is not
// a reason to drop, reorder or otherwise disturb its neighbours.
func TestABriefRowBesideAnUnresolvedOneIsUntouched(t *testing.T) {
	resolved, unresolved := ids.NewV7(), ids.NewV7()
	amount := int64(50_00)
	svc := (&Service{}).WithDealFacts(&stubDealFacts{figures: map[ids.UUID]DealFigures{
		resolved: {AmountMinor: &amount, Currency: "EUR"},
	}})
	day := crmcontracts.Attention{AsOf: rankInstant, ThisMorning: []crmcontracts.AttentionItem{
		briefRow(unresolved), briefRow(resolved),
	}}

	if err := svc.nameTheMoney(context.Background(), &day); err != nil {
		t.Fatalf("naming the money: %v", err)
	}

	if len(day.ThisMorning) != 1 {
		t.Fatalf("ThisMorning carries %d row(s), wanted exactly the resolved one", len(day.ThisMorning))
	}
	if day.ThisMorning[0].Id != resolved.String() {
		t.Fatalf("the surviving row is %q, wanted the resolved deal %q", day.ThisMorning[0].Id, resolved.String())
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

// A brief row carries the SAME overdue verdict the at-risk lane gives the
// identical deal — deals.Store.Figures now answers it calendar-date, workspace-
// zone aware, the same way deals.CloseIsOverdue does for that sibling lane, so
// the two rows for one deal state one verdict about whether it is late.
func TestABriefRowCarriesTheDealsOverdueVerdict(t *testing.T) {
	dealID := ids.NewV7()
	closes := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	svc := (&Service{}).WithDealFacts(&stubDealFacts{figures: map[ids.UUID]DealFigures{
		dealID: {ExpectedCloseDate: &closes, CloseOverdue: true},
	}})
	day := crmcontracts.Attention{AsOf: rankInstant, ThisMorning: []crmcontracts.AttentionItem{briefRow(dealID)}}

	if err := svc.nameTheMoney(context.Background(), &day); err != nil {
		t.Fatalf("naming the money: %v", err)
	}

	got := day.ThisMorning[0]
	if got.Overdue == nil || !*got.Overdue {
		t.Fatalf("Overdue = %v, wanted the deal's own overdue verdict (true)", got.Overdue)
	}
}

// A close date that has not passed carries no overdue verdict — the false is
// as load-bearing as the true, and a row that always answered true once a close
// date existed would badge every dated deal late.
func TestABriefRowCarriesAFutureCloseDateAsNotOverdue(t *testing.T) {
	dealID := ids.NewV7()
	closes := time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC)
	svc := (&Service{}).WithDealFacts(&stubDealFacts{figures: map[ids.UUID]DealFigures{
		dealID: {ExpectedCloseDate: &closes, CloseOverdue: false},
	}})
	day := crmcontracts.Attention{AsOf: rankInstant, ThisMorning: []crmcontracts.AttentionItem{briefRow(dealID)}}

	if err := svc.nameTheMoney(context.Background(), &day); err != nil {
		t.Fatalf("naming the money: %v", err)
	}

	got := day.ThisMorning[0]
	if got.Overdue == nil || *got.Overdue {
		t.Fatalf("Overdue = %v, wanted false for a close date that has not passed", got.Overdue)
	}
}
