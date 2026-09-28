// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Putting the author of an imported record on the wire.
//
// The write half is sourceauthor.go; this is what a reader sees. It is spelled
// once per module rather than once per product because a module never imports a
// sibling and the helper returns a contract type, which `internal/shared` may
// not name — so `activities`, `deals`, `projects` and this package each carry
// their own copy of these fifteen lines. The alternative was a new
// platform→contracts edge for a struct literal.

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// sourceAuthorSeatNameSQL resolves the author's CURRENT name from the member
// directory, for a record that names a seat.
//
// NO liveness filter, deliberately, and it is the whole reason this is a
// subselect rather than a join the caller writes: a colleague who has since
// left still wrote what they wrote, and an inner join would drop the byline of
// every departed author — which is most of them on an import. The same reason
// readEmailParties gives for refusing the filter one table over.
//
// ALIASED, and the alias is load-bearing. An unaliased subselect takes its
// output name from the column inside it, so this arrives as `display_name` —
// which the company list already draws as its own first column. `ORDER BY
// "display_name"` then matches two output columns and Postgres refuses the
// whole query with SQLSTATE 42702. That is not hypothetical: it took the
// accounts list down until this alias was added, and it broke that list alone
// because contact and lead spell their name column something else.
func sourceAuthorSeatNameSQL(table string) string {
	return `(SELECT u.display_name FROM app_user u WHERE u.id = ` +
		table + `.source_author_id) AS source_author_seat_name`
}

// sourceAuthorOf builds the record's `author` from the column pair and the seat
// name the read resolved, or answers nil when the row has no author.
//
// PRECEDENCE: the seat's CURRENT name wins over the name the source carried. An
// author who works here may have married, corrected a spelling, or been entered
// into the old system wrong, and the directory is the thing that knows. The
// source's spelling is the fallback for somebody who never held a seat — and it
// is also what survives a hard-deleted seat, because the FK nulls the id and
// leaves the name standing.
//
// A row with an id that resolves to nothing keeps the id: the reader still
// learns that an identified member wrote it, and rendering it unattributed
// would be a stronger claim than the data supports.
func sourceAuthorOf(id *ids.UUID, seatName, sourceName, sourceSystem *string) *crmcontracts.SourceAuthor {
	name := ""
	switch {
	case seatName != nil && *seatName != "":
		name = *seatName
	case sourceName != nil && *sourceName != "":
		name = *sourceName
	}
	if id == nil && name == "" {
		return nil
	}
	return &crmcontracts.SourceAuthor{
		UserId:      uuidPtr(id),
		DisplayName: name,
		Via:         provenance.DisplayVia(sourceSystem),
	}
}
