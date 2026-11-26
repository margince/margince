// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package companyclockrefold refolds the stored company clocks when the clock's
// rule changed, or when who works at a company changes without a write. Its
// data tasks run once after the upgrades that changed the rule. RefoldEnded
// runs whenever a departure day arrives.
package companyclockrefold

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// batch bounds one transaction, so no pass holds record locks long enough to
// stall the writes queued behind them.
const batch = 200

// DataTask refolds the company clocks stored before the clock read current
// employment like the account-reach walk.
func DataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1795101500_company_clock_reads_current_employment", Version: 2,
		Title:    systemtext.Literal("datatask.title.core.1795101500_company_clock_reads_current_employment"),
		Requires: []string{"core/1795101710"},
		Steps:    []datatask.Step{refoldStep()},
	}
}

// PersonalMailTask refolds the company clocks stored before personal mail
// stopped counting (relstrength.NotPersonalSQL).
func PersonalMailTask() datatask.Task {
	return datatask.Task{
		ID: "core/1795175006_company_clock_counts_no_personal_mail", Version: 1,
		Title:    systemtext.Literal("datatask.title.core.1795175006_company_clock_counts_no_personal_mail"),
		Requires: []string{"core/1795175006"},
		Steps:    []datatask.Step{refoldStep()},
	}
}

// refoldStep walks the companies by id and refolds each stored clock that no
// longer matches its folded value. A refolded company leaves the selection.
func refoldStep() datatask.Step {
	return datatask.Step{
		Name: "companies", Limit: batch, MinLimit: 10,
		Audit: datatask.AuditPolicy{Reason: "the last-activity triggers move the same derived clock without an audit"},
		Estimate: func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
			if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
				return 0, false, err
			}
			count, err := storekit.CountClockRecords(ctx, q, storekit.ClockCompanies, maximum)
			return min(count, maximum), count > maximum, err
		},
		Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
			if err := auth.RequireSystem(ctx); err != nil {
				return datatask.BatchOut{}, err
			}
			after, err := datatask.UUIDCursor(in.Cursor)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			inspected, selected, err := storekit.ClockPage(ctx, tx, storekit.ClockCompanies, storekit.ClockNotFolded, after, in.Limit)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			if err := Refold(ctx, tx, selected); err != nil {
				return datatask.BatchOut{}, err
			}
			return datatask.AdvanceUUID(in, inspected, len(selected))
		},
	}
}

// Refold moves the stored clock of each company, locking them in the one order
// every last-activity writer shares.
func Refold(ctx context.Context, tx pgx.Tx, companies []ids.UUID) error {
	targets := storekit.LastActivityTargets{Companies: companies}
	if err := storekit.LockLastActivityTargets(ctx, tx, targets); err != nil {
		return err
	}
	return contacts.RefoldClocksTx(ctx, tx, targets)
}

// RefoldEnded refolds the companies that have an employment whose departure
// day arrived in (from, to], a batch to a transaction, and answers how many.
// It moves no mark, so a crash repeats the window.
func RefoldEnded(ctx context.Context, db *database.DB, from, to time.Time) (int, error) {
	refolded := 0
	var after ids.UUID
	for {
		var companies []ids.UUID
		err := db.Tx(ctx, func(tx pgx.Tx) (err error) {
			companies, err = contacts.CompaniesWhoseEmploymentEndedTx(ctx, tx, from, to, after, batch)
			if err != nil || len(companies) == 0 {
				return err
			}
			return Refold(ctx, tx, companies)
		})
		if err != nil {
			return refolded, err
		}
		refolded += len(companies)
		if len(companies) < batch {
			return refolded, nil
		}
		after = companies[len(companies)-1]
	}
}
