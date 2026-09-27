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
func withheldReason(shielded string, underRequest bool) string {
	clause := `CASE
		WHEN a.restricted_at IS NOT NULL THEN '` + withheldByHold + `'
		WHEN (` + shielded + `) THEN '` + withheldByStatute + `'`
	if underRequest {
		clause += `
		WHEN (` + underAnOpenRequest + `) THEN '` + withheldByRequest + `'`
	}
	return clause + `
		ELSE '' END`
}
