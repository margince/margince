// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// The rows every mutation writes BESIDE the caller's own record.
//
// The write shape is domain row + audit entry + outbox event in one
// transaction, and only the first of those three carries anything the caller
// sent. That distinction is invisible from a SQLSTATE — a CHECK is a CHECK —
// and it decides who is at fault: a constraint on the record the request names
// is the caller's input to fix, while a constraint on one of these is ours,
// written by code the caller cannot see and cannot change.
//
// Named here because this package is what writes them (AuditEvent and Emit,
// whose statements are built from these constants), so the classifier reading
// them cannot drift from the writer that fills them.

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

const (
	// TableAudit holds one row per mutation: what happened and who did it.
	TableAudit = "audit_log"
	// TableOutbox holds the event a mutation publishes, taken from here by the
	// relay rather than sent from domain code.
	TableOutbox = "event_outbox"
)

// IsSideEffectTable reports whether a table is one this package writes on every
// mutation's behalf rather than one the caller named.
func IsSideEffectTable(table string) bool {
	return table == TableAudit || table == TableOutbox
}

// ViolatedTable names the table whose constraint refused a write.
//
// Postgres sends it alongside the constraint name for every integrity
// violation (23xxx), which is what makes this answerable without guessing at
// the constraint's spelling: `audit_log_action_check` happens to start with its
// table's name, and a table called `audit_log_export` would too.
//
// Absent for a failure that names no table — a serialization conflict, a
// connection that went away — so a caller must treat "no table" as "this is not
// a constraint on any one row" rather than as any particular table.
func ViolatedTable(err error) (table string, ok bool) {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.TableName == "" {
		return "", false
	}
	return pgErr.TableName, true
}
