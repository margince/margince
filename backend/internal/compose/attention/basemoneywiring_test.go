// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The endpoint's own wiring, driven through Worklist rather than through the
// projection beneath it.
//
// Both facts here are about which SERVICE COPY the projection reads. A read
// narrows onto a copy — the scope, the named owner, this day's rates — and a
// projection that walked the shared service instead would answer with none of
// them, silently and in the shape of a plausible page. They live together
// because that one line is what they both hold, not because money and scope
// are one subject.

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// worklistOverDeals assembles the queue over one at-risk lane, with the FX seam
// bound the way production binds it.
func worklistOverDeals(fx BaseMoney, rows []RiskyDeal) *Service {
	return NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{}, nil, stubAtRisk{rows: rows}, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock).
		WithBaseMoney(fx)
}

func riskyDeal(name string, amount CurrencyAmount, owner ids.UUID) RiskyDeal {
	minor, units := amount.Minor, amount.Currency
	deal := RiskyDeal{DealID: ids.NewV7(), Name: name, AmountMinor: &minor, Currency: &units, QuietDays: 20}
	if !owner.IsZero() {
		holder := owner
		deal.OwnerID = &holder
	}
	return deal
}

// The seam reaches the ordering through the ENDPOINT, not only through the
// projection a unit test can call directly.
//
// The yen deal is worth about €30,000 and the euro deal €4,000, while their
// raw integers say the opposite ordering by a factor of twelve. Unwired, the
// page ranks the integers.
func TestTheEndpointRanksOnConvertedFigures(t *testing.T) {
	svc := worklistOverDeals(
		stubFX{base: "EUR", answers: map[CurrencyAmount]int64{
			fiveMillionYen: 3_000_000,
			eur(400_000):   400_000,
		}},
		[]RiskyDeal{
			riskyDeal("yen", fiveMillionYen, ids.UUID{}),
			riskyDeal("euro", eur(400_000), ids.UUID{}),
		})

	day, err := svc.Worklist(meetingPrepReader(), "", "", ids.UUID{}, 25)
	if err != nil {
		t.Fatalf("worklist: %v", err)
	}

	if len(day.Queue) != 2 {
		t.Fatalf("the queue carries %d rows, wanted the two deals", len(day.Queue))
	}
	if day.Queue[0].Title == nil || *day.Queue[0].Title != "yen" {
		t.Fatalf("the queue leads with %v, wanted the yen deal — it is worth €30,000 against €4,000", day.Queue[0].Title)
	}
	if day.Summary.BaseCurrency == nil || *day.Summary.BaseCurrency != "EUR" {
		t.Fatalf("the summary names %v, wanted EUR", day.Summary.BaseCurrency)
	}
}

// A named owner's queue carries THEIR work.
//
// The projection reads taskOwner off the service it is called on, and that
// field lives only on the read's own copy — so a page assembled through the
// shared service answers with the reader's own rows under the rep's name.
func TestOpeningANamedRepsQueueReachesTheProjection(t *testing.T) {
	lena := ids.MustParse("01a05500-0000-7000-8000-0000000000bb")
	svc := worklistOverDeals(
		stubFX{base: "EUR", answers: map[CurrencyAmount]int64{eur(900_000): 900_000}},
		[]RiskyDeal{
			riskyDeal("lenas", eur(900_000), lena),
			riskyDeal("somebody elses", eur(900_000), ids.MustParse("01a05500-0000-7000-8000-0000000000cc")),
		})

	day, err := svc.Worklist(meetingPrepReader(), "", "", lena, 25)
	if err != nil {
		t.Fatalf("worklist: %v", err)
	}

	if len(day.Queue) != 1 || day.Queue[0].Title == nil || *day.Queue[0].Title != "lenas" {
		titles := make([]string, 0, len(day.Queue))
		for _, row := range day.Queue {
			if row.Title != nil {
				titles = append(titles, *row.Title)
			}
		}
		t.Fatalf("Lena's queue came back as %v, wanted only the deal she owns", titles)
	}
}
