// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Refolding the contact and company last-activity clocks outside a write: when
// an activity's time comes, and where an older rule stored a future clock.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// RefoldClocksTx moves the clock of every contact and company a write filed
// under targets reaches, and the stored strength dates of its contacts. The caller has taken storekit.LockLastActivityTargets
// for the whole set.
func RefoldClocksTx(ctx context.Context, tx pgx.Tx, targets storekit.LastActivityTargets) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		SELECT move_last_activity(reach.kind::regclass, reach.id)
		  FROM last_activity_reach(coalesce($1::uuid[], '{}'), coalesce($2::uuid[], '{}'), coalesce($3::uuid[], '{}')) reach
		 WHERE reach.kind IN ('contact', 'company')
		 ORDER BY reach.kind, reach.id`,
		targets.Contacts, targets.Deals, targets.Companies); err != nil {
		return fmt.Errorf("contacts: refolding last-activity clocks: %w", err)
	}
	return RefreshStrengthDatesTx(ctx, tx, targets.Contacts)
}

// CompaniesWhoseEmploymentEndedTx answers the next limit companies after
// `after`, in id order, that have an employment which stopped being current
// in (from, to]. No row changes when a future ended_at day arrives, so no
// trigger moves the company's clock. The moment is the start of the end day in
// the installation zone, the day last_activity_of_company judges as today.
func CompaniesWhoseEmploymentEndedTx(ctx context.Context, tx pgx.Tx, from, to time.Time, after ids.UUID, limit int) ([]ids.UUID, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	ended := occurredday.DayStartSQL("(" + employment.EndDaySQL("r.ended_at", "r.ended_precision") + ")")
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT r.company_id FROM relationship r
		 WHERE r.kind = 'employment' AND r.archived_at IS NULL AND r.company_id > $3
		   AND `+ended+` > $1 AND `+ended+` <= $2
		 ORDER BY r.company_id LIMIT $4`, from, to, after, limit)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading companies whose employment ended: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}
