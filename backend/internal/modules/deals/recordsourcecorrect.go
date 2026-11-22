// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

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

// CorrectDealSourcesTx moves one page of deals whose source predates the fixed
// list onto its channel, and audits each. A row on the list is left out, so a
// replay changes nothing.
// Each deal announces it as deal.updated, as an edit of the field would.
func CorrectDealSourcesTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int, runID ids.UUID) ([]ids.UUID, error) {
	corrected, err := storekit.CorrectRecordSources(ctx, tx, "deal",
		`SELECT id, source, `+provenance.LegacyRecordSourceChannelSQL("source")+` FROM deal
		  WHERE id > $1 AND NOT (`+provenance.ListedRecordSourceSQL("source")+`) ORDER BY id LIMIT $2 FOR UPDATE`,
		`UPDATE deal r SET source = moved.source
		   FROM unnest($1::uuid[], $2::text[]) AS moved(id, source) WHERE r.id = moved.id`,
		after, limit, runID)
	if err != nil {
		return nil, err
	}
	for _, row := range corrected {
		if err := storekit.EmitEvent(ctx, tx, row.AuditID, row.ID, crmcontracts.PublicEventDealUpdated{
			ChangedFields: map[string]any{storekit.RecordSourceField: row.Source},
		}); err != nil {
			return nil, fmt.Errorf("emit deal.updated: %w", err)
		}
	}
	moved := make([]ids.UUID, len(corrected))
	for i, row := range corrected {
		moved[i] = row.ID
	}
	return moved, nil
}

// CountUnlistedDealSources counts the deals CorrectDealSourcesTx would still
// move, reading at most maximum+1.
func CountUnlistedDealSources(ctx context.Context, q datatask.Querier, maximum int64) (int64, error) {
	var count int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM deal WHERE NOT (`+provenance.ListedRecordSourceSQL("source")+`)
		 LIMIT $1) unlisted`, maximum+1).Scan(&count); err != nil {
		return 0, fmt.Errorf("deals: counting deals with an unlisted source: %w", err)
	}
	return count, nil
}

// CorrectOfferSourcesTx moves one page of offers whose source predates the fixed
// list onto its channel, and audits each. A row on the list is left out, so a
// replay changes nothing.
// No event follows: offer.updated says a draft was edited, and this edits none.
func CorrectOfferSourcesTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int, runID ids.UUID) ([]ids.UUID, error) {
	corrected, err := storekit.CorrectRecordSources(ctx, tx, "offer",
		`SELECT id, source, `+provenance.LegacyRecordSourceChannelSQL("source")+` FROM offer
		  WHERE id > $1 AND NOT (`+provenance.ListedRecordSourceSQL("source")+`) ORDER BY id LIMIT $2 FOR UPDATE`,
		`UPDATE offer r SET source = moved.source
		   FROM unnest($1::uuid[], $2::text[]) AS moved(id, source) WHERE r.id = moved.id`,
		after, limit, runID)
	if err != nil {
		return nil, err
	}
	moved := make([]ids.UUID, len(corrected))
	for i, row := range corrected {
		moved[i] = row.ID
	}
	return moved, nil
}

// CountUnlistedOfferSources counts the offers CorrectOfferSourcesTx would still
// move, reading at most maximum+1.
func CountUnlistedOfferSources(ctx context.Context, q datatask.Querier, maximum int64) (int64, error) {
	var count int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM offer WHERE NOT (`+provenance.ListedRecordSourceSQL("source")+`)
		 LIMIT $1) unlisted`, maximum+1).Scan(&count); err != nil {
		return 0, fmt.Errorf("deals: counting offers with an unlisted source: %w", err)
	}
	return count, nil
}
