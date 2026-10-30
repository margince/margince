// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// How the backlog tail merges its sources into one order.
//
// The order is one every source sorts by in SQL: a timestamp per row, oldest
// first and an undated row last, then the source's name, then the source's
// own order. A task's timestamp is when it falls due, a waiting card's when
// its message arrived, so the oldest obligation leads whichever source holds it.
//
// Each source reads a batch after its own position. A row is merged only when
// no source with more rows could still hold an earlier one. A source moves to
// the position after its last merged row, so what was read past that is read
// again next time, and it ends only when it had no more and all was merged.

import (
	"context"
	"errors"
	"maps"
	"sort"
	"time"

	"github.com/margince/margince/backend/internal/compose/worklistsnap"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// tailRow is one row a source read for the tail: the row, its timestamp in the
// tail's order, the position after it, and whether the page's steps keep it.
type tailRow struct {
	row   ranked
	at    *time.Time
	after string
	kept  bool
}

// tailBatch is one read of one source, in the source's own order. more says
// the source holds rows past it.
type tailBatch struct {
	source crmcontracts.WorklistItemSource
	rows   []tailRow
	more   bool
}

// tailPlace is a row's place in the tail's order, short of its source's own
// order, which breaks the ties left.
type tailPlace struct {
	at     *time.Time
	source crmcontracts.WorklistItemSource
}

// before orders two places: the earlier timestamp, an undated row last, then
// the source's name.
func (p tailPlace) before(q tailPlace) bool {
	switch {
	case p.at == nil && q.at == nil:
		return p.source < q.source
	case p.at == nil:
		return false
	case q.at == nil:
		return true
	case !p.at.Equal(*q.at):
		return p.at.Before(*q.at)
	}
	return p.source < q.source
}

// tailFrontier is the earliest last row among the batches with more rows past
// them. A row after it may come behind a row no read has reached yet. found is
// false when every batch was read to its source's end.
func tailFrontier(batches []tailBatch) (frontier tailPlace, found bool) {
	for _, batch := range batches {
		if !batch.more || len(batch.rows) == 0 {
			continue
		}
		last := tailPlace{batch.rows[len(batch.rows)-1].at, batch.source}
		if !found || last.before(frontier) {
			frontier, found = last, true
		}
	}
	return frontier, found
}

// mergeTail takes up to need rows from one round's batches in the tail's
// order, leaving out what the page's steps drop and what the ranked rows
// hold, and moves each source in rest past what it merged. A source's rows
// arrive in its own order, which the tail's order keeps, so what one source
// merges is always a run from the start of its batch.
func mergeTail(batches []tailBatch, need int, above map[worklistsnap.Row]bool, rest *backlogCursor) []ranked {
	frontier, bounded := tailFrontier(batches)
	type candidate struct {
		batch, index int
		place        tailPlace
	}
	var merged []candidate
	for b, batch := range batches {
		for i, row := range batch.rows {
			place := tailPlace{row.at, batch.source}
			if bounded && frontier.before(place) {
				break
			}
			merged = append(merged, candidate{b, i, place})
		}
	}
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].place.before(merged[j].place) })
	taken := make([]int, len(batches))
	var tail []ranked
	for _, next := range merged {
		if len(tail) >= need {
			break
		}
		row := batches[next.batch].rows[next.index]
		taken[next.batch] = next.index + 1
		if row.kept && !above[rowIdentity(row.row)] {
			tail = append(tail, row.row)
		}
	}
	for b, batch := range batches {
		switch {
		case taken[b] == len(batch.rows) && (!batch.more || len(batch.rows) == 0):
			delete(rest.At, string(batch.source))
		case taken[b] > 0:
			rest.At[string(batch.source)] = batch.rows[taken[b]-1].after
		}
	}
	return tail
}

// tailDone is the Done marks a tail batch is judged by: the ranked rows'
// marks, and what the reader marked among the batch's rows that offer Done.
// A refused read is no Done, as readingDone takes it.
func (s *Service) tailDone(ctx context.Context, rows []ranked) (doneState, error) {
	done := s.marked
	if done.store == nil || done.probe != nil {
		return done, nil
	}
	var among []RowRef
	for _, row := range rows {
		if offersDone(row.item.Source) {
			among = append(among, refOf(row))
		}
	}
	if len(among) == 0 {
		return done, nil
	}
	marked, err := done.store.DoneRows(ctx, among)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return done, nil
	}
	if err != nil {
		return doneState{}, err
	}
	at := make(map[RowRef]time.Time, len(done.at)+len(marked))
	maps.Copy(at, done.at)
	maps.Copy(at, marked)
	done.at = at
	return done, nil
}
