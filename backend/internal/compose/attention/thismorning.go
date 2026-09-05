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
	"time"

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
func (s *Service) thisMorning(ctx context.Context) (theNight, error) {
	queue, ran, asOf, err := s.briefing.Queue(ctx)
	if err != nil {
		return theNight{}, err
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
	return theNight{
		items:    items,
		state:    state,
		findings: findingsOf(queue),
		scores:   scoresOf(queue),
		cutoff:   asOf,
	}, nil
}

// theNight is everything one read of the brief lane yields.
//
// A struct rather than four return values, because only the first two are a
// LANE. The other two are facts about deals that no lane draws: the findings
// stand in as a deal's reading where no status card is cached, and the scores
// break a tie inside a level. Both are consumed by the worklist long after the
// lane itself has been rendered, and naming them here is what keeps the brief
// read once for all four.
type theNight struct {
	items    []crmcontracts.AttentionItem
	state    crmcontracts.AttentionThisMorningState
	findings map[ids.UUID]string
	scores   map[ids.UUID]float64
	// cutoff is the instant the night read the records — the run's as_of, not
	// its generated_at. Zero when there was no run, which is why a row's
	// `changed_since_brief` is ABSENT rather than false in that case: "the night
	// saw this" and "there was no night" are different answers.
	cutoff time.Time
}

// scoresOf collects the night's composite per deal.
//
// A pure function of the queue handed in and returned to its caller, for the
// reason findingsOf gives: anything per-read left on the Service crosses
// readers.
func scoresOf(queue []BriefEntry) map[ids.UUID]float64 {
	scores := make(map[ids.UUID]float64, len(queue))
	for _, entry := range queue {
		if entry.Composite == 0 {
			continue
		}
		scores[entry.DealID] = entry.Composite
	}
	return scores
}
