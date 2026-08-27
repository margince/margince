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
		if spec.update != "" {
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
		// evidence, which is what source=human + captured_by=human:<id>
		// record. Confidence is 1: they are not guessing about themselves.
		if _, err := tx.Exec(ctx, `
			INSERT INTO organization_profile_field (organization_id, field, value, evidence_snippet, source_url, confidence, source, captured_by)
			VALUES ($1, $2, $3, '', '', 1, 'human', $4)
			ON CONFLICT (organization_id, field)
			DO UPDATE SET value = EXCLUDED.value, evidence_snippet = '', source_url = '',
			              confidence = 1, source = 'human',
			              captured_by = EXCLUDED.captured_by, captured_at = now()`,
			orgID, field, trimmed, by); err != nil {
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
func setCompanyColumn(ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID, spec companyField, value string) (bool, error) {
	var stored *string
	if value != "" {
		stored = &value
	}
	tag, err := tx.Exec(ctx, spec.update, orgID, stored)
	if err != nil {
		return false, fmt.Errorf("set %s: %w", spec.name, err)
	}
	return tag.RowsAffected() > 0, nil
}
