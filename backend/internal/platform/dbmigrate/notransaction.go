// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dbmigrate

// The unwrapped mode: how a migration asks not to be put in a transaction, and what
// applying it then means. Its own file because it is one concept with a cost of its
// own, and because the wrapped path beside it is the ordinary one a reader wants
// first.

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
)

// NoTransactionMarker asks the runner not to wrap a migration. It counts on a line of
// its own anywhere in the up-migration, so a file that merely names it in prose — as
// this package and the gate in backend/migrations both do — is still wrapped.
//
// It exists for CREATE INDEX CONCURRENTLY, which Postgres refuses inside a
// transaction block and which is the only way to add an index to a large table
// without blocking its writers for the whole build.
//
// THREE THINGS GO AWAY, and the gate in backend/migrations holds the file to the
// shape that survives them:
//
//   - Atomicity. A file that fails halfway leaves the schema between two versions.
//   - The bookkeeping row can no longer be written in the work's transaction, so a
//     crash between the DDL and the INSERT leaves the work done and unrecorded. The
//     next run applies the file again.
//   - A failed concurrent build leaves an INVALID index behind, which a retry must
//     drop before rebuilding — CREATE INDEX CONCURRENTLY IF NOT EXISTS sees the name
//     and skips, leaving the invalid one forever.
//
// All three are answered by the same property: the file has to be re-runnable from
// the top. A concurrent build pairs with a DROP INDEX CONCURRENTLY IF EXISTS above it
// for exactly that reason.
const NoTransactionMarker = "-- pgmigrate:no-transaction"

// Unwrapped reports whether this migration asked not to be wrapped.
//
// Read off the SQL every time rather than stored, so there is nothing for a caller to
// set differently from what the file says. A migration built in Go carries the marker
// in its UpSQL or it does not.
//
// What it costs is in NoTransactionMarker's own comment. The short version is that such
// a file must survive being run twice.
func (m Migration) Unwrapped() bool { return unwrapped(m.UpSQL) }

// unwrapped reports whether sql has the marker as an entire line, trimmed.
//
// Whole-line and nothing cleverer, so prose that NAMES the marker — this package and
// the gate both do — does not ask for it. What it cannot tell is a marker alone on a
// line inside a quoted body or a block comment; the shape gate keeps an unwrapped file
// to concurrent index statements, which have neither, and a file that had one would
// have to be wrapped to say it.
func unwrapped(sql string) bool {
	for line := range strings.SplitSeq(sql, "\n") {
		if strings.TrimSpace(line) == NoTransactionMarker {
			return true
		}
	}
	return false
}

// Namespace is one migration ownership domain with its own tracking table.
type Namespace struct {
	// Name keys the tracking table: schema_migrations_<name>.
	Name       string
	Migrations []Migration
}

// apply runs one migration's SQL and records it, wrapped or not as the file asked.
//
// The WRAPPED path is the ordinary one: the work and its bookkeeping row commit
// together, so the ledger can never disagree with the schema.
//
// The UNWRAPPED path cannot have that. Postgres refuses CREATE INDEX CONCURRENTLY in a
// transaction block, so the work runs on the connection and the bookkeeping follows as
// its own statement — and a crash in between leaves the work done and unrecorded. The
// next run applies the file again, which is why NoTransactionMarker's contract is that
// such a file survives being run twice. The bookkeeping is written SECOND for that
// reason: recorded-but-not-done would make the next run skip work the schema never got.
func apply(ctx context.Context, conn *pgx.Conn, m Migration, sql, bookkeeping string, args []any) error {
	if !m.Unwrapped() {
		return inTx(ctx, conn, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, sql); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, bookkeeping, args...)
			return err
		})
	}
	// ONE STATEMENT PER QUERY. A multi-statement query is itself an implicit
	// transaction, so sending the DROP and the CREATE together earns the same refusal
	// the wrapper did — "DROP INDEX CONCURRENTLY cannot run inside a transaction
	// block" — and that pair is exactly the shape the retry contract asks for.
	//
	// Splitting on the semicolon is safe because of the gate, not in general: an
	// unwrapped file may contain only concurrent index statements
	// (TestEveryUnwrappedMigrationOnlyBuildsIndexesConcurrently), and those carry no
	// semicolon inside a string or a body. A file that could would have to be wrapped.
	for statement := range strings.SplitSeq(sql, ";") {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		if _, err := conn.Exec(ctx, statement); err != nil {
			return err
		}
	}
	_, err := conn.Exec(ctx, bookkeeping, args...)
	return err
}
