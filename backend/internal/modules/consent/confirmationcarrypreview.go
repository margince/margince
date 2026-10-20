// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

const contactCarrySource = "contact_id = @source"

func previewCarriedConfirmations(ctx context.Context, tx pgx.Tx, source ids.ContactID) ([]commsauthz.CarriedConfirmation, error) {
	rows, err := tx.Query(ctx, `SELECT id,delivered_to,issued_at,expires_at,opened_at,consumed_at FROM confirm_token WHERE `+contactCarrySource+` ORDER BY id LIMIT 501`, pgx.NamedArgs{"source": source})
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (commsauthz.CarriedConfirmation, error) {
		var out commsauthz.CarriedConfirmation
		err := row.Scan(&out.ID, &out.Address, &out.IssuedAt, &out.ExpiresAt, &out.OpenedAt, &out.ConsumedAt)
		return out, err
	})
	if err != nil {
		return nil, err
	}
	if len(out) > 500 {
		return nil, apperrors.ErrInvalidArgument
	}
	return out, nil
}

func previewCarriedDoubleOptIns(ctx context.Context, tx pgx.Tx, source ids.ContactID) ([]commsauthz.CarriedDoubleOptIn, error) {
	rows, err := tx.Query(ctx, `SELECT d.id,d.purpose_id,p.label,d.issued_at,d.expires_at,d.consumed_at
 FROM consent_doi_token d JOIN consent_purpose p ON p.id=d.purpose_id WHERE `+contactCarrySource+` ORDER BY d.id LIMIT 501`, pgx.NamedArgs{"source": source})
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (commsauthz.CarriedDoubleOptIn, error) {
		var out commsauthz.CarriedDoubleOptIn
		err := row.Scan(&out.ID, &out.PurposeID, &out.PurposeLabel, &out.IssuedAt, &out.ExpiresAt, &out.ConsumedAt)
		return out, err
	})
	if err != nil {
		return nil, err
	}
	if len(out) > 500 {
		return nil, apperrors.ErrInvalidArgument
	}
	return out, nil
}
