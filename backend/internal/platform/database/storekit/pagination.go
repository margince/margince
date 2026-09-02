// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Page is a keyset-paginated result window.
type Page struct {
	NextCursor string
	HasMore    bool
}

// Cursor is the opaque keyset token: the last row's (created_at, id)
// under the default -created_at,id sort. Keyset, never offset (CAP-PAGE).
// A non-default sort (listquery.go) extends the tuple with the sort
// field, its direction, and the last row's key in Postgres text form
// (nil = the row sits in the NULL tail), so a token can only continue
// the ordering it was minted under.
type Cursor struct {
	CreatedAt time.Time `json:"t"`
	ID        ids.UUID  `json:"id"`
	SortField string    `json:"s,omitempty"`
	SortDesc  bool      `json:"d,omitempty"`
	SortKey   *string   `json:"v,omitempty"`
}

// EncodeCursor mints the token that continues a page after this row.
//
// It ANSWERS an error rather than returning an empty token, because the caller
// pairs what comes back with HasMore: true. A position that cannot be written
// down would otherwise reach the client as "there is another page, and here is
// nothing to fetch it with" — a page they can ask for and never receive, silent
// on the server and permanent for that list.
//
// The failure is reachable. time.Time refuses to marshal an instant outside
// years 0000-9999, and Postgres timestamptz reaches year 294276, so a row with
// an absurd-but-storable created_at produces exactly this. Every caller is a
// store method that already answers an error, so there is a channel for it.
func EncodeCursor(createdAt time.Time, id ids.UUID) (string, error) {
	return EncodeOpaque(Cursor{CreatedAt: createdAt, ID: id})
}

// SweepCursor is a position in a walk across SEVERAL streams: which stream the
// page stopped in, and where inside that stream it stopped.
//
// A walk with no ordering to interleave its streams by — a search across
// record types, whose rows carry no common rank — can still be resumed if the
// token says both. The stream alone would restart it; the inner position alone
// would not say what it indexes into.
//
// Both providers that sweep mint this ONE token, rather than a shape each:
// what a caller pages with must not depend on which system of record answered
// them, and two codecs for one wire value drift the first time either changes.
// The inner half stays opaque here — a keyset token on one side, an incumbent
// mirror's own cursor on the other — because this type carries a position, not
// a meaning.
type SweepCursor struct {
	Stream string `json:"s"`
	Inner  string `json:"c"`
}

// EncodeSweepCursor renders a resume position opaquely: a caller never builds
// or edits one.
//
// It answers an error rather than an empty token because the caller pairs the
// result with "there is more" — a silent empty cursor there would report a
// remainder with no way to reach it, which is the defect a resumable sweep
// exists to remove.
func EncodeSweepCursor(position SweepCursor) (string, error) { return EncodeOpaque(position) }

// DecodeSweepCursor reads a resume position back. An empty token is the start
// of the walk, not a fault.
//
// It answers MalformedCursorError for anything this package could not have
// minted. Whether the CALLER still walks the stream named is a different
// question with a different answer — a narrowed request, or a grant lost
// between pages, is not the caller mistyping a token — so it is left to the
// provider, which knows its own vocabulary.
func DecodeSweepCursor(token string) (SweepCursor, error) {
	if token == "" {
		return SweepCursor{}, nil
	}
	position, err := DecodeOpaque[SweepCursor](token)
	if err != nil {
		return SweepCursor{}, err
	}
	// Well-formed JSON is not yet a position: `{}` unmarshals cleanly and
	// leaves an unnamed stream, which would resume a sweep from nowhere.
	if position.Stream == "" {
		return SweepCursor{}, &MalformedCursorError{}
	}
	return position, nil
}

// MalformedCursorError is a client fault: the opaque keyset token is
// client-supplied input, so failing to decode it — or a decoded sort key
// that does not parse as the sort column's kind — maps to a 4xx at the
// transport (httperr), never a 500.
type MalformedCursorError struct{}

func (*MalformedCursorError) Error() string { return "store: malformed cursor" }

// CursorSortMismatchError is the other cursor client fault: the token
// decodes fine but was minted under a different sort (field or
// direction), so its keyset tuple cannot continue this list. Distinct
// from MalformedCursorError because the contract's Cursor parameter
// promises its own code (422 cursor_param_mismatch) for exactly this
// case — the caller drops the cursor or restores the original sort.
type CursorSortMismatchError struct{}

func (*CursorSortMismatchError) Error() string {
	return "store: cursor was minted under a different sort"
}

func DecodeCursor(token string) (Cursor, error) {
	c, err := DecodeOpaque[Cursor](token)
	if err != nil {
		return Cursor{}, err
	}
	// Well-formed JSON is not yet a position. `null` and `{}` both unmarshal
	// cleanly and leave the keyset at its zero value, which every caller reads
	// as the TOP of the list — so a token nobody minted would silently restart
	// the walk rather than being refused. Both halves of the tuple have to be
	// there: an absent id is the tell on one side, and a zero instant on the
	// other would order before every real row.
	if c.CreatedAt.IsZero() || c.ID == (ids.UUID{}) {
		return Cursor{}, &MalformedCursorError{}
	}
	return c, nil
}

// SQLf keeps store-side SQL assembly lines readable; arguments are
// always positional parameters or fixed identifiers, never user input.
func SQLf(format string, a ...any) string { return fmt.Sprintf(format, a...) }

// ClampLimit applies the contract's CAP-PAGE bounds (default 50, max 200).
func ClampLimit(limit *int) int {
	switch {
	case limit == nil:
		return 50
	case *limit < 1:
		return 1
	case *limit > 200:
		return 200
	default:
		return *limit
	}
}

// QuickFindClause renders the list-q predicate: the full-text match
// (websearch syntax, accent-folded) OR a trigram contains-match on the
// entity's name expression — the as-you-type quick-find ("Rech" finds
// "Rechnung GmbH", "Muller" finds "Müller") that token-based tsquery
// cannot serve. The contains-match folds apostrophes on both sides
// ("oreilly" finds "Tim O'Reilly"; f_unaccent maps the typographic ’
// to ' first, so every spelling collapses the same way). nameExpr must
// mirror the expression of the entity's *_name_trgm index so the LIKE
// stays indexed; the query text is a bind parameter (LIKE
// metacharacters at worst widen the caller's own match).
func QuickFindClause(pos int, nameExpr string) string {
	return fmt.Sprintf(`(search_tsv @@ websearch_to_tsquery('simple', f_unaccent($%[1]d))
	   OR f_fold_apostrophes(lower(%[2]s)) LIKE '%%' || f_fold_apostrophes(lower($%[1]d)) || '%%')`, pos, nameExpr)
}
