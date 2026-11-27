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
					type clockWriter struct {
						tx         pgx.Tx
						activityID string
					}
					var writers []clockWriter
					// Foreign-key checks leave key-share locks in the transaction.
					// Clock writes must stay compatible with the other writer's lock.
					for _, activityID := range []string{f.early, f.late} {
						writer := connect(t, ownerDSN)
						tx, err := writer.Begin(ctx)
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
						writers = append(writers, clockWriter{tx: tx, activityID: activityID})
					}
					write := insertPrefix + ` VALUES (@activity, @kind, @parent)`
					if operation == "update" {
						write = `UPDATE activity SET occurred_at = occurred_at + interval '1 day' WHERE id = @activity`
					} else if operation == "delete" {
						write = `DELETE FROM activity_link WHERE activity_id = @activity AND entity_type = @kind AND ` +
							pgx.Identifier{kind + "_id"}.Sanitize() + ` = @parent`
					}
					finished := make(chan error, len(writers))
					for _, writer := range writers {
						go func() {
							args := pgx.NamedArgs{"activity": writer.activityID}
							if operation != "update" {
								args["kind"], args["parent"] = kind, parentID
							}
							_, err := writer.tx.Exec(ctx, write, args)
							if err == nil {
								err = writer.tx.Commit(ctx)
							} else if rollbackErr := writer.tx.Rollback(context.Background()); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
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
					if t.Failed() {
						return
					}
					assertLinkClocks(t, conn, "concurrent link clocks", []clockWant{{kind, kind, parentID, expectedAt}})
				})
			}
		})
	}
}
