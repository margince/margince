// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Why a purge kept what it kept.
//
// Its own file because the question is not the selectors': they ask which of a
// seat's mail a rule reaches, and this asks what to tell the owner about the
// part of it that survived. A count alone says something outlived their
// deletion and not what would have to change for it to go, and the three
// reasons answer that differently: a hold lifts when somebody lifts it, a
// statutory window expires on a date, an open request closes when it is
// finished.

import "github.com/margince/margince/backend/internal/shared/kernel/ids"

// Purge reasons, as the selectors report them: the vocabulary the three
// selectors write and the collector reads back.
const (
	withheldByHold    = "hold"
	withheldByStatute = "statute"
	withheldByRequest = "request"
	// withheldByUndeterminedFloor is what shields a row when the installation
	// could not say what the law requires of it. The row is kept — that is the
	// safe answer — but `statute` is a CLAIM about why, and this one was never
	// established. Reported apart so the owner is told the rule could not be
	// determined rather than told their mail is commercial correspondence.
	withheldByUndeterminedFloor = "floor_undetermined"
)

// noteWithheld files one kept activity under the reason the selector gave.
//
// An unrecognized reason is filed nowhere rather than guessed at: the union
// above already counts it, so the total still balances, and inventing a
// category would tell an owner something the query never said.
func (s *PurgeSubject) noteWithheld(reason string, id ids.UUID) {
	switch reason {
	case withheldByHold:
		s.Held = append(s.Held, id)
	case withheldByStatute:
		s.UnderStatute = append(s.UnderStatute, id)
	case withheldByRequest:
		s.UnderRequest = append(s.UnderRequest, id)
	case withheldByUndeterminedFloor:
		s.UnderUndeterminedFloor = append(s.UnderUndeterminedFloor, id)
	}
}

// withheldReason renders the reason a row was kept, in precedence order: the
// most specific act about THIS record first.
//
// A row can satisfy several at once — a pinned Handelsbrief named by an open
// request is all three — and the owner is owed one answer rather than a list,
// so the order decides. A hand-placed hold outranks the statutory window
// because somebody decided it about this record; the window outranks an open
// request because it outlives the request's resolution.
// shieldedAs is the reason the shield's own arm reports — the floor names it,
// because only the floor knows whether it measured anything.
func withheldReason(shielded, shieldedAs string, underRequest bool) string {
	clause := `CASE
		WHEN a.restricted_at IS NOT NULL THEN '` + withheldByHold + `'
		WHEN (` + shielded + `) THEN '` + shieldedAs + `'`
	if underRequest {
		clause += `
		WHEN (` + underAnOpenRequest + `) THEN '` + withheldByRequest + `'`
	}
	return clause + `
		ELSE '' END`
}
