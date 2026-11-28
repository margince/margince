// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

func TestAttachmentOwnerUpgradeKeepsLegacyRowsWritableAndRefusesNewOrphans(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()
	core, err := migrations.CoreHistory()
	if err != nil {
		t.Fatal(err)
	}
	through := namespaceThroughVersion(t, core, "1795850103")
	before := through
	before.Migrations = before.Migrations[:len(before.Migrations)-4]
	if _, err := dbmigrate.Up(ctx, conn, before); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"contact", "company", "deal", "lead", "project", "activity", "offer"} {
		if _, err := conn.Exec(ctx, `INSERT INTO attachment(entity_type,entity_id,filename,storage_key,source,captured_by)
 VALUES($1,gen_random_uuid(),'older.txt','old/'||$1,'manual','human:test')`, kind); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if _, err := dbmigrate.Up(ctx, conn, through); err != nil {
			t.Fatalf("upgrade: %v", err)
		}
		var kept int
		if err := conn.QueryRow(ctx, `SELECT count(*) FROM attachment WHERE owner_unkeyed AND num_nonnulls(owner_contact_id,owner_company_id,owner_deal_id,owner_lead_id,owner_project_id,owner_activity_id)=0`).Scan(&kept); err != nil || kept != 7 {
			t.Fatalf("legacy rows=%d, %v", kept, err)
		}
		if _, err := conn.Exec(ctx, `UPDATE attachment SET pinned=NOT pinned WHERE entity_type='offer'`); err != nil {
			t.Fatalf("legacy file cannot be edited: %v", err)
		}
		for _, kind := range []string{"contact", "offer"} {
			_, err := conn.Exec(ctx, `INSERT INTO attachment(entity_type,entity_id,filename,storage_key,source,captured_by)
 VALUES($1,gen_random_uuid(),'new.txt','new/'||$1,'manual','human:test')`, kind)
			fault, ok := errors.AsType[*pgconn.PgError](err)
			want := "23503"
			if kind == "offer" {
				want = "23514"
			}
			if !ok || fault.Code != want {
				t.Fatalf("new %s owner: %v, want %s", kind, err, want)
			}
		}
		if _, err := dbmigrate.Down(ctx, conn, through, 4); err != nil {
			t.Fatalf("rollback: %v", err)
		}
		var columns int
		if err := conn.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='attachment' AND column_name LIKE 'owner_%'`).Scan(&columns); err != nil || columns != 0 {
			t.Fatalf("owner columns after rollback=%d, %v", columns, err)
		}
	}
}
