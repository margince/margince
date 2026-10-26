// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package customfields

// ActiveColumns is this module's half of the fieldcatalog cross-module
// seam (shared/ports/fieldcatalog): the catalog READ a record store
// (contacts, deals, …) needs to drive its cf_* columns, exposed through a
// port those stores can depend on without importing this module
// directly (ADR-0054 §3). Compose wires the concrete *Service in; the
// stores themselves see only fieldcatalog.Reader.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// var _ fieldcatalog.Reader = (*Service)(nil) documents the seam at its
// provider — the compile-time assertion itself lives in the compose
// integration suite (customfields_fieldcatalog_integration_test.go),
// which is where a cross-module wiring defect would otherwise first
// surface.

// ActiveColumns answers the active custom-field columns for one object,
// ordered by column_name (a stable, deterministic order for SELECT/INSERT
// column-list building).
//
// Deliberately runs no auth.Require: this is called from inside a record
// store's Get/List/Create/Update, whose own RBAC gate already ran before
// the store reaches for its custom columns. What ActiveColumns exposes —
// which cf_* columns exist and their type — is schema (the same shape the
// admin catalog list already answers to anyone holding custom_field:read),
// not row data a second gate would need to narrow; the row-level RBAC the
// calling store enforces is what actually protects the values stored in
// those columns.
func (s *Service) ActiveColumns(ctx context.Context, object string) ([]fieldcatalog.Column, error) {
	return s.queryColumns(ctx, "AND status = $2 ORDER BY column_name", object, statusActive)
}

// FilterableColumns answers every cf_* column that physically exists for one
// object — active and retired — ordered by column_name like its sibling.
//
// Retirement is a status change and never a DROP COLUMN (lifecycle.go), so a
// retired field's column and values are still there to be filtered on. Excluding
// it here would turn every saved segment naming it into a 422 at read time, which
// is a worse answer than a stale one — the same reasoning the retired
// `classification` field carries in the collections vocabulary.
//
// Runs no auth.Require, for the reason ActiveColumns states: which columns exist
// is schema, and the calling engine's own row-scope clause is what protects the
// values in them.
func (s *Service) FilterableColumns(ctx context.Context, object string) ([]fieldcatalog.Column, error) {
	return s.queryColumns(ctx, "ORDER BY column_name", object)
}

// ReportableColumnsIn answers the active columns of one object an administrator
// opted into reporting, with their labels, ordered by column_name. It reads in
// the caller's transaction, so a report's vocabulary and count share a snapshot.
//
// Runs no auth.Require: the opt-in is what published the label and options, and
// the reporting surface that asks gates the object itself.
func ReportableColumnsIn(ctx context.Context, tx pgx.Tx, object string) ([]fieldcatalog.ReportableColumn, error) {
	rows, err := tx.Query(ctx,
		`SELECT column_name, type, COALESCE(options, '[]'::jsonb), label
		   FROM custom_field
		  WHERE object = $1 AND status = $2 AND reportable
		  ORDER BY column_name`,
		object, statusActive)
	if err != nil {
		return nil, fmt.Errorf("customfields: reading reportable columns: %w", err)
	}
	defer rows.Close()
	var cols []fieldcatalog.ReportableColumn
	for rows.Next() {
		var (
			c          fieldcatalog.ReportableColumn
			optionsRaw []byte
		)
		if err := rows.Scan(&c.Name, &c.Type, &optionsRaw, &c.Label); err != nil {
			return nil, fmt.Errorf("customfields: reading reportable columns: %w", err)
		}
		if c.Options, err = unmarshalOptions(optionsRaw); err != nil {
			return nil, err
		}
		cols = append(cols, c)
	}
	return cols, rows.Err()
}

// queryColumns runs the custom_field scan both ActiveColumns and
// FilterableColumns need, varying only in the WHERE-clause fragment
// (appended after "WHERE object = $1") and its bind args ($2, $3, …
// following object). Kept private: the two questions it answers —
// active-only vs. active-and-retired — belong on the methods above, not
// on this shared plumbing.
func (s *Service) queryColumns(ctx context.Context, whereTail string, args ...any) ([]fieldcatalog.Column, error) {
	var cols []fieldcatalog.Column
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		cols, err = queryColumnsIn(ctx, tx, whereTail, args...)
		return err
	})
	return cols, err
}

// ActiveColumnsIn is ActiveColumns on a transaction the caller already holds,
// for a caller that cannot take a second connection from inside it. Same
// answer, same lack of a gate of its own.
func ActiveColumnsIn(ctx context.Context, tx pgx.Tx, object string) ([]fieldcatalog.Column, error) {
	return queryColumnsIn(ctx, tx, "AND status = $2 ORDER BY column_name", object, statusActive)
}

func queryColumnsIn(ctx context.Context, tx pgx.Tx, whereTail string, args ...any) ([]fieldcatalog.Column, error) {
	rows, err := tx.Query(ctx,
		// COALESCE because options is NULL for every non-picklist type, and an
		// empty jsonb array decodes to the empty set a caller means to read.
		fmt.Sprintf(
			`SELECT column_name, type, COALESCE(options, '[]'::jsonb), COALESCE(currency, '')
			   FROM custom_field WHERE object = $1 %s`, whereTail),
		args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []fieldcatalog.Column
	for rows.Next() {
		var (
			c          fieldcatalog.Column
			optionsRaw []byte
		)
		if err := rows.Scan(&c.Name, &c.Type, &optionsRaw, &c.Currency); err != nil {
			return nil, err
		}
		// Decoded through the module's own spelling, not scanned into []string.
		// A malformed column then fails naming the column and what it should
		// have held, where pgx's JSON codec names a driver type.
		c.Options, err = unmarshalOptions(optionsRaw)
		if err != nil {
			return nil, err
		}
		cols = append(cols, c)
	}
	return cols, rows.Err()
}
