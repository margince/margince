// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// Retention for the AI-side stores: the voice signal corpus and the model-call
// payload log. Kept apart from the record retention ladder because the subject
// is different — these hold what a model was SHOWN and what it produced, not
// the CRM record it was shown about, and their windows are argued from a
// different place.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// aiRetentionStages is how many batched stages this file adds to a workspace
// pass — the embedding-kind ai_call sweep and the voice-signal sweep — each
// claiming its own retentionBatch. MaxPassDuration counts them alongside the
// policy ladder, so a third sweep here has to move this number with it.
const aiRetentionStages = 2

// evaluateVoiceSignalRetention erases the drafted text of over-age voice
// learning signals. The counters row survives, so the learning statistics
// still count it.
func (s *RetentionService) evaluateVoiceSignalRetention(ctx context.Context) error {
	var due []ids.UUID
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id FROM voice_learning_signal
			WHERE retention_until < now() AND content_erased_at IS NULL
			  AND generated_original IS NOT NULL
			LIMIT $1`, retentionBatch)
		if err != nil {
			return err
		}
		due, err = pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		return err
	})
	if err != nil {
		return fmt.Errorf("retention voice_learning_signal: select: %w", err)
	}
	for _, id := range due {
		if err := s.eraseVoiceSignalContent(ctx, id); err != nil {
			return fmt.Errorf("retention voice_learning_signal on %s: %w", id, err)
		}
	}
	return nil
}

func (s *RetentionService) eraseVoiceSignalContent(ctx context.Context, id ids.UUID) error {
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		// The content_erased_at predicate is the CAS: a rival sweep that
		// already erased this row matches zero rows, and nothing is audited
		// twice for one erasure.
		tag, err := tx.Exec(ctx, `
			UPDATE voice_learning_signal
			SET generated_original = NULL, content_erased_at = now(),
			    version = version + 1, updated_at = now()
			WHERE id = $1 AND content_erased_at IS NULL`, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		auditID, err := storekit.AuditWithEvidence(ctx, tx, actionErase, "voice_learning_signal", id, nil, nil, map[string]any{
			evidenceKeyRetentionAction: actionErase,
		})
		if err != nil {
			return err
		}
		if err := storekit.RedactAuditHistory(ctx, tx, auditID); err != nil {
			return err
		}
		return storekit.EmitEventForEntity(ctx, tx, auditID, "voice_learning_signal", id, retentionAppliedPayload(crmcontracts.RetentionAppliedErase, nil, nil))
	})
}

// evaluateCallRetention deletes the call traces past their fixed caps, a set
// per transaction, and records each pass once. They are engine telemetry with
// no subject, so no row gets its own audit.
//
// An answer trace with a stored payload always stays. The payload has its own
// retention rule, and deleting the trace would take the payload with it.
func (s *RetentionService) evaluateCallRetention(ctx context.Context) error {
	if s.pruneEmbedCalls == nil || s.pruneAnswerCalls == nil {
		s.log.Warn("retention: call traces are kept, because their prunes were not supplied")
		return nil
	}
	if err := s.pruneCalls(ctx, s.pruneEmbedCalls, embedCallPrune, embedCallRetention, false); err != nil {
		return err
	}
	return s.pruneCalls(ctx, s.pruneAnswerCalls, answerCallPrune, answerCallRetention, true)
}

// pruneCalls reads the retain-only posture in each batch's transaction. A
// switch to retain-only during a long pass then holds from the next batch.
func (s *RetentionService) pruneCalls(ctx context.Context, prune CallPrune, rows string, days int, keepWithPayload bool) error {
	cutoff := time.Now().AddDate(0, 0, -days)
	var total int64
	for {
		var removed int64
		err := s.db.Tx(ctx, func(tx pgx.Tx) error {
			retainOnly, err := retainOnlyTx(ctx, tx)
			if err != nil {
				return err
			}
			removed, err = prune(ctx, tx, cutoff, operationalPruneBatch, keepWithPayload || retainOnly)
			return err
		})
		if err != nil {
			return fmt.Errorf("retention %s: %w", rows, err)
		}
		total += removed
		if removed < operationalPruneBatch {
			break
		}
	}
	return s.recordPrune(ctx, rows, total, days)
}

// CallPrune removes up to limit call traces of its kinds from before cutoff.
// keepWithPayload leaves a trace that still has a stored payload.
type CallPrune func(ctx context.Context, tx pgx.Tx, cutoff time.Time, limit int, keepWithPayload bool) (int64, error)

// WithCallPrunes returns a service that ages out call traces: the embedding
// ones, then the completion and decision ones. The ai module owns the table
// and supplies both deletes.
func (s *RetentionService) WithCallPrunes(embedding, answers CallPrune) *RetentionService {
	out := *s
	out.pruneEmbedCalls, out.pruneAnswerCalls = embedding, answers
	return &out
}
