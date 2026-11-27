// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestConcurrentLinkClockWritersKeepForeignKeyLocksCompatible(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	for _, operation := range []string{"insert", "update", "delete"} {
		t.Run(operation, func(t *testing.T) {
			for _, kind := range []string{"company", "contact", "deal", "project"} {
				t.Run(kind, func(t *testing.T) {
					headSchema(t, conn)
					ctx, cancel := context.WithTimeout(context.Background(), 2*testdb.ProbeBudget)
					defer cancel()
					early := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
					late := early.AddDate(0, 0, 7)
					f := seedLinkClockFixture(t, conn, early, late, late.AddDate(0, 0, 7))
					parents := map[string]string{"company": f.company, "contact": f.contactA, "deal": f.deal, "project": f.projectP}
					parentID, ok := parents[kind]
					if !ok {
						t.Fatalf("no parent for %s", kind)
					}
					insertPrefix := `INSERT INTO activity_link (activity_id, entity_type, ` + pgx.Identifier{kind + "_id"}.Sanitize() + `)`
					clockAt := late
					if operation != "insert" {
						if _, err := conn.Exec(ctx, insertPrefix+` VALUES (@early, @kind, @parent), (@late, @kind, @parent)`,
							pgx.NamedArgs{"early": f.early, "late": f.late, "kind": kind, "parent": parentID}); err != nil {
							t.Fatal(err)
						}
					}
					if operation == "update" {
						clockAt = late.AddDate(0, 0, 1)
					}
					expectedAt := &clockAt
					if operation == "delete" {
						expectedAt = nil
					}
					write := insertPrefix + ` VALUES (@activity, @kind, @parent)`
					if operation == "update" {
						write = `UPDATE activity SET occurred_at = occurred_at + interval '1 day' WHERE id = @activity`
					} else if operation == "delete" {
						write = `DELETE FROM activity_link WHERE activity_id = @activity AND entity_type = @kind AND ` +
							pgx.Identifier{kind + "_id"}.Sanitize() + ` = @parent`
					}
					activities := []string{f.early, f.late}
					parentUUID, err := ids.Parse(parentID)
					if err != nil {
						t.Fatal(err)
					}
					targetIDs := []ids.UUID{parentUUID}
					targetsByKind := map[string]storekit.LastActivityTargets{
						"company": {Companies: targetIDs}, "contact": {Contacts: targetIDs},
						"deal": {Deals: targetIDs}, "project": {Projects: targetIDs},
					}
					targets, ok := targetsByKind[kind]
					if !ok {
						t.Fatalf("no lock targets for %s", kind)
					}
					assertConcurrentClockWrites(t, ownerDSN, kind, parentID, targets, func(ctx context.Context, tx pgx.Tx, writerIndex int) error {
						args := pgx.NamedArgs{"activity": activities[writerIndex]}
						if operation != "update" {
							args["kind"], args["parent"] = kind, parentID
						}
						written, err := tx.Exec(ctx, write, args)
						if err != nil {
							return err
						}
						if written.RowsAffected() != 1 {
							return fmt.Errorf("wrote %d clock rows, want 1", written.RowsAffected())
						}
						return nil
					})
					if t.Failed() {
						return
					}
					assertLinkClocks(t, conn, "concurrent link clocks", []clockWant{{kind, kind, parentID, expectedAt}})
				})
			}
		})
	}
}

func TestActivityStrengthPrelocksKeepForeignKeyLocksCompatible(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()
	early := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	late := early.AddDate(0, 0, 7)
	f := seedLinkClockFixture(t, conn, early, late, late.AddDate(0, 0, 7))
	if _, err := conn.Exec(ctx, `UPDATE activity SET kind = 'email', direction = 'inbound' WHERE id = @private`,
		pgx.NamedArgs{"private": f.private}); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO activity_link (activity_id, entity_type, contact_id, project_id)
		VALUES (@early, 'project', NULL, @project), (@late, 'project', NULL, @project),
		       (@private, 'contact', @contact, NULL)`,
		pgx.NamedArgs{"early": f.early, "late": f.late, "private": f.private, "project": f.projectP, "contact": f.contactA}); err != nil {
		t.Fatal(err)
	}
	before := readClockRow(t, conn, "contact", f.contactA)
	projects := []string{f.early, f.late}
	contactID, err := ids.Parse(f.contactA)
	if err != nil {
		t.Fatal(err)
	}
	projectID, err := ids.Parse(f.projectP)
	if err != nil {
		t.Fatal(err)
	}
	targets := storekit.LastActivityTargets{
		Contacts: []ids.UUID{contactID}, Projects: []ids.UUID{projectID},
	}
	assertConcurrentClockWrites(t, ownerDSN, "contact", f.contactA, targets, func(ctx context.Context, tx pgx.Tx, writerIndex int) error {
		projectActivity := projects[writerIndex]
		written, err := tx.Exec(ctx, `UPDATE activity
			SET occurred_at = CASE WHEN id = @project THEN occurred_at + interval '1 day' ELSE occurred_at END,
			    direction = CASE WHEN id = @private THEN 'outbound' ELSE direction END
			WHERE id = ANY (@activities::uuid[])`,
			pgx.NamedArgs{"project": projectActivity, "private": f.private, "activities": []string{projectActivity, f.private}})
		if err != nil {
			return err
		}
		if written.RowsAffected() != 2 {
			return fmt.Errorf("updated %d activities, want the project and strength-only rows", written.RowsAffected())
		}
		return nil
	})
	if t.Failed() {
		return
	}
	next := late.AddDate(0, 0, 1)
	assertLinkClocks(t, conn, "concurrent clock and strength changes", []clockWant{
		{"contact", "private strength-only contact", f.contactA, nil},
		{"project", "redated project", f.projectP, &next},
	})
	after := readClockRow(t, conn, "contact", f.contactA)
	if after.version != before.version || !after.updatedAt.Equal(before.updatedAt) {
		t.Errorf("strength locking edited the contact: before %+v, after %+v", before, after)
	}
}

func assertConcurrentClockWrites(t *testing.T, ownerDSN, kind, parentID string, targets storekit.LastActivityTargets, write func(context.Context, pgx.Tx, int) error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*testdb.ProbeBudget)
	defer cancel()
	var writers []pgx.Tx
	// Foreign-key checks leave key-share locks in each transaction.
	// Clock writes must stay compatible with the other writer's lock.
	for range 2 {
		conn := connect(t, ownerDSN)
		tx, err := conn.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
				t.Errorf("release the foreign-key lock: %v", err)
			}
		})
		var lockedID string
		if err := tx.QueryRow(ctx, `SELECT id::text FROM `+pgx.Identifier{kind}.Sanitize()+
			` WHERE id = @parent FOR KEY SHARE`, pgx.NamedArgs{"parent": parentID}).Scan(&lockedID); err != nil {
			t.Fatal(err)
		}
		if lockedID != parentID {
			t.Fatalf("locked %s, want %s", lockedID, parentID)
		}
		writers = append(writers, tx)
	}
	finished := make(chan error, len(writers))
	for writerIndex, tx := range writers {
		go func() {
			err := storekit.LockLastActivityTargets(ctx, tx, targets)
			if err == nil {
				err = write(ctx, tx, writerIndex)
			}
			if err == nil {
				err = tx.Commit(ctx)
			} else if rollbackErr := tx.Rollback(context.Background()); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
				err = errors.Join(err, rollbackErr)
			}
			finished <- err
		}()
	}
	for range writers {
		if err := <-finished; err != nil {
			t.Errorf("concurrent %s clock writer failed: %v", kind, err)
		}
	}
}
