// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestContactChildBatchDrainsAfterPhoneDecodingFailure(t *testing.T) {
	e, ctx, id, _ := childBatchFixture(t)
	err := e.store.tx(ctx, func(tx pgx.Tx) error {
		// Corrupt one database projection without changing the real writer's rows.
		if _, err := tx.Exec(ctx, `CREATE TEMP VIEW contact_phone AS
			SELECT contact_id, id, NULL::text AS phone, phone_type, is_primary,
			       position, source, captured_by, archived_at, created_at
			FROM public.contact_phone`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "SET LOCAL search_path = pg_temp, public"); err != nil {
			return err
		}
		row, err := e.store.GetContactTx(ctx, tx, id, storekit.LiveOnly, CustomColumns{})
		if err == nil {
			t.Fatal("NULL phone decoded successfully, want the whole contact read to fail")
		}
		if ids.UUID(row.Id) != ids.Nil {
			t.Fatal("failed child batch returned a partial contact")
		}
		var one int
		if err := tx.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
			return err
		}
		if one != 1 {
			t.Fatal("batch left unread results on the connection")
		}
		if _, err := tx.Exec(ctx, "DROP VIEW pg_temp.contact_phone"); err != nil {
			return err
		}
		row, err = e.store.GetContactTx(ctx, tx, id, storekit.LiveOnly, CustomColumns{})
		if err != nil {
			return err
		}
		assertChildBatchValues(t, row)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
