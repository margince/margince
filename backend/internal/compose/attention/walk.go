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
	"github.com/margince/margince/backend/internal/platform/database/storekit"
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
	// held is how many of the walk's rows this read could not judge, because
	// the lane that raised them was withheld or failed. Neither served nor
	// counted gone: a source that did not answer says nothing about the work it
	// carries, and reporting silence as resolution would tell a reader a backend
	// hiccup had cleared their morning.
	held int
}

// pageFrozenWalk serves one page in the order the walk was frozen in.
//
// LIVE ROWS, FROZEN SEQUENCE. The rows come from this read; only their order
// and membership come from the snapshot. A row the snapshot names that this
// read did not produce has gone — answered, deleted, or withdrawn — and is
// counted rather than substituted for. A row this read produced that the
// snapshot does not name has arrived behind the reader, and waits.
func pageFrozenWalk(
	rows []ranked, limit int, cursor worklistCursor, walk worklistsnap.Snapshot,
	unread map[string]bool,
) walkPage {
	// Today's rows, reachable by every identity a walk might name them by.
	//
	// A group answers to its MEMBERS' identities, because that is what the walk
	// froze — and to its own as well, so a token minted before this rule still
	// resolves. Several member identities can therefore reach one group row,
	// which is exactly right: the group is one row standing for all of them, and
	// `shown` de-duplicates so it is drawn once.
	live := make(map[worklistsnap.Row]ranked, len(rows))
	for _, row := range rows {
		live[rowIdentity(row)] = row
		for _, member := range row.members {
			live[member] = row
		}
	}
	// The walk's own sequence, keeping only what is still here. Walked in the
	// frozen order rather than sorted again: re-ranking is exactly what this
	// exists to avoid, and a comparator run over the survivors would reorder
	// them the moment any input to it moved.
	carried := make(map[worklistsnap.Row]bool, len(walk.Rows))
	for _, at := range walk.Rows {
		carried[at] = true
	}
	// A row missing because its LANE did not answer has not gone anywhere.
	//
	// A withheld or failed source contributes no rows, so every identity the
	// walk holds from it is absent from this read — and counting those as
	// resolved would tell a reader their morning had been dealt with by a
	// backend hiccup. Worse, the recovery reads as new work arriving: the same
	// still-open rows come back and are counted as having turned up behind them.
	//
	// So a walk judges only the lanes that answered. Rows from a silent one are
	// held: not served, because this read produced nothing to serve, and not
	// counted as gone either.
	gone, held := 0, 0
	for _, at := range walk.Rows {
		if _, here := live[at]; here {
			continue
		}
		if unread[at.Source] {
			held++
			continue
		}
		gone++
	}
	page := walkPage{
		gone: gone,
		held: held,
		// Everything this read holds that the walk does not. Counted over the
		// whole live set rather than the page, because the question is how much
		// a refresh would add to their day and not to their screen.
		arrived: countArrived(rows, carried),
	}
	// THE OFFSET COUNTS THE FROZEN LIST, never the surviving one, and this is
	// the whole correctness of the resume.
	//
	// The frozen list does not change; the surviving one shrinks as work is
	// dealt with. An offset read against the shorter list lands further into
	// the walk than the reader ever got: page one covers the first three, two of
	// those three are answered, and offset three now points two rows past where
	// they stopped — skipping the rows in between, silently, on a queue whose
	// whole purpose is that work is not forgotten. That was a real defect here
	// and TestAWalkDoesNotSkipRowsWhenEarlierOnesAreDealtWith is what caught it.
	//
	// So the cursor is a position among the walk's OWN rows, and the rows that
	// have gone are simply skipped as it walks past them.
	from := min(cursor.At, len(walk.Rows))
	shown := make([]ranked, 0, limit)
	drawn := make(map[worklistsnap.Row]bool, limit)
	at := from
	for ; at < len(walk.Rows) && len(shown) < limit; at++ {
		row, here := live[walk.Rows[at]]
		if !here {
			continue
		}
		// Once each. A group folded from several frozen members is reached by
		// every one of them, and drawing it per member would repeat one row as
		// many times as it stands for.
		if drawn[rowIdentity(row)] {
			continue
		}
		drawn[rowIdentity(row)] = true
		shown = append(shown, row)
	}
	// Anything left in the frozen list past this page — rows still to serve, or
	// only departed ones. Asked rather than inferred from the count: a tail made
	// entirely of gone rows must not mint a cursor that answers empty.
	page.shown, page.reached = shown, at
	page.more = anyLive(walk.Rows[at:], live)
	return page
}

// anyLive reports whether this tail of a walk still holds a servable row.
//
// The cut cannot be inferred from a count. A walk whose remaining rows have all
// been dealt with has a non-empty tail and nothing to serve from it, and a
// cursor minted there would invite one more request that can only answer
// empty — the exact failure `next_cursor` documents as the one it must never
// make wrongly.
func anyLive(tail []worklistsnap.Row, live map[worklistsnap.Row]ranked) bool {
	for _, at := range tail {
		if _, here := live[at]; here {
			return true
		}
	}
	return false
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
		// A FOLDED GROUP freezes its members, never itself. Its own id is minted
		// from the key and cause, so it exists only while the fold produces it:
		// deal with one member, drop the rest below the floor, and the group
		// stops being minted. Frozen by its own id it would read as gone, and
		// its still-unresolved members would read as newly arrived — real work
		// falling out of a walk the reader was part-way through.
		//
		// Frozen by member, the group is simply whatever today's fold makes of
		// the members that remain.
		if len(row.members) > 0 {
			out = append(out, row.members...)
			continue
		}
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
	// buckets is the partition the walk STARTED with, read back from the store.
	//
	// A resumed page must state this rather than a fresh count: the live day
	// holds work that arrived behind the reader, and recomputing would let the
	// headline climb — the exact movement freezing the walk exists to stop.
	buckets worklistsnap.Buckets
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
	unread map[string]bool, asOf time.Time,
) (shown []ranked, more bool, reached int, walk walkState) {
	if s.walks == nil {
		shown, more, reached = pageFrom(rows, limit, cursor)
		return shown, more, reached, walkState{}
	}
	// The walk was resolved before the day was read — early, so a token that
	// cannot be continued does not cost a dozen lane reads — and carried here on
	// a per-request copy. Resuming again would be a second read of one fact.
	if s.walk != nil {
		id, _ := snapshotOf(cursor)
		page := pageFrozenWalk(rows, limit, cursor, *s.walk, unread)
		return page.shown, page.more, page.reached, walkState{
			id:      id,
			frozen:  s.walk.AsOf,
			gone:    page.gone,
			arrived: page.arrived,
			buckets: s.walk.Buckets,
			carried: true,
			resumed: true,
		}
	}
	shown, more, reached = pageFrom(rows, limit, cursor)
	return shown, more, reached, s.freezeWalk(ctx, rows, asOf, scope, filter)
}

// freezeWalk records what this first page assembled.
//
// The WHOLE ranked set, not the page: a walk is the day the reader started on,
// and freezing only the first twenty-five would end it at the bottom of page
// one. Its buckets are the day's too, for the same reason.
//
// The DAY's instant, not a second clock read. The response publishes the
// assembly's `as_of` and the contract says a first page's walk shares it, so
// reading the clock again here made the two disagree by however long the
// assembly took.
//
// A failure here is swallowed deliberately, and it is the one place in this
// change where that is right: the alternative is failing a reader's whole day
// because a convenience could not be recorded. The cost is that their next page
// is unfrozen, which the absent walk on the response already tells the client.
func (s *Service) freezeWalk(
	ctx context.Context, rows []ranked, asOf time.Time, scope, filter string,
) walkState {
	id, err := s.walks.Freeze(
		ctx, fingerprint(scope, filter, s.taskOwner), asOf,
		frozenBuckets(bucketsOf(rows)), frozenRows(rows))
	if err != nil {
		return walkState{}
	}
	return walkState{id: id, frozen: asOf, carried: true}
}

// unreadSources names the lanes this read could not see.
//
// A walk judges only what answered. A row missing because its own source was
// withheld or failed has not been dealt with — the queue simply has no news
// about it — and the difference decides whether a reader is told their morning
// shrank or that part of it could not be read.
func unreadSources(missing []crmcontracts.WorklistSourceUnavailable) map[string]bool {
	if len(missing) == 0 {
		return nil
	}
	out := make(map[string]bool, len(missing))
	for _, lane := range missing {
		out[string(lane.Source)] = true
	}
	return out
}

// statedOver puts the walk's own partition on a summary computed live.
//
// The sibling signals — urgent, due, in_play, lower_priority — stay this
// read's: they are questions about the day, and a reader asking "how much is
// overdue" wants today's answer.
//
// The PARTITION is the walk's, because it is the additive headline drawn beside
// the queue. Recomputed live it would count work that arrived behind the reader
// and is deliberately not in their walk, so the sentence would climb while the
// rows below it did not — which is precisely the movement freezing a walk
// exists to stop.
//
// Only on a RESUMED page. A first page's live count IS the walk's, having just
// been frozen from it, and reading back a snapshot to restate what was just
// computed would be a second answer to one question.
func (w walkState) statedOver(summary crmcontracts.WorklistSummary) crmcontracts.WorklistSummary {
	if !w.resumed {
		return summary
	}
	frozen := crmcontracts.WorklistBuckets{
		Urgent:   w.buckets.Urgent,
		DueToday: w.buckets.DueToday,
		Planned:  w.buckets.Planned,
		Review:   w.buckets.Review,
	}
	summary.Buckets = &frozen
	return summary
}

// walkNamedBy resolves the walk a cursor names, before the day is assembled.
//
// EARLY, for the reason decodeCursor is early: a token that cannot be continued
// is the caller's to fix, and assembling a dozen lanes to then discard the page
// spends real reads on an answer nobody receives.
//
// It answers three ways. No store or no walk named: nothing to resume, and the
// page is served fresh. A walk that resumes: carried to the projection so it
// need not read the store twice. A walk that refuses: the error travels, and
// the caller re-issues without the cursor.
func (s *Service) walkNamedBy(
	ctx context.Context, cursor worklistCursor, scope, filter string, owner ids.UUID,
) (walk worklistsnap.Snapshot, named bool, err error) {
	id, carried := snapshotOf(cursor)
	if s.walks == nil || !carried {
		return worklistsnap.Snapshot{}, false, nil
	}
	frozen, err := s.walks.Resume(ctx, id, fingerprint(scope, filter, owner))
	if err != nil {
		// The same refusal a cursor minted under a different question earns,
		// and the same remedy: ask again without it. A dedicated code would
		// tell the client nothing it can act on differently.
		return worklistsnap.Snapshot{}, false, &storekit.CursorSortMismatchError{}
	}
	return frozen, true, nil
}

// readingWalk returns a copy of this service holding the walk this page serves.
//
// A copy, for the reason readingPins takes one: a single Service serves every
// request, so a field set on it would follow one reader's page onto another's.
func (s *Service) readingWalk(walk worklistsnap.Snapshot, walking bool) *Service {
	reading := *s
	if walking {
		reading.walk = &walk
	}
	return &reading
}

// readingScores carries this read's overnight composites onto a copy of the
// service, the way readingWalk carries the walk.
//
// A COPY, always. One Service serves every request, and a score map left on the
// shared one would rank the next reader's page by the last reader's night.
func (s *Service) readingScores(scores map[ids.UUID]float64, cutoff time.Time) *Service {
	reading := *s
	reading.briefScores = scores
	reading.briefCutoff = cutoff
	return &reading
}
