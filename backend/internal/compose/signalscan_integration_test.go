// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// A mail written at 22:00 local and judged at 01:00 local, fifteen calendar
// days later, is fourteen days and three hours by the clock. The summary counts
// the calendar days the installation's people see.
func TestAGhostedSignalCountsCalendarDaysInTheInstallationZone(t *testing.T) {
	e := integration.Setup(t)
	e.WsExec(t, `UPDATE setting SET value = '"Asia/Ho_Chi_Minh"'::jsonb WHERE key = 'installation.timezone'`)
	zone, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		t.Fatalf("load zone: %v", err)
	}
	wrote := time.Date(2026, 9, 1, 22, 0, 0, 0, zone)
	now := time.Date(2026, 9, 16, 1, 0, 0, 0, zone)

	company := e.SeedCompany(t, "Zone Ghost Co", nil)
	e.WsExec(t, `UPDATE company SET lifecycle = 'opportunity' WHERE id = $1`, company)
	contact := employedAt(t, e, company, "Zoe Ghost")
	logMail(t, e, contact, "inbound", wrote.AddDate(0, 0, -5))
	logMail(t, e, contact, "outbound", wrote)

	ctx := principal.SystemActing(e.Admin(), signalScanActor)
	var summary string
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		if _, err := WriteGhostedSignals(ctx, tx, now); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT summary FROM signal WHERE entity_id = $1 AND kind = $2`,
			company, kindGhostedThread).Scan(&summary)
	}); err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf(signalSummaryCopyFor(textlang.English).ghostedThread, 15); summary != want {
		t.Errorf("summary = %q, want %q", summary, want)
	}
}
