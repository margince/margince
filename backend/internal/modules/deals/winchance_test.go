// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/winprobability"
)

func TestAnOpenDealCarriesItsOneWinChanceAndAClosedDealKeepsItsStageFigure(t *testing.T) {
	t.Parallel()
	open, won := ids.NewV7(), ids.NewV7()
	evidence := 40
	reads := 0
	store := &Store{clock: func() time.Time { return time.Date(2026, 11, 22, 9, 0, 0, 0, time.UTC) }, winChances: func(_ context.Context, _ pgx.Tx, _ time.Time, dealIDs []ids.UUID) (map[ids.UUID]winprobability.Chance, error) {
		reads++
		if len(dealIDs) != 1 || dealIDs[0] != open {
			t.Fatalf("asked the chance of %v, want only the open deal", dealIDs)
		}
		return map[ids.UUID]winprobability.Chance{open: {
			Percent: 33, Source: "stage", Typed: 60, InUse: 50, EvidenceN: &evidence, WonShare: 45,
			SelfCorrecting: true, Effect: new(0.5), EffectSignal: "quiet", EffectLearned: true, EffectSample: 20,
		}}, nil
	}}
	page := []crmcontracts.Deal{
		{Id: openapi_types.UUID(open), Status: crmcontracts.DealStatusOpen, WinProbability: new(60)},
		{Id: openapi_types.UUID(won), Status: crmcontracts.DealStatusWon, WinProbability: new(100)},
	}
	if err := store.stampWinChances(context.Background(), nil, page); err != nil {
		t.Fatal(err)
	}
	if reads != 1 || *page[0].WinProbability != 33 {
		t.Fatalf("reads %d, open deal at %v; want one read and 33", reads, page[0].WinProbability)
	}
	chance := page[0].WinChance
	if chance == nil || chance.InUse != 50 || chance.Typed != 60 || *chance.EffectSignal != crmcontracts.DealWinChanceEffectSignalQuiet || !*chance.EffectLearned {
		t.Fatalf("win chance %+v does not carry the parts the resolver gave", chance)
	}
	if want := "Stage typed 60% · 40 deals from here closed, 45% won · quiet lowers it (learned from 20 closed deals) · 33%"; chance.Explanation != want {
		t.Fatalf("explanation %q, want %q", chance.Explanation, want)
	}
	if page[1].WinChance != nil || *page[1].WinProbability != 100 {
		t.Fatalf("closed deal %+v must keep its stage figure and carry no chance", page[1])
	}
}

func TestAStoreWithNoWinChanceReadKeepsTheStoredProbability(t *testing.T) {
	t.Parallel()
	page := []crmcontracts.Deal{{Id: openapi_types.UUID(ids.NewV7()), Status: crmcontracts.DealStatusOpen, WinProbability: new(60)}}
	if err := (&Store{}).stampWinChances(context.Background(), nil, page); err != nil {
		t.Fatal(err)
	}
	if *page[0].WinProbability != 60 || page[0].WinChance != nil {
		t.Fatalf("deal %+v changed with no reader injected", page[0])
	}
}
