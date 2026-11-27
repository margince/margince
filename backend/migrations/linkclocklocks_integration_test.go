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
	racer := connect(t, ownerDSN)
	assertContactLockBeforeProject(t, conn, f.contactA, f.projectP, func(ctx context.Context) error {
		_, err := racer.Exec(ctx, `INSERT INTO activity_link (activity_id, entity_type, contact_id, project_id)
			VALUES (@private, 'contact', @contact, NULL), (@public, 'project', NULL, @project)`,
			pgx.NamedArgs{"private": f.private, "public": f.late, "contact": f.contactA, "project": f.projectP})
		return err
	})
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

func TestActivityClockAndStrengthChangesLockContactsBeforeProjects(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()
	early := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	f := seedLinkClockFixture(t, conn, early, early.AddDate(0, 0, 7), early.AddDate(0, 0, 14))
	activities := []string{f.early, f.late}
	if _, err := conn.Exec(ctx, `UPDATE activity SET kind = 'email', direction = 'inbound'
		WHERE id = ANY (@activities::uuid[])`, pgx.NamedArgs{"activities": activities}); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO activity_link (activity_id, entity_type, contact_id, project_id)
		VALUES (@early, 'project', NULL, @project), (@late, 'contact', @contact, NULL)`,
		pgx.NamedArgs{"early": f.early, "late": f.late, "project": f.projectP, "contact": f.contactA}); err != nil {
		t.Fatal(err)
	}
	racer := connect(t, ownerDSN)
	assertContactLockBeforeProject(t, conn, f.contactA, f.projectP, func(ctx context.Context) error {
		_, err := racer.Exec(ctx, `UPDATE activity
			SET occurred_at = CASE WHEN id = @early THEN occurred_at + interval '1 day' ELSE occurred_at END,
			    direction = CASE WHEN id = @late THEN 'outbound' ELSE direction END
			WHERE id = ANY (@activities::uuid[])`,
			pgx.NamedArgs{"early": f.early, "late": f.late, "activities": activities})
		return err
	})
	late := early.AddDate(0, 0, 7)
	next := early.AddDate(0, 0, 1)
	assertLinkClocks(t, conn, "mixed clock and strength update", []clockWant{
		{"contact", "direction-only contact", f.contactA, &late},
		{"project", "redated project", f.projectP, &next},
	})
}

func assertContactLockBeforeProject(t *testing.T, conn *pgx.Conn, contactID, projectID string, write func(context.Context) error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*testdb.ProbeBudget)
	t.Cleanup(cancel)
	holder, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	var writeErr error
	t.Cleanup(func() {
		if err := holder.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("releasing the contact: %v", err)
		}
		<-done
		if writeErr != nil {
			t.Errorf("mutating the activity clocks: %v", writeErr)
		}
	})
	// Foreign-key key-share locks pass this holder; clock and strength writers wait.
	if _, err := holder.Exec(ctx, `SELECT 1 FROM contact WHERE id = @contact FOR NO KEY UPDATE`,
		pgx.NamedArgs{"contact": contactID}); err != nil {
		close(done)
		t.Fatal(err)
	}
	go func() {
		defer close(done)
		writeErr = write(ctx)
	}()
	testdb.WaitForContention(t, done,
		"the writer returned without waiting for its contact",
		"the writer never reached the held contact",
		func(ctx context.Context) (bool, error) {
			queued, err := testdb.QueuedBehind(ctx, conn, int32(conn.PgConn().PID()))
			return queued > 0, err
		})
	if _, err := holder.Exec(ctx, `SELECT 1 FROM project WHERE id = @project FOR NO KEY UPDATE NOWAIT`,
		pgx.NamedArgs{"project": projectID}); err != nil {
		t.Fatalf("the writer locked its project before its contact: %v", err)
	}
	if err := holder.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	<-done
}
