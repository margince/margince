// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestPendingHistoricalAuditCorrectionRevisitsReleasedHolds(t *testing.T) {
	ctx := principal.SystemActing(context.Background(), "privacy_test")
	tx := subjectColumnsTx(ctx, t)
	contact, activity := ids.NewV7(), ids.NewV7()
	mustExec(ctx, t, tx, `INSERT INTO contact(id,full_name,source,captured_by,legal_hold)
 VALUES (@id,'Held subject','manual','system:privacy_test',true)`, pgx.StrictNamedArgs{"id": contact})
	mustExec(ctx, t, tx, `INSERT INTO activity(id,kind,subject,occurred_at,source,captured_by)
 VALUES (@id,'note','Retained subject text',now(),'manual','system:privacy_test')`, pgx.StrictNamedArgs{"id": activity})
	mustExec(ctx, t, tx, `INSERT INTO activity_link(activity_id,entity_type,contact_id)
 VALUES (@activity,'contact',@contact)`, pgx.StrictNamedArgs{"activity": activity, "contact": contact})
	original := seedPersonalAudit(ctx, t, tx, "activity", activity)
	_, err := storekit.AuditWithEvidence(ctx, tx, "erase", "activity", activity, nil, nil,
		map[string]any{"cause": "contact_erasure"})
	if err != nil {
		t.Fatal(err)
	}
	later := seedPersonalAudit(ctx, t, tx, "activity", activity)
	page, err := RedactHistoricalAuditPageTx(ctx, tx, original, ids.NewV7(), 100, nil)
	if err != nil {
		t.Fatal(err)
	}
	if page.Changed != 0 || len(page.Scanned) == 0 {
		t.Fatalf("held certificate page=%+v", page)
	}
	mustExec(ctx, t, tx, `UPDATE contact SET legal_hold=false WHERE id=@id`, pgx.StrictNamedArgs{"id": contact})
	page, err = RedactHistoricalAuditPageTx(ctx, tx, original, ids.NewV7(), 100, nil)
	if err != nil {
		t.Fatal(err)
	}
	if page.Changed != 1 {
		t.Fatalf("released certificate changed=%d, want 1", page.Changed)
	}
	var cleared bool
	if err := tx.QueryRow(ctx, `SELECT before IS NULL AND after IS NULL FROM audit_log WHERE id=@id`,
		pgx.StrictNamedArgs{"id": original}).Scan(&cleared); err != nil {
		t.Fatal(err)
	}
	if !cleared {
		t.Fatal("released hold left personal audit values behind")
	}
	var preserved bool
	if err := tx.QueryRow(ctx, `SELECT after IS NOT NULL FROM audit_log WHERE id=@id`,
		pgx.StrictNamedArgs{"id": later}).Scan(&preserved); err != nil {
		t.Fatal(err)
	}
	if !preserved {
		t.Fatal("historical cleanup cleared work written after the erasure")
	}
	page, err = RedactHistoricalAuditPageTx(ctx, tx, original, ids.NewV7(), 100, nil)
	if err != nil {
		t.Fatal(err)
	}
	if page.Changed != 0 {
		t.Fatalf("completed certificate was selected again: %+v", page)
	}
}
