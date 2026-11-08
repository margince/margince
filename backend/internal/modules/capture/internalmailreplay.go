// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func (s *Sink) admitInternalMailCopyTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, rec connector.NormalizedRecord, fields ActivityFields, birth birthDecision, memberBound bool) (datasource.EntityRef, bool, error) {
	var internal bool
	err := tx.QueryRow(ctx, `SELECT true FROM activity
 WHERE id = $1 AND internal_mail_authorization IS NOT NULL
 AND archived_at IS NULL AND restricted_at IS NULL FOR UPDATE`, id).Scan(&internal)
	if errors.Is(err, pgx.ErrNoRows) {
		return datasource.EntityRef{}, false, nil
	}
	if err != nil {
		return datasource.EntityRef{}, false, err
	}
	// Possession is checked while the row is held, before recording a reader.
	same, err := replayClaimIsProvenTx(ctx, tx, id, fields, rec.Parts)
	if err != nil || !same {
		return datasource.EntityRef{}, false, err
	}
	delivered, err := seatDeliveredTx(ctx, tx, id, rec, memberBound)
	if err != nil || !delivered {
		return datasource.EntityRef{}, false, err
	}
	if err := s.recordThisImport(ctx, tx, id, rec, fields, birth, memberBound); err != nil {
		return datasource.EntityRef{}, false, err
	}
	if err := auth.EnsureActivityVisible(ctx, tx, id.UUID); err != nil {
		return datasource.EntityRef{}, false, err
	}
	if err := s.joinThread(ctx, tx, id, rec, false); err != nil {
		return datasource.EntityRef{}, false, err
	}
	return datasource.EntityRef{Type: datasource.EntityActivity, ID: id.UUID}, true, nil
}
