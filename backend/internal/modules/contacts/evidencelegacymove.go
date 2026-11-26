// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The evidence of a company or contact lives in field_provenance. Rows written
// before that wait in the three _legacy tables until they move. The views read
// both, and a write reaches only field_provenance, so every writer moves its
// subject's legacy rows first. The data task moves the rest the same way.

// tableEvidence is where every evidence row of a company or contact is stored.
const tableEvidence = "field_provenance"

// uniqueEvidenceKey holds one evidence row per subject, field and value key.
const uniqueEvidenceKey = "uq_field_provenance_evidence"

// legacyEvidenceMoves are the statements that move the rows of one kind of
// subject, those the subjects query names, each keeping its id, version and
// times.
func legacyEvidenceMoves(subjectKind, subjects string) ([]string, bool) {
	switch subjectKind {
	case entityCompany:
		return []string{
			`WITH moved AS (DELETE FROM company_fact_legacy WHERE company_id IN (` + subjects + `) RETURNING *)
			 INSERT INTO field_provenance (id, object_type, object_id, field_name, value, value_key,
			        evidence_snippet, evidence_ref, confidence, source, captured_by, site_read_id, captured_at,
			        version, updated_at, retrieved_at, verified_at, verified_by)
			 SELECT id, 'company', company_id, field, value, value_key, evidence_snippet, source_url, confidence,
			        source, captured_by, site_read_id, captured_at, version, updated_at, retrieved_at, verified_at,
			        verified_by
			   FROM moved
			 ON CONFLICT DO NOTHING`,
			`WITH moved AS (DELETE FROM company_profile_field_legacy WHERE company_id IN (` + subjects + `) RETURNING *)
			 INSERT INTO field_provenance (id, object_type, object_id, field_name, value, evidence_snippet,
			        evidence_ref, confidence, source, captured_by, captured_at, version, updated_at, retrieved_at,
			        verified_at, verified_by, source_activity_id)
			 SELECT id, 'company', company_id, field, value, evidence_snippet, source_url, confidence, source,
			        captured_by, captured_at, version, updated_at, retrieved_at, verified_at, verified_by,
			        source_activity_id
			   FROM moved
			 ON CONFLICT DO NOTHING`,
		}, true
	case entityContact:
		return []string{
			`WITH moved AS (DELETE FROM contact_profile_field_legacy WHERE contact_id IN (` + subjects + `) RETURNING *)
			 INSERT INTO field_provenance (id, object_type, object_id, field_name, value, value_key,
			        evidence_snippet, evidence_ref, confidence, source, captured_by, captured_at, version,
			        updated_at, observed_at, superseded_value, superseded_captured_by, superseded_observed_at,
			        signature_support)
			 SELECT id, 'contact', contact_id, field, value, value_key, evidence_snippet, source_ref, confidence,
			        source, captured_by, created_at, version, updated_at, observed_at, superseded_value,
			        superseded_captured_by, superseded_observed_at, signature_support
			   FROM moved
			 ON CONFLICT DO NOTHING`,
		}, true
	}
	return nil, false
}

// moveLegacyEvidence moves the legacy evidence rows of the given subjects into
// field_provenance.
func moveLegacyEvidence(ctx context.Context, tx pgx.Tx, subjectKind string, subjects ...ids.UUID) error {
	if len(subjects) == 0 {
		return nil
	}
	return moveLegacySubjectsOf(ctx, tx, subjectKind, `SELECT unnest($1::uuid[])`, subjects)
}

// moveLegacySubjectsOf moves the subjects a query names, for a writer that
// reaches rows across many subjects at once. A move is not a change to a
// company's facts, so it leaves the fact revision counter where it was.
func moveLegacySubjectsOf(ctx context.Context, tx pgx.Tx, subjectKind, subjects string, args ...any) error {
	statements, ok := legacyEvidenceMoves(subjectKind, subjects)
	if !ok {
		return fmt.Errorf("contacts: no evidence of a %q to move", subjectKind)
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('margince.evidence_move', 'on', true)`); err != nil {
		return fmt.Errorf("contacts: marking an evidence move: %w", err)
	}
	for _, statement := range statements {
		if _, err := tx.Exec(ctx, statement, args...); err != nil {
			return fmt.Errorf("contacts: moving %s evidence to field_provenance: %w", subjectKind, err)
		}
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('margince.evidence_move', '', true)`); err != nil {
		return fmt.Errorf("contacts: ending an evidence move: %w", err)
	}
	return nil
}

// moveLegacyMeetingEvidence moves the companies whose legacy profile fields
// cite a meeting, up to limit of them, so a sweep over cited fields sees them.
func moveLegacyMeetingEvidence(ctx context.Context, tx pgx.Tx, limit int) error {
	return moveLegacySubjectsOf(ctx, tx, entityCompany, `SELECT DISTINCT company_id
		  FROM company_profile_field_legacy WHERE source = 'meeting' LIMIT $1`, limit)
}
