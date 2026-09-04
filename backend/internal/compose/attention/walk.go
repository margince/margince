// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// Paging a walk that was frozen when it started.
//
// The ordinary page ranks the day and cuts it. That is honest and it moves
// under the reader: the ranking is rebuilt on every read, so a row crossing the
// page boundary between two of them is served twice or not at all, and the
// count above the queue climbs as work arrives behind somebody who is still
// paging.
//
// A frozen walk fixes the ORDER and the MEMBERSHIP at the first page. Later
// pages re-assemble the live day exactly as before — every row is read through
// the same gated lanes, so nothing here widens what a caller may see — and then
// take their sequence from the snapshot instead of from the fresh ranking.
//
// WHAT THE SNAPSHOT DECIDES AND WHAT IT DOES NOT. It decides which rows the
// walk covers and in what order. It decides nothing about what those rows SAY:
// the content is whatever this read produced, under this caller's grants, at
// this instant. A row whose visibility was withdrawn between two pages is
// simply not in the live set, so it does not appear — which is the behaviour a
// frozen copy of the row's text could not have given.

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/compose/worklistsnap"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// walkPage is one page of a frozen walk, plus what the reader must be told
// about how the walk has moved.
type walkPage struct {
	shown []ranked
	more  bool
	// reached is where in the SNAPSHOT's order this page stopped, so the next
	// token resumes into the same sequence rather than into a fresh ranking.
	reached int
	// gone is how many of the walk's rows are no longer here — resolved,
	// deleted, or no longer visible to this reader. Cumulative over the whole
	// walk rather than per page: the reader's question is how much of their
	// morning has been dealt with, not how much went between two clicks.
	gone int
	// arrived is how much work exists now that the walk does not carry. It is
	// an offer to refresh, never a promise that these rows are reachable — they
	// are reachable by starting a new walk, which is what refreshing does.
	arrived int
}

// pageFrozenWalk serves one page in the order the walk was frozen in.
//
// LIVE ROWS, FROZEN SEQUENCE. The rows come from this read; only their order
// and membership come from the snapshot. A row the snapshot names that this
// read did not produce has gone — answered, deleted, or withdrawn — and is
// counted rather than substituted for. A row this read produced that the
// snapshot does not name has arrived behind the reader, and waits.
func pageFrozenWalk(rows []ranked, limit int, cursor worklistCursor, walk worklistsnap.Snapshot) walkPage {
	live := make(map[worklistsnap.Row]ranked, len(rows))
	for _, row := range rows {
		live[rowIdentity(row)] = row
	}
	// The walk's own sequence, keeping only what is still here. Walked in the
	// frozen order rather than sorted again: re-ranking is exactly what this
	// exists to avoid, and a comparator run over the survivors would reorder
	// them the moment any input to it moved.
	surviving := make([]ranked, 0, len(walk.Rows))
	carried := make(map[worklistsnap.Row]bool, len(walk.Rows))
	for _, at := range walk.Rows {
		carried[at] = true
		if row, here := live[at]; here {
			surviving = append(surviving, row)
		}
	}
	page := walkPage{
		gone: len(walk.Rows) - len(surviving),
		// Everything this read holds that the walk does not. Counted over the
		// whole live set rather than the page, because the question is how much
		// a refresh would add to their day and not to their screen.
		arrived: countArrived(rows, carried),
	}
	from := min(cursor.At, len(surviving))
	surviving = surviving[from:]
	if len(surviving) > limit {
		page.shown, page.more, page.reached = surviving[:limit], true, from+limit
		return page
	}
	page.shown, page.more, page.reached = surviving, false, from+len(surviving)
	return page
}

// countArrived is how much of this read the walk never carried.
func countArrived(rows []ranked, carried map[worklistsnap.Row]bool) int {
	arrived := 0
	for _, row := range rows {
		if !carried[rowIdentity(row)] {
			arrived++
		}
	}
	return arrived
}

// rowIdentity names a row the way the walk stores it.
//
// The SOURCE and the id together, because that pair is what identifies a row on
// this queue: the lanes mint ids independently, so an id alone can match a row
// in a lane the reader was not looking at. The same pair the pin store keys on,
// and for the same reason.
func rowIdentity(row ranked) worklistsnap.Row {
	return worklistsnap.Row{Source: string(row.item.Source), RowID: row.item.Id}
}

// frozenRows is the identity list a first page hands the snapshot store.
//
// Order matters and content does not: this is what the walk will be replayed
// in, and every row's text is re-read live on each page.
func frozenRows(rows []ranked) []worklistsnap.Row {
	out := make([]worklistsnap.Row, 0, len(rows))
	for _, row := range rows {
		out = append(out, rowIdentity(row))
	}
	return out
}

// frozenBuckets is the partition a walk starts with, in the store's shape.
//
// Carried so the headline states what the walk BEGAN with while the response
// separately reports what has left it. Recomputing it from the surviving rows
// would give the falling number twice and the starting one never.
func frozenBuckets(summary crmcontracts.WorklistBuckets) worklistsnap.Buckets {
	return worklistsnap.Buckets{
		Urgent:   summary.Urgent,
		DueToday: summary.DueToday,
		Planned:  summary.Planned,
		Review:   summary.Review,
	}
}

// snapshotOf is the walk a cursor names, and whether it named one at all.
//
// A zero id is a token minted before walks existed, or by an installation that
// wires no store. It is not a fault: the caller pages the old way, which is the
// behaviour cursor.go documents and the cost resumeAt states.
func snapshotOf(cursor worklistCursor) (ids.UUID, bool) {
	return cursor.Snapshot, !cursor.Snapshot.IsZero()
}

// Walks holds one reader's position in one walk still while they page it.
//
// OPTIONAL like every lane seam: nil means this feed freezes nothing and every
// page is an offset into a freshly ranked day — the behaviour this queue has
// always had, with the cost cursor.go states.
//
// It takes and returns identities and counts only. The assembler never hands it
// a title, a name or a subject, and could not usefully receive one back: the
// content of every row is re-read live on each page under the caller's own
// grants, so a stored copy would be a second answer to a question the gates
// have already answered.
type Walks interface {
	// Freeze records the walk a first page just assembled and answers its id.
	Freeze(
		ctx context.Context, fingerprint string, asOf time.Time,
		buckets worklistsnap.Buckets, rows []worklistsnap.Row,
	) (ids.UUID, error)
	// Resume reads back a walk this reader started, refusing one that belongs
	// to somebody else, has expired, or was minted under a different question.
	Resume(ctx context.Context, id ids.UUID, fingerprint string) (worklistsnap.Snapshot, error)
}

// walkState is what this page knows about the walk it served.
//
// Carried out of pageOf rather than read back from the response, because two of
// its three fields exist only during the page: `gone` and `arrived` are
// differences between the frozen list and the live one, and neither survives
// into the rows.
type walkState struct {
	// id is the walk the next token resumes, zero where none was frozen.
	id ids.UUID
	// frozen is when the walk was assembled — older than this read on any page
	// but the first, which is what lets a client say how stale it has become.
	frozen  time.Time
	gone    int
	arrived int
	// carried says whether there is a walk at all. A first page that could not
	// freeze one, and an installation with no store, both leave this false.
	carried bool
	// resumed says this page continued a walk rather than starting one.
	//
	// Its own field rather than a comparison of instants: a first page and its
	// walk share an `as_of` by construction, so testing them for equality asks
	// the clock a question only the caller can answer — and gets it wrong the
	// moment two reads land on the same instant, which under an injected clock
	// is every read.
	resumed bool
}

// state renders what a client is told about the walk.
//
// Nil where no walk was frozen: the fields would otherwise claim a walk exists
// and report zero changes to it, which a reader would take as "nothing has
// moved" rather than as "nothing is being tracked".
func (w walkState) state() *crmcontracts.WorklistWalk {
	if !w.carried {
		return nil
	}
	out := &crmcontracts.WorklistWalk{
		AsOf:                 w.frozen,
		ChangedSinceSnapshot: w.gone,
	}
	// Absent on a first page, where the question has no meaning: the walk was
	// just assembled, so nothing can have arrived behind it. Reporting zero
	// there would be an answer to a question nobody can ask yet.
	if w.resumed {
		arrived := w.arrived
		out.NewAvailable = &arrived
	}
	return out
}

// pageOf serves one page, freezing a new walk or resuming the one named.
//
// THREE CASES, and the third is the one worth spelling out.
//
// No store wired: page the old way. An offset into a freshly ranked day is what
// this queue has always done, and an installation with no snapshot store gets
// exactly that rather than a degraded version of something else.
//
// No cursor: this is a first page, so freeze what it assembled and hand back
// the id. A failure to freeze does NOT fail the page — the reader gets their
// day, paged the old way, which is strictly better than an error over a
// convenience they never asked for. It does mean the next page is unfrozen, and
// the response says so by carrying no walk.
//
// A cursor naming a walk: resume it. A walk that cannot be resumed — expired,
// somebody else's, minted under a different question — is REFUSED rather than
// quietly restarted. Silently serving a fresh page under a resumed token would
// hand the reader rows they have already seen, in a new order, with nothing
// saying the walk they were on had ended.
func (s *Service) pageOf(
	ctx context.Context, rows []ranked, limit int, cursor worklistCursor, scope, filter string,
) (shown []ranked, more bool, reached int, walk walkState) {
	if s.walks == nil {
		shown, more, reached = pageFrom(rows, limit, cursor)
		return shown, more, reached, walkState{}
	}
	if id, named := snapshotOf(cursor); named {
		return s.resumeWalk(ctx, rows, limit, cursor, id, scope, filter)
	}
	shown, more, reached = pageFrom(rows, limit, cursor)
	return shown, more, reached, s.freezeWalk(ctx, rows, scope, filter)
}

// freezeWalk records what this first page assembled.
//
// The WHOLE ranked set, not the page: a walk is the day the reader started on,
// and freezing only the first twenty-five would end it at the bottom of page
// one. Its buckets are the day's too, for the same reason.
//
// A failure here is swallowed deliberately, and it is the one place in this
// change where that is right: the alternative is failing a reader's whole day
// because a convenience could not be recorded. The cost is that their next page
// is unfrozen, which the absent walk on the response already tells the client.
func (s *Service) freezeWalk(
	ctx context.Context, rows []ranked, scope, filter string,
) walkState {
	asOf := s.now()
	id, err := s.walks.Freeze(
		ctx, fingerprint(scope, filter, s.taskOwner), asOf,
		frozenBuckets(bucketsOf(rows)), frozenRows(rows))
	if err != nil {
		return walkState{}
	}
	return walkState{id: id, frozen: asOf, carried: true}
}

// resumeWalk continues a walk this reader started.
func (s *Service) resumeWalk(
	ctx context.Context, rows []ranked, limit int, cursor worklistCursor,
	id ids.UUID, scope, filter string,
) (shown []ranked, more bool, reached int, walk walkState) {
	frozen, err := s.walks.Resume(ctx, id, fingerprint(scope, filter, s.taskOwner))
	if err != nil {
		// Refused rather than restarted. The caller sees the same 422 a
		// malformed cursor earns, and its remedy is the same: ask again without
		// a cursor and start a fresh walk.
		return nil, false, 0, walkState{}
	}
	page := pageFrozenWalk(rows, limit, cursor, frozen)
	return page.shown, page.more, page.reached, walkState{
		id:      id,
		frozen:  frozen.AsOf,
		gone:    page.gone,
		arrived: page.arrived,
		carried: true,
		resumed: true,
	}
}
