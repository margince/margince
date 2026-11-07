// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/margince/margince/backend/migrations"
)

func TestActivityClockRollbackRespectsAnEarlierInstalledAdvance(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	core, err := migrations.Core()
	if err != nil {
		t.Fatal(err)
	}
	through := namespaceThroughVersion(t, core, "1793983521")
	migration := through.Migrations[len(through.Migrations)-1]
	for _, previous := range []bool{false, true} {
		t.Run(fmt.Sprintf("earlier_advance=%t", previous), func(t *testing.T) {
			ctx := context.Background()
			tx, err := conn.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := tx.Rollback(ctx); err != nil {
					t.Error(err)
				}
			}()
			if _, err := tx.Exec(ctx, `DELETE FROM schema_migrations_core WHERE version='1793878884'`); err != nil {
				t.Fatal(err)
			}
			if previous {
				if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations_core(version,name) VALUES('1793878884','a_new_link_moves_the_last_activity_clock_forward')`); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := tx.Exec(ctx, migration.DownSQL); err != nil {
				t.Fatal(err)
			}
			var body string
			if err := tx.QueryRow(ctx, `SELECT prosrc FROM pg_proc WHERE oid='trg_activity_link_last_activity()'::regprocedure`).Scan(&body); err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(body, "advance_last_activity"); got != previous {
				t.Fatalf("forward-only clock=%t want=%t: %s", got, previous, body)
			}
			if _, err := tx.Exec(ctx, migration.UpSQL); err != nil {
				t.Fatal(err)
			}
			if err := tx.QueryRow(ctx, `SELECT prosrc FROM pg_proc WHERE oid='trg_activity_link_last_activity()'::regprocedure`).Scan(&body); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(body, "advance_last_activity") {
				t.Fatalf("reapply did not restore the advancing clock: %s", body)
			}
		})
	}
}
