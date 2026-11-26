// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Writing the fields a human states about the installation's own company —
// from the company form, and from the human-edited half of a site-read
// confirmation.
//
// A value lands on its provenance row always, and on a company column
// when the field is one of the few that is column-backed; clearing a value
// deletes the provenance row rather than storing a blank one. The rows carry
// source=human and no evidence snippet, because on this path the human IS the
// evidence.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// writeCompanyFields applies the submitted fields: the column-backed ones onto
// their column (a human's own form overwrites — unlike a read-back, which only
// fills blanks), and every one onto its provenance row. Returns what changed,
// for the audit delta.
func writeCompanyFields(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, by string, fields map[string]*string) (map[string]any, error) {
	applied := map[string]any{}
	renamed := false
	for _, spec := range companyFields {
		field := spec.name
		value, sent := fields[field]
		if !sent || value == nil {
			continue
		}
		trimmed := strings.TrimSpace(*value)
		if spec.column != "" {
			moved, err := setCompanyColumn(ctx, tx, companyID, spec, trimmed)
			if err != nil {
				return nil, err
			}
			renamed = renamed || (moved && field == fieldLegalName)

		}
		if trimmed == "" {
			if _, err := tx.Exec(ctx,
				`DELETE FROM company_profile_field
				 WHERE company_id = $1 AND field = $2`,
				companyID, field); err != nil {
				return nil, fmt.Errorf("clear company field %s: %w", field, err)
			}
			applied[field] = nil
			continue
		}
		// A typed value has no snippet to quote: its author is the evidence,
		// which is what source and captured_by record.
		// The last argument is the precedence flag upsertCompanyProfileField gates
		// on: a contact's answer always lands, including over their own earlier
		// one, which is the half a read-back never gets.
		if _, err := tx.Exec(ctx, upsertCompanyProfileField,
			companyID, field, trimmed, "", "", humanAuthoredConfidence, companyAuthorSource(ctx), by, true, nil); err != nil {
			return nil, fmt.Errorf("save company field %s: %w", field, err)
		}
		applied[field] = trimmed
	}
	// A legal name is the axis on which two records of one company converge, so
	// a rename has to ask whether it just created a duplicate.
	//
	// The re-check lives here rather than at either call site because BOTH of
	// this function's callers need it — the company form and a site-read
	// confirmation — and both took the name lock in resolveOrCreateAnchor ahead
	// of the row lock, so the ordering already holds.
	//
	// It is NOT the only writer that a contact sets in motion: accepting a
	// coldstart read-back renames the same column through writeCompanyColumn, and
	// re-checks there. Seven call sites of recheckCompanyNameForDuplicates each
	// remembered the rule on their own, which is why the rule is now derived
	// from the tree instead of asserted here.
	//
	// Held by: TestEveryCompanyRenameReachesTheDuplicateRecheck
	// (backend/gates/companyrenamerecheck_test.go)
	if renamed {
		if err := recheckCompanyNameForDuplicates(ctx, tx, companyID, by); err != nil {
			return nil, err
		}
	}
	return applied, nil
}

// The anchor row lock serializes form saves; unchanged values write nothing.
func setCompanyColumn(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, spec companyField, value string) (bool, error) {
	if spec.column == columnAddress {
		return setCompanyAddress(ctx, tx, companyID, spec, value)
	}
	stored, _ := companyColumnArgument(spec.column, value)
	write, ok := companyColumnWrites[spec.column]
	if !ok {
		// A field the form declares a column for that the shared table does not
		// write. Reported rather than sent: the lookup would otherwise hand
		// tx.Exec an empty statement and the failure would name syntax.
		return false, fmt.Errorf("contacts: the company form names column %q, which nothing writes", spec.column)
	}
	tag, err := tx.Exec(ctx, write.statementFor(spec.authority), companyID, stored)
	if err != nil {
		return false, fmt.Errorf("set %s: %w", spec.name, err)
	}
	return tag.RowsAffected() > 0, nil
}

// The form resends its line on every save. An unchanged line must preserve
// structured parts somebody may have edited directly on the company record.
func setCompanyAddress(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, spec companyField, value string) (bool, error) {
	var stated string
	err := tx.QueryRow(ctx,
		`SELECT value FROM company_profile_field WHERE company_id = $1 AND field = $2`,
		companyID, spec.name).Scan(&stated)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("read stated %s: %w", spec.name, err)
	}
	if err == nil && stated == value {
		return false, nil
	}
	return writeCompanyAddress(ctx, tx, companyID, value, spec.authority)
}
