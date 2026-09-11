// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// The SQLSTATEs the stores branch on, named once.
const (
	pgUniqueViolation     = "23505"
	pgForeignKeyViolation = "23503"
	pgCheckViolation      = "23514"
	pgExclusionViolation  = "23P01"
	pgQueryCanceled       = "57014"
	pgLockNotAvailable    = "55P03"
	pgProgramLimitExceed  = "54000"

	// The representation errors: a value the caller supplied is not of the type
	// the column it was compared against holds. Listed rather than matched on
	// the whole "22" class, which also carries arithmetic — a division by zero
	// is a server's sum, not a caller's spelling — and substring faults that
	// say nothing about the request.
	pgInvalidTextRepresentation = "22P02"
	pgNumericValueOutOfRange    = "22003"
	pgStringDataRightTruncation = "22001"
	pgInvalidDatetimeFormat     = "22007"
	pgDatetimeFieldOverflow     = "22008"
)

// pgViolation names the violated constraint when err is the given
// SQLSTATE class — the single spelling of "which constraint fired".
func pgViolation(err error, code string) (constraint string, ok bool) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == code {
		return pgErr.ConstraintName, true
	}
	return "", false
}

// IsLockTimeout detects a 55P03: a statement gave up waiting for a lock under
// the caller's own lock_timeout. It is not a failure of the write — the row is
// simply held by a transaction that has not committed — so a caller that set
// the bound is expected to answer it rather than surface it.
func IsLockTimeout(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgLockNotAvailable
}

// IsUniqueViolation detects the 23505 dedupe path (409 + existing id).
func IsUniqueViolation(err error) bool {
	_, ok := UniqueViolation(err)
	return ok
}

// UniqueViolation names the violated constraint of a 23505, so callers
// can tell an email/domain dedupe hit from an unrelated uniqueness rule
// (e.g. the one-primary-email index) instead of mislabeling both as
// duplicates.
func UniqueViolation(err error) (constraint string, ok bool) {
	return pgViolation(err, pgUniqueViolation)
}

func IsForeignKeyViolation(err error) bool {
	_, ok := ForeignKeyViolation(err)
	return ok
}

// ForeignKeyViolation names the violated constraint of a 23503.
func ForeignKeyViolation(err error) (constraint string, ok bool) {
	return pgViolation(err, pgForeignKeyViolation)
}

// ForeignKeyColumn answers WHICH column of a 23503 pointed nowhere, so a
// transport can name the caller's own field instead of making them diff every
// id they sent.
//
// It reads the column off the constraint name by removing the TABLE name
// Postgres reports alongside it — exactly, not by splitting on underscores.
// Both halves contain them, so `company_parent_company_id_fkey` splits as
// `company` + `parent_company_id` and no guess at the boundary gets that
// right. A hand-named constraint yields nothing rather than a wrong name.
func ForeignKeyColumn(err error) (column string, ok bool) {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != pgForeignKeyViolation {
		return "", false
	}
	trimmed, isDefaultName := strings.CutSuffix(pgErr.ConstraintName, "_fkey")
	if !isDefaultName || pgErr.TableName == "" {
		return "", false
	}
	column, isOnThisTable := strings.CutPrefix(trimmed, pgErr.TableName+"_")
	if !isOnThisTable || column == "" {
		return "", false
	}
	return column, true
}

// ExclusionViolation names a fired EXCLUDE constraint — the overlap
// guards (double-booking) map it to their domain conflict.
func ExclusionViolation(err error) (constraint string, ok bool) {
	return pgViolation(err, pgExclusionViolation)
}

// CheckViolation exposes a fired CHECK constraint's name so the transport
// can answer a typed 422 instead of an opaque 500 — the defense-in-depth
// net under the per-path validations: a CHECK is a business rule, and a
// business-rule breach is never a server fault.
func CheckViolation(err error) (constraint string, ok bool) {
	return pgViolation(err, pgCheckViolation)
}

// IsQueryCanceled detects the 57014 a statement raises when it stops before
// answering: a spent statement_timeout, an operator's pg_cancel_backend, or
// the client going away.
//
// It deliberately does NOT say which of the three, because the SQLSTATE does
// not. A caller that means to report a spent budget owes the second half of
// that judgement itself — a cancelled request is not a degraded one, and only
// the caller holds the context that tells them apart.
func IsQueryCanceled(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgQueryCanceled
}

// IsProgramLimitExceeded detects a 54000: a value the database accepted as
// input but cannot store or index at that size.
//
// The case that earned it is `activity.search_tsv`, a GENERATED column whose
// to_tsvector output has a hard 1,048,575-byte ceiling. Output runs about
// 1.10x input for word-dense text, so a body around 950 KB overflows it —
// comfortably inside the HTTP chassis's 1 MiB request cap, which means a
// legal-sized request produced an unexplained server fault.
//
// It is a CLASS rather than that one limit, and it is named that way on
// purpose: 54000 also covers a row too wide for an index and a statement with
// too many arguments. What every member has in common is the only thing a
// caller can act on — the value they sent is too large — and none of them is a
// server fault to retry.
func IsProgramLimitExceeded(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgProgramLimitExceed
}

// IsInvalidValueForType detects a value the database could not read as the type
// it was compared against: a malformed uuid, a number where a numeric column
// was expected, a date that is not one.
//
// It is the caller's spelling, not a server fault. The report engine binds a
// caller's own `filters` and derivation predicates straight onto typed columns,
// so `{"stage_id": "not-a-uuid"}` reached the transport as an opaque 500 whose
// advice was to retry — advice that can never work, since the same text is the
// same non-uuid forever.
func IsInvalidValueForType(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	switch pgErr.Code {
	case pgInvalidTextRepresentation, pgNumericValueOutOfRange,
		pgStringDataRightTruncation, pgInvalidDatetimeFormat, pgDatetimeFieldOverflow:
		return true
	default:
		return false
	}
}
