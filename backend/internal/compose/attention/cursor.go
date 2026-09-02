// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"crypto/sha256"
	"encoding/hex"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A worklist cursor is a position in the RANKING, not a keyset over a table.
//
// Every other paginated read in this product walks rows a database already
// ordered, so its cursor carries the last row's sort key and the next page is a
// WHERE clause. This queue has no such column: it assembles from a dozen lanes
// and orders them by the nine-step comparator in ranksteps.go, which reads
// fields no table holds — whether a wait is crowded, what a deal is worth in
// base currency, which band a row landed in.
//
// So the resume is: assemble the same day again, rank it the same way, and skip
// what the caller already has. That is only correct because two properties
// hold, and both are load-bearing:
//
//   - The order is DETERMINISTIC. less() ends at `a.item.Id < b.item.Id`, so
//     one read's sequence is the next read's sequence rather than whatever the
//     lanes happened to return. Two rows sharing an id do tie there, and the
//     sort is stable, so their relative order is the producers' — which is
//     itself deterministic for one assembled day. Their (source, id) anchors
//     would be indistinguishable, and resume() would take the first; the
//     position half of the token is what keeps that from stalling a walk.
//   - The candidate set does not depend on `limit`. Crowding is marked over the
//     whole narrowed set (worklist.go), and the page is cut afterwards, so
//     asking for rows 26-50 weighs exactly the candidates that asking for 1-25
//     weighed.
//
// WHY THE TOKEN CARRIES BOTH A POSITION AND AN IDENTITY. The day is live: a
// deal closes, a task is reprioritised, a message arrives. Between two pages a
// row can move ACROSS the anchor in either direction, and each direction breaks
// one of the two obvious cursors on its own.
//
// An identity alone ("resume after this row") fails when the anchor moves down
// the ranking: a row the caller already received now sorts after it and is
// handed out twice, and if the anchor slides to the very end the remainder is
// empty while real work is still owed. A position alone ("resume at offset N")
// fails when a row moves up past the anchor: everything shifts by one and the
// row at the old offset is skipped, never shown to anybody.
//
// So the resume takes whichever of the two is FURTHER along. Skipping forward
// can only repeat work at worst; falling back would drop it, and a row nobody
// ever sees is the failure this queue must not have. The result is not a
// snapshot and does not pretend to be — see resume() for what each half costs.
type worklistCursor struct {
	// Source and Row name the last row of the previous page. Together they are
	// the row's identity: Id alone is the owning RECORD's id, which two sources
	// can both point at — a deal that is both quiet and has a customer waiting.
	Source string `json:"s"`
	Row    string `json:"r"`
	// Served is how many rows the caller has already been handed, which is the
	// anchor's own position plus one. It is the floor the resume cannot fall
	// below, so a row that moves up past the anchor cannot push an unserved row
	// into an already-served slot.
	Served int `json:"n"`
	// Params fingerprints the request this token was minted under. A cursor
	// carried onto a different scope, filter or owner would silently answer a
	// question the caller did not ask: page two of "my tasks" continuing into
	// "the team's deals", with nothing in the response saying so.
	Params string `json:"p"`
}

// fingerprint renders the request parameters that decide WHICH rows exist.
//
// `limit` is deliberately absent: it decides how many rows a page carries, not
// which candidates were weighed, so changing it mid-walk is a legitimate thing
// for a caller to do and refusing it would be a lie about what changed.
//
// The three values join on NUL, which none of them can contain: `scope` and
// `filter` are closed contract enums the handler validates before this is
// reached, and `owner` renders through ids.UUID. So no two distinct requests
// can join to one string — the ambiguity a delimiter scheme exists to prevent —
// and this needs no length prefixing.
//
// Eight bytes, and it is not a security boundary. A collision would let one
// token resume a different question, but every row that question then returns
// has already passed the caller's own scope and row-visibility gates: the page
// is re-assembled from scratch under the principal, and the token chooses only
// where to resume within what they may already see. What this defends is a
// caller's coherence, not their permissions.
//
// Every input is NORMALISED first, because two spellings of one question must
// fingerprint alike or a walk breaks on a difference the caller cannot see.
// Both cases are real and both are things the contract itself calls the same
// question: an omitted `filter` and `filter=all` weigh identical candidates,
// and naming yourself as `owner` is the question the default already answers.
// A generated client that sends its documented default on page two would
// otherwise be refused for agreeing with the server.
func fingerprint(scope, filter string, owner ids.UUID) string {
	sum := sha256.Sum256([]byte(
		scope + "\x00" + normalizedFilter(filter) + "\x00" + owner.String()))
	return hex.EncodeToString(sum[:8])
}

// normalizedFilter collapses the two spellings of "everything" onto one.
//
// worklistFrom narrows on exactly this test (`filter != "" && filter != all`),
// so the two spellings are one candidate set there and must be one here.
func normalizedFilter(filter string) string {
	if filter == string(crmcontracts.WorklistFilterAll) {
		return ""
	}
	return filter
}

// encodeCursor mints the token for the row a page stopped on.
//
// It goes through storekit's codec rather than spelling base64-of-JSON again:
// one wire format with two encoders drifts the first time either changes, and a
// token this product mints must be readable by the one decoder that refuses it.
//
// It answers a bare string, and the reason is worth stating because the codec
// beside it does not. storekit.EncodeOpaque can fail, but only where
// json.Marshal can, and its own note says what that means in practice: a
// keyset carrying a time.Time outside year 0..9999. worklistCursor carries
// three strings and no instant, so the failure has no reachable cause here.
//
// The alternative was threading an error nothing can raise up through
// worklistFrom and its thirty-odd callers, where every one of them would need a
// branch for a case that never runs — a branch that is never exercised is never
// known to be right, and the only honest thing to write in it (a page claiming
// the backlog ended early) is worse than the case it guards against.
//
// Held by: TestACursorRoundTripsEveryRowShapeTheQueueCanCarry, which mints one
// from every source the queue can raise and reads each back.
func encodeCursor(row ranked, served int, scope, filter string, owner ids.UUID) string {
	token, err := storekit.EncodeOpaque(worklistCursor{
		Source: string(row.item.Source),
		Row:    row.item.Id,
		Served: served,
		Params: fingerprint(scope, filter, owner),
	})
	if err != nil {
		// Unreachable for three strings. An empty token is refused by
		// decodeCursor on the way back in, so the walk ends with a 422 the
		// caller can see rather than a page silently claiming to be the last.
		return ""
	}
	return token
}

// decodeCursor reads a token back, refusing anything this package did not mint
// for this exact question.
//
// An empty token is the start of the walk rather than a fault, which is what
// lets one code path serve both the first page and the rest.
func decodeCursor(token, scope, filter string, owner ids.UUID) (worklistCursor, error) {
	if token == "" {
		return worklistCursor{}, nil
	}
	cursor, err := storekit.DecodeOpaque[worklistCursor](token)
	if err != nil {
		return worklistCursor{}, err
	}
	// Well-formed JSON is not yet a position: `{}` decodes cleanly and would
	// name no row, which the resume below reads as "drop nothing" — a token
	// nobody minted silently restarting the walk.
	if cursor.Source == "" || cursor.Row == "" {
		return worklistCursor{}, &storekit.MalformedCursorError{}
	}
	if cursor.Params != fingerprint(scope, filter, owner) {
		return worklistCursor{}, &storekit.CursorSortMismatchError{}
	}
	return cursor, nil
}

// resume drops the rows the caller has already been given.
//
// It reads the token's two halves as two claims about one boundary and takes
// the EARLIER of them. Each half is wrong on its own, and in opposite
// directions, so neither can be trusted alone:
//
//   - The IDENTITY ("resume after this row") is right while the anchor keeps
//     its place. If the anchor moves DOWN the ranking it re-serves everything
//     between its old and new place, and an anchor that slides to the end — or
//     is answered between pages — leaves nothing behind it at all, which reads
//     as a finished walk while real work is still owed.
//   - The POSITION ("resume at offset N") is right while the set keeps its
//     shape, and it survives an anchor that moved or vanished. But a row that
//     moves UP past the anchor shifts everything below it by one, and the row
//     that lands on the old offset is stepped over.
//
// Taking the earlier of the two makes the unacceptable failure unreachable. The
// two errors are not equal and must not be traded off as though they were: a
// repeated row is one the caller has already seen and can dismiss, while a
// skipped row is work shown to nobody, with no failing signal anywhere. This
// queue exists to make sure work is not forgotten, so it errs toward showing a
// row twice and never toward showing it never.
//
// The cost is real and worth stating: a day that churns heavily between pages
// can hand back rows the caller already has. That is why the contract promises
// termination and no LOSS rather than a stable snapshot — a snapshot is not
// available over a set that is re-assembled and re-ranked on every read.
func resume(rows []ranked, cursor worklistCursor) []ranked {
	if cursor.Row == "" && cursor.Served == 0 {
		return rows
	}
	// The position, clamped: a token naming more rows than the day still holds
	// is a walk whose tail has been dealt with, not a fault.
	from := min(cursor.Served, len(rows))
	// The identity, when the anchor is still here. `i+1` because the anchor is
	// the last row already served, not the first one owed.
	for i, row := range rows {
		if string(row.item.Source) == cursor.Source && row.item.Id == cursor.Row {
			from = min(from, i+1)
			break
		}
	}
	return rows[from:]
}
