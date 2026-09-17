// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package briefs

// A field mask withholds a deal's amount on the deal list. The morning brief is
// a SECOND way to the same number, and it does not print it: the revenue factor
// is min(1, base_value / REVENUE_NORM), both halves are served — the factor in
// each item's feature vector, the norm beside the queue — and a ratio against a
// known divisor is the figure with one division in front of it.
//
// A unit test cannot fail any of this. The mask is rendered INTO the statement,
// so what has to be true is that Postgres agrees, and that is only knowable
// against a real database.

import (
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// maskedBriefRep is a seat that can rank a brief with the deal amount masked.
//
// RowScopeTeam and not All: auth.Unbounded reads row_scope=all as "every row"
// and skips masks outright, so a fixture on All would assert nothing about
// masking at all.
func maskedBriefRep(masks ...principal.FieldMask) principal.Permissions {
	return principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects: map[string]principal.ObjectGrant{
			"deal": {Read: true, Update: true}, "pipeline": {Read: true},
			"activity": {Read: true}, "contact": {Read: true},
			"company": {Read: true}, "relationship": {Read: true},
			"installation_settings": {Read: true},
		},
		RowScope:   principal.RowScopeTeam,
		FieldMasks: masks,
	}
}

// amountMask is the one mask this fixture ever sets, under the condition the
// case is about.
func amountMask(condition principal.MaskCondition) principal.FieldMask {
	return principal.FieldMask{Object: "deal", Field: "amount_minor", Condition: condition}
}

// seedColleagueDealWithMyTask is the shape that makes this more than theory: a
// deal on another team, carrying an open task assigned to the reader. The
// responsibility arm of the candidate query admits it — the rep has work on it
// — and the mask's write-authority arm does not, because the deal is not
// theirs to change. So it is a card the brief must draw and a figure the brief
// must not score.
func (b *briefEnv) seedColleagueDealWithMyTask(t *testing.T) ids.UUID {
	t.Helper()
	owner := integration.OwnerConn(t)
	deal := b.seedBriefDeal(t, owner, "Theirs", b.stageA, int64Ptr(60_000_00), closeOn(briefClock, 5), &b.Rep3)
	task := integration.SeedIDRow(t, owner, `INSERT INTO activity
		(id, kind, subject, occurred_at, source, captured_by, assignee_id, is_done)
		VALUES ($1, 'task', 'Send the offer', '2026-06-03T09:00:00Z', 'manual', 'human:x', $2, false)`, b.Rep1)
	integration.LinkActivity(t, owner, task, "deal", deal)
	return deal
}

// revenueOf is the ranked item's revenue factor, and a failure when the deal
// never reached the queue: "not there" and "scored zero" are different answers
// and only one of them is this test's subject.
func revenueOf(t *testing.T, ranking BriefRanking, deal ids.UUID) float64 {
	t.Helper()
	for _, item := range ranking.Queue {
		if item.DealID == deal {
			return item.Features.Revenue
		}
	}
	t.Fatalf("deal %s is not in the queue %v — the mask must withhold the figure, not the card", deal, queueDeals(ranking.Queue))
	return 0
}

// THE defect: a deal the reader may not price, scored on its price.
func TestAMaskedDealsAmountDoesNotScoreTheBrief(t *testing.T) {
	b := setupBrief(t)
	theirs := b.seedColleagueDealWithMyTask(t)

	masked := b.As(b.Rep1, []ids.UUID{b.Team1}, maskedBriefRep(amountMask(principal.MaskOutsideWriteAuthority)))
	ranking, err := b.engine.Rank(masked, briefClock)
	if err != nil {
		t.Fatal(err)
	}

	if got := revenueOf(t, ranking, theirs); got != 0 {
		t.Fatalf("the colleague's deal scored revenue %.6f, want the floor 0 — its €60k is masked, and "+
			"min(1, value/norm) hands it back divided", got)
	}
	// The control, in the same run: the reader's OWN deal carries the same
	// amount and is theirs to change, so the mask does not reach it and the
	// factor still reads. A test where everything floors proves only that
	// something broke.
	if got := revenueOf(t, ranking, b.dealA); got != 1.0 {
		t.Fatalf("the reader's own deal scored revenue %.6f, want 1.0 — the mask withholds a colleague's "+
			"figure, not every figure", got)
	}
}

// The norm is the OTHER half of the ratio, and a percentile taken over figures
// the reader may not see is that reading — one aggregate at a time, over the
// whole installation rather than over a deal they have work on.
//
// Ten priced deals is the fixture's load-bearing number, not decoration: below
// briefRevenueNormMinDeals the P90 is called noise and the fallback stands in,
// so a masked seat and an unmasked one agree by accident and a test asserting
// the fallback asserts nothing. Twelve puts a real percentile on the unmasked
// side, which is what makes the masked side's fallback an answer.
func TestTheRevenueNormIsTakenOverAmountsTheReaderMayRead(t *testing.T) {
	b := setupBrief(t)
	owner := integration.OwnerConn(t)
	for i := range 12 {
		b.seedBriefDeal(t, owner, fmt.Sprintf("Colleague %d", i), b.stageA,
			int64Ptr(int64(200_000_00+i)), closeOn(briefClock, 30), &b.Rep3)
	}

	unmasked, err := b.engine.Rank(b.As(b.Rep1, []ids.UUID{b.Team1}, maskedBriefRep()), briefClock)
	if err != nil {
		t.Fatal(err)
	}
	if unmasked.RevenueNormMinor == briefRevenueNormFallbackMinor {
		t.Fatalf("revenue norm = %d with twelve priced deals and no mask — the fixture never reached a "+
			"real percentile, so nothing below it is being tested", unmasked.RevenueNormMinor)
	}

	for _, condition := range []principal.MaskCondition{principal.MaskAlways, principal.MaskOutsideWriteAuthority} {
		t.Run(string(condition), func(t *testing.T) {
			masked, err := b.engine.Rank(b.As(b.Rep1, []ids.UUID{b.Team1}, maskedBriefRep(amountMask(condition))), briefClock)
			if err != nil {
				t.Fatal(err)
			}
			if masked.RevenueNormMinor != briefRevenueNormFallbackMinor {
				t.Fatalf("revenue norm = %d, want the %d fallback — every deal it could be taken over is "+
					"one this seat may not price, and the norm is served beside the queue",
					masked.RevenueNormMinor, briefRevenueNormFallbackMinor)
			}
		})
	}
}

// The always-masked seat scores nothing on revenue: not one card, however the
// norm came out.
func TestAnAlwaysMaskedSeatScoresNoRevenueAtAll(t *testing.T) {
	b := setupBrief(t)
	b.seedColleagueDealWithMyTask(t)

	masked := b.As(b.Rep1, []ids.UUID{b.Team1}, maskedBriefRep(amountMask(principal.MaskAlways)))
	ranking, err := b.engine.Rank(masked, briefClock)
	if err != nil {
		t.Fatal(err)
	}
	if len(ranking.Queue) == 0 {
		t.Fatal("the queue is empty — a masked seat must still get its cards, only not their figures")
	}
	for _, item := range ranking.Queue {
		if item.Features.Revenue != 0 {
			t.Fatalf("deal %s scored revenue %.6f for a seat that may read no amount at all",
				item.DealID, item.Features.Revenue)
		}
	}
}

// The unmasked seat is what the other two are measured against: same rows, same
// clock, and the figure still reads. Without it a mask that broke the statement
// outright would pass every assertion above.
func TestAnUnmaskedSeatStillScoresTheSameDeal(t *testing.T) {
	b := setupBrief(t)
	theirs := b.seedColleagueDealWithMyTask(t)

	unmasked := b.As(b.Rep1, []ids.UUID{b.Team1}, maskedBriefRep())
	ranking, err := b.engine.Rank(unmasked, briefClock)
	if err != nil {
		t.Fatal(err)
	}
	if got := revenueOf(t, ranking, theirs); got != 1.0 {
		t.Fatalf("with no mask the colleague's deal scored revenue %.6f, want 1.0", got)
	}
}
