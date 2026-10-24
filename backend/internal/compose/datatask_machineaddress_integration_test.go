// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// machineAddressContact creates a contact through the real writer, then gives
// it the address in place, as a contact stored before the refusal existed.
func machineAddressContact(t *testing.T, e *integration.Env, conn *pgx.Conn, name, address string) ids.UUID {
	t.Helper()
	made, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: name, Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: "seed-" + ids.NewV7().String() + "@machine-task.test", EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("creating %s: %v", name, err)
	}
	if _, err := conn.Exec(context.Background(), `UPDATE contact_email SET email = $2 WHERE contact_id = $1`, made.Id, address); err != nil {
		t.Fatal(err)
	}
	return ids.UUID(made.Id)
}

func machineTagged(t *testing.T, e *integration.Env, contact ids.UUID) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM taggable g JOIN tag ON tag.id = g.tag_id
		WHERE tag.name = $1 AND tag.archived_at IS NULL AND g.entity_type = 'contact' AND g.entity_id = $2`,
		machineAddressTag, contact)
}

func runMachineAddressTask(ctx context.Context, t *testing.T, e *integration.Env, task datatask.Task) {
	t.Helper()
	now := clocktest.Now(t)
	engine, err := datatask.New(e.Pool, []datatask.Task{task}, "test", func() time.Time { return now }, func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	for range 20 {
		if _, err := engine.Execute(ctx); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTheMachineAddressTaskTagsOnlyMachineContactsAndReplaysSafely(t *testing.T) {
	e := integration.Setup(t)
	conn := integration.OwnerConn(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	receipts := machineAddressContact(t, e, conn, "Expense Receipts", "receipts@machine-task.test")
	noreply := machineAddressContact(t, e, conn, "Signing Service", "signing-noreply@machine-task.test")
	anna := machineAddressContact(t, e, conn, "Anna Notify Weber", "anna.notify.weber@machine-task.test")
	docsReply := machineAddressContact(t, e, conn, "D", "d+"+strings.Repeat("aorgb9", 34)+"@docs.google.com")
	// A user who already coined the word keeps it: the task files under theirs.
	if _, err := conn.Exec(ctx, `INSERT INTO tag (name) VALUES ($1)`, machineAddressTag); err != nil {
		t.Fatal(err)
	}

	task := machineAddressDataTask()
	task.Steps[0].Limit = 1
	runMachineAddressTask(ctx, t, e, task)

	for name, id := range map[string]ids.UUID{"receipts@": receipts, "signing-noreply@": noreply, "d+token@": docsReply} {
		if got := machineTagged(t, e, id); got != 1 {
			t.Errorf("%s contact carries the tag %d times, want once", name, got)
		}
	}
	if got := machineTagged(t, e, anna); got != 0 {
		t.Errorf("a human whose name holds a marker word was tagged")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM tag WHERE name = $1`, machineAddressTag); got != 1 {
		t.Errorf("tags named %s = %d, want the user's one reused", machineAddressTag, got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM contact WHERE archived_at IS NOT NULL`); got != 0 {
		t.Errorf("the task archived %d contacts, want none", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'tag' AND after ? 'applied' AND evidence ? $1`, machineAddressTaggedBy); got != 3 {
		t.Errorf("tagging audit rows = %d, want one per tagged contact, each naming the run", got)
	}

	step := task.Steps[0]
	in := datatask.BatchIn{Limit: 2, RunID: ids.NewV7()}
	for {
		var out datatask.BatchOut
		err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error { var err error; out, err = step.Batch(ctx, tx, in); return err })
		if err != nil {
			t.Fatal(err)
		}
		if out.Changed != 0 {
			t.Fatalf("a replay tagged %d contacts again", out.Changed)
		}
		if out.Done {
			break
		}
		in.Cursor = out.Cursor
	}
}

// A user who archived the word gets it back with the records filed under it,
// rather than a second tag of the same name.
func TestTheMachineAddressTaskRestoresAnArchivedTagOfItsName(t *testing.T) {
	e := integration.Setup(t)
	conn := integration.OwnerConn(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	receipts := machineAddressContact(t, e, conn, "Expense Receipts", "receipts@machine-task.test")
	if _, err := conn.Exec(ctx, `INSERT INTO tag (name, archived_at) VALUES ($1, now())`, machineAddressTag); err != nil {
		t.Fatal(err)
	}

	runMachineAddressTask(ctx, t, e, machineAddressDataTask())

	if got := e.WsCount(t, `SELECT count(*) FROM tag WHERE name = $1`, machineAddressTag); got != 1 {
		t.Errorf("tags named %s = %d, want the archived one restored", machineAddressTag, got)
	}
	if got := machineTagged(t, e, receipts); got != 1 {
		t.Errorf("the receipts@ contact carries the restored tag %d times, want once", got)
	}
}

func TestTheMachineAddressTaskRollsItsTagsBackWithItsProgress(t *testing.T) {
	e := integration.Setup(t)
	conn := integration.OwnerConn(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	receipts := machineAddressContact(t, e, conn, "Expense Receipts", "receipts@machine-task.test")

	task := machineAddressDataTask()
	original := task.Steps[0].Batch
	task.Steps[0].Batch = func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		out, err := original(ctx, tx, in)
		if err != nil {
			return out, err
		}
		return out, errors.New("transaction interrupted after domain writes")
	}
	runMachineAddressTask(ctx, t, e, task)

	if got := machineTagged(t, e, receipts); got != 0 {
		t.Errorf("a tagging survived the rolled-back batch")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM tag WHERE name = $1`, machineAddressTag); got != 0 {
		t.Errorf("the tag survived the rolled-back batch")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'tag'`); got != 0 {
		t.Errorf("tag audit rows escaped the rollback: %d", got)
	}
	if _, err := datatask.UUIDCursor([]byte(`"not-a-uuid"`)); err == nil {
		t.Error("a malformed cursor was read as a position")
	}
}

// An installation that finished version 1 keeps its ledger row. Only a higher
// version restarts the cursor, so the contacts the first run missed get tagged.
func TestTheMachineAddressTaskRunsAgainOverAFinishedFirstVersion(t *testing.T) {
	e := integration.Setup(t)
	conn := integration.OwnerConn(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")

	finished := machineAddressDataTask()
	finished.Version = 1
	runMachineAddressTask(ctx, t, e, finished)
	token := machineAddressContact(t, e, conn, "D", "d+"+strings.Repeat("aorgb9", 34)+"@docs.google.com")
	if got := machineTagged(t, e, token); got != 0 {
		t.Fatalf("contact tagged before the second run: %d", got)
	}

	runMachineAddressTask(ctx, t, e, machineAddressDataTask())
	if got := machineTagged(t, e, token); got != 1 {
		t.Errorf("token-address contact carries the tag %d times after the version bump, want once", got)
	}
}
