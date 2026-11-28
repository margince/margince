// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ChatReconfirmationCurrentTx holds source state while checking a quoted reconfirmation.
func ChatReconfirmationCurrentTx(ctx context.Context, tx pgx.Tx, id ids.UUID, quote string) (bool, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return false, err
	}
	if err := auth.EnsureActivityContentVisibleLive(ctx, tx, id); err != nil {
		return false, err
	}
	var current bool
	var subject, body, fingerprint string
	err := tx.QueryRow(ctx, `SELECT coalesce(a.raw->>'source_deleted','false')<>'true'
 AND coalesce(a.raw->>'source_changed','false')<>'true'
 AND coalesce(a.raw->'reconfirmation'->>'quote','')=@quote
 ,coalesce(a.subject,''),coalesce(a.body,''),coalesce(a.raw->>'generated_fingerprint','')
 FROM activity a WHERE a.id=@id AND `+chatSummarySourceSQL()+` FOR SHARE OF a`, pgx.StrictNamedArgs{"id": id, "quote": quote}).Scan(&current, &subject, &body, &fingerprint)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, apperrors.ErrNotFound
	}
	return current && fingerprint == ChatSummaryFingerprint(subject, body), err
}
