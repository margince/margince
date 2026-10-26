// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package network

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// edgesTo is one colleague's edges to several contacts, n exchanges each.
func edgesTo(user ids.UUID, contacts int, n int) []ColleagueEdge {
	out := make([]ColleagueEdge, 0, contacts)
	for range contacts {
		out = append(out, ColleagueEdge{UserID: user, ContactID: ids.NewV7(), Count90d: n})
	}
	return out
}

func findKind(risks []crmcontracts.DealCoverageRisk, kind string) *crmcontracts.DealCoverageRisk {
	for i := range risks {
		if string(risks[i].Kind) == kind {
			return &risks[i]
		}
	}
	return nil
}

// One colleague who writes to six seats is one colleague, and the finding
// that names them names the one entry our side carries.
func TestOneColleagueOnSixSeatsIsOneColleagueOnOurSide(t *testing.T) {
	lena := ids.NewV7()
	coverage := DealCoverage{DealID: ids.NewV7(), OurSide: edgesTo(lena, 6, 3)}
	coverage.Risks = foldRisks(coverage, testEnglish)

	out := wireCoverage(coverage, map[ids.UUID]string{lena: "Lena Fischer"}, nil)

	if len(out.OurSide) != 1 {
		t.Fatalf("our side lists %d entries for one colleague, want 1", len(out.OurSide))
	}
	if out.OurSide[0].Interactions90d != 18 {
		t.Errorf("interactions_90d = %d, want the 18 exchanges across all six seats",
			out.OurSide[0].Interactions90d)
	}
	risk := findKind(out.Risks, RiskSingleThreadedOurs)
	if risk == nil || risk.UserIds == nil || len(*risk.UserIds) != 1 ||
		ids.UUID((*risk.UserIds)[0]) != ids.UUID(out.OurSide[0].UserId) {
		t.Errorf("the finding does not name the one colleague our side lists: %+v", out.Risks)
	}
}

// Six colleagues sharing the exchanges are six entries, and no finding
// claims that one of them carries the deal.
func TestSixColleaguesSharingTheWorkRaiseNoOneColleagueFinding(t *testing.T) {
	var edges []ColleagueEdge
	for range 6 {
		edges = append(edges, edgesTo(ids.NewV7(), 2, 3)...)
	}
	coverage := DealCoverage{DealID: ids.NewV7(), OurSide: edges}
	coverage.Risks = foldRisks(coverage, testEnglish)

	out := wireCoverage(coverage, nil, nil)

	if len(out.OurSide) != 6 {
		t.Errorf("our side lists %d entries for six colleagues, want 6", len(out.OurSide))
	}
	if findKind(out.Risks, RiskSingleThreadedOurs) != nil {
		t.Errorf("six colleagues with equal shares raised %q", RiskSingleThreadedOurs)
	}
}

func TestOurColleaguesKeepsFirstSeenOrderAndTheWarmestScore(t *testing.T) {
	warm, cool := ids.NewV7(), ids.NewV7()
	edges := []ColleagueEdge{
		{UserID: warm, Count90d: 9},
		{UserID: cool, Count90d: 1},
		{UserID: warm, Count90d: 2},
	}
	edges[0].Strength.Strength = 80
	edges[2].Strength.Strength = 20

	got := OurColleagues(edges)

	if len(got) != 2 || got[0].UserID != warm || got[1].UserID != cool {
		t.Fatalf("OurColleagues = %+v, want warm then cool", got)
	}
	if got[0].Count90d != 11 || got[0].Strength.Strength != 80 {
		t.Errorf("warm folded to %d exchanges at %d, want 11 at 80",
			got[0].Count90d, got[0].Strength.Strength)
	}
}

// A seat dated by its day goes out with the day precision, so the page draws
// a date and never a midday time nobody logged.
func TestASeatGoesOutWithWhenTheContactLastWrote(t *testing.T) {
	wrote, quiet := ids.NewV7(), ids.NewV7()
	at := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	out := wireCoverage(DealCoverage{
		DealID: ids.NewV7(),
		Stakeholders: []deals.DealStakeholder{
			{ContactID: wrote, Role: "champion", Engaged: true},
			{ContactID: quiet, Role: "user"},
		},
		SeatsInbound: map[ids.UUID]SeatInbound{wrote: {At: at, Precision: "day"}},
	}, nil, nil)

	first := out.Stakeholders[0]
	if first.LastInboundAt == nil || !first.LastInboundAt.Equal(at) ||
		first.LastInboundPrecision == nil || *first.LastInboundPrecision != "day" {
		t.Errorf("the seat that wrote went out as %+v, want 20 May as a day", first)
	}
	if second := out.Stakeholders[1]; second.LastInboundAt != nil || second.LastInboundPrecision != nil {
		t.Errorf("a seat with no message went out dated: %+v", second)
	}
}
