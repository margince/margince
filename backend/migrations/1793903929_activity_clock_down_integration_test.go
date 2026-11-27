// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/migrations"
)

func TestActivityClockRollbackRespectsAnEarlierInstalledAdvance(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	core, err := migrations.CoreHistory()
	if err != nil {
		t.Fatal(err)
	}
	through := namespaceThroughVersion(t, core, "1793983521")
	migration := through.Migrations[len(through.Migrations)-1]
	// The statement-level link triggers replaced the function this rollback
	// keeps, so each case first rewinds the schema to before them.
	statementTriggers := namespaceThroughVersion(t, core, "1795786618")
	rewind := statementTriggers.Migrations[len(statementTriggers.Migrations)-1]
	// By name: each case creates the function anew, and a cached plan would
	// keep the regprocedure OID of the case before.
	const clockFunctionSource = `SELECT prosrc FROM pg_proc WHERE proname = 'trg_activity_link_last_activity' AND pronargs = 0`
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
			if _, err := tx.Exec(ctx, rewind.DownSQL); err != nil {
				t.Fatal(err)
			}
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
			if err := tx.QueryRow(ctx, clockFunctionSource).Scan(&body); err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(body, "advance_last_activity"); got != previous {
				t.Fatalf("forward-only clock=%t want=%t: %s", got, previous, body)
			}
			if _, err := tx.Exec(ctx, migration.UpSQL); err != nil {
				t.Fatal(err)
			}
			if err := tx.QueryRow(ctx, clockFunctionSource).Scan(&body); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(body, "advance_last_activity") {
				t.Fatalf("reapply did not restore the advancing clock: %s", body)
			}
		})
	}
}

func TestTheBatchClockMigrationReappliesAfterRollback(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()
	early := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	f := seedLinkClockFixture(t, conn, early, early.AddDate(0, 0, 7), early.AddDate(0, 0, 14))
	if _, err := conn.Exec(ctx, `INSERT INTO activity_link (activity_id, entity_type, contact_id, project_id)
		VALUES (@activity, 'contact', @contact, NULL), (@activity, 'project', NULL, @project)`,
		pgx.NamedArgs{"activity": f.early, "contact": f.contactA, "project": f.projectP}); err != nil {
		t.Fatal(err)
	}
	core, err := migrations.CoreHistory()
	if err != nil {
		t.Fatal(err)
	}
	through := namespaceThroughVersion(t, core, "1795786618")
	migration := through.Migrations[len(through.Migrations)-1]
	before := catalogDigest(t, conn)
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Errorf("rolling back the migration round trip: %v", err)
		}
	})
	if _, err := tx.Exec(ctx, migration.DownSQL); err != nil {
		t.Fatal(err)
	}
	var rowTriggers []string
	if err := tx.QueryRow(ctx, `SELECT array_agg(tgname::text ORDER BY tgname) FROM pg_trigger
		WHERE tgrelid = 'activity'::regclass AND tgname IN ('activity_last_activity', 'activity_project_last_activity')
		  AND pg_get_triggerdef(oid) LIKE '%FOR EACH ROW%'`).Scan(&rowTriggers); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(rowTriggers, []string{"activity_last_activity", "activity_project_last_activity"}) {
		t.Fatalf("rollback restored row triggers %v", rowTriggers)
	}
	next := early.AddDate(0, 0, 1)
	if _, err := tx.Exec(ctx, `UPDATE activity SET occurred_at = @at WHERE id = @activity`,
		pgx.NamedArgs{"at": next, "activity": f.early}); err != nil {
		t.Fatal(err)
	}
	assertLinkClocks(t, conn, "rolled-back activity triggers", []clockWant{
		{"contact", "contact", f.contactA, &next}, {"project", "project", f.projectP, &next},
	})
	if _, err := tx.Exec(ctx, migration.UpSQL); err != nil {
		t.Fatal(err)
	}
	later := early.AddDate(0, 0, 2)
	if _, err := tx.Exec(ctx, `UPDATE activity SET occurred_at = @at WHERE id = @activity`,
		pgx.NamedArgs{"at": later, "activity": f.early}); err != nil {
		t.Fatal(err)
	}
	assertLinkClocks(t, conn, "reapplied activity trigger", []clockWant{
		{"contact", "contact", f.contactA, &later}, {"project", "project", f.projectP, &later},
	})
	if after := catalogDigest(t, conn); after != before {
		t.Fatalf("reapplied catalog digest %s, want %s", after, before)
	}
}
