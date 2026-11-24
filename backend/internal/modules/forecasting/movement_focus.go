// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"cmp"
	"context"
	"math"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// CauseFocusActions marks a deal on the opening snapshot's focus list that was
// worked between the two snapshots: tasks done, mails sent or meetings held.
// It is a cause beside the buckets, never a bucket, so the waterfall adds up.
const CauseFocusActions = "focus_actions"

// focusActionsPerDeal bounds the links one deal carries, so a busy deal does
// not turn a movement into a timeline.
// basisPointsPerWhole is how many basis points make the whole change.
const basisPointsPerWhole = 100 * 100

const focusActionsPerDeal = 5

// FocusAction is one thing done on a listed deal between two snapshots.
type FocusAction struct {
	ActivityID ids.UUID
	// Kind is task_done, mail_sent or meeting_held.
	Kind    string
	At      time.Time
	Subject string
}

// FocusActionsFunc reads, for each deal, what was done on it after one moment
// and up to another. It is injected because the actions are activities, which
// another module owns.
type FocusActionsFunc func(ctx context.Context, tx pgx.Tx, dealIDs []ids.UUID, after, until time.Time) (map[string][]FocusAction, error)

// WithFocusActions binds the reader that lets a movement name how much of it
// came from actions on the stored focus list. Without it a movement carries no
// such attribution.
func (s *Store) WithFocusActions(read FocusActionsFunc) *Store {
	s.focusActions = read
	return s
}

// FocusAttribution is the part of a movement that landed on deals from the
// opening snapshot's stored focus list that were worked in the window.
type FocusAttribution struct {
	// AmountMinor is the change of those deals in the reading drawn.
	AmountMinor int64
	// ShareBps is AmountMinor over the whole change, in basis points. Nil when
	// the whole change is zero, so there is nothing to take a share of.
	ShareBps *int
	Deals    []FocusDeal
}

// FocusDeal is one listed deal that was worked, with its change and the actions.
type FocusDeal struct {
	DealID      string
	AmountMinor int64
	Actions     []FocusAction
}

// attributeFocus names the focus-list deals that were worked in the window and
// what they moved, and tags each one's DealDelta with CauseFocusActions.
// Only deals the opening snapshot ranked count: the list the manager worked
// from is the stored one, not whatever the pipeline looks like afterwards.
func attributeFocus(opening snapshotSide, move *Movement, actions map[string][]FocusAction) *FocusAttribution {
	listed := map[string]bool{}
	for _, row := range opening.Contributions {
		if row.Focus != nil {
			listed[row.DealID] = true
		}
	}
	var out FocusAttribution
	for i := range move.Deals {
		delta := &move.Deals[i]
		done := actions[delta.DealID]
		if !listed[delta.DealID] || len(done) == 0 {
			continue
		}
		delta.Causes = append(delta.Causes, CauseFocusActions)
		out.AmountMinor += delta.AmountMinor
		out.Deals = append(out.Deals, FocusDeal{
			DealID: delta.DealID, AmountMinor: delta.AmountMinor,
			Actions: done[:min(len(done), focusActionsPerDeal)],
		})
	}
	if len(out.Deals) == 0 {
		return nil
	}
	slices.SortFunc(out.Deals, func(a, b FocusDeal) int {
		return cmp.Or(cmp.Compare(max(b.AmountMinor, -b.AmountMinor), max(a.AmountMinor, -a.AmountMinor)), cmp.Compare(a.DealID, b.DealID))
	})
	if whole := move.ClosingMinor - move.OpeningMinor; whole != 0 {
		out.ShareBps = new(int(math.Round(float64(out.AmountMinor) / float64(whole) * basisPointsPerWhole)))
	}
	return &out
}

// focusAttribution reads the actions of the opening snapshot's listed deals and
// attributes the movement to them.
func (s *Store) focusAttribution(
	ctx context.Context, tx pgx.Tx, opening, closing snapshotSide, move *Movement,
) error {
	if s.focusActions == nil {
		return nil
	}
	var listed []ids.UUID
	for _, row := range opening.Contributions {
		if row.Focus == nil {
			continue
		}
		id, err := ids.Parse(row.DealID)
		if err != nil {
			return err
		}
		listed = append(listed, id)
	}
	if len(listed) == 0 {
		return nil
	}
	actions, err := s.focusActions(ctx, tx, listed, opening.TakenAt, closing.TakenAt)
	if err != nil {
		return err
	}
	move.FocusActions = attributeFocus(opening, move, actions)
	return nil
}

// attributed is move with its focus-list attribution added.
func (s *Store) attributed(ctx context.Context, tx pgx.Tx, opening, closing snapshotSide, move Movement) (Movement, error) {
	err := s.focusAttribution(ctx, tx, opening, closing, &move)
	return move, err
}
