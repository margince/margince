// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The backlog tail: the work below the ranked rows, loaded batch by batch as
// the reader scrolls.
//
// Only the top is ranked, because ranking needs every candidate in hand.
// Below it, each cut source continues in its store's own order from its own
// keyset. A page reads only what it shows. Reaching the end of every source
// means the list is complete.

import (
	"context"
	"log/slog"
	"maps"
	"slices"

	"github.com/margince/margince/backend/internal/compose/worklistsnap"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// backlogCursor is where each backlog source's continuation stands.
type backlogCursor struct {
	// At is a source's store position. An absent source starts at its beginning.
	At map[string]string `json:"a,omitempty"`
	// Ended names each source read to its end, or given up on because it failed.
	Ended []string `json:"e,omitempty"`
}

func (c *backlogCursor) ended(source crmcontracts.WorklistItemSource) bool {
	return c != nil && slices.Contains(c.Ended, string(source))
}

// backlogRounds bounds the store reads one page spends on one source. Without
// it, a filter that keeps few rows walks the whole backlog in one request. The
// cursor carries on from where the rounds stopped.
const backlogRounds = 4

// withTail fills what the ranked rows left of a page from the backlog, once
// they have run out. above is what the ranked rows hold: the frozen walk when
// one is resumed, else this read's own ranking with every group's members.
func (s *Service) withTail(
	ctx context.Context, page tailPage, shown []ranked, more bool, limit int, from *backlogCursor, ranking []ranked,
) ([]ranked, *backlogCursor, []crmcontracts.WorklistSourceUnavailable) {
	if more {
		return shown, nil, nil
	}
	above := map[worklistsnap.Row]bool{}
	if s.walk != nil {
		for _, at := range s.walk.Rows {
			above[at] = true
		}
	} else {
		for _, at := range frozenRows(ranking) {
			above[at] = true
		}
	}
	tail, rest, failed := s.backlogTail(ctx, page, limit-len(shown), from, above)
	return append(shown, tail...), rest, failed
}

// backlogTail reads up to need rows below the ranked ones and answers where
// every source then stands. A nil position means no source has more.
func (s *Service) backlogTail(
	ctx context.Context, page tailPage, need int, from *backlogCursor, above map[worklistsnap.Row]bool,
) ([]ranked, *backlogCursor, []crmcontracts.WorklistSourceUnavailable) {
	rest := &backlogCursor{At: map[string]string{}}
	if from != nil {
		rest.Ended = slices.Clone(from.Ended)
		maps.Copy(rest.At, from.At)
	}
	var tail []ranked
	var failed []crmcontracts.WorklistSourceUnavailable
	if !page.bounded[sourceTask] || s.backlog.tasks == nil || !sourceAnswers(taskCategories, s.view, page.filter) {
		rest.end(sourceTask)
	}
	for round := 0; round < backlogRounds && !rest.ended(sourceTask) && len(tail) < need; round++ {
		rows, err := s.moreTasks(ctx, page, rest, need-len(tail))
		if err != nil {
			slog.Log(ctx, laneFailureLevel(ctx), "a worklist backlog source failed; scrolling stops short of it",
				"source", sourceTask, "error", err)
			failed = append(failed, crmcontracts.WorklistSourceUnavailable{Source: string(sourceTask), Reason: "failed"})
			rest.end(sourceTask)
			break
		}
		for _, row := range rows {
			if !above[rowIdentity(row)] {
				tail = append(tail, row)
			}
		}
	}
	if rest.ended(sourceTask) {
		return tail, nil, failed
	}
	return tail, rest, failed
}

// tailPage is what one page's tail is judged by: the page's question and
// which sources the lanes cut.
type tailPage struct {
	scope, filter string
	day           crmcontracts.Attention
	bounded       map[crmcontracts.WorklistItemSource]bool
}

func (c *backlogCursor) end(source crmcontracts.WorklistItemSource) {
	if !c.ended(source) {
		c.Ended = append(c.Ended, string(source))
	}
	delete(c.At, string(source))
}

// moreTasks reads the next batch of the task window this page's filter shows,
// and moves the task position past it.
func (s *Service) moreTasks(ctx context.Context, page tailPage, rest *backlogCursor, limit int) ([]ranked, error) {
	window := s.taskStream(page)
	var batch []Task
	var next string
	err := s.degradable(ctx, laneBudget, func(ctx context.Context) error {
		var err error
		batch, next, err = s.backlog.tasks.TasksAfter(ctx, window, rest.At[string(sourceTask)], limit)
		return err
	})
	if err != nil {
		return nil, err
	}
	if next == "" {
		rest.end(sourceTask)
	} else {
		rest.At[string(sourceTask)] = next
	}
	items := plannedItems(batch, page.day.AsOf, s.backlog.until, s.backlog.zone)
	return s.admitTail(ctx, page, appendLane(nil, sourceTask, &items, page.day.AsOf, classifyTask)), nil
}

// taskStream is the task window below the ranked rows: today's work, or the
// later window when the page asks for upcoming work.
func (s *Service) taskStream(page tailPage) TaskWindow {
	for key, window := range s.taskWindows(page.day, s.backlog.until) {
		if (key.category == crmcontracts.WorklistCountCategory(categoryUpcoming)) == (page.filter == string(categoryUpcoming)) {
			return window
		}
	}
	return TaskWindow{}
}

// admitTail runs a tail batch through the steps the ranked rows took. A row
// below the cap is kept or dropped as it would be above it.
func (s *Service) admitTail(ctx context.Context, page tailPage, rows []ranked) []ranked {
	rows, _ = admitBySource(ctx, rows)
	rows, _ = s.narrowToScope(ctx, rows, page.scope, s.taskOwner)
	rows = dropDone(rows, s.marked, page.day.AsOf)
	rows = markChangedSinceBrief(rows, s.briefCutoff)
	rows = keepInView(rows, s.view, page.filter)
	if page.filter != "" && page.filter != string(crmcontracts.WorklistFilterAll) {
		rows = keepFiltered(rows, crmcontracts.WorklistFilter(page.filter))
	}
	return rows
}

// taskCategories are the categories a task row can carry.
var taskCategories = []crmcontracts.WorklistItemCategory{categoryOfSource(sourceTask), categoryUpcoming}

// sourceAnswers says whether any of a source's categories survives the view
// and the filter. A page that can show none of its rows reads none.
func sourceAnswers(categories []crmcontracts.WorklistItemCategory, view, filter string) bool {
	for _, category := range categories {
		if !shownIn(category, view, filter) {
			continue
		}
		// A filter naming a category is decided by the category alone, as
		// keepsRow decides it. Every other filter is decided per row.
		named := crmcontracts.WorklistItemCategory(filter).Valid() && filter != string(crmcontracts.WorklistFilterLeads)
		if !named || filter == string(category) {
			return true
		}
	}
	return false
}
