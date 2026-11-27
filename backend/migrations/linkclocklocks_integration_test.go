// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/testdb"
)

func TestALinkInsertLocksAnIneligibleContactBeforeAnEligibleProject(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	early := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	f := seedLinkClockFixture(t, conn, early, early.AddDate(0, 0, 7), early.AddDate(0, 0, 14))
	before := readClockRow(t, conn, "contact", f.contactA)
	ctx, cancel := context.WithTimeout(context.Background(), 2*testdb.ProbeBudget)
	t.Cleanup(cancel)
	racer := connect(t, ownerDSN)
	holder, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	var insertErr error
	t.Cleanup(func() {
		if err := holder.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("releasing the contact: %v", err)
		}
		<-done
		if insertErr != nil {
			t.Errorf("inserting the mixed-audience links: %v", insertErr)
		}
	})
	// Key-share locks from foreign keys can pass this holder. The clock and
	// strength writers must still wait for its row lock.
	if _, err := holder.Exec(ctx, `SELECT 1 FROM contact WHERE id = @contact FOR NO KEY UPDATE`,
		pgx.NamedArgs{"contact": f.contactA}); err != nil {
		close(done)
		t.Fatal(err)
	}
	go func() {
		defer close(done)
		_, insertErr = racer.Exec(ctx, `
			INSERT INTO activity_link (activity_id, entity_type, contact_id, project_id)
			VALUES (@private, 'contact', @contact, NULL), (@public, 'project', NULL, @project)`,
			pgx.NamedArgs{"private": f.private, "public": f.late, "contact": f.contactA, "project": f.projectP})
	}()
	testdb.WaitForContention(t, done,
		"the insert returned without waiting for its contact",
		"the insert never reached the held contact",
		func(ctx context.Context) (bool, error) {
			queued, err := testdb.QueuedBehind(ctx, conn, int32(conn.PgConn().PID()))
			return queued > 0, err
		})
	if _, err := holder.Exec(ctx, `SELECT 1 FROM project WHERE id = @project FOR NO KEY UPDATE NOWAIT`,
		pgx.NamedArgs{"project": f.projectP}); err != nil {
		t.Fatalf("the insert locked its project before its ineligible contact: %v", err)
	}
	if err := holder.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	<-done
	after := readClockRow(t, conn, "contact", f.contactA)
	if after.lastActivity != nil || after.version != before.version || !after.updatedAt.Equal(before.updatedAt) {
		t.Errorf("locking the private contact changed its clock or record: before %+v, after %+v", before, after)
	}
	late := early.AddDate(0, 0, 7)
	assertLinkClocks(t, conn, "mixed-audience insert", []clockWant{
		{"contact", "private contact", f.contactA, nil},
		{"project", "public project", f.projectP, &late},
	})
}
