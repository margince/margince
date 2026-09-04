// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Writing the fields a human states about the installation's own company —
// from the company form, and from the human-edited half of a site-read
// confirmation.
//
// A value lands on its provenance row always, and on an organization column
// when the field is one of the few that is column-backed; clearing a value
// deletes the provenance row rather than storing a blank one. The rows carry
// source=human and no evidence snippet, because on this path the human IS the
// evidence.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// writeCompanyFields applies the submitted fields: the column-backed ones onto
// their column (a human's own form overwrites — unlike a read-back, which only
// fills blanks), and every one onto its provenance row. Returns what changed,
// for the audit delta.
func writeCompanyFields(ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID, by string, fields map[string]*string) (map[string]any, error) {
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
			moved, err := setCompanyColumn(ctx, tx, orgID, spec, trimmed)
			if err != nil {
				return nil, err
			}
			renamed = renamed || (moved && field == fieldLegalName)
			// A description this form actually landed is a person's sentence,
			// and a later site read asks field_provenance whose it is before
			// replacing it. The profile-field row above says source=human too,
			// but that table answers for the FIELD; the column has its own
			// owner, and descriptionHeldByHuman reads this layer.
			if moved && field == fieldOfferSummary {
				if err := stampDescriptionAuthor(ctx, tx, orgID, by); err != nil {
					return nil, err
				}
			}
		}
		if trimmed == "" {
			if _, err := tx.Exec(ctx,
				`DELETE FROM organization_profile_field
				 WHERE organization_id = $1 AND field = $2`,
				orgID, field); err != nil {
				return nil, fmt.Errorf("clear company field %s: %w", field, err)
			}
			applied[field] = nil
			continue
		}
		// A human-typed value has no snippet to quote — the human IS the
		// evidence, which is what source=human + captured_by=human:<id> record.
		// The last argument is the precedence flag upsertOrgProfileField gates
		// on: a person's answer always lands, including over their own earlier
		// one, which is the half a read-back never gets.
		if _, err := tx.Exec(ctx, upsertOrgProfileField,
			orgID, field, trimmed, "", "", humanAuthoredConfidence, companySourceHuman, by, true); err != nil {
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
	// It is NOT the only writer that a person sets in motion: accepting a
	// coldstart read-back renames the same column through writeOrgColumn, and
	// re-checks there. Seven call sites of recheckOrgNameForDuplicates each
	// remembered the rule on their own, which is why the rule is now derived
	// from the tree instead of asserted here.
	//
	// Held by: TestEveryOrganizationRenameReachesTheDuplicateRecheck
	// (backend/gates/orgrenamerecheck_test.go)
	if renamed {
		if err := recheckOrgNameForDuplicates(ctx, tx, orgID, by); err != nil {
			return nil, err
		}
	}
	return applied, nil
}

// setCompanyColumn writes one column-backed field and reports whether the value
// actually moved — which is what keeps the duplicate re-check off a save that
// renamed nothing, and the description-author stamp off one that typed no
// description.
//
// "Moved" is each statement's own question rather than one rule. The three
// replacing arms answer it with IS DISTINCT FROM, so a resubmission of an
// unchanged form touches no row; offer_summary FILLS, so it answers false for
// every save after the first whether or not the text changed. Both are the
// right answer for their column, and neither is the other's.
//
// A value clears to NULL rather than to the empty string — an unfilled field
// reads as absent, never as the empty answer.
//
// The concurrency guard is the ANCHOR'S ROW LOCK, two frames up, not the
// RowsAffected report below. Both callers reach here through
// resolveOrCreateAnchor, whose anchorOrganization holds
// `WHERE is_anchor AND archived_at IS NULL FOR UPDATE` for the rest of the
// transaction, so two company saves serialize on the row instead of racing on
// it. Said here because the form carries no version to pin, so nothing else in
// this file records what stops the second save silently losing the first.
func setCompanyColumn(ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID, spec companyField, value string) (bool, error) {
	var stored *string
	if value != "" {
		stored = &value
	}
	write, ok := orgColumnWrites[spec.column]
	if !ok {
		// A field the form declares a column for that the shared table does not
		// write. Reported rather than sent: the lookup would otherwise hand
		// tx.Exec an empty statement and the failure would name syntax.
		return false, fmt.Errorf("people: the company form names column %q, which nothing writes", spec.column)
	}
	tag, err := tx.Exec(ctx, write.statementFor(spec.authority), orgID, stored)
	if err != nil {
		return false, fmt.Errorf("set %s: %w", spec.name, err)
	}
	return tag.RowsAffected() > 0, nil
}
