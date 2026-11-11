// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/extprivacy"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	extdecl "github.com/margince/margince/backend/pkg/extension/privacy"
)

// An extension row of a correspondence table is under the core's correspondence
// floor. A two-year-old row is kept where such records are kept six years, and
// due where no floor is set.
func TestTheFloorOfAnExtensionTablesClassKeepsAYoungerRow(t *testing.T) {
	ownerDSN := os.Getenv("MARGINCE_TEST_DSN")
	if ownerDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing owner connection: %v", err)
		}
	})
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}
	table := extdecl.Table{
		Name: "ext_probe_floor", Links: []extdecl.Link{{Column: "contact_id", Record: extdecl.Contact}},
		Personal: []string{"body"}, Retention: jurisdiction.CommercialCorrespondence, AgeColumn: "kept_at",
	}
	pol := retentionPolicy{ID: ids.NewV7(), ObjectType: extensionObjectType, RetainDays: 30, Action: actionAnonymize}
	for _, c := range []struct {
		country jurisdiction.Code
		due     bool
	}{{"zu", false}, {"zv", true}} {
		t.Run(string(c.country), func(t *testing.T) {
			tx, err := owner.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
					t.Errorf("rolling back: %v", err)
				}
			}()
			mustExec(ctx, t, tx, `CREATE SCHEMA IF NOT EXISTS ext`)
			mustExec(ctx, t, tx, `CREATE TABLE ext.ext_probe_floor (id uuid PRIMARY KEY, contact_id uuid, body text, kept_at timestamptz NOT NULL)`)
			ref := clocktest.Now(t)
			mustExec(ctx, t, tx, `INSERT INTO ext.ext_probe_floor (id, body, kept_at) VALUES ($1, 'kept words', $2)`, ids.NewV7(), ref.AddDate(-2, 0, 0))
			country := c.country
			registry := extprivacy.Declared([]extdecl.Table{table})
			svc := &RetentionService{eraser: NewEraser(nil).
				WithCountry(func(context.Context, pgx.Tx) (jurisdiction.Code, error) { return country, nil }).
				WithExtensions(registry)}
			w, err := svc.extensionWindow(ctx, tx, table, pol, ref)
			if err != nil {
				t.Fatal(err)
			}
			due, err := registry.Due(ctx, tx, table, w, true, retentionBatch)
			if err != nil {
				t.Fatal(err)
			}
			if got := len(due) == 1; got != c.due {
				t.Fatalf("under %s the two-year-old row is due=%v, want %v", c.country, got, c.due)
			}
		})
	}
}
