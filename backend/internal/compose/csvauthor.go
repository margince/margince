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
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

const csvTargetAuthor = "author"

// isCreateOnlyTarget reports a target the import writes when it CREATES a
// record and never on a re-import's update. The author is who wrote the record
// where it came from, fixed when it landed; a later file does not rewrite that.
func isCreateOnlyTarget(field string) bool {
	return field == csvTargetAuthor
}

// refuseAuthorFromNonImporter keeps the author column behind the same door as
// the create wires: only a declared importer states who wrote a record. The
// import writes through the stores, not the wire mappers, so it asks here — at
// staging and again at commit, which an agent's tools reach too.
func refuseAuthorFromNonImporter(ctx context.Context, fields map[string]string) error {
	for _, target := range fields {
		if target == csvTargetAuthor && !auth.DeclaredImporter(ctx) {
			return &provenance.AuthorError{
				Field: "mapping", Code: "reserved_source_author",
				Message: "the author column is imported only by a signed-in person holding import_run:create; unmap it",
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
func authorUnwritableReason(fields map[string]string) string {
	name := importString(fields, csvTargetAuthor)
	if name == nil || utf8.RuneCountInString(*name) <= provenance.SourceAuthorNameMax {
		return ""
	}
	return fmt.Sprintf("author is longer than %d characters", provenance.SourceAuthorNameMax)
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
		var id ids.UUID
		err := tx.QueryRow(ctx, `SELECT id FROM app_user WHERE lower(email) = $1`, key).Scan(&id)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			// No seat holds that address: the spelling stands alone.
		case err != nil:
			return fmt.Errorf("import: resolving the author %q to a seat: %w", key, err)
		default:
			seat = &id
		}
		s[key] = seat
	}
	author.AuthorID = seat
	return nil
}
