// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const participantEvidenceIDsParam = "ids"

// FoldParticipantEvidenceTx advances over every inspected primary key, including current rows.
func FoldParticipantEvidenceTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]ids.UUID, int, error) {
	rows, err := tx.Query(ctx, `SELECT id FROM activity_participant
 WHERE id>@after ORDER BY id LIMIT @limit`, pgx.StrictNamedArgs{argLimit: limit, "after": after})
	if err != nil {
		return nil, 0, fmt.Errorf("activities: reading participant evidence: %w", err)
	}
	return storeParticipantEvidenceRows(ctx, tx, rows)
}

// RepairParticipantEvidenceTx repairs incomplete snapshots after the cursor pass finishes.
func RepairParticipantEvidenceTx(ctx context.Context, tx pgx.Tx, limit int) (int, error) {
	rows, err := tx.Query(ctx, `SELECT id FROM activity_participant ap WHERE `+storekit.ParticipantEvidenceMissingSQL("ap")+` ORDER BY id LIMIT @limit`, storekit.ParticipantEvidenceArgs(int64(limit)))
	if err != nil {
		return 0, fmt.Errorf("activities: reading incomplete participant evidence: %w", err)
	}
	_, changed, err := storeParticipantEvidenceRows(ctx, tx, rows)
	return changed, err
}

// CountParticipantEvidence never reads more than maximum+1 incomplete snapshots.
func CountParticipantEvidence(ctx context.Context, q datatask.Querier, maximum int64) (int64, error) {
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT id FROM activity_participant ap WHERE `+storekit.ParticipantEvidenceMissingSQL("ap")+` ORDER BY id LIMIT @limit) pending`, storekit.ParticipantEvidenceArgs(maximum+1)).Scan(&count)
	return count, err
}

func storeParticipantEvidenceRows(ctx context.Context, tx pgx.Tx, rows pgx.Rows) ([]ids.UUID, int, error) {
	keys, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, 0, fmt.Errorf("activities: decoding participant evidence: %w", err)
	}
	if len(keys) == 0 {
		return keys, 0, nil
	}
	if err := lockParticipantEvidenceParents(ctx, tx, keys); err != nil {
		return nil, 0, err
	}
	// Match the parent cascade's lock order before the trigger takes its snapshot.
	tag, err := tx.Exec(ctx, `UPDATE activity_participant ap SET domain_evidence_parent_version=NULL,
 domain_evidence_at=NULL,domain_evidence_precision=NULL,domain_evidence_visible=NULL
 WHERE id=ANY(@ids) AND `+storekit.ParticipantEvidenceMissingSQL("ap"), pgx.StrictNamedArgs{participantEvidenceIDsParam: keys})
	if err != nil {
		return nil, 0, fmt.Errorf("activities: storing participant evidence: %w", err)
	}
	return keys, int(tag.RowsAffected()), nil
}

func lockParticipantEvidenceParents(ctx context.Context, tx pgx.Tx, keys []ids.UUID) error {
	rows, err := tx.Query(ctx, `SELECT id FROM activity WHERE id IN (
 SELECT activity_id FROM activity_participant WHERE id=ANY(@ids)) ORDER BY id FOR KEY SHARE`, pgx.StrictNamedArgs{participantEvidenceIDsParam: keys})
	if err != nil {
		return fmt.Errorf("activities: locking participant evidence parents: %w", err)
	}
	rows.Close()
	return rows.Err()
}
