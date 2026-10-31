// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type recoverableMailBody struct {
	ID      ids.UUID
	Version int64
	Body    string
	Payload []byte
}

// recoverableMailBodyScope is captured mail not yet known to be whole whose
// stored original survives. Whether its body is short is decided per row, by
// the parser, from that original.
const recoverableMailBodyScope = `FROM activity a ` + mailOriginalJoin + `
 WHERE a.kind = 'email' AND a.body_completeness IN ('unassessed', 'unreadable', 'partial')
   AND a.restricted_at IS NULL AND a.archived_at IS NULL AND a.body IS NOT NULL
   AND rc.payload IS NOT NULL AND ` + capturedByMailConnector

func restoreMailBodiesBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	return pageStoredMail(ctx, tx, in,
		"a.id, a.version, a.body, rc.payload",
		recoverableMailBodyScope,
		func(c recoverableMailBody) ids.UUID { return c.ID },
		restoreMailBody)
}

// restoreMailBody writes the body the original yields, when the stored one is
// the short shape an older parser or cap left. The conversation it is on was
// read from the old text, so it is made due again.
func restoreMailBody(ctx context.Context, tx pgx.Tx, candidate recoverableMailBody) (bool, error) {
	raw, readable := decodedMailOriginal(candidate.Payload)
	if !readable {
		return false, nil
	}
	body, completeness, ok := mailmap.RecoverStoredBody(raw, candidate.Body)
	if !ok {
		return false, nil
	}
	restored, threadKey, err := activities.RestoreMailBodyTx(ctx, tx, activities.MailBodyRestoration{
		Activity: candidate.ID, Version: candidate.Version, StoredBody: candidate.Body,
		Body: body, Completeness: string(completeness),
	})
	if err != nil || !restored || threadKey == nil {
		return restored, err
	}
	if err := markConversationDueTx(ctx, tx, *threadKey); err != nil {
		return false, fmt.Errorf("mail body restore: %w", err)
	}
	return true, nil
}

func estimateRecoverableMailBodies(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	return countStoredMail(ctx, q, recoverableMailBodyScope, maximum)
}
