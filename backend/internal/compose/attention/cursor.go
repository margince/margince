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
// So the resume is: assemble the day again, rank it the same way, and start at
// the offset the token names. That works because the ordering is DETERMINISTIC
// — less() ends at `a.item.Id < b.item.Id`, so one read's sequence is the next
// read's sequence rather than whatever the lanes happened to return — and
// because the candidate set does not depend on `limit`: crowding is marked over
// the whole narrowed set before the cut, so rows 26-50 weigh exactly what rows
// 1-25 weighed.
//
// WHY AN OFFSET AND NOT THE LAST ROW'S IDENTITY, which is the obvious choice
// and was the first thing tried here. The day is live: between two pages a deal
// closes, a task is reprioritised, a message arrives. An identity anchor breaks
// on that in a way an offset does not — when the anchor is answered or sinks to
// the end of the ranking, "everything after this row" is empty, and the walk
// reports itself finished with work still owed. A queue whose whole purpose is
// that work is not forgotten cannot have that failure.
//
// Both were tried together, and the pair is worse than either alone rather than
// better. What a caller has seen is the top K of the PREVIOUS ranking, and after
// a re-rank that set can be scattered anywhere in the new order; a position plus
// one anchor cannot encode an arbitrary subset, so every rule for combining them
// trades one skipped row for a different skipped row. The offset has ONE
// failure, stated below and in the contract, and that is what makes it the
// honest choice.
type worklistCursor struct {
	// At is where in the ranking the next page starts: the number of rows this
	// walk has already covered.
	At int `json:"n"`
	// Params fingerprints the request this token was minted under. A cursor
	// carried onto a different scope, filter or owner would silently answer a
	// question the caller did not ask: page two of "my tasks" continuing into
	// "the team's deals", with nothing in the response saying so.
	Params string `json:"p"`
	// Snapshot names the frozen walk this token resumes, and is what turns the
	// offset above from a position in a REBUILT ranking into a position in one
	// the reader has already been shown.
	//
	// ZERO IS A LEGITIMATE TOKEN, not a fault: one minted before this field
	// existed, or by an installation that wires no snapshot store. It resumes
	// the old way — an offset into a freshly ranked day — which is the
	// behaviour the paragraphs above describe and the cost resumeAt states.
	// Refusing it would break every walk in flight at the moment this shipped,
	// to no reader's benefit.
	Snapshot ids.UUID `json:"s,omitempty"`
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

// encodeCursor mints the token for the position a page stopped at.
//
// It goes through storekit's codec rather than spelling base64-of-JSON again:
// one wire format with two encoders drifts the first time either changes, and a
// token this product mints must be readable by the one decoder that refuses it.
//
// It answers a bare string, and the reason is worth stating because the codec
// beside it does not. storekit.EncodeOpaque can fail, but only where
// json.Marshal can, and its own note says what that means in practice: a keyset
// carrying a time.Time outside year 0..9999. worklistCursor carries an int and
// a string, so the failure has no reachable cause here.
//
// The alternative was threading an error nothing can raise up through
// worklistFrom and its thirty-odd callers, where every one of them would need a
// branch for a case that never runs — a branch that is never exercised is never
// known to be right, and the only honest thing to write in it (a page claiming
// the backlog ended early) is worse than the case it guards against.
//
// Held by: TestACursorRoundTripsThePositionItWasMintedAt.
func encodeCursor(at int, scope, filter string, owner, snapshot ids.UUID) string {
	token, err := storekit.EncodeOpaque(worklistCursor{
		At:       at,
		Params:   fingerprint(scope, filter, owner),
		Snapshot: snapshot,
	})
	if err != nil {
		// Unreachable for an int and a string. An empty token is refused by
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
	// Well-formed JSON is not yet a position, and the token is client-supplied
	// and unsigned, so every shape this never mints is refused rather than
	// interpreted:
	//
	//   - A NEGATIVE offset reaches `rows[n:]` and panics. A crafted token must
	//     be a 422, never a crash on an authenticated endpoint.
	//   - ZERO is the top of the list, which this only ever reaches by minting a
	//     token after a full page — so an offset of zero came from somewhere
	//     else. It grants nothing (an absent cursor answers the same first page)
	//     and is refused anyway, because a token the server cannot have written
	//     should not be read as one it did.
	//   - An EMPTY fingerprint is what `{}` and `null` decode to, and without
	//     this they would pass as a legitimate first page.
	if cursor.At <= 0 || cursor.Params == "" {
		return worklistCursor{}, &storekit.MalformedCursorError{}
	}
	if cursor.Params != fingerprint(scope, filter, owner) {
		return worklistCursor{}, &storekit.CursorSortMismatchError{}
	}
	return cursor, nil
}

// resumeAt answers where in the current ranking this page starts.
//
// A clamp, and deliberately nothing more: a token naming more rows than the day
// still holds is a walk whose tail has been dealt with, not a fault, so it ends
// quietly instead of reaching past the slice.
//
// WHAT AN OFFSET COSTS, stated here because it is the one thing this cursor
// gets wrong and the contract repeats it to callers. The ranking is rebuilt on
// every read. If a row crosses the page boundary between two reads — rising
// above it, or sinking below it — that row is served twice or not at all on
// this walk. It is not lost from the product: the next read of the queue ranks
// it afresh and shows it.
//
// The alternative was anchoring on the last row's identity, and it is worse
// here. When that anchor is answered between pages, or sinks to the end of the
// ranking, "everything after this row" is empty and the walk reports itself
// finished with work still owed — a queue whose purpose is that work is not
// forgotten cannot fail that way. Carrying both an offset and an anchor was
// tried and is worse than either: what a caller has seen is the top K of the
// PREVIOUS ranking, which after a re-rank can be scattered anywhere, and no
// pair of small values encodes an arbitrary subset. Every rule for combining
// them traded one skipped row for a different one.
func resumeAt(rows []ranked, cursor worklistCursor) int {
	return min(cursor.At, len(rows))
}
