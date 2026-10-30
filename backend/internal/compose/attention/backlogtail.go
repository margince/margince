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
	"errors"
	"log/slog"
	"maps"
	"slices"
	"time"

	"github.com/margince/margince/backend/internal/compose/worklistsnap"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// backlogCursor is the tail a walk carries: which sources it continues, where
// each stands, and the window every page of it reads.
type backlogCursor struct {
	// At maps each source still continued to its store position. An empty
	// position is its beginning; a source read to its end leaves the map.
	At map[string]string `json:"a"`
	// Window is the first page's day. A later page reads the same window, so
	// a walk crossing midnight does not resume one window's keyset in another.
	Window tailWindow `json:"w"`
}

// tailWindow is the instant, the day's end and the calendar a walk began in.
type tailWindow struct {
	AsOf  time.Time `json:"t"`
	Until time.Time `json:"u"`
	Zone  string    `json:"z"`
}

// tailSources are the sources a tail can continue, by the categories their
// rows carry.
var tailSources = map[string][]crmcontracts.WorklistItemCategory{
	string(sourceTask):    taskCategories,
	string(sourceWaiting): {categoryOfSource(sourceWaiting)},
	string(sourceAtRisk):  {categoryOfSource(sourceAtRisk)},
}

// valid says whether a client-held tail is one this binary could have minted.
func (c *backlogCursor) valid() bool {
	if len(c.At) == 0 || c.Window.AsOf.IsZero() || c.Window.Until.IsZero() {
		return false
	}
	if _, err := time.LoadLocation(c.Window.Zone); err != nil {
		return false
	}
	for source, at := range c.At {
		if _, known := tailSources[source]; !known {
			return false
		}
		if _, err := storekit.DecodeOpaque[map[string]any](at); at != "" && err != nil {
			return false
		}
	}
	return true
}

// backlogRounds bounds the store reads one page spends on one source. Without
// it, a filter that keeps few rows walks the whole backlog in one request. The
// cursor carries on from where the rounds stopped.
const backlogRounds = 4

// startTail is the tail a first page mints: every source its lane cut and
// the tail can continue, each at its beginning. Nil when there is none. A
// source decided here runs until its own read ends. Work dealt with above
// therefore never ends the tail early.
func (s *Service) startTail(page tailPage) *backlogCursor {
	at := map[string]string{}
	for source, categories := range tailSources {
		bound := s.tailReader(crmcontracts.WorklistItemSource(source)) != nil
		if page.bounded[crmcontracts.WorklistItemSource(source)] && bound && sourceAnswers(categories, s.view, page.filter) {
			at[source] = ""
		}
	}
	if len(at) == 0 {
		return nil
	}
	zone := time.UTC
	if s.backlog.zone != nil {
		zone = s.backlog.zone
	}
	return &backlogCursor{
		At:     at,
		Window: tailWindow{AsOf: page.day.AsOf, Until: s.backlog.until, Zone: zone.String()},
	}
}

// withTail fills what the ranked rows left of a page from the backlog, once
// they have run out. above is what the ranked rows hold: the frozen walk when
// one is resumed, else this read's own ranking with every group's members.
func (s *Service) withTail(
	ctx context.Context, page tailPage, shown []ranked, more bool, limit int, from *backlogCursor, ranking []ranked,
) ([]ranked, *backlogCursor, []crmcontracts.WorklistSourceUnavailable) {
	if page.first {
		from = s.startTail(page)
	}
	if from == nil || more {
		return shown, from, nil
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
	page.dealsAbove = dealsReported(ranking, above)
	tail, rest, failed := s.backlogTail(ctx, page, limit-len(shown), from, above)
	return append(shown, tail...), rest, failed
}

// backlogTail reads up to need rows below the ranked ones and answers where
// every source then stands. A nil position means no source has more. Each
// round reads every source still continued once and merges what it may.
func (s *Service) backlogTail(
	ctx context.Context, page tailPage, need int, from *backlogCursor, above map[worklistsnap.Row]bool,
) ([]ranked, *backlogCursor, []crmcontracts.WorklistSourceUnavailable) {
	rest := &backlogCursor{At: maps.Clone(from.At), Window: from.Window}
	var tail []ranked
	var failed []crmcontracts.WorklistSourceUnavailable
	for round := 0; round < backlogRounds && len(rest.At) > 0 && len(tail) < need; round++ {
		var batches []tailBatch
		for _, source := range slices.Sorted(maps.Keys(rest.At)) {
			batch, err := s.readTail(ctx, page, rest, crmcontracts.WorklistItemSource(source), need-len(tail))
			if err != nil {
				slog.Log(ctx, laneFailureLevel(ctx), "a worklist backlog source failed; scrolling stops short of it",
					"source", source, "error", err)
				failed = append(failed, crmcontracts.WorklistSourceUnavailable{Source: source, Reason: "failed"})
				delete(rest.At, source)
				continue
			}
			batches = append(batches, batch)
		}
		tail = append(tail, mergeTail(batches, need-len(tail), above, rest)...)
	}
	if len(rest.At) == 0 {
		return tail, nil, failed
	}
	return tail, rest, failed
}

// readTail reads one source's next batch after its position and runs it
// through the page's admission, marking which rows the page keeps.
func (s *Service) readTail(
	ctx context.Context, page tailPage, rest *backlogCursor, source crmcontracts.WorklistItemSource, limit int,
) (tailBatch, error) {
	read := s.tailReader(source)
	if read == nil {
		return tailBatch{}, errors.New("no reader is bound for this backlog source")
	}
	var batch tailBatch
	err := s.degradable(ctx, laneBudget, func(ctx context.Context) error {
		var err error
		batch, err = read(ctx, page, rest.Window, rest.At[string(source)], limit)
		return err
	})
	if err != nil {
		return tailBatch{}, err
	}
	batch.source = source
	rows := make([]ranked, 0, len(batch.rows))
	for _, row := range batch.rows {
		if !row.elsewhere {
			rows = append(rows, row.row)
		}
	}
	admitted, err := s.admitTail(ctx, page, rest.Window.AsOf, rows)
	if err != nil {
		return tailBatch{}, err
	}
	kept := map[worklistsnap.Row]bool{}
	for _, row := range admitted {
		kept[rowIdentity(row)] = true
	}
	for i := range batch.rows {
		batch.rows[i].kept = kept[rowIdentity(batch.rows[i].row)]
	}
	return batch, nil
}

// tailReader is how one source reads its next batch, or nil when the source
// cannot be continued here.
func (s *Service) tailReader(source crmcontracts.WorklistItemSource) func(
	context.Context, tailPage, tailWindow, string, int) (tailBatch, error) {
	switch {
	case source == sourceTask && s.backlog.tasks != nil:
		return s.moreTasks
	case source == sourceWaiting && s.backlog.waiting != nil:
		return s.moreWaiting
	case source == sourceAtRisk && s.backlog.risk != nil:
		return s.moreRisk
	}
	return nil
}

// tailPage is what one page's tail is judged by: the page's question, which
// sources the lanes cut, and whether this is the walk's first page.
type tailPage struct {
	scope, filter string
	day           crmcontracts.Attention
	bounded       map[crmcontracts.WorklistItemSource]bool
	first         bool
	// dealsAbove are the deals a ranked row already reports. A deal is one row.
	dealsAbove map[ids.UUID]bool
}

// moreTasks reads the next batch of the task window this page's filter shows,
// in the walk's own window. A task's place in the tail is its DueAt, which the
// store selects as the same storekit.TaskDueSQL its keyset sorts by.
func (s *Service) moreTasks(ctx context.Context, page tailPage, walk tailWindow, after string, limit int) (tailBatch, error) {
	zone, err := time.LoadLocation(walk.Zone)
	if err != nil {
		return tailBatch{}, err
	}
	read, more, err := s.backlog.tasks.TasksAfter(ctx, s.taskStream(walk, page.filter), after, limit)
	if err != nil {
		return tailBatch{}, err
	}
	tasks := make([]Task, len(read))
	for i, task := range read {
		tasks[i] = task.Row
	}
	items := plannedItems(tasks, walk.AsOf, walk.Until, zone)
	// plannedItems and appendLane each draw one row per task, in order.
	rows := appendLane(nil, sourceTask, &items, walk.AsOf, classifyTask)
	batch := tailBatch{more: more, rows: make([]tailRow, len(read))}
	for i, task := range read {
		batch.rows[i] = tailRow{row: rows[i], at: task.Row.DueAt, after: task.After}
	}
	return batch, nil
}

// moreWaiting reads the next cards of who is waiting, as of the walk's own
// instant. A card's place in the tail is when its message arrived.
func (s *Service) moreWaiting(ctx context.Context, _ tailPage, walk tailWindow, after string, limit int) (tailBatch, error) {
	read, err := s.backlog.waiting.WaitingAfter(ctx, walk.AsOf, s.waitingFor(ctx), after, waitingJudged(limit))
	if err != nil {
		return tailBatch{}, err
	}
	batch := tailBatch{more: read.More, through: read.Through, rows: make([]tailRow, len(read.Cards))}
	if read.Through != "" {
		batch.throughAt = &read.ThroughAt
	}
	for i, card := range read.Cards {
		since := card.Row.Since
		batch.rows[i] = tailRow{row: classifyWaiting(card.Row, walk.AsOf), at: &since, after: card.After}
	}
	return batch, nil
}

// waitingJudged is how many messages one waiting read judges for a batch of
// limit rows. Some judged messages fold or fall to the queue's rules, so it
// judges more than it shows; the cost per read stays fixed.
func waitingJudged(limit int) int { return max(2*limit, 20) }

// taskStream is the task window below the ranked rows: today's work, or the
// later window when the page asks for upcoming work.
func (s *Service) taskStream(walk tailWindow, filter string) TaskWindow {
	for key, window := range s.taskWindows(walk.AsOf, walk.Until) {
		if (key.category == crmcontracts.WorklistCountCategory(categoryUpcoming)) == (filter == string(categoryUpcoming)) {
			return window
		}
	}
	return TaskWindow{}
}

// admitTail runs a tail batch through the steps the ranked rows took. A row
// below the cap is kept or dropped as it would be above it.
func (s *Service) admitTail(ctx context.Context, page tailPage, asOf time.Time, rows []ranked) ([]ranked, error) {
	rows, _ = admitBySource(ctx, rows)
	rows, _ = s.narrowToScope(ctx, rows, page.scope, s.taskOwner)
	done, err := s.tailDone(ctx, rows)
	if err != nil {
		return nil, err
	}
	rows = dropDone(rows, done, asOf)
	rows = markChangedSinceBrief(rows, s.briefCutoff)
	rows = keepInView(rows, s.view, page.filter)
	if page.filter != "" && page.filter != string(crmcontracts.WorklistFilterAll) {
		rows = keepFiltered(rows, crmcontracts.WorklistFilter(page.filter))
	}
	return rows, nil
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
