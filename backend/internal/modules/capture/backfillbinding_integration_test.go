// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type boundHistory struct {
	checkpointHistory
	mailbox string
}

func TestALegacyRunAheadOfTheClockCannotBindTheNextAccount(t *testing.T) {
	provider := &checkpointHistory{}
	ctx, registry, previous := admissionRunWith(t, provider)
	owner, _ := setupCaptureDB(t)
	if _, err := owner.Exec(ctx, `UPDATE capture_backfill SET created_at='2100-01-01T00:00:00Z' WHERE id=@run`, pgx.NamedArgs{"run": previous}); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Connect(ctx, "gmail", connector.Auth("replacement-account")); err != nil {
		t.Fatal(err)
	}
	if _, completed, _, err := registry.RunBackfillStep(ctx, previous); err != nil || completed || provider.calls != 0 {
		t.Fatalf("old account survived the clock boundary: calls=%d completed=%v err=%v", provider.calls, completed, err)
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("fixture actor missing")
	}
	run, err := registry.StartBackfill(ctx, "gmail", ids.From[ids.UserKind](actor.UserID), 3,
		connector.BackfillEstimate{Messages: 20}, func(context.Context, pgx.Tx, ids.UUID) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	latest, err := registry.BackfillStatus(ctx, "gmail", ids.From[ids.UserKind](actor.UserID))
	if err != nil || latest == nil || latest.ID != run.ID || latest.WindowMonths != 3 {
		t.Fatalf("new account inherited the previous run or sort order: run=%v err=%v", latest, err)
	}
	if _, completed, _, err := registry.RunBackfillStep(ctx, run.ID); !errors.Is(err, connector.ErrUnreachable) || completed || provider.calls != 1 {
		t.Fatalf("new account lost its own saved place: calls=%d completed=%v err=%v", provider.calls, completed, err)
	}
}

func TestSyncRevivesOnlyTheCurrentAccountsHistory(t *testing.T) {
	for _, sameAccount := range []bool{false, true} {
		t.Run(fmt.Sprintf("same_account_%v", sameAccount), func(t *testing.T) {
			provider := &boundHistory{mailbox: "first@mail.example"}
			ctx, registry, _ := admissionRunWith(t, provider)
			connection, old := truncatedRun(ctx, t, "auth")
			if !sameAccount {
				provider.mailbox = "second@mail.example"
			}
			if _, err := registry.Connect(ctx, "gmail", connector.Auth("replacement")); err != nil {
				t.Fatal(err)
			}
			if err := registry.SyncOnce(ctx, connection); err != nil {
				t.Fatal(err)
			}
			status, _, cursor := runState(ctx, t, old)
			if sameAccount {
				if status != "queued" || len(cursor) == 0 {
					t.Fatalf("same-account sync lost its history: status=%s cursor=%s", status, cursor)
				}
				return
			}
			if status != "error" {
				t.Fatalf("sync revived another account's run: %s", status)
			}
			startFixtureBackfill(ctx, t, registry)
		})
	}
}

func TestAccountChangesCancelHistoryEvenWhileAdmissionIsHeld(t *testing.T) {
	for _, admitted := range []bool{false, true} {
		t.Run(fmt.Sprintf("admitted_%v", admitted), func(t *testing.T) {
			provider := &checkpointHistory{}
			ctx, registry, previous := admissionRunWith(t, provider)
			if admitted {
				queue := &recordedBackfillQueue{generations: map[ids.UUID]int64{}}
				if err := registry.DispatchBackfills(ctx, queue.scheduler(), false); err != nil {
					t.Fatal(err)
				}
			}
			registry.WithBackfillAdmission(1, true)
			if _, err := registry.Connect(ctx, "gmail", connector.Auth("replacement-account")); err != nil {
				t.Fatal(err)
			}
			status, _, _ := runState(ctx, t, previous)
			owner, _ := setupCaptureDB(t)
			var released bool
			if err := owner.QueryRow(ctx, `SELECT admission_slot IS NULL AND execution_owner_pid IS NULL
 AND execution_due_at IS NULL FROM capture_backfill WHERE id=@run`, pgx.NamedArgs{"run": previous}).Scan(&released); err != nil {
				t.Fatal(err)
			}
			if status != "cancelled" || !released || provider.calls != 0 {
				t.Fatalf("rebind waited for held admission: status=%s released=%v calls=%d", status, released, provider.calls)
			}
			startFixtureBackfill(ctx, t, registry)
		})
	}
}

func (p *boundHistory) AccountLabel(connector.Auth) (string, error) {
	return p.mailbox, nil
}

func TestHistoryPagesKeepTheAccountThatStartedTheirWindow(t *testing.T) {
	for _, phase := range []struct {
		name                   string
		admitted, betweenPages bool
	}{
		{"queued", false, false},
		{"admitted", true, false},
		{"between_pages", true, true},
	} {
		for _, sameAccount := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s_same_account_%v", phase.name, sameAccount), func(t *testing.T) {
				provider := &boundHistory{mailbox: "first@mail.example"}
				ctx, reg, id := admissionRunWith(t, provider)
				queue := &recordedBackfillQueue{generations: map[ids.UUID]int64{}}
				admit := func() {
					t.Helper()
					if err := reg.DispatchBackfills(ctx, queue.scheduler(), false); err != nil {
						t.Fatal(err)
					}
				}
				if phase.admitted {
					admit()
				}
				if phase.betweenPages {
					if _, _, _, err := reg.RunBackfillExecution(ctx, id, queue.generations[id], queue.scheduler()); !errors.Is(err, connector.ErrUnreachable) {
						t.Fatalf("initial checkpoint: %v", err)
					}
				}
				callsBefore := provider.calls
				if !sameAccount {
					provider.mailbox = "second@mail.example"
				}
				if _, err := reg.Connect(ctx, "gmail", connector.Auth("replacement-credential")); err != nil {
					t.Fatal(err)
				}
				if !phase.admitted {
					admit()
				}
				owner, _ := setupCaptureDB(t)
				if _, err := owner.Exec(ctx, `UPDATE capture_backfill SET execution_due_at=now()-interval '1 second' WHERE id=@run AND status='running'`, pgx.NamedArgs{"run": id}); err != nil {
					t.Fatal(err)
				}
				done, completed, _, err := reg.RunBackfillExecution(ctx, id, queue.generations[id], queue.scheduler())
				var status string
				var scanned int
				var released bool
				if readErr := owner.QueryRow(ctx, `SELECT status,scanned,
 admission_slot IS NULL AND execution_owner_pid IS NULL AND execution_due_at IS NULL AND inflight_scanned=0
 FROM capture_backfill WHERE id=@run`, pgx.NamedArgs{"run": id}).Scan(&status, &scanned, &released); readErr != nil {
					t.Fatal(readErr)
				}
				if sameAccount {
					if provider.calls != callsBefore+1 || status == "cancelled" || phase.betweenPages && (err != nil || !completed || scanned != 20) || !phase.betweenPages && !errors.Is(err, connector.ErrUnreachable) {
						t.Fatalf("credential rotation lost progress: calls=%d status=%s scanned=%d completed=%v err=%v", provider.calls, status, scanned, completed, err)
					}
					return
				}
				wantScanned := 0
				if phase.betweenPages {
					wantScanned = 10
				}
				if err != nil || !done || completed || provider.calls != callsBefore || status != "cancelled" || scanned != wantScanned || !released {
					t.Fatalf("old account run reached the replacement: calls=%d want=%d status=%s scanned=%d want=%d released=%v done=%v completed=%v err=%v", provider.calls, callsBefore, status, scanned, wantScanned, released, done, completed, err)
				}
			})
		}
	}
}
