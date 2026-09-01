// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The endpoint's own wiring, driven through Worklist rather than through the
// projection beneath it.
//
// Both facts here are about which SERVICE COPY the projection reads. A read
// narrows onto a copy — the scope, the named owner, this day's rates — and a
// projection that walked the shared service instead would answer with none of
// them, silently and in the shape of a plausible page.

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

func riskyDeal(name string, minor int64, currency string, owner ids.UUID) RiskyDeal {
	amount, units := minor, currency
	deal := RiskyDeal{DealID: ids.NewV7(), Name: name, AmountMinor: &amount, Currency: &units, QuietDays: 20}
	if !owner.IsZero() {
		holder := owner
		deal.OwnerID = &holder
	}
	return deal
}

// The seam reaches the ordering through the endpoint, not only through the
// projection a unit test can call directly. Unwired, the yen deal leads by
// being the larger integer.
func TestTheEndpointRanksOnConvertedFigures(t *testing.T) {
	svc := worklistOverDeals(
		stubFX{base: "EUR", milli: map[string]int64{"JPY": 6}},
		[]RiskyDeal{
			riskyDeal("yen", 5_000_000, "JPY", ids.UUID{}),
			riskyDeal("euro", 40_000, "EUR", ids.UUID{}),
		})

	day, err := svc.Worklist(meetingPrepReader(), "", "", ids.UUID{}, 25)
	if err != nil {
		t.Fatalf("worklist: %v", err)
	}

	if len(day.Queue) != 2 {
		t.Fatalf("the queue carries %d rows, wanted the two deals", len(day.Queue))
	}
	if day.Queue[0].Title == nil || *day.Queue[0].Title != "euro" {
		t.Fatalf("the queue leads with %v, wanted the euro deal — the yen figure is larger only as an integer", day.Queue[0].Title)
	}
	if day.Summary.BaseCurrency == nil || *day.Summary.BaseCurrency != "EUR" {
		t.Fatalf("the summary names %v, wanted EUR", day.Summary.BaseCurrency)
	}
}

// A named owner's queue carries their work. The projection reads taskOwner off
// the service it is called on, and that field is only ever set on the read's
// own copy — so this fails whenever the endpoint projects through the shared
// service instead, with a page that looks like the rep's day and is not.
func TestOpeningANamedRepsQueueReachesTheProjection(t *testing.T) {
	lena := ids.MustParse("01a05500-0000-7000-8000-0000000000bb")
	svc := worklistOverDeals(
		stubFX{base: "EUR", milli: map[string]int64{}},
		[]RiskyDeal{
			riskyDeal("lenas", 90_000, "EUR", lena),
			riskyDeal("somebody elses", 90_000, "EUR", ids.MustParse("01a05500-0000-7000-8000-0000000000cc")),
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
