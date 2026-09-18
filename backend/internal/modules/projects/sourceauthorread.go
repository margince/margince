// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

// Putting the author of an imported project on the wire.
//
// The write half is sourceauthor.go; this is what a reader sees. It is spelled
// once per module rather than once per product because a module never imports a
// sibling and the helper returns a contract type, which `internal/shared` may
// not name — so `activities`, `contacts`, `deals` and this package each carry
// their own copy. The alternative was a new platform→contracts edge for a
// struct literal.

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// sourceAuthorSeatNameSQL resolves the author's CURRENT name from the member
// directory, for a record that names a seat.
//
// NO liveness filter, deliberately: a colleague who has since left still wrote
// what they wrote, and an inner join would drop the byline of every departed
// author — which is most of them on an import.
//
// ALIASED, and the alias is load-bearing. An unaliased subselect takes its
// output name from the column inside it, so this would arrive as
// `display_name` — and any list whose own name column is spelled that way then
// has two output columns of one name, which makes `ORDER BY "display_name"` an
// error (SQLSTATE 42702). The accounts list is the one that has it today.
func sourceAuthorSeatNameSQL(table string) string {
	return `(SELECT u.display_name FROM app_user u WHERE u.id = ` +
		table + `.source_author_id) AS source_author_seat_name`
}

// sourceAuthorOf builds the record's `author` from the column pair and the seat
// name the read resolved, or answers nil when the row has no author.
//
// PRECEDENCE: the seat's CURRENT name wins over the name the source carried.
// The directory is the thing that knows a colleague's name today; the source's
// spelling is the fallback for somebody who never held a seat, and it is also
// what survives a hard-deleted seat, because the FK nulls the id and leaves the
// name standing.
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
		Via:         sourceSystem,
	}
}
