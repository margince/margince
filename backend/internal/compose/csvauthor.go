// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Who wrote a spreadsheet row in the system the file was exported from.
//
// The cell is kept as written, as the source's own spelling of the author.
// When it is the email of a seat here, the seat is named too, so the byline
// follows that colleague's current name — and the spelling still stands if the
// seat is ever deleted. Display names are never matched: two colleagues may
// share one, and a wrong author is worse than an unresolved one.

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

const csvTargetAuthor = "author"

// isCreateOnlyTarget reports a target the import writes when it CREATES a
// record and never on a re-import's update. The author is who wrote the record
// where it came from. A lost reason is why the deal closed. An activity's type,
// direction and links are what it was and about whom. A later file rewrites
// none of them.
func isCreateOnlyTarget(field string) bool {
	switch field {
	case csvTargetAuthor, dealTargetLostReason, activityTargetType, activityTargetDirection, activityTargetLinks:
		return true
	}
	return false
}

// isCreateOnly is isCreateOnlyTarget for one object. An activity's owner_email
// names its author, which the activities store cannot rewrite.
func isCreateOnly(object, field string) bool {
	return isCreateOnlyTarget(field) || (object == migration.ObjectActivity && field == csvTargetOwner)
}

// refuseAuthorFromNonImporter keeps the author column behind the same door as
// the create wires: only a declared importer states who wrote a record. The
// import writes through the stores, not the wire mappers, so it asks itself.
// It asks at staging and again at commit, and an agent's tools reach both.
// Every part of a ZIP is asked, since each file carries its own mapping. An
// activity's owner column is its author, so it is behind the same door.
func refuseAuthorFromNonImporter(ctx context.Context, mapping migration.RunMapping) error {
	if auth.DeclaredImporter(ctx) {
		return nil
	}
	for _, part := range mapping.PartList() {
		for _, target := range part.Fields {
			if target == csvTargetAuthor || (part.Object == migration.ObjectActivity && target == csvTargetOwner) {
				return &provenance.AuthorError{
					Field: "mapping", Code: "reserved_source_author",
					Message: "the author column, and an activity's owner column, are imported only by a signed-in person holding import_run:create; unmap it",
				}
			}
		}
	}
	return nil
}

// authorFrom reads the row's author cell. The seat is resolved at write time,
// inside the row's transaction, by authorSeats.
func authorFrom(fields map[string]string) storekit.SourceAuthorInput {
	return storekit.SourceAuthorInput{AuthorName: importString(fields, csvTargetAuthor)}
}

// authorUnwritableReason refuses an author cell the create would refuse, so the
// dry run promises what the commit does.
func authorUnwritableReason(fields map[string]string) migration.Reason {
	name := importString(fields, csvTargetAuthor)
	if name == nil || utf8.RuneCountInString(*name) <= provenance.SourceAuthorNameMax {
		return migration.Reason{}
	}
	return migration.WhyCount(migration.ReasonAuthorTooLong, "max", provenance.SourceAuthorNameMax)
}

// authorSeats caches one answer per author cell for the run, so a file naming
// one colleague on every row asks the database once. Bounded by the file.
type authorSeats map[string]*ids.UUID

// resolve names the seat whose email the author cell is, when there is one.
// No liveness filter: a colleague who has left is still who wrote the row.
func (s authorSeats) resolve(ctx context.Context, tx pgx.Tx, author *storekit.SourceAuthorInput) error {
	if author.AuthorName == nil || !strings.Contains(*author.AuthorName, "@") {
		return nil
	}
	key := strings.ToLower(*author.AuthorName)
	seat, known := s[key]
	if !known {
		seats, err := seatsByEmail(ctx, tx, []string{key})
		if err != nil {
			return fmt.Errorf("import: resolving the author %q to a seat: %w", key, err)
		}
		// No seat holds that address: the spelling stands alone.
		if id, found := seats[key]; found {
			seat = &id
		}
		s[key] = seat
	}
	author.AuthorID = seat
	return nil
}

func (w *csvWriters) resolveSourceAuthor(ctx context.Context, author *storekit.SourceAuthorInput) error {
	return database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		return w.authors.resolve(ctx, tx, author)
	})
}
