// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/modules/continuity"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// drillFlags are the values the two restore-drill verbs read.
type drillFlags struct {
	restoredTo string
	by         string
	note       string
	drill      string
	outcome    string
}

// drillActor is the principal the drill verbs act as. They run under the owner
// DSN with no session behind them, so the operator is named by --by instead.
const drillActor = "system:restore-drill"

// drillStart records that a restore drill began, and prints its id for
// drill-finish. The database clock takes the start time. So the recovery window
// is measured by the product, never typed in by whoever ran the drill.
func drillStart(ctx context.Context, dsn string, f drillFlags, stdout io.Writer) error {
	if strings.TrimSpace(f.by) == "" {
		return errors.New("migrate drill-start: --by is required: name who runs the drill")
	}
	if f.restoredTo == "" {
		return errors.New("migrate drill-start: --restored-to is required: the point in time the backup restores to, as RFC 3339")
	}
	restoredTo, err := time.Parse(time.RFC3339, f.restoredTo)
	if err != nil {
		return fmt.Errorf("migrate drill-start: --restored-to %q is not RFC 3339 (2026-10-09T03:00:00Z): %w", f.restoredTo, err)
	}
	if restoredTo.After(time.Now()) {
		return fmt.Errorf("migrate drill-start: --restored-to %s is in the future; a backup restores to a point that has passed", f.restoredTo)
	}
	var id ids.UUID
	err = withDrillLedger(ctx, dsn, func(ctx context.Context, ledger *continuity.Store) error {
		var beginErr error
		id, beginErr = ledger.Begin(ctx, restoredTo, f.by, f.note)
		return beginErr
	})
	if err != nil {
		return fmt.Errorf("migrate drill-start: %w", err)
	}
	if _, err := fmt.Fprintf(stdout, "drill %s started; finish it with: margince-migrate drill-finish --drill %s --outcome passed|failed\n", id, id); err != nil {
		return fmt.Errorf("migrate drill-start: writing the confirmation: %w", err)
	}
	return nil
}

// drillFinish closes a running drill as passed or failed.
func drillFinish(ctx context.Context, dsn string, f drillFlags, stdout io.Writer) error {
	id, err := ids.Parse(f.drill)
	if err != nil {
		return fmt.Errorf("migrate drill-finish: --drill %q is not the id drill-start printed: %w", f.drill, err)
	}
	err = withDrillLedger(ctx, dsn, func(ctx context.Context, ledger *continuity.Store) error {
		return ledger.Finish(ctx, id, f.outcome, f.note)
	})
	if err != nil {
		return fmt.Errorf("migrate drill-finish: drill %s (it must exist and still be running): %w", id, err)
	}
	if _, err := fmt.Fprintf(stdout, "drill %s closed as %s\n", id, f.outcome); err != nil {
		return fmt.Errorf("migrate drill-finish: writing the confirmation: %w", err)
	}
	return nil
}

// withDrillLedger opens the ledger over the installation's workspace.
func withDrillLedger(ctx context.Context, dsn string,
	fn func(ctx context.Context, ledger *continuity.Store) error,
) error {
	pool, err := database.NewPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()
	db := database.Bind(pool, identity.NewService(pool).InstallationWorkspace)
	return fn(principal.SystemActing(ctx, drillActor), continuity.NewStore(db))
}
