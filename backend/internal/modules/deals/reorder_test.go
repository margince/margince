// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The order arithmetic behind a reorder, without a database: which lists are
// refused, which records move, and the shape every ladder write keeps.

import (
	"errors"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func ladderFixture(closing ...bool) []ranked {
	out := make([]ranked, len(closing))
	for i, c := range closing {
		out[i] = ranked{id: ids.NewV7(), position: i + 1, closing: c}
	}
	return out
}

func idsOf(order []ranked) []ids.UUID {
	out := make([]ids.UUID, len(order))
	for i, r := range order {
		out[i] = r.id
	}
	return out
}

func TestArrangeFollowsTheNamedOrder(t *testing.T) {
	ladder := ladderFixture(false, false, true)
	wanted := []ids.UUID{ladder[1].id, ladder[0].id, ladder[2].id}
	order, err := arrange(ladder, wanted, "stage_ids", "stage")
	if err != nil {
		t.Fatalf("arranging a complete order: %v", err)
	}
	if !slices.Equal(idsOf(order), wanted) {
		t.Fatalf("arranged %v, want %v", idsOf(order), wanted)
	}
}

func TestArrangeRefusesAListItCannotTrust(t *testing.T) {
	ladder := ladderFixture(false, false)
	foreign := ids.NewV7()
	cases := map[string]struct {
		wanted []ids.UUID
		stale  bool
	}{
		"a record twice":            {wanted: []ids.UUID{ladder[0].id, ladder[0].id}},
		"a record missing":          {wanted: []ids.UUID{ladder[0].id}, stale: true},
		"a record from elsewhere":   {wanted: []ids.UUID{ladder[0].id, foreign}, stale: true},
		"nothing for a full ladder": {wanted: []ids.UUID{}, stale: true},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := arrange(ladder, c.wanted, "stage_ids", "stage")
			if c.stale != errors.Is(err, apperrors.ErrConflict) {
				t.Fatalf("got %v, want stale=%v", err, c.stale)
			}
			var parse *values.ParseError
			if !c.stale && (!errors.As(err, &parse) || parse.Code != codeOrderDuplicate) {
				t.Fatalf("got %v, want the %s refusal", err, codeOrderDuplicate)
			}
		})
	}
}

func TestArrangeAcceptsNothingForAnEmptyLadder(t *testing.T) {
	order, err := arrange(nil, []ids.UUID{}, "stage_ids", "stage")
	if err != nil || len(order) != 0 {
		t.Fatalf("an empty order of an empty ladder gave %v, %v", order, err)
	}
}

func TestPlacementsMoveOnlyWhatChangedPlace(t *testing.T) {
	ladder := ladderFixture(false, false, false)
	if moves := placements(ladder); len(moves) != 0 {
		t.Fatalf("an order equal to the ladder moved %v", moves)
	}
	swapped := []ranked{ladder[0], ladder[2], ladder[1]}
	moves := placements(swapped)
	want := []placement{
		{id: ladder[2].id, from: 3, to: 2},
		{id: ladder[1].id, from: 2, to: 3},
	}
	if !slices.Equal(moves, want) {
		t.Fatalf("moves %v, want %v", moves, want)
	}
}

func TestPlacementsCloseAGappedLadder(t *testing.T) {
	gapped := []ranked{{id: ids.NewV7(), position: 2}, {id: ids.NewV7(), position: 5}}
	moves := placements(gapped)
	if len(moves) != 2 || moves[0].to != 1 || moves[1].to != 2 {
		t.Fatalf("a gapped ladder renumbers to %v, want 1 and 2", moves)
	}
}

func TestTheClosingStagesComeAfterEveryOpenOne(t *testing.T) {
	if err := refuseClosingBeforeOpen(ladderFixture(false, true, true), "stage_ids"); err != nil {
		t.Fatalf("open then won and lost was refused: %v", err)
	}
	// Lost before won is two ways out in either order, not a closing stage
	// above an open one.
	lostFirst := ladderFixture(false, true, true)
	lostFirst[1], lostFirst[2] = lostFirst[2], lostFirst[1]
	if err := refuseClosingBeforeOpen(lostFirst, "stage_ids"); err != nil {
		t.Fatalf("the closing pair in either order was refused: %v", err)
	}
	var parse *values.ParseError
	err := refuseClosingBeforeOpen(ladderFixture(false, true, false), "position")
	if !errors.As(err, &parse) || parse.Code != codeClosingStageBeforeOpen || parse.Field != "position" {
		t.Fatalf("a closing stage above an open one gave %v, want %s on the named field", err, codeClosingStageBeforeOpen)
	}
}

func TestOpenFirstKeepsEachRunInItsOrder(t *testing.T) {
	ladder := ladderFixture(false, true, false, true, false)
	got := idsOf(openFirst(ladder))
	want := []ids.UUID{ladder[0].id, ladder[2].id, ladder[4].id, ladder[1].id, ladder[3].id}
	if !slices.Equal(got, want) {
		t.Fatalf("open first gave %v, want %v", got, want)
	}
}

func TestATerminalStagesOddsOutrankTheCallers(t *testing.T) {
	fifty := 50
	cases := []struct {
		semantic string
		given    *int
		want     int
	}{
		{"won", nil, 100},
		{"won", &fifty, 100},
		{"lost", &fifty, 0},
		{"open", &fifty, 50},
		{"open", nil, 0},
	}
	for _, c := range cases {
		if got := stageProbability(c.semantic, c.given); got != c.want {
			t.Errorf("a new %s stage given %v is written at %d, want %d", c.semantic, c.given, got, c.want)
		}
	}
	// An update commits the same pinned odds, and leaves an open stage's odds
	// alone when it names none.
	won := "won"
	if got := committedWinProbability(UpdateStageInput{Semantic: &won, WinProbability: &fifty}); got == nil || *got != 100 {
		t.Errorf("an update to won given 50 commits %v, want 100", got)
	}
	open := "open"
	if got := committedWinProbability(UpdateStageInput{Semantic: &open}); got != nil {
		t.Errorf("an update to open naming no odds commits %d, want the column left alone", *got)
	}
}

func TestAStagePositionStaysInsideTheParkableRange(t *testing.T) {
	for _, ok := range []int{0, 1, stagePositionCeiling} {
		if err := checkStagePosition(ok); err != nil {
			t.Errorf("position %d was refused: %v", ok, err)
		}
	}
	for _, bad := range []int{-1, stagePositionCeiling + 1} {
		var parse *values.ParseError
		if err := checkStagePosition(bad); !errors.As(err, &parse) || parse.Field != "position" {
			t.Errorf("position %d gave %v, want a refusal on position", bad, err)
		}
	}
}

func TestANewPipelinesStagesTakeTheLaddersShape(t *testing.T) {
	stage := func(position int, semantic StageSemantic) StageInput {
		return StageInput{Name: "s", Position: position, Semantic: string(semantic)}
	}
	if err := checkNewLadder([]StageInput{stage(3, SemanticLost), stage(1, SemanticOpen), stage(2, SemanticWon)}); err != nil {
		t.Fatalf("open, won, lost named out of list order was refused: %v", err)
	}
	cases := map[string]struct {
		stages []StageInput
		code   string
	}{
		"won above an open stage": {[]StageInput{stage(1, SemanticWon), stage(2, SemanticOpen)}, codeClosingStageBeforeOpen},
		"one position twice":      {[]StageInput{stage(1, SemanticOpen), stage(1, SemanticWon)}, codeOrderDuplicate},
		"a position out of range": {[]StageInput{stage(-1, SemanticOpen)}, codePositionOutOfRange},
	}
	for name, c := range cases {
		var parse *values.ParseError
		if err := checkNewLadder(c.stages); !errors.As(err, &parse) || parse.Code != c.code {
			t.Errorf("%s gave %v, want the %s refusal", name, err, c.code)
		}
	}
}
