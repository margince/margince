// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// embedCallWithoutPayload narrows the prune to the traces whose deletion
// destroys nothing but telemetry. ai_call_payload cascades from ai_call, and
// a payload on an embedding call holds the texts that were embedded, which is
// record content. The retain-only posture keeps that content, so it keeps
// the call the payload hangs from.
const embedCallWithoutPayload = `
	AND NOT EXISTS (SELECT 1 FROM ai_call_payload p WHERE p.ai_call_id = ai_call.id)`

// PruneEmbedCallsTx removes up to limit embedding call traces from before
// cutoff, and answers how many. keepWithPayload leaves a trace that still has
// a stored payload. A trace holds routing, spend and identity facts and no
// subject, so no row gets an audit of its own.
func PruneEmbedCallsTx(ctx context.Context, tx pgx.Tx, cutoff time.Time, limit int, keepWithPayload bool) (int64, error) {
	due := `SELECT id FROM ai_call WHERE kind = '` + callKindEmbedding + `' AND occurred_at < $1`
	if keepWithPayload {
		due += embedCallWithoutPayload
	}
	tag, err := tx.Exec(ctx, `DELETE FROM ai_call WHERE id IN (`+due+` ORDER BY id LIMIT $2)`, cutoff, limit)
	if err != nil {
		return 0, fmt.Errorf("ai: removing old embedding call traces: %w", err)
	}
	return tag.RowsAffected(), nil
}
