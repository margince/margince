// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// RemoveFxRate takes a currency off the sheet. Every live row of the pair is
// marked replaced and no successor is written.
//
// The sheet head and every conversion as of now stop seeing the currency. A read
// pinned to an earlier rate version still finds the rate its figure used. Update
// is the grant: a removal corrects the sheet, and no role holds delete.
func (s *Store) RemoveFxRate(ctx context.Context, fromCurrency string) error {
	if err := auth.Require(ctx, "fx_rate", principal.ActionUpdate); err != nil {
		return err
	}
	from := strings.ToUpper(strings.TrimSpace(fromCurrency))
	if !values.AssignableCurrency(from) {
		return fxInvalid("from_currency", "fx_rate_currency", "from_currency must be a 3-letter ISO code")
	}
	return s.Tx(ctx, func(tx pgx.Tx) error {
		return s.removeFxRate(ctx, tx, from)
	})
}

func (s *Store) removeFxRate(ctx context.Context, tx pgx.Tx, from string) error {
	if err := storekit.LockWriteIdentity(ctx, tx, "fx_rate", from); err != nil {
		return err
	}
	base, err := s.installation.BaseCurrency(ctx, tx)
	if err != nil {
		return fmt.Errorf("resolve base currency: %w", err)
	}
	var removedAt time.Time
	if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&removedAt); err != nil {
		return fmt.Errorf("read the removal instant: %w", err)
	}
	rows, err := tx.Query(ctx, `
		UPDATE fx_rate SET superseded_at = $3
		WHERE from_currency = $1 AND to_currency = $2 AND `+RateVersionSQL("", "")+`
		RETURNING id, rate::text, rate_date`, from, base, removedAt)
	if err != nil {
		return fmt.Errorf("remove fx_rate: %w", err)
	}
	type removedRate struct {
		id    ids.UUID
		image map[string]any
	}
	removed, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (removedRate, error) {
		var (
			r    removedRate
			rate string
			day  time.Time
		)
		err := row.Scan(&r.id, &rate, &day)
		r.image = fxRateImage(from, base, rate, day)
		return r, err
	})
	if err != nil {
		return fmt.Errorf("scan removed fx_rate: %w", err)
	}
	if len(removed) == 0 {
		return apperrors.ErrNotFound
	}
	for _, r := range removed {
		if _, err := storekit.Audit(ctx, tx, "delete", "fx_rate", r.id, r.image, nil); err != nil {
			return fmt.Errorf("audit fx_rate delete: %w", err)
		}
	}
	return nil
}
