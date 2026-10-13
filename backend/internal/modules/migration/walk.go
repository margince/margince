// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"context"
	"time"
)

// Budget bounds one slice; a zero field is no bound. Time runs from when the
// slice is ready to walk, so setup never spends it. FlushRows and FlushEvery
// are how often a leased slice writes its progress before the slice ends.
type Budget struct {
	Rows       int
	Time       time.Duration
	FlushRows  int
	FlushEvery time.Duration
	// Clock is the slice's one clock; nil is the wall clock.
	Clock func() time.Time
	until time.Time
}

func (b Budget) now() time.Time {
	if b.Clock == nil {
		return time.Now()
	}
	return b.Clock()
}

// started is the budget of a slice whose walk begins now.
func (b Budget) started() Budget {
	if b.Time > 0 {
		b.until = b.now().Add(b.Time)
	}
	return b
}

// spent answers whether a started slice ends before its next move. A slice
// lands one move first, so even a slow setup moves the run.
func (b Budget) spent(moves int) bool {
	if moves == 0 {
		return false
	}
	return (b.Rows > 0 && moves >= b.Rows) || (!b.until.IsZero() && !b.now().Before(b.until))
}

func (b Budget) flushDue(moves int, since time.Duration) bool {
	return (b.FlushRows > 0 && moves >= b.FlushRows) || (b.FlushEvery > 0 && since >= b.FlushEvery)
}

// ledger is where a walk records itself: the flip's per-row checkpoint, or a
// lease's fenced flushes.
type ledger interface {
	open(ctx context.Context, id RunID) (Run, error)
	// replayed names the rows an earlier attempt of this walk landed after its
	// last flush, keyed object/external id.
	replayed(ctx context.Context) (map[string]bool, error)
	landed(ctx context.Context, wk *walk) error
	// moved runs after every row and edge.
	moved(ctx context.Context, wk *walk) error
	// stop runs before every row and edge, and answers whether the slice ends there.
	stop(ctx context.Context, wk *walk) (bool, error)
	complete(ctx context.Context, wk *walk) error
	// fail answers whether it recorded the stop on the run.
	fail(ctx context.Context, wk *walk, cause error) (bool, error)
}

// flipLedger keeps the flip's contract: the checkpoint advances after every
// row, and the attempt's whole report is folded in when it ends.
type flipLedger struct{ runs runRecords }

func (f flipLedger) open(ctx context.Context, id RunID) (Run, error) { return f.runs.Get(ctx, id) }

func (flipLedger) replayed(context.Context) (map[string]bool, error) { return map[string]bool{}, nil }

func (f flipLedger) landed(ctx context.Context, wk *walk) error {
	return f.runs.advanceCheckpoint(ctx, wk.runID, wk.cursor)
}

func (flipLedger) moved(context.Context, *walk) error { return nil }

func (flipLedger) stop(context.Context, *walk) (bool, error) { return false, nil }

func (f flipLedger) complete(ctx context.Context, wk *walk) error {
	return f.runs.complete(ctx, wk.runID, wk.leg())
}

func (f flipLedger) fail(ctx context.Context, wk *walk, cause error) (bool, error) {
	return true, f.runs.failRun(ctx, wk.runID, wk.leg(), cause)
}

// walk is one slice's position and what it has landed since its last flush.
type walk struct {
	runID RunID
	// start is the checkpoint the slice began from; cursor is the global index
	// of the next row, then of the next edge after every row.
	start, cursor int
	// legStart is the cursor the unflushed leg began at; rep and cur hold it.
	legStart int
	rep      Report
	cur      *ObjectReport
	counts   ProgressCounts
	objects  []string
	totals   map[string]int
	links    int
	phase    string
	object   string
	replayed map[string]bool
	budget   Budget
	// moves counts this slice's rows and edges; sinceFlush those since the
	// last flush, which flushedAt dates.
	moves, sinceFlush int
	flushedAt         time.Time
}

func newWalk(id RunID, run Run, b Budget) *walk {
	wk := &walk{
		runID: id, start: run.Checkpoint, cursor: run.Checkpoint, legStart: run.Checkpoint,
		budget: b, flushedAt: b.now(),
	}
	if run.Progress != nil {
		wk.counts = run.Progress.Counts
	}
	return wk
}

// landed records one row's outcome. A row this walk landed before a crash
// lost its flush reads Unchanged on replay, and counts as the create it was.
func (wk *walk) landed(object string, row Row, res EnsureResult) {
	key := object + "/" + row.ExternalID
	if res.Unchanged && wk.replayed[key] {
		delete(wk.replayed, key)
		res.Unchanged, res.Created = false, true
	}
	wk.cur.record(row, res)
	wk.counts.add(res)
	wk.cursor++
}

func (wk *walk) linked(a Assoc, res AssocResult) {
	if res.Applied {
		wk.rep.Associations++
	} else {
		wk.rep.AssociationsSkipped = append(wk.rep.AssociationsSkipped, SkippedAssoc{
			From: a.FromType + "/" + a.FromID, To: a.ToType + "/" + a.ToID, Reason: res.Reason,
		})
	}
	wk.cursor++
}

func (wk *walk) finishObject() {
	wk.rep.Objects = append(wk.rep.Objects, *wk.cur)
	wk.rep.Imported += int64(wk.cur.Created + wk.cur.Updated)
	wk.cur = nil
}

// leg is what the walk landed since its last flush, the class in progress included.
func (wk *walk) leg() Report {
	if wk.cur == nil {
		return wk.rep
	}
	return withPartial(wk.rep, *wk.cur)
}

// flush is the fenced write of the leg. A leg that starts the walk says so,
// which counts the walk once in the stored report.
func (wk *walk) flush() Flush {
	leg := wk.leg()
	if wk.legStart == 0 {
		leg.Attempts = 1
	}
	return Flush{Checkpoint: wk.cursor, Leg: leg, Progress: wk.progress(), Phase: wk.phase, Object: wk.object}
}

// startLeg begins the next leg after a flush landed.
func (wk *walk) startLeg() {
	wk.rep = Report{}
	if wk.cur != nil {
		wk.cur = &ObjectReport{Object: wk.cur.Object, MirrorCount: wk.cur.MirrorCount}
	}
	wk.legStart, wk.sinceFlush, wk.flushedAt = wk.cursor, 0, wk.budget.now()
}

func (wk *walk) progress() Progress {
	p := Progress{Counts: wk.counts}
	offset := 0
	for _, object := range wk.objects {
		total := wk.totals[object]
		p.Objects = append(p.Objects, objectProgress(object, wk.cursor-offset, total))
		offset += total
	}
	p.Objects = append(p.Objects, objectProgress(progressLinks, wk.cursor-offset, wk.links))
	return p
}

func objectProgress(object string, done, total int) ObjectProgress {
	return ObjectProgress{Object: object, RowsDone: min(max(done, 0), total), RowsTotal: total}
}

func (c *ProgressCounts) add(res EnsureResult) {
	switch {
	case res.Skipped:
		c.Skipped++
	case res.Unchanged:
		c.Unchanged++
	case res.Created:
		c.Created++
	default:
		c.Updated++
	}
	if res.Problem != nil {
		c.Problems++
	}
}
