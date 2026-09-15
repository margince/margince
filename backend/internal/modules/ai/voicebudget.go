// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// BudgetDeferredVoiceBuilds shares the status/recovery predicate for saved builds.
const BudgetDeferredVoiceBuilds = "status = 'deferred' AND status_code = 'budget_deferred' AND next_attempt_at IS NOT NULL AND archived_at IS NULL"

// ResumeBudgetBuilds advances only deferred builds whose unique job can run.
func (s *VoiceStore) ResumeBudgetBuilds(ctx context.Context, enqueue func(context.Context, pgx.Tx, VoiceBuild) (bool, error)) error {
	if err := auth.Require(ctx, "ai_budget", principal.ActionUpdate); err != nil {
		return err
	}
	return storekit.RecoverPages(ctx, s.db.Tx, func(tx pgx.Tx, after ids.UUID) ([]VoiceBuild, error) {
		args := []any{after}
		query := fmt.Sprintf(`SELECT `+voiceBuildColumns+` FROM voice_build WHERE `+BudgetDeferredVoiceBuilds+` AND next_attempt_at>now() AND id>$%d ORDER BY id LIMIT 100 FOR UPDATE SKIP LOCKED`, len(args))
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		return pgx.CollectRows(rows, func(row pgx.CollectableRow) (VoiceBuild, error) { return scanVoiceBuild(row) })
	}, func(row VoiceBuild) ids.UUID { return row.ID }, func(tx pgx.Tx, row VoiceBuild) error {
		ok, err := enqueue(ctx, tx, row)
		if err != nil || !ok {
			return err
		}
		args := []any{row.ID}
		query := fmt.Sprintf(`UPDATE voice_build SET next_attempt_at=now(),version=version+1,updated_at=now() WHERE archived_at IS NULL AND id=$%d RETURNING `+voiceBuildColumns, len(args))
		updated, err := scanVoiceBuild(tx.QueryRow(ctx, query, args...))
		if err != nil {
			return err
		}
		auditID, err := storekit.Audit(ctx, tx, "update", "voice_build", row.ID, map[string]any{voiceNextAttemptField: row.NextAttemptAt}, map[string]any{voiceNextAttemptField: updated.NextAttemptAt})
		if err != nil {
			return err
		}
		return emitVoiceBuild(ctx, tx, auditID, updated, 0)
	})
}

const voiceNextAttemptField = "next_attempt_at"
