// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
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

// TestContactChildBatchServerErrorAttachesNothing breaks the batch's first query at
// the server. Nothing may be attached, and the connection must stay usable.
func TestContactChildBatchServerErrorAttachesNothing(t *testing.T) {
	e, ctx, id, _ := childBatchFixture(t)
	err := e.store.tx(ctx, func(tx pgx.Tx) error {
		// A savepoint keeps the outer transaction usable after the server error.
		inner, err := tx.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := inner.Exec(ctx, `CREATE TEMP VIEW contact_email AS SELECT 1 AS nothing`); err != nil {
			return err
		}
		if _, err := inner.Exec(ctx, "SET LOCAL search_path = pg_temp, public"); err != nil {
			return err
		}
		row := crmcontracts.Contact{Id: openapi_types.UUID(id.UUID)}
		idx := map[openapi_types.UUID]*crmcontracts.Contact{row.Id: &row}
		if err := attachContactChannels(ctx, inner, idx, []ids.UUID{id.UUID}); err == nil {
			t.Fatal("broken email query succeeded, want the batch to fail")
		}
		assertChildBatchEmpty(t, row)
		if err := inner.Rollback(ctx); err != nil {
			return err
		}
		return selectOne(ctx, t, tx)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestContactChildBatchRidesPendingSnapshotSetup sends the batch as the first
// statement of a workspace snapshot, while its setup is still deferred.
func TestContactChildBatchRidesPendingSnapshotSetup(t *testing.T) {
	e, ctx, id, empty := childBatchFixture(t)
	rows := []crmcontracts.Contact{{Id: openapi_types.UUID(id.UUID)}, {Id: openapi_types.UUID(empty.UUID)}}
	err := database.WithSnapshot(ctx, e.store.db.Pool(), func(snap context.Context) error {
		return e.store.tx(snap, func(tx pgx.Tx) error {
			idx := map[openapi_types.UUID]*crmcontracts.Contact{rows[0].Id: &rows[0], rows[1].Id: &rows[1]}
			if err := attachContactChannels(snap, tx, idx, []ids.UUID{id.UUID, empty.UUID}); err != nil {
				return err
			}
			return selectOne(snap, t, tx)
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	assertChildBatchValues(t, rows[0])
	assertChildBatchEmpty(t, rows[1])
}

func selectOne(ctx context.Context, t *testing.T, tx pgx.Tx) error {
	t.Helper()
	var one int
	if err := tx.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
		return err
	}
	if one != 1 {
		t.Fatal("batch left unread results on the connection")
	}
	return nil
}
