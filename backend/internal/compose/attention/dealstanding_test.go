// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The standing pass.
//
// The row it exists for is a deal row naming a step over a deal the reader
// knows nothing about: "draft a reply" reads one way on a deal closing Friday
// and another on one that went cold in June, and the row did not say which.

import (
	"context"
	"errors"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type stubDealStandings struct {
	standings map[ids.UUID]DealStanding
	asked     []ids.UUID
	calls     int
	err       error
}

func (s *stubDealStandings) CachedStandings(
	_ context.Context, dealIDs []ids.UUID,
) (map[ids.UUID]DealStanding, error) {
	s.calls++
	s.asked = append(s.asked, dealIDs...)
	return s.standings, s.err
}

func standingOf(word, line string) DealStanding {
	taken := time.Date(2026, 9, 4, 6, 42, 0, 0, time.UTC)
	return DealStanding{Standing: word, DecisiveLine: line, AsOf: &taken}
}

// THE case this pass exists for. The deal page already read this deal and said
// it is blocked; the row says so too, in the same words, rather than sending the
// reader to that page to find out.
func TestADealRowCarriesTheCachedVerdictBesideItsMove(t *testing.T) {
	dealID := ids.NewV7()
	svc := (&Service{}).WithDealStandings(&stubDealStandings{
		standings: map[ids.UUID]DealStanding{
			dealID: standingOf("blocked", "Legal has not returned the DPA."),
		},
	})
	queue := []crmcontracts.WorklistItem{riskRow(dealID)}

	if err := svc.nameTheStanding(context.Background(), queue, nil); err != nil {
		t.Fatalf("naming the standing: %v", err)
	}

	verdict := queue[0].Verdict
	if verdict == nil {
		t.Fatal("the row carries no verdict")
	}
	if verdict.Standing == nil || *verdict.Standing != crmcontracts.WorklistStandingBlocked {
		t.Errorf("standing = %v, want blocked", verdict.Standing)
	}
	if verdict.Line != "Legal has not returned the DPA." {
		t.Errorf("line = %q", verdict.Line)
	}
	if verdict.Source != crmcontracts.WorklistInsightSourceDealStatus {
		t.Errorf("source = %q, want deal_status", verdict.Source)
	}
	if verdict.AsOf == nil {
		t.Error("a card-derived reading carries no as_of, so the reader cannot tell how old it is")
	}
}

// No card cached, but the night looked at this deal and wrote down what it
// found. That finding is grounded and citation-checked where it was written, so
// it stands in rather than leaving the row with no reading at all.
func TestAnUncachedDealUsesTheGroundedBriefFinding(t *testing.T) {
	dealID := ids.NewV7()
	svc := (&Service{}).WithDealStandings(&stubDealStandings{
		standings: map[ids.UUID]DealStanding{},
	})
	queue := []crmcontracts.WorklistItem{riskRow(dealID)}
	findings := map[ids.UUID]string{dealID: "The buyer asked for pricing and nobody answered."}

	if err := svc.nameTheStanding(context.Background(), queue, findings); err != nil {
		t.Fatalf("naming the standing: %v", err)
	}

	verdict := queue[0].Verdict
	if verdict == nil {
		t.Fatal("the row carries no verdict, so the night's finding was thrown away")
	}
	if verdict.Source != crmcontracts.WorklistInsightSourceBriefFinding {
		t.Errorf("source = %q, want brief_finding", verdict.Source)
	}
	if verdict.Line != "The buyer asked for pricing and nobody answered." {
		t.Errorf("line = %q", verdict.Line)
	}
	// A finding is prose about the deal and NOT one of the four standings. A
	// word invented here would be this pass deciding the judgement dealstatus
	// owns, which is the second answer the whole ordering exists to prevent.
	//
	// ABSENT rather than empty: the field is a pointer precisely so this case
	// can be expressed without sending "" for a value outside the enum, which is
	// what a client validating against the contract would reject.
	if verdict.Standing != nil {
		t.Errorf("standing = %v, want none: a finding is not a verdict word", *verdict.Standing)
	}
}

// The floor. No card, no finding — and the row is still fully explained, by the
// typed reasons and the consequence it has carried all along. The pass leaves
// no verdict rather than computing one.
func TestADealWithNeitherAIReadingKeepsItsDeterministicExplanationAndMove(t *testing.T) {
	dealID := ids.NewV7()
	svc := (&Service{}).WithDealStandings(&stubDealStandings{
		standings: map[ids.UUID]DealStanding{},
	})
	row := riskRow(dealID)
	row.Because = []crmcontracts.WorklistReason{{Kind: "quiet_days"}}
	row.Consequence = "deal_drifts"
	row.Move = &crmcontracts.WorklistMove{Action: crmcontracts.WorklistMoveActionDraftEmail}
	queue := []crmcontracts.WorklistItem{row}

	if err := svc.nameTheStanding(context.Background(), queue, nil); err != nil {
		t.Fatalf("naming the standing: %v", err)
	}

	if queue[0].Verdict != nil {
		t.Errorf("a verdict was invented from nothing: %+v", queue[0].Verdict)
	}
	if len(queue[0].Because) != 1 || queue[0].Because[0].Kind != "quiet_days" {
		t.Error("the row lost the typed reason that explains it without any model")
	}
	if queue[0].Consequence != "deal_drifts" {
		t.Error("the row lost its consequence")
	}
	if queue[0].Move == nil {
		t.Error("the row lost the move the step pass put on it")
	}
}

// A standing word this build does not carry is dropped WHOLE. Serving it would
// reach the browser as a verdict it cannot draw or colour, and the row's typed
// reasons already explain it.
func TestAVerdictWhoseStandingThisBuildCannotDrawIsDroppedWhole(t *testing.T) {
	dealID := ids.NewV7()
	svc := (&Service{}).WithDealStandings(&stubDealStandings{
		standings: map[ids.UUID]DealStanding{
			dealID: standingOf("smouldering", "Something the next release added."),
		},
	})
	queue := []crmcontracts.WorklistItem{riskRow(dealID)}

	if err := svc.nameTheStanding(context.Background(), queue, nil); err != nil {
		t.Fatalf("naming the standing: %v", err)
	}

	if queue[0].Verdict != nil {
		t.Errorf("an undrawable standing reached the wire: %+v", queue[0].Verdict)
	}
}

// A standing with no line behind it is dropped for the same reason the card's
// own reader drops it: "this deal is blocked" with no way to ask why is a
// judgement the reader cannot check.
func TestAStandingWithNothingBehindItIsNotServed(t *testing.T) {
	dealID := ids.NewV7()
	svc := (&Service{}).WithDealStandings(&stubDealStandings{
		standings: map[ids.UUID]DealStanding{dealID: standingOf("cold", "")},
	})
	queue := []crmcontracts.WorklistItem{riskRow(dealID)}

	if err := svc.nameTheStanding(context.Background(), queue, nil); err != nil {
		t.Fatalf("naming the standing: %v", err)
	}

	if queue[0].Verdict != nil {
		t.Errorf("a standing with no sentence reached the wire: %+v", queue[0].Verdict)
	}
}

// The cache wins over the night. Both readings are about the same deal, and the
// card is the one the deal page prints — a row disagreeing with that page is the
// defect the whole ordering exists to prevent.
func TestTheCachedCardOutranksTheNightsFinding(t *testing.T) {
	dealID := ids.NewV7()
	svc := (&Service{}).WithDealStandings(&stubDealStandings{
		standings: map[ids.UUID]DealStanding{dealID: standingOf("live", "They booked the security review.")},
	})
	queue := []crmcontracts.WorklistItem{riskRow(dealID)}
	findings := map[ids.UUID]string{dealID: "Quiet since June."}

	if err := svc.nameTheStanding(context.Background(), queue, findings); err != nil {
		t.Fatalf("naming the standing: %v", err)
	}

	if queue[0].Verdict.Line != "They booked the security review." {
		t.Errorf("line = %q, want the card's", queue[0].Verdict.Line)
	}
}

// One question per deal, however many rows name it. A page holds thirty rows
// and a deal can raise several.
func TestOneDealIsAskedAboutOnceHoweverManyRowsNameIt(t *testing.T) {
	dealID := ids.NewV7()
	stub := &stubDealStandings{standings: map[ids.UUID]DealStanding{}}
	queue := []crmcontracts.WorklistItem{riskRow(dealID), riskRow(dealID), riskRow(dealID)}

	if err := (&Service{}).WithDealStandings(stub).
		nameTheStanding(context.Background(), queue, nil); err != nil {
		t.Fatalf("naming the standing: %v", err)
	}

	if stub.calls != 1 {
		t.Errorf("asked %d times, want 1", stub.calls)
	}
	if len(stub.asked) != 1 {
		t.Errorf("asked about %d deals, want 1", len(stub.asked))
	}
}

// A page with no deal row asks nothing at all.
func TestAPageWithoutADealAsksForNoStandings(t *testing.T) {
	stub := &stubDealStandings{}
	queue := []crmcontracts.WorklistItem{{
		Id: ids.NewV7().String(), Source: "task", Category: "tasks",
		Actions: []crmcontracts.WorklistItemActions{},
	}}

	if err := (&Service{}).WithDealStandings(stub).
		nameTheStanding(context.Background(), queue, nil); err != nil {
		t.Fatalf("naming the standing: %v", err)
	}

	if stub.calls != 0 {
		t.Errorf("asked %d times over a page with no deal", stub.calls)
	}
}

// An unbound seam leaves the card reading absent and the night's finding still
// reaching the row — the degradation WithDealStandings documents.
func TestAnUnboundStandingSeamStillCarriesTheNightsFinding(t *testing.T) {
	dealID := ids.NewV7()
	queue := []crmcontracts.WorklistItem{riskRow(dealID)}
	findings := map[ids.UUID]string{dealID: "Quiet since June."}

	if err := (&Service{}).nameTheStanding(context.Background(), queue, findings); err != nil {
		t.Fatalf("naming the standing: %v", err)
	}

	if queue[0].Verdict == nil || queue[0].Verdict.Source != crmcontracts.WorklistInsightSourceBriefFinding {
		t.Error("an unbound card seam took the night's finding away with it")
	}
}

// A refused read reaches the caller. The page is wrong in a way its reader
// cannot see if a failure here is swallowed into "no deal has a standing".
func TestARefusedStandingReadFailsThePage(t *testing.T) {
	dealID := ids.NewV7()
	wanted := errors.New("the cache is unreachable")
	svc := (&Service{}).WithDealStandings(&stubDealStandings{err: wanted})
	queue := []crmcontracts.WorklistItem{riskRow(dealID)}

	err := svc.nameTheStanding(context.Background(), queue, nil)

	if !errors.Is(err, wanted) {
		t.Errorf("err = %v, want the read's own", err)
	}
}

// The finding a read gathers belongs to THAT read and reaches no other.
//
// The defect this catches shipped as a field on the Service: assembleDay is
// reached on the process-wide instance by /attention and by the team exceptions
// read, so one reader's findings sat there when the next reader's page was
// built — and a reader whose own brief ran empty inherited them, because
// nothing overwrote what nothing wrote. Another rep's mail-derived prose, on
// this rep's row.
//
// Written against findingsOf rather than against two HTTP requests because the
// property is about WHERE the map lives: a pure function of the queue handed in
// has nowhere to leave a previous caller's answer. Mutation-checked by making
// it write to a package-level map, which fails the second assertion.
func TestAReadersFindingsNeverReachAnotherReadersPage(t *testing.T) {
	dealID := ids.NewV7()

	theirs := findingsOf([]BriefEntry{
		{ID: ids.NewV7(), DealID: dealID, Finding: "Their CFO froze the budget."},
	})
	if theirs[dealID] != "Their CFO froze the budget." {
		t.Fatalf("the first read did not gather its own finding: %v", theirs)
	}

	// The second reader's brief ran empty. Their answer must be empty too.
	mine := findingsOf(nil)

	if len(mine) != 0 {
		t.Errorf("a reader whose brief ran empty received %d findings: %v", len(mine), mine)
	}
	if _, found := mine[dealID]; found {
		t.Error("one reader's brief finding reached another reader's page")
	}
}

// A brief entry with no finding contributes no key, so a deal the night ranked
// but never annotated falls through to its typed reasons rather than to an
// empty sentence.
func TestABriefEntryWithNoFindingContributesNothing(t *testing.T) {
	dealID := ids.NewV7()

	findings := findingsOf([]BriefEntry{{ID: ids.NewV7(), DealID: dealID}})

	if _, found := findings[dealID]; found {
		t.Error("an unannotated brief entry produced a finding key")
	}
}
