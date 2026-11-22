// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// CorrectProjectSourcesTx moves one page of projects whose source predates the fixed
// list onto its channel, and audits each. A row on the list is left out, so a
// replay changes nothing.
// Each project announces it as project.updated, as an edit of the field would.
func CorrectProjectSourcesTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int, runID ids.UUID) ([]ids.UUID, error) {
	corrected, err := storekit.CorrectRecordSources(ctx, tx, "project",
		`SELECT id, source, `+provenance.LegacyRecordSourceChannelSQL("source")+` FROM project
		  WHERE id > $1 AND NOT (`+provenance.ListedRecordSourceSQL("source")+`) ORDER BY id LIMIT $2 FOR UPDATE`,
		`UPDATE project r SET source = moved.source
		   FROM unnest($1::uuid[], $2::text[]) AS moved(id, source) WHERE r.id = moved.id`,
		after, limit, runID)
	if err != nil {
		return nil, err
	}
	for _, row := range corrected {
		if err := storekit.EmitEvent(ctx, tx, row.AuditID, row.ID, crmcontracts.PublicEventProjectUpdated{
			ChangedFields: map[string]any{storekit.RecordSourceField: row.Source},
		}); err != nil {
			return nil, fmt.Errorf("emit project.updated: %w", err)
		}
	}
	moved := make([]ids.UUID, len(corrected))
	for i, row := range corrected {
		moved[i] = row.ID
	}
	return moved, nil
}

// CountUnlistedProjectSources counts the projects CorrectProjectSourcesTx would still
// move, reading at most maximum+1.
func CountUnlistedProjectSources(ctx context.Context, q datatask.Querier, maximum int64) (int64, error) {
	var count int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM project WHERE NOT (`+provenance.ListedRecordSourceSQL("source")+`)
		 LIMIT $1) unlisted`, maximum+1).Scan(&count); err != nil {
		return 0, fmt.Errorf("projects: counting projects with an unlisted source: %w", err)
	}
	return count, nil
}
