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
	"time"

	"github.com/margince/margince/backend/internal/compose/worklistsnap"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
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

// tailSources are the sources a tail can continue.
var tailSources = map[string]bool{string(sourceTask): true}

// valid says whether a client-held tail is one this binary could have minted.
func (c *backlogCursor) valid() bool {
	if len(c.At) == 0 || c.Window.AsOf.IsZero() || c.Window.Until.IsZero() {
		return false
	}
	if _, err := time.LoadLocation(c.Window.Zone); err != nil {
		return false
	}
	for source, at := range c.At {
		if !tailSources[source] {
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
	if !page.bounded[sourceTask] || s.backlog.tasks == nil || !sourceAnswers(taskCategories, s.view, page.filter) {
		return nil
	}
	zone := time.UTC
	if s.backlog.zone != nil {
		zone = s.backlog.zone
	}
	return &backlogCursor{
		At:     map[string]string{string(sourceTask): ""},
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
	tail, rest, failed := s.backlogTail(ctx, page, limit-len(shown), from, above)
	return append(shown, tail...), rest, failed
}

// backlogTail reads up to need rows below the ranked ones and answers where
// every source then stands. A nil position means no source has more.
func (s *Service) backlogTail(
	ctx context.Context, page tailPage, need int, from *backlogCursor, above map[worklistsnap.Row]bool,
) ([]ranked, *backlogCursor, []crmcontracts.WorklistSourceUnavailable) {
	rest := &backlogCursor{At: maps.Clone(from.At), Window: from.Window}
	var tail []ranked
	var failed []crmcontracts.WorklistSourceUnavailable
	for round := 0; round < backlogRounds && rest.continues(sourceTask) && len(tail) < need; round++ {
		rows, err := s.moreTasks(ctx, page, rest, need-len(tail))
		if err != nil {
			slog.Log(ctx, laneFailureLevel(ctx), "a worklist backlog source failed; scrolling stops short of it",
				"source", sourceTask, "error", err)
			failed = append(failed, crmcontracts.WorklistSourceUnavailable{Source: string(sourceTask), Reason: "failed"})
			delete(rest.At, string(sourceTask))
			break
		}
		for _, row := range rows {
			if !above[rowIdentity(row)] {
				tail = append(tail, row)
			}
		}
	}
	if len(rest.At) == 0 {
		return tail, nil, failed
	}
	return tail, rest, failed
}

func (c *backlogCursor) continues(source crmcontracts.WorklistItemSource) bool {
	_, live := c.At[string(source)]
	return live
}

// tailPage is what one page's tail is judged by: the page's question, which
// sources the lanes cut, and whether this is the walk's first page.
type tailPage struct {
	scope, filter string
	day           crmcontracts.Attention
	bounded       map[crmcontracts.WorklistItemSource]bool
	first         bool
}

// moreTasks reads the next batch of the task window this page's filter shows,
// in the walk's own window, and moves the task position past it. The task
// source ends only when its own keyset read has nothing after the batch.
func (s *Service) moreTasks(ctx context.Context, page tailPage, rest *backlogCursor, limit int) ([]ranked, error) {
	zone, err := time.LoadLocation(rest.Window.Zone)
	if err != nil {
		return nil, err
	}
	window := s.taskStream(rest.Window, page.filter)
	var batch []Task
	var next string
	err = s.degradable(ctx, laneBudget, func(ctx context.Context) error {
		var err error
		batch, next, err = s.backlog.tasks.TasksAfter(ctx, window, rest.At[string(sourceTask)], limit)
		return err
	})
	if err != nil {
		return nil, err
	}
	if next == "" {
		delete(rest.At, string(sourceTask))
	} else {
		rest.At[string(sourceTask)] = next
	}
	asOf := rest.Window.AsOf
	items := plannedItems(batch, asOf, rest.Window.Until, zone)
	return s.admitTail(ctx, page, asOf, appendLane(nil, sourceTask, &items, asOf, classifyTask)), nil
}

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
func (s *Service) admitTail(ctx context.Context, page tailPage, asOf time.Time, rows []ranked) []ranked {
	rows, _ = admitBySource(ctx, rows)
	rows, _ = s.narrowToScope(ctx, rows, page.scope, s.taskOwner)
	rows = dropDone(rows, s.marked, asOf)
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
