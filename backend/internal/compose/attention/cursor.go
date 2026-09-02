// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"crypto/sha256"
	"encoding/hex"

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
// So the resume is: assemble the same day again, rank it the same way, and drop
// everything up to and through the row this token names. That is only correct
// because two properties hold, and both are load-bearing:
//
//   - The order is TOTAL. less() ends at `a.item.Id < b.item.Id`, so no two
//     rows compare equal and the sequence does not depend on which order the
//     lanes happened to return.
//   - The candidate set does not depend on `limit`. Crowding is marked over the
//     whole narrowed set (worklist.go), and the page is cut afterwards, so
//     asking for rows 26-50 weighs exactly the candidates that asking for 1-25
//     weighed.
//
// The day itself can move between pages — a rep answers a message, a deal
// closes. That is honest rather than a fault: this is a queue of live work, and
// a page that showed rows already dealt with would be worse than one that
// skipped them. What the cursor guarantees is that a row is not silently
// REPEATED, and that the walk terminates.
type worklistCursor struct {
	// Source and Row name the last row of the previous page. Together they are
	// the row's identity: Id alone is the owning RECORD's id, which two sources
	// can both point at — a deal that is both quiet and has a customer waiting.
	Source string `json:"s"`
	Row    string `json:"r"`
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
func fingerprint(scope, filter string, owner ids.UUID) string {
	sum := sha256.Sum256([]byte(scope + "\x00" + filter + "\x00" + owner.String()))
	return hex.EncodeToString(sum[:8])
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
func encodeCursor(row ranked, scope, filter string, owner ids.UUID) string {
	token, err := storekit.EncodeOpaque(worklistCursor{
		Source: string(row.item.Source),
		Row:    row.item.Id,
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
// The named row is dropped WITH them: a cursor says where a page stopped, so
// the next one starts after it.
//
// A row the token names may be GONE by the time the next page is asked for —
// answered, reassigned, or folded into a group. The walk then ends, and that is
// a deliberate choice between two imperfect answers. Restarting from the top
// would be the other, and it is worse in a way that does not announce itself: a
// client that pages until `has_more` is false would be handed page one again
// and page forward again, forever, each pass looking like ordinary progress.
// Ending the walk loses the tail of one read; looping loses the client.
//
// The row is looked up by (source, id) rather than by rank position. Positions
// move whenever the day moves, so an index would resume at whatever row had
// slid into that slot — silently skipping work rather than repeating it, which
// is the direction this queue must never fail in.
func resume(rows []ranked, cursor worklistCursor) []ranked {
	if cursor.Row == "" {
		return rows
	}
	for i, row := range rows {
		if string(row.item.Source) == cursor.Source && row.item.Id == cursor.Row {
			return rows[i+1:]
		}
	}
	return nil
}
