// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The reader's own override, applied to the day they are shown.
//
// The ranking has carried a pin level since it was written and nothing could
// ever set it. Every other control on this page changes what the SERVER thinks
// — a disposition, a filter, a scope. This is the only one that says "I know,
// and I want this first anyway", which is the difference between a queue a rep
// works and a queue a rep argues with.

import "context"

// Pins is the reader's own pinned rows. OPTIONAL like every lane seam: nil
// means this feed applies no pins, and a day assembled without it ranks exactly
// as it did before pinning existed.
type Pins interface {
	// PinnedRows answers which rows THIS reader put at the top. Keyed on the
	// row's identity — source and id together — because that pair is what
	// names a row: the lanes mint ids independently, so an id alone can match a
	// row in a lane the reader was not looking at.
	PinnedRows(ctx context.Context) (map[RowRef]bool, error)
}

// RowRef names one row of the assembled day, the way the client names it.
type RowRef struct {
	Source string
	RowID  string
}

// refOf is a candidate's own identity.
func refOf(row ranked) RowRef {
	return RowRef{Source: string(row.item.Source), RowID: row.item.Id}
}

// applyPins raises every pinned row to the pin level.
//
// It runs BEFORE the ranking, not after: the pin is a level, and the ordering,
// the band headings and the "why here" explanation all read the level. Sorting
// the page and then moving rows would leave every one of those three saying
// something the page contradicts.
//
// A pin naming a row the day did not assemble does nothing, and that is the
// honest behaviour rather than a gap. The pin says which row leads WHEN it is
// there; a queue that fabricated a row to honour one would be answering with
// something other than the day.
func applyPins(rows []ranked, pinned map[RowRef]bool) []ranked {
	if len(pinned) == 0 {
		return rows
	}
	for i := range rows {
		if pinned[refOf(rows[i])] {
			rows[i].item.Level = levelPinned
		}
	}
	return rows
}

// readingPins returns a copy of this service holding the pins THIS reader made.
//
// A copy, for the reason countingDecisions takes one: a single Service serves
// every request, so a field set on it would follow one reader's page onto
// another's — and the failure would be a rep seeing a colleague's row at the
// top of their own day with nothing to explain it.
//
// An UNBOUND seam answers no pins, which is a real answer: an installation that
// never wired pinning has none, and that day ranks exactly as it did before
// pinning existed.
//
// A read that FAILS is an error and travels as one. It would be easy to answer
// "no pins" and draw the page anyway — the order would still be a correct order
// — but the reader put a row at the top and would be shown a page that silently
// did not, with nothing on it to say so. A queue that quietly ignores the one
// override it offers teaches the rep that pinning does not work.
func (s *Service) readingPins(ctx context.Context) (*Service, error) {
	if s.pins == nil {
		return s, nil
	}
	pinned, err := s.pins.PinnedRows(ctx)
	if err != nil {
		return nil, err
	}
	reading := *s
	reading.pinned = pinned
	return &reading, nil
}
