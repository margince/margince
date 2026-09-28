// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// pricedRisk is one overdue deal carrying an amount, as the at-risk lane
// returns it.
func pricedRisk(amount CurrencyAmount) RiskyDeal {
	minor, currency := amount.Minor, amount.Currency
	return RiskyDeal{
		DealID: ids.NewV7(), Name: "Renewal", CloseOverdue: true,
		AmountMinor: &minor, Currency: &currency,
	}
}

// A yen amount the fixture prices BELOW the median. As a bare integer it is
// the largest figure on the page, so a record that skipped conversion would
// call it material and a record that converted would not.
var smallInYen = CurrencyAmount{Minor: 5_000_000, Currency: "JPY"}

func judgedService(risky []RiskyDeal) *Service {
	svc := unboundService()
	svc.atRisk = stubAtRisk{rows: risky}
	return svc.WithBaseMoney(stubFX{base: "EUR", answers: map[CurrencyAmount]int64{
		eur(100): 100, eur(200): 200, eur(50_000): 50_000, smallInYen: 150,
	}})
}

// The record keeps exactly the deals the queue calls material, in base money.
//
// Base amounts 100, 150, 200 and 50,000 put the lower median at 150, so the
// €200 and €50,000 deals clear it. Read as raw integers the median would be
// 200 and the yen deal would clear it instead of the €200 one.
func TestTheRecordedVerdictIsTheMaterialBarInBaseMoney(t *testing.T) {
	t.Parallel()
	small, yen, mid, big := pricedRisk(eur(100)), pricedRisk(smallInYen), pricedRisk(eur(200)), pricedRisk(eur(50_000))
	svc := judgedService([]RiskyDeal{small, yen, mid, big})

	got, err := svc.MaterialAtRisk(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	want := []ids.UUID{mid.DealID, big.DealID}
	if !slices.Equal(got, want) {
		t.Fatalf("recorded %v as material, want the €200 and €50,000 deals %v", got, want)
	}
}

// What is recorded is what the queue ranked by: every deal the record keeps
// carries the queue's `material` reason on the same day, and no other does.
func TestTheRecordAgreesWithTheQueuesMaterialReason(t *testing.T) {
	t.Parallel()
	risky := []RiskyDeal{pricedRisk(eur(100)), pricedRisk(smallInYen), pricedRisk(eur(200)), pricedRisk(eur(50_000))}
	svc := judgedService(risky)

	recorded, err := svc.MaterialAtRisk(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	items := renderEach(risky, riskItem)
	day := crmcontracts.Attention{AsOf: fixedClock(), AtRisk: &items}
	money, err := svc.priceTheDay(context.Background(), day)
	if err != nil {
		t.Fatal(err)
	}
	var ranked []ids.UUID
	for _, row := range classifyDay(day, day.AsOf, money) {
		for _, why := range row.item.Because {
			if why.Kind == "material" {
				ranked = append(ranked, ids.UUID(row.item.Subject.Id))
			}
		}
	}
	byText := func(a, b ids.UUID) int { return strings.Compare(a.String(), b.String()) }
	slices.SortFunc(ranked, byText)
	slices.SortFunc(recorded, byText)
	if len(ranked) == 0 || !slices.Equal(recorded, ranked) {
		t.Fatalf("the record kept %v and the queue called %v material", recorded, ranked)
	}
}

// A pipeline with nothing priced has no median, so nothing is material and
// nothing is recorded — rather than every deal being promoted.
func TestAnUnpricedPipelineRecordsNoVerdict(t *testing.T) {
	t.Parallel()
	svc := judgedService([]RiskyDeal{{DealID: ids.NewV7(), Name: "Unpriced", CloseOverdue: true}})

	got, err := svc.MaterialAtRisk(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("recorded %v over a pipeline with no priced deal", got)
	}
}

// The next-step figure counts whole local days: the first day that starts
// inside the window, up to but not including the day still running.
func TestTheFigureCountsTheWholeDaysTheWindowHolds(t *testing.T) {
	t.Parallel()
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	date := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }
	cases := []struct {
		name       string
		from, to   time.Time
		first, end time.Time
	}{
		{
			// 23:30 UTC is already the 11th in Berlin, and 22:00 UTC on the
			// 12th is still the 12th there — UTC days would be off by one.
			name: "mid-day bounds skip the partial first day and stop before today",
			from: time.Date(2026, 3, 10, 23, 30, 0, 0, time.UTC),
			to:   time.Date(2026, 3, 13, 22, 0, 0, 0, time.UTC),
			// Berlin: from = 11th 00:30, so the first whole day is the 12th.
			first: date(2026, 3, 12), end: date(2026, 3, 13),
		},
		{
			name:  "a window opening at local midnight counts that day",
			from:  time.Date(2026, 3, 10, 0, 0, 0, 0, berlin),
			to:    time.Date(2026, 3, 12, 9, 0, 0, 0, berlin),
			first: date(2026, 3, 10), end: date(2026, 3, 12),
		},
		{
			name:  "a window shorter than a day holds none",
			from:  time.Date(2026, 3, 10, 9, 0, 0, 0, berlin),
			to:    time.Date(2026, 3, 10, 18, 0, 0, 0, berlin),
			first: date(2026, 3, 11), end: date(2026, 3, 11),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := wholeDaysWithin(c.from, c.to, berlin)
			if !got.First.Equal(c.first) || !got.End.Equal(c.end) {
				t.Fatalf("days [%s, %s), want [%s, %s)",
					got.First.Format(time.DateOnly), got.End.Format(time.DateOnly),
					c.first.Format(time.DateOnly), c.end.Format(time.DateOnly))
			}
		})
	}
}
