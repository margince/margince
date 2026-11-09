// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func assertParticipantEvidence(ctx context.Context, tx pgx.Tx, activity ids.ActivityID, at time.Time, visible bool, version int64) error {
	var count int
	var agrees bool
	err := tx.QueryRow(ctx, `SELECT count(*), coalesce(bool_and(
 domain_evidence_at=@at AND domain_evidence_visible=@visible AND domain_evidence_parent_version=@version),false)
 FROM activity_participant WHERE activity_id=@activity`, pgx.NamedArgs{"activity": activity, "at": at, "visible": visible, "version": version}).Scan(&count, &agrees)
	if err != nil {
		return err
	}
	if count == 0 || !agrees {
		return fmt.Errorf("participant snapshots disagree with their activity: count=%d agree=%v", count, agrees)
	}
	return nil
}

func TestParticipantEvidenceFollowsRealCorrectionsAndArchiveRollback(t *testing.T) {
	store, ctx, id, original := domainEvidenceActivity(t)
	check := func(at time.Time, visible bool, version int64) {
		t.Helper()
		if err := store.tx(ctx, func(tx pgx.Tx) error { return assertParticipantEvidence(ctx, tx, id, at, visible, version) }); err != nil {
			t.Fatal(err)
		}
	}
	check(original, true, 0)
	subject := "Only the subject changed"
	if _, err := store.UpdateActivity(ctx, id, UpdateActivityInput{Subject: &subject}); err != nil {
		t.Fatal(err)
	}
	check(original, true, 0)
	changed := original.AddDate(0, 1, 0)
	if _, err := store.UpdateActivity(ctx, id, UpdateActivityInput{OccurredAt: &changed}); err != nil {
		t.Fatal(err)
	}
	check(changed, true, 1)
	if err := store.tx(ctx, func(tx pgx.Tx) error {
		savepoint, err := tx.Begin(ctx)
		if err != nil {
			return err
		}
		if err := store.ArchiveActivityTx(ctx, savepoint, id, nil); err != nil {
			return err
		}
		if err := assertParticipantEvidence(ctx, savepoint, id, changed, false, 2); err != nil {
			return err
		}
		if err := savepoint.Rollback(ctx); err != nil {
			return err
		}
		return assertParticipantEvidence(ctx, tx, id, changed, true, 1)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ArchiveActivity(ctx, id, nil); err != nil {
		t.Fatal(err)
	}
	check(changed, false, 2)
	replacement, _, err := store.LogActivity(ctx, LogActivityInput{Kind: "email", Source: "manual", Subject: &subject, OccurredAt: &original})
	if err != nil {
		t.Fatal(err)
	}
	replacementID := ids.From[ids.ActivityKind](ids.UUID(replacement.Id))
	// Direct reparenting and deletion exercise the database protocol rather than a second projector.
	if err := store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE activity_participant SET activity_id=@replacement WHERE activity_id=@original AND user_id IS NULL`, pgx.StrictNamedArgs{"replacement": replacementID, "original": id}); err != nil {
			return err
		}
		if err := assertParticipantEvidence(ctx, tx, replacementID, original, true, 0); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM activity WHERE id=@activity`, pgx.StrictNamedArgs{"activity": replacementID}); err != nil {
			return err
		}
		var remaining int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM activity_participant WHERE activity_id=@activity`, pgx.StrictNamedArgs{"activity": replacementID}).Scan(&remaining); err != nil {
			return err
		}
		if remaining != 0 {
			return fmt.Errorf("deleted parent retained %d participant snapshots", remaining)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func domainEvidenceActivity(t *testing.T) (*Store, context.Context, ids.ActivityID, time.Time) {
	t.Helper()
	e := setupSend(t)
	store := e.store(nil)
	ctx := e.as(principal.RowScopeAll)
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("fixture has no actor")
	}
	actor.Permissions.Objects["activity"] = principal.ObjectGrant{Read: true, Create: true, Update: true, Delete: true}
	ctx = principal.WithActor(ctx, actor)
	contact := e.seedContact(t, "Evidence counterparty")
	original := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	subject := "Evidence correction"
	record, _, err := store.LogActivity(ctx, LogActivityInput{Kind: "call", Subject: &subject, OccurredAt: &original, Source: "manual", Links: []ActivityLinkInput{{EntityType: linkEntityContact, EntityID: contact}}})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.ActivityKind](ids.UUID(record.Id))

	return store, ctx, id, original
}

func TestParticipantEvidenceSerializesWithConcurrentParentCorrections(t *testing.T) {
	for _, childFirst := range []bool{true, false} {
		t.Run(fmt.Sprintf("child-first=%v", childFirst), func(t *testing.T) {
			store, ctx, id, original := domainEvidenceActivity(t)
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			changed := original.AddDate(0, 1, 0)
			done := make(chan error, 1)
			err := store.tx(ctx, func(tx pgx.Tx) error {
				if childFirst {
					if err := insertParticipantRow(ctx, tx, id, "cc", nil, nil, "other@racing.test", nil); err != nil {
						return err
					}
					go func() {
						_, err := store.UpdateActivity(ctx, id, UpdateActivityInput{OccurredAt: &changed})
						done <- err
					}()
				} else {
					if _, err := updateActivityInTx(ctx, tx, id, UpdateActivityInput{OccurredAt: &changed}); err != nil {
						return err
					}
					go func() {
						done <- store.tx(ctx, func(child pgx.Tx) error {
							return insertParticipantRow(ctx, child, id, "cc", nil, nil, "other@racing.test", nil)
						})
					}()
				}
				return waitForEvidenceBlock(ctx, tx, done)
			})
			if err != nil {
				cancel()
				t.Fatal(err)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if err := store.tx(ctx, func(tx pgx.Tx) error { return assertParticipantEvidence(ctx, tx, id, changed, true, 1) }); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func waitForEvidenceBlock(ctx context.Context, tx pgx.Tx, done chan error) error {
	for range 20000 {
		select {
		case err := <-done:
			done <- err
			return fmt.Errorf("concurrent writer did not wait for the evidence snapshot: %v", err)
		default:
		}
		if _, err := tx.Exec(ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
			return err
		}
		var blocked bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks
   WHERE NOT granted AND pg_backend_pid()=ANY(pg_blocking_pids(pid)))`).Scan(&blocked); err != nil {
			return err
		}
		if blocked {
			return nil
		}
	}
	return errors.New("concurrent writer never reached the evidence lock")
}

func TestParticipantEvidenceDirectParentUpdatesPreserveTheSnapshot(t *testing.T) {
	// Direct SQL isolates the trigger protocol from lifecycle row locks.
	const correction = `UPDATE activity SET occurred_at=@at WHERE id=@activity`
	for _, childFirst := range []bool{true, false} {
		t.Run(fmt.Sprintf("child-first=%v", childFirst), func(t *testing.T) {
			store, ctx, id, original := domainEvidenceActivity(t)
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			changed := original.AddDate(0, 1, 0)
			args := pgx.NamedArgs{"at": changed, "activity": id}
			done := make(chan error, 1)
			err := store.tx(ctx, func(tx pgx.Tx) error {
				if childFirst {
					if err := insertParticipantRow(ctx, tx, id, "cc", nil, nil, "direct@racing.test", nil); err != nil {
						return err
					}
					go func() {
						done <- store.tx(ctx, func(parent pgx.Tx) error { _, err := parent.Exec(ctx, correction, args); return err })
					}()
				} else {
					if _, err := tx.Exec(ctx, correction, args); err != nil {
						return err
					}
					go func() {
						done <- store.tx(ctx, func(child pgx.Tx) error {
							return insertParticipantRow(ctx, child, id, "cc", nil, nil, "direct@racing.test", nil)
						})
					}()
				}
				return waitForEvidenceBlock(ctx, tx, done)
			})
			if err != nil {
				cancel()
				t.Fatal(err)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if err := store.tx(ctx, func(tx pgx.Tx) error { return assertParticipantEvidence(ctx, tx, id, changed, true, 1) }); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestEvidenceBackfillsLockParentsBeforeExistingChildren(t *testing.T) {
	folds := map[string]func(context.Context, pgx.Tx, ids.UUID, int) ([]ids.UUID, int, error){
		"evidence": FoldParticipantEvidenceTx, "address": FoldParticipantDomainsTx,
	}
	for name, fold := range folds {
		for _, parentFirst := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/parent-first=%v", name, parentFirst), func(t *testing.T) {
				store, ctx, id, original := domainEvidenceActivity(t)
				ctx, cancel := context.WithCancel(ctx)
				defer cancel()
				testdb.WithFixtureTriggerDisabled(t, "activity_participant", "stamp_participant_domain_evidence", func(tx pgx.Tx) error {
					_, err := tx.Exec(ctx, `UPDATE activity_participant SET address=coalesce(address,'legacy@unfolded.test'), address_domain_folded=NULL,
 domain_evidence_parent_version=NULL,domain_evidence_at=NULL,domain_evidence_precision=NULL,domain_evidence_visible=NULL WHERE activity_id=@activity`, pgx.StrictNamedArgs{"activity": id})
					return err
				})
				changed := original.AddDate(0, 1, 0)
				args := pgx.StrictNamedArgs{"at": changed, "activity": id}
				const correction = `UPDATE activity SET occurred_at=@at WHERE id=@activity`
				done := make(chan error, 1)
				err := store.tx(ctx, func(tx pgx.Tx) error {
					if parentFirst {
						var held ids.ActivityID
						if err := tx.QueryRow(ctx, `SELECT id FROM activity WHERE id=@activity FOR UPDATE`, pgx.StrictNamedArgs{"activity": id}).Scan(&held); err != nil {
							return err
						}
						go func() {
							done <- store.tx(ctx, func(child pgx.Tx) error { _, _, err := fold(ctx, child, ids.Nil, 500); return err })
						}()
						if err := waitForEvidenceBlock(ctx, tx, done); err != nil {
							return err
						}
						children, err := tx.Query(ctx, `SELECT id FROM activity_participant
                         WHERE activity_id=@activity ORDER BY id FOR UPDATE NOWAIT`, pgx.StrictNamedArgs{"activity": id})
						if err != nil {
							return err
						}
						children.Close()
						if err := children.Err(); err != nil {
							return err
						}
						_, err = tx.Exec(ctx, correction, args)
						return err
					}
					if _, _, err := fold(ctx, tx, ids.Nil, 500); err != nil {
						return err
					}
					go func() {
						done <- store.tx(ctx, func(parent pgx.Tx) error { _, err := parent.Exec(ctx, correction, args); return err })
					}()
					return waitForEvidenceBlock(ctx, tx, done)
				})
				if err != nil {
					cancel()
					t.Fatal(err)
				}
				if err := <-done; err != nil {
					t.Fatal(err)
				}
				if err := store.tx(ctx, func(tx pgx.Tx) error { return assertParticipantEvidence(ctx, tx, id, changed, true, 1) }); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestAnUnrelatedParentUpdateKeepsItsNonKeyLock(t *testing.T) {
	store, ctx, id, original := domainEvidenceActivity(t)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := store.tx(ctx, func(tx pgx.Tx) error {
		if err := insertParticipantRow(ctx, tx, id, "cc", nil, nil, "unchanged@racing.test", nil); err != nil {
			return err
		}
		done := make(chan error, 1)
		go func() {
			done <- store.tx(ctx, func(parent pgx.Tx) error {
				_, err := parent.Exec(ctx, `UPDATE activity SET subject='Unrelated correction' WHERE id=@activity`, pgx.StrictNamedArgs{"activity": id})
				return err
			})
		}()
		for range 20000 {
			select {
			case err := <-done:
				return err
			default:
			}
			if _, err := tx.Exec(ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
				return err
			}
			var blocked bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE NOT granted
    AND pg_backend_pid()=ANY(pg_blocking_pids(pid)))`).Scan(&blocked); err != nil {
				return err
			}
			if blocked {
				return errors.New("an unrelated correction took an exclusive parent lock")
			}
		}
		return errors.New("unrelated correction did not finish")
	}); err != nil {
		cancel()
		t.Fatal(err)
	}
	if err := store.tx(ctx, func(tx pgx.Tx) error { return assertParticipantEvidence(ctx, tx, id, original, true, 0) }); err != nil {
		t.Fatal(err)
	}
}
