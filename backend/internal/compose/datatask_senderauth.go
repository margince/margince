// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

var senderAuthOriginalScope = `FROM activity a JOIN raw_capture rc ON ` + storedMailOriginal + `
 WHERE a.kind = 'email' AND a.direction = 'inbound' AND a.bulk_mail_attested
 AND NOT a.inbound_sender_authenticated AND a.archived_at IS NULL AND a.restricted_at IS NULL
 AND a.counterparty_email IS NOT NULL
 AND split_part(a.captured_by, ':', 1) = 'connector' AND split_part(a.captured_by, ':', 2) = 'gmail'`

type senderAuthOriginal struct {
	ID      ids.UUID
	Sender  string
	Payload []byte
}

func estimateSenderAuthOriginals(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	return countStoredMail(ctx, q, senderAuthOriginalScope, maximum)
}

func restoreSenderAuthBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	return pageStoredMail(ctx, tx, in, "a.id, a.counterparty_email, rc.payload", senderAuthOriginalScope,
		func(row senderAuthOriginal) ids.UUID { return row.ID }, restoreSenderAuth)
}

func restoreSenderAuth(ctx context.Context, tx pgx.Tx, row senderAuthOriginal) (bool, error) {
	raw, readable := decodedMailOriginal(row.Payload)
	if !readable {
		return false, nil
	}
	parsed, err := mailmap.Parse(raw, "")
	if err != nil {
		return false, nil //nolint:nilerr // An unreadable stored original is not evidence to alter a sender.
	}
	if !strings.EqualFold(parsed.Counterparty(), row.Sender) {
		return false, nil
	}
	record := parsed.ToRecord("gmail", raw)
	if !capture.SenderAuthenticated(record, row.Sender) {
		return false, nil
	}
	return activities.StampSenderAuthenticationTx(ctx, tx, row.ID)
}
