// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The briefing lane: what the overnight run put at the top of the day.
//
// Its own file because it answers two things at once and the second is not a
// lane at all. The items are rows like any other lane's; the FINDINGS are the
// night's prose per deal, which no lane draws and only the worklist folds onto
// a deal row as its standing. feed.go's assembleDay states what the pair is for.

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// thisMorning is the briefing lane: the overnight run's unanswered queue, in
// its own rank order.
//
// No cap here. The brief is already honest-short — its own ranking bounds the
// queue and refuses to pad — so a second bound would hide items the engine had
// decided were worth the morning. The seam drops the answered ones, because
// this lane is a worklist that must be finishable and a row that cannot be
// removed is the opposite of finishing. Home still shows what was answered,
// which is where a rep looks to see what she did.
func (s *Service) thisMorning(
	ctx context.Context,
) ([]crmcontracts.AttentionItem, crmcontracts.AttentionThisMorningState, map[ids.UUID]string, error) {
	queue, ran, err := s.briefing.Queue(ctx)
	if err != nil {
		return nil, "", nil, err
	}
	items := make([]crmcontracts.AttentionItem, 0, len(queue))
	for _, entry := range queue {
		items = append(items, briefItem(entry))
	}
	// The state names WHY the lane holds what it holds. A run that ranked
	// nothing reads all_answered too: "nothing worth your first hour" and
	// "you answered everything" are the same message to a reader — nothing
	// to do here — while no_run_today is the one that must not wear a tick.
	state := crmcontracts.ItemsWaiting
	switch {
	case !ran:
		state = crmcontracts.NoRunToday
	case len(items) == 0:
		state = crmcontracts.AllAnswered
	}
	return items, state, findingsOf(queue), nil
}
