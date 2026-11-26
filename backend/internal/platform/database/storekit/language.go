// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// rowQuerier is what a transaction or a pool gives a one-row read.
type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// LanguageRegistered reports whether the language table holds a code. The table
// is the one source. A code stays in it after its unit is gone, and a code
// another process wrote is seen without a restart.
func LanguageRegistered(ctx context.Context, q rowQuerier, code string) (bool, error) {
	var held bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM language WHERE code = $1)`, code).Scan(&held)
	return held, err
}

// CheckLanguage refuses a code the language table does not hold, as a field
// error. A writer asks it in its own transaction before storing a language, so
// the client sees a 422 on the field. A delete never asks.
func CheckLanguage(ctx context.Context, q rowQuerier, field, code string) error {
	held, err := LanguageRegistered(ctx, q, code)
	if err != nil || held {
		return err
	}
	return &values.ParseError{Field: field, Code: "invalid_language",
		Message: field + " is a language this installation offers, for example de"}
}
