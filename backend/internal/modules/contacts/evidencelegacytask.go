// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// legacyEvidenceTable is one table whose rows the data task moves, and the
// kind of subject a row of it belongs to, which its <kind>_id column names.
type legacyEvidenceTable struct {
	step        string
	table       string
	subjectKind string
}

// legacyEvidenceTables are walked in this order. A company's move takes both
// of its tables, so the second step finds only companies the first did not.
var legacyEvidenceTables = []legacyEvidenceTable{
	{step: "move_company_facts", table: "company_fact_legacy", subjectKind: entityCompany},
	{step: "move_company_profile_fields", table: "company_profile_field_legacy", subjectKind: entityCompany},
	{step: "move_contact_profile_fields", table: "contact_profile_field_legacy", subjectKind: entityContact},
}

// LegacyEvidenceDataTask moves the evidence rows written before field_provenance
// held them. Until it has passed a subject, the views read that subject's rows
// from the legacy tables, and the subject's next write moves them itself.
func LegacyEvidenceDataTask() datatask.Task {
	steps := make([]datatask.Step, 0, len(legacyEvidenceTables))
	for _, legacy := range legacyEvidenceTables {
		steps = append(steps, datatask.Step{
			Name: legacy.step, Limit: 200, MinLimit: 1,
			Audit: datatask.AuditPolicy{Reason: "A move keeps each row's id, value, version and times, " +
				"unaudited as in contacts.moveLegacyEvidence, which every evidence writer calls first."},
			EmptyWhenDone: true,
			Estimate: func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
				return countLegacyEvidence(ctx, q, legacy, maximum)
			},
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				return moveLegacyEvidenceBatch(ctx, tx, legacy, in)
			},
		})
	}
	return datatask.Task{
		ID: "core/1795672332_evidence_rows_move_to_field_provenance", Version: 1,
		Title:    systemtext.Literal("datatask.title.core.1795672332_evidence_rows_move_to_field_provenance"),
		Requires: []string{"core/1795672329", "core/1795672330", "core/1795672331"},
		Steps:    steps,
	}
}

// countLegacyEvidence counts the rows still waiting in one legacy table, up to
// maximum plus one.
func countLegacyEvidence(ctx context.Context, q datatask.Querier, legacy legacyEvidenceTable, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var left int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM `+pgx.Identifier{legacy.table}.Sanitize()+
		` LIMIT $1) waiting`, maximum+1).Scan(&left); err != nil {
		return 0, false, fmt.Errorf("contacts: counting the rows waiting in %s: %w", legacy.table, err)
	}
	return min(left, maximum), left > maximum, nil
}

// moveLegacyEvidenceBatch moves the subjects of the next page of one legacy
// table, all their rows at once. A replay finds the page already gone.
func moveLegacyEvidenceBatch(ctx context.Context, tx pgx.Tx, legacy legacyEvidenceTable, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	var page []ids.UUID
	if err := tx.QueryRow(ctx, `
		SELECT coalesce(array_agg(id ORDER BY id), '{}')
		  FROM (SELECT id FROM `+pgx.Identifier{legacy.table}.Sanitize()+` WHERE id > $1 ORDER BY id LIMIT $2) page`,
		after, in.Limit).Scan(&page); err != nil {
		return datatask.BatchOut{}, fmt.Errorf("contacts: reading the next rows of %s: %w", legacy.table, err)
	}
	if len(page) == 0 {
		return datatask.AdvanceUUID(in, page, 0)
	}
	if err := moveLegacySubjectsOf(ctx, tx, legacy.subjectKind, `SELECT `+pgx.Identifier{legacy.subjectKind + "_id"}.Sanitize()+
		` FROM `+pgx.Identifier{legacy.table}.Sanitize()+` WHERE id = ANY($1)`, page); err != nil {
		return datatask.BatchOut{}, err
	}
	return datatask.AdvanceUUID(in, page, len(page))
}
