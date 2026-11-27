// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package recordsources

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

type journalMeterTx struct {
	pgx.Tx
	transferred *int
}

func (t journalMeterTx) Unwrap() pgx.Tx { return t.Tx }
func (t journalMeterTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	rows, err := t.Tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	if strings.Contains(sql, "source_changes") {
		return journalMeterRows{Rows: rows, transferred: t.transferred}, nil
	}
	return rows, nil
}
func (t journalMeterTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	row := t.Tx.QueryRow(ctx, sql, args...)
	if strings.Contains(sql, "source_changes") {
		return journalMeterRow{Row: row, transferred: t.transferred}
	}
	return row
}

type journalMeterRows struct {
	pgx.Rows
	transferred *int
}

func (r journalMeterRows) Scan(dest ...any) error {
	if err := r.Rows.Scan(dest...); err != nil {
		return err
	}
	countJournalBytes(r.transferred, dest)
	return nil
}

type journalMeterRow struct {
	pgx.Row
	transferred *int
}

func (r journalMeterRow) Scan(dest ...any) error {
	if err := r.Row.Scan(dest...); err != nil {
		return err
	}
	countJournalBytes(r.transferred, dest)
	return nil
}
func countJournalBytes(total *int, dest []any) {
	for _, value := range dest {
		switch raw := value.(type) {
		case *[]byte:
			*total += len(*raw)
		case *string:
			*total += len(*raw)
		}
	}
}

// Unrelated edits must not copy the complete pending journal on each audit.
// The generous byte allowance depends on pending changes, not machine speed.
func TestSourceJournalTransfersEachPendingChangeWithoutRepeatingTheWholeDocument(t *testing.T) {
	o := seedOlder(t)
	const pending = 500
	const edits = 100
	integrationSeed := `INSERT INTO deal(id,name,status,pipeline_id,stage_id,source,captured_by)
 SELECT CASE WHEN n=1 THEN $1 ELSE public.uuidv7() END,'Pending journal '||n,'open',$2,$3,'seed','human:old'
 FROM generate_series(1,500) AS n`
	// The same historical rows the correction task upgrades, before it audits them.
	integration.SeedBeforeSourceCheck(t, o.owner, "deal", integrationSeed, o.pipeline, o.stage)
	company := o.e.SeedCompany(t, "Unrelated journal audit", nil)
	ctx := systemCtx(o.e)
	transferred := 0
	if err := database.WithTx(ctx, o.e.Pool, func(tx pgx.Tx) error {
		if err := auth.HoldWritableLive(ctx, tx, "company", company); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE deal SET source='api' WHERE name LIKE 'Pending journal %'`); err != nil {
			return err
		}
		meter := journalMeterTx{Tx: tx, transferred: &transferred}
		previous := "Unrelated journal audit"
		for i := 0; i < edits; i++ {
			name := fmt.Sprintf("Unrelated journal audit %d", i)
			if _, err := tx.Exec(ctx, `UPDATE company SET display_name=$2,version=version+1,updated_at=now() WHERE id=$1`, company, name); err != nil {
				return err
			}
			audit, err := storekit.Audit(ctx, meter, "update", "company", company, map[string]any{"display_name": previous}, map[string]any{"display_name": name})
			if err != nil {
				return err
			}
			if err := storekit.EmitEvent(ctx, meter, audit, company, crmcontracts.PublicEventCompanyUpdated{ChangedFields: map[string]any{"display_name": name}}); err != nil {
				return err
			}
			previous = name
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if transferred == 0 {
		t.Fatal("the meter saw no source journal; its read path changed")
	}
	if transferred > pending*2048 {
		t.Fatalf("%d unrelated audits transferred %d journal bytes for %d pending changes; the whole journal is being copied repeatedly", edits, transferred, pending)
	}
	t.Logf("%d pending changes across %d unrelated audits transferred %d journal bytes", pending, edits, transferred)
	if audits := o.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='deal' AND before->>'source'='seed' AND after->>'source'='api'`); audits != pending {
		t.Fatalf("the pending source writes lost their audits: %d", audits)
	}
}
