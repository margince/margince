// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extingress

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MemberGuard is the in-transaction half of the ingest authority. The member
// must still be live, hold a seat that may write, and still hold the credential
// consented reports. A missing member is a refusal, never a pass. The row is
// locked for share, so a removal waits for the capture or the capture sees it.
func MemberGuard(unit string, consented func(context.Context, pgx.Tx, string, ids.UUID) (bool, error)) capture.MemberGuard {
	return func(ctx context.Context, tx pgx.Tx, member ids.UUID) error {
		var writes bool
		err := tx.QueryRow(ctx, `SELECT seat_type = 'full' FROM app_user u WHERE u.id = $1 AND `+identity.LiveMemberSQL("u")+` FOR SHARE`, member).Scan(&writes)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !writes) {
			return fmt.Errorf("%w: that member is no longer live", apperrors.ErrPermissionDenied)
		}
		if err != nil {
			return err
		}
		ok, err := consented(ctx, tx, unit, member)
		if err == nil && !ok {
			err = fmt.Errorf("%w: that member has withdrawn the credential this unit acts on", apperrors.ErrPermissionDenied)
		}
		return err
	}
}
