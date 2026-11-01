// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func mailRecipientsDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1792592656_mail_recipient_evidence", Version: 1,
		Title: "Restore captured mail recipients", Requires: []string{"core/1792501453"},
		Steps: []datatask.Step{{
			Name: "restore_recipient_headers", Limit: 50, MinLimit: 1,
			Audit:    datatask.AuditPolicy{Reason: "capture stamps the same parsed participant evidence without a separate audit"},
			Estimate: estimateMailRecipientOriginals, Batch: restoreMailRecipientsBatch,
		}},
	}
}

type mailRecipientOriginal struct {
	ID      ids.UUID
	Payload []byte
}

func restoreMailRecipientsBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	rows, err := tx.Query(ctx, `SELECT a.id, rc.payload `+mailRecipientOriginalScope+` AND a.id > @after ORDER BY a.id LIMIT @batch_limit FOR UPDATE OF a`, pgx.NamedArgs{dataTaskAfterArg: after, "batch_limit": in.Limit})
	if err != nil {
		return datatask.BatchOut{}, err
	}
	originals, err := pgx.CollectRows(rows, pgx.RowToStructByPos[mailRecipientOriginal])
	if err != nil {
		return datatask.BatchOut{}, err
	}
	scanned := make([]ids.UUID, 0, len(originals))
	evidence := make([]capture.MailRecipientEvidence, 0, len(originals))
	for _, original := range originals {
		scanned = append(scanned, original.ID)
		// Unreadable evidence must keep the correction pending rather than report success.
		raw, err := capture.DecodeStoredOriginal(original.Payload)
		if err != nil {
			return datatask.BatchOut{}, fmt.Errorf("stored mail recipient evidence is unreadable: %w", apperrors.ErrInvalidArgument)
		}
		parsed, err := mailmap.Parse(raw, "")
		if err != nil {
			return datatask.BatchOut{}, fmt.Errorf("stored mail recipient evidence is unreadable: %w", apperrors.ErrInvalidArgument)
		}
		fields, ok := parsed.ToRecord("gmail", nil).Fields.(capture.ActivityFields)
		if !ok {
			return datatask.BatchOut{}, apperrors.ErrInvalidArgument
		}
		evidence = append(evidence, capture.MailRecipientEvidence{Activity: ids.From[ids.ActivityKind](original.ID), Envelope: fields.Envelope})
	}
	changed, err := capture.RestoreMailRecipientsTx(ctx, tx, evidence)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	return datatask.AdvanceUUID(in, scanned, changed)
}

var mailRecipientOriginalScope = `FROM activity a JOIN raw_capture rc ON ` + storedMailOriginal + `
 WHERE a.kind = 'email' AND a.archived_at IS NULL AND ` + storedMailProvider

// storedMailOriginal matches raw_capture rc to the activity a it is the
// original of. storedMailProvider limits a to the mail connectors whose
// original is the message's own RFC 822 bytes.
var storedMailOriginal = storekit.StoredOriginalSQL("a", "rc")

const (
	storedMailProvider = `split_part(a.captured_by, ':', 1) = 'connector'
 AND split_part(a.captured_by, ':', 2) IN ('gmail', 'imap', 'graph')`
)

func estimateMailRecipientOriginals(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var count int64
	err := q.QueryRow(ctx, "SELECT count(*) FROM (SELECT a.id "+mailRecipientOriginalScope+" LIMIT @cap) candidates", pgx.NamedArgs{dataTaskCapArg: maximum + 1}).Scan(&count)
	return min(count, maximum), count > maximum, err
}
