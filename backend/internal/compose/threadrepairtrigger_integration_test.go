// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func finishSubjectThreadRepair(t *testing.T, f threadOwnerFixture, key string) {
	t.Helper()
	if err := database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error { return capture.QueueThreadRepairTx(f.Admin(), tx, key) }); err != nil {
		t.Fatal(err)
	}
	for range 2000 {
		done := false
		if err := database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error {
			_, _, finished, err := capture.ThreadRepairBatchTx(f.Admin(), tx, 7, ids.NewV7(), threadRepairApplier(f.Pool))
			done = finished
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if done {
			return
		}
	}
	t.Fatal("repair failed to finish")
}

func latestThreadUpdate(t *testing.T, e *integration.Env, id ids.ActivityID) events.Envelope {
	t.Helper()
	var raw []byte
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(e.Admin(), `SELECT envelope FROM event_outbox
 WHERE envelope->>'type'='activity.updated' AND envelope->'entity'->>'id'=@id ORDER BY seq DESC LIMIT 1`, pgx.NamedArgs{"id": id.String()}).Scan(&raw)
	}); err != nil {
		t.Fatal(err)
	}
	var env events.Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	return env
}

func TestAnOrdinarySubjectEditWakesTheSharedRepairWithoutPublicationLoops(t *testing.T) {
	f := newThreadOwnerFixture(t)
	finishSubjectThreadRepair(t, f, f.source)
	if got := threadKeyOfMessage(t, f.Env, f.destination); got != f.destination {
		t.Fatalf("initial partition=%q", got)
	}
	subject := "Price"
	acting := f.As(f.Rep1, []ids.UUID{f.Team1}, integration.AdminPerms)
	if _, err := f.Activities.UpdateActivity(acting, f.second, activities.UpdateActivityInput{Subject: &subject}); err != nil {
		t.Fatal(err)
	}
	env := latestThreadUpdate(t, f.Env, f.second)
	gen := NewThreadRepairGen(f.Pool)
	for range 2 {
		if err := gen.HandleEvent(context.Background(), env); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.WsCount(t, `SELECT count(*) FROM capture_thread_pending WHERE source_key=$1`, f.destination); n != 1 {
		t.Fatalf("queued aliases=%d", n)
	}
	finishSubjectThreadRepair(t, f, f.destination)
	if got := threadKeyOfMessage(t, f.Env, f.destination); got != f.source {
		t.Fatalf("edited topic grouped as %q", got)
	}
	wake := latestThreadUpdate(t, f.Env, f.second)
	if err := gen.HandleEvent(context.Background(), wake); err != nil {
		t.Fatal(err)
	}
	if n := f.WsCount(t, `SELECT count(*) FROM capture_thread_pending`); n != 0 {
		t.Fatalf("publication requeued %d aliases", n)
	}
}

func TestPublishedThreadAttributionUsesTheCapturingSeatsCurrentAuthority(t *testing.T) {
	for _, granted := range []bool{false, true} {
		name := "no_current_grant"
		if granted {
			name = "current"
		}
		t.Run(name, func(t *testing.T) {
			f := newThreadOwnerFixture(t)
			if granted {
				f.GrantRole(t, f.Rep1, "admin")
			}
			account := seedLadderAccount(t, f.Env)
			project, key := account.project(t, "Published correspondence")
			subject := "[" + key + "] Logo"
			acting := f.As(f.Rep1, []ids.UUID{f.Team1}, integration.AdminPerms)
			if _, err := f.Activities.UpdateActivity(acting, f.second, activities.UpdateActivityInput{Subject: &subject}); err != nil {
				t.Fatal(err)
			}
			finishSubjectThreadRepair(t, f, f.source)
			got := filedUnder(t, f.Env, f.second.UUID)
			if granted && got != project {
				t.Fatalf("current seat filed under %s, want %s", got, project)
			}
			if !granted && !got.IsZero() {
				t.Fatalf("seat without current grants filed under %s", got)
			}
		})
	}
}
