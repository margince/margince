// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The agenda reads the same frozen deal scorecard as the rep's review.
// Missing historical coverage supplies no finding, rather than a healthy verdict.
func memberDealRecovery(ctx context.Context, tx pgx.Tx, userID ids.UUID, week time.Time) (string, error) {
	var d DealBlock
	err := tx.QueryRow(ctx, `SELECT coalesce(s.deal_forecast_down, 0), coalesce(s.deal_regressions, 0),
   coalesce(s.deal_open, 0), coalesce(s.deal_close_date_sound, 0), coalesce(s.deal_with_next_step, 0)
   FROM weekly_review r JOIN weekly_review_scorecard s ON s.weekly_review_id = r.id
   WHERE r.user_id = $1 AND r.local_week_start = $2 AND s.has_deal_block`, userID, week).
		Scan(&d.ForecastDown, &d.Regressions, &d.Open, &d.CloseDateSound, &d.WithNextStep)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("weekly: reading the member's frozen deal risks: %w", err)
	}
	return dealRecoveryFocus(d), nil
}

func dealRecoveryFocus(d DealBlock) string {
	switch {
	case d.ForecastDown > 0:
		return fmt.Sprintf("Forecast downgraded on %s — review the recovery plan", plural(d.ForecastDown, "deal"))
	case d.Regressions > 0:
		return fmt.Sprintf("%s moved backwards — review the recovery plan", plural(d.Regressions, "deal"))
	case d.Open > d.CloseDateSound:
		return fmt.Sprintf("%s without a sound close date — confirm the forecast", plural(d.Open-d.CloseDateSound, "deal"))
	case d.Open > d.WithNextStep:
		return fmt.Sprintf("%s without a next step — agree an owner and date", plural(d.Open-d.WithNextStep, "deal"))
	default:
		return ""
	}
}
