// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	listedWorked  = "00000000-0000-0000-0000-0000000000a1"
	listedIdle    = "00000000-0000-0000-0000-0000000000a2"
	unlistedWork  = "00000000-0000-0000-0000-0000000000a3"
	listedWorkedB = "00000000-0000-0000-0000-0000000000a4"
)

func listed(c Contribution) Contribution {
	c.Focus = &Focus{Rank: 1}
	return c
}

func focusSides() (snapshotSide, snapshotSide) {
	opening := side(DefinitionVersion,
		listed(open(listedWorked, 100_00)), listed(open(listedIdle, 100_00)),
		open(unlistedWork, 100_00), listed(open(listedWorkedB, 100_00)))
	closing := side(DefinitionVersion,
		open(listedWorked, 150_00), open(listedIdle, 130_00),
		open(unlistedWork, 120_00), open(listedWorkedB, 90_00))
	return opening, closing
}

func actionsOn(kinds ...string) []FocusAction {
	var out []FocusAction
	for i, kind := range kinds {
		out = append(out, FocusAction{
			ActivityID: ids.MustParse(fmt.Sprintf("00000000-0000-0000-0000-0000000000b%d", i)),
			Kind:       kind, At: time.Date(2026, 3, 3+i, 9, 0, 0, 0, time.UTC),
		})
	}
	return out
}

func TestFocusAttributionNamesTheShareMovedByWorkedListedDeals(t *testing.T) {
	t.Parallel()
	opening, closing := focusSides()
	move := Classify(ReadingOpen, opening, closing)

	got := attributeFocus(opening, &move, map[string][]FocusAction{
		listedWorked:  actionsOn("task_done", "mail_sent"),
		unlistedWork:  actionsOn("meeting_held"),
		listedWorkedB: actionsOn("meeting_held"),
	})

	if got == nil {
		t.Fatal("two listed deals were worked and moved, so the movement must name them")
	}
	// +50 and -10 on the two worked listed deals; +20 on the unlisted one and
	// +30 on the idle listed one stay out. The whole change is +90.
	if got.AmountMinor != 40_00 {
		t.Errorf("attributed %d, want 4000: the worked listed deals moved +5000 and -1000", got.AmountMinor)
	}
	if got.ShareBps == nil || *got.ShareBps != 4444 {
		t.Errorf("share = %v, want 4444 basis points of the whole +9000 change", got.ShareBps)
	}
	order := []string{got.Deals[0].DealID, got.Deals[1].DealID}
	if !slices.Equal(order, []string{listedWorked, listedWorkedB}) {
		t.Errorf("deals = %v, want largest change first", order)
	}
	if n := len(got.Deals[0].Actions); n != 2 {
		t.Errorf("worked deal links %d actions, want its 2", n)
	}
	for _, delta := range move.Deals {
		tagged := slices.Contains(delta.Causes, CauseFocusActions)
		want := delta.DealID == listedWorked || delta.DealID == listedWorkedB
		if tagged != want {
			t.Errorf("deal %s tagged %v, want %v", delta.DealID, tagged, want)
		}
	}
	reconciles(t, move)
}

func TestFocusAttributionIsAbsentWhenNoListedDealWasWorked(t *testing.T) {
	t.Parallel()
	opening, closing := focusSides()
	move := Classify(ReadingOpen, opening, closing)
	if got := attributeFocus(opening, &move, map[string][]FocusAction{unlistedWork: actionsOn("task_done")}); got != nil {
		t.Errorf("attribution = %+v, want none: the only worked deal was not on the stored list", got)
	}
}

func TestFocusAttributionHasNoShareOfAZeroChange(t *testing.T) {
	t.Parallel()
	opening := side(DefinitionVersion, listed(open(listedWorked, 100_00)), open(unlistedWork, 100_00))
	closing := side(DefinitionVersion, open(listedWorked, 110_00), open(unlistedWork, 90_00))
	move := Classify(ReadingOpen, opening, closing)
	got := attributeFocus(opening, &move, map[string][]FocusAction{listedWorked: actionsOn("task_done")})
	if got == nil || got.ShareBps != nil || got.AmountMinor != 10_00 {
		t.Errorf("attribution = %+v, want +1000 and no share, because the whole change is zero", got)
	}
}

func TestFocusAttributionCapsTheLinksOfOneDeal(t *testing.T) {
	t.Parallel()
	opening, closing := focusSides()
	move := Classify(ReadingOpen, opening, closing)
	kinds := slices.Repeat([]string{"mail_sent"}, 9)
	got := attributeFocus(opening, &move, map[string][]FocusAction{listedWorked: actionsOn(kinds...)})
	if n := len(got.Deals[0].Actions); n != focusActionsPerDeal {
		t.Errorf("deal links %d actions, want the cap of %d", n, focusActionsPerDeal)
	}
}

func TestAMovementWithoutAFocusReaderCarriesNoAttribution(t *testing.T) {
	t.Parallel()
	opening, closing := focusSides()
	move := Classify(ReadingOpen, opening, closing)
	store := &Store{}
	if err := store.focusAttribution(context.Background(), nil, opening, closing, &move); err != nil || move.FocusActions != nil {
		t.Errorf("err=%v attribution=%+v, want none from a store with no reader bound", err, move.FocusActions)
	}
	store.WithFocusActions(func(context.Context, pgx.Tx, []ids.UUID, time.Time, time.Time) (map[string][]FocusAction, error) {
		return map[string][]FocusAction{listedWorked: actionsOn("task_done")}, nil
	})
	if err := store.focusAttribution(context.Background(), nil, opening, closing, &move); err != nil || move.FocusActions == nil {
		t.Errorf("err=%v attribution=%+v, want the bound reader's actions attributed", err, move.FocusActions)
	}
}
