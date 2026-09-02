// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package owedwork ranks the promises a record is carrying, whichever of the
// two places they were written down.
//
// THE PRODUCT DECISION THIS ENCODES: a promise made in a conversation and a
// promise somebody typed as a task are the SAME THING to a reader. Where it
// was recorded is a fact about this system, not about what is owed. Every
// surface that answers "what do we owe them?" therefore ranks both sources by
// one rule, and this package is that rule.
//
// WHY A PACKAGE RATHER THAN A HELPER ON ONE SIDE. The two sources live in
// tables owned by different modules — `activity` in activities,
// `conversation_claim` in people — and a module never imports a sibling, so
// neither side can host a comparison over both. Ranking them is not a
// database question anyway: it is a pure function of due dates, and putting it
// where every tier may import it is what stops the next surface from writing a
// third spelling.
//
// Stdlib only, by the shared tier's rule. That is also why the type below is
// this package's own rather than the wire contract's: a caller maps its rows in
// and reads the ranking out, and the contract stays where it belongs.
package owedwork

import (
	"sort"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/deadline"
)

// Source names where a promise was written down. It travels with the item
// because a surface renders the two differently — a claim can quote the
// sentence it was read from, a task cannot — even though both rank alike.
type Source string

const (
	// FromClaim is a promise an extractor read out of a captured conversation.
	FromClaim Source = "claim"
	// FromTask is a promise somebody filed as a task.
	FromTask Source = "task"
)

// Item is one thing owed, reduced to what ranking needs. A caller keeps its
// own row and uses Ref to find it again.
type Item struct {
	// Ref identifies the caller's row. This package never reads it — it only
	// hands it back — so a caller may put an id, an index, or the row itself
	// behind an interface value in it.
	Ref any
	// Source is which of the two places this promise was recorded.
	Source Source
	// DueAt is the promised moment, or nil where none was set. An undated
	// promise is real work that is not yet late, which is what NotYetDue and
	// Overdue between them say.
	DueAt *time.Time
	// FiledAt is when the promise was made. It breaks ties between two items
	// sharing a due date, and orders the undated ones among themselves.
	//
	// The zero time means "not known", and it LOSES every tie rather than
	// winning one. A caller whose row carries no moment must leave this zero:
	// the zero time is before every real one, so ordering on it naively would
	// make the promise nobody can date the oldest thing on the record.
	FiledAt time.Time
}

// Overdue reports whether this promise is behind its date. Undated work is
// never overdue: nothing with no date can have missed it.
func (i Item) Overdue(now time.Time) bool { return deadline.Passed(i.DueAt, now) }

// MostRecentlySlipped is the overdue promise whose date passed LAST, and
// whether any is overdue at all.
//
// The latest slip rather than the oldest, and the choice matters on a record
// owing several. The one that went past yesterday is the one still
// recoverable — an apology today is worth something, and the reader can
// plausibly deliver it. The promise three months late has already done its
// damage, and naming it first sends a reader to the one thing they cannot
// rescue while the rescuable one goes unmentioned.
//
// A tie goes to the CLAIM, because a claim carries the sentence the promise
// was made in and a task carries only what somebody retyped. Shown a choice
// between two equally late promises, the one that can quote itself is the one
// a reader can act on without going to look it up.
func MostRecentlySlipped(items []Item, now time.Time) (Item, bool) {
	var best Item
	found := false
	for _, item := range items {
		if !item.Overdue(now) {
			continue
		}
		if !found || slippedLater(item, best) {
			best, found = item, true
		}
	}
	return best, found
}

// slippedLater reports whether a beats b for the most-recently-slipped seat.
// Both are known overdue, so both due dates are set.
func slippedLater(a, b Item) bool {
	if a.DueAt.Equal(*b.DueAt) {
		return a.Source == FromClaim && b.Source != FromClaim
	}
	return a.DueAt.After(*b.DueAt)
}

// Soonest is the promise a reader should do next among those NOT yet late,
// and whether there is one.
//
// The nearest deadline first, then the undated ones. An undated promise sorts
// behind every dated one rather than being dropped: "I'll send you the
// whitepaper" with no date is owed, and a queue that silently excludes it
// tells a reader they are clear when they are not. Among the undated, the
// oldest promise comes first — it has been waiting longest.
func Soonest(items []Item, now time.Time) (Item, bool) {
	var best Item
	found := false
	for _, item := range items {
		if item.Overdue(now) {
			continue
		}
		if !found || beforeInQueue(item, best) {
			best, found = item, true
		}
	}
	return best, found
}

// Sorted returns the items in the order every surface shows them: overdue
// first, most recently slipped at the top, then the rest by nearest deadline
// with the undated behind them.
//
// The overdue block leads because lateness is the thing a reader must be told
// about, and it is ordered by the same rule MostRecentlySlipped applies, so
// the card at the top of a page and the first row of the list beneath it name
// the same promise. Two surfaces disagreeing about "what first" was the defect
// this package exists to remove.
//
// The sort is stable, so items this rule considers equal keep the order the
// caller read them in rather than shuffling between two renders of one page.
func Sorted(items []Item, now time.Time) []Item {
	out := make([]Item, len(items))
	copy(out, items)
	sort.SliceStable(out, func(a, b int) bool {
		x, y := out[a], out[b]
		xLate, yLate := x.Overdue(now), y.Overdue(now)
		if xLate != yLate {
			return xLate
		}
		if xLate {
			return slippedLater(x, y)
		}
		return beforeInQueue(x, y)
	})
	return out
}

// beforeInQueue orders two promises that are not yet late: nearest deadline
// first, undated last, oldest promise first among the undated.
func beforeInQueue(a, b Item) bool {
	switch {
	case a.DueAt == nil && b.DueAt == nil:
		return filedEarlier(a, b)
	case a.DueAt == nil:
		return false
	case b.DueAt == nil:
		return true
	case a.DueAt.Equal(*b.DueAt):
		return filedEarlier(a, b)
	default:
		return a.DueAt.Before(*b.DueAt)
	}
}

// filedEarlier reports whether a was promised before b, with an unknown moment
// losing to a known one.
//
// The zero time is before every real time, so comparing it directly would rank
// the promise nobody can date as the oldest on the record — first in a queue
// ordered oldest-first, on the strength of a timestamp that is missing rather
// than early. An unknown moment sorts last instead, and two unknowns are equal
// so the caller's own order survives the stable sort.
func filedEarlier(a, b Item) bool {
	switch {
	case a.FiledAt.IsZero() && b.FiledAt.IsZero():
		return false
	case a.FiledAt.IsZero():
		return false
	case b.FiledAt.IsZero():
		return true
	default:
		return a.FiledAt.Before(b.FiledAt)
	}
}
