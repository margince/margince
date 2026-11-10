// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type mailboxScopeEnv struct {
	ctx               context.Context
	tx                pgx.Tx
	seat, first, next ids.UUID
}

func withMailboxScope(t *testing.T, check func(mailboxScopeEnv)) {
	t.Helper()
	ctx, pool := meetingTenureEnv(t)
	f := mailboxScopeEnv{seat: ids.NewV7(), first: ids.NewV7(), next: ids.NewV7()}
	by := "human:" + f.seat.String()
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: by, UserID: f.seat,
		Permissions: principal.Permissions{RoleKeys: []string{"admin"}, RowScope: principal.RowScopeAll},
	})
	if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO app_user (id,email,display_name)
 VALUES (@user,@email,'Mailbox owner')`, pgx.StrictNamedArgs{
			"user": f.seat, "email": "mailbox-" + f.seat.String() + "@example.test",
		}); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO legal_entity (id,display_name,legal_name,country,captured_by)
 VALUES (@first,@first_name,'Original Ltd','at',@by),(@next,@next_name,'Next Ltd','at',@by)`, pgx.StrictNamedArgs{
			"first": f.first, "first_name": "Original " + f.first.String(),
			"next": f.next, "next_name": "Next " + f.next.String(), "by": by,
		}); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO legal_entity_member (user_id,legal_entity_id,assigned_by)
 VALUES (@user,@entity,@by)`, pgx.StrictNamedArgs{"user": f.seat, "entity": f.first, "by": by}); err != nil {
			return err
		}
		f.ctx, f.tx = ctx, tx
		check(f)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func entityMailbox(t *testing.T, f mailboxScopeEnv, label string) ids.UUID {
	t.Helper()
	id, _, err := upsertConnection(f.ctx, f.tx, connectionUpsert{
		userID: f.seat, provider: "gmail", scopes: []string{},
		ref: keyvault.Ref("fixture-credential"), accountLabel: &label,
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func moveMailboxOwner(t *testing.T, f mailboxScopeEnv) {
	t.Helper()
	if _, err := f.tx.Exec(f.ctx, `UPDATE legal_entity_member SET legal_entity_id=@entity
 WHERE user_id=@user`, pgx.StrictNamedArgs{"entity": f.next, "user": f.seat}); err != nil {
		t.Fatal(err)
	}
}

func expectMailboxEntity(t *testing.T, f mailboxScopeEnv, id, want ids.UUID) {
	t.Helper()
	var got *ids.UUID
	if err := f.tx.QueryRow(f.ctx, `SELECT legal_entity_id FROM capture_connection WHERE id=@id`,
		pgx.StrictNamedArgs{"id": id}).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if want.IsZero() {
		if got != nil {
			t.Fatalf("legacy mailbox scope = %v, want unknown", got)
		}
	} else if got == nil || *got != want {
		t.Fatalf("mailbox entity = %v, want %s", got, want)
	}
}

func TestAMailboxKeepsItsCompanyUntilAnotherAccountIsBound(t *testing.T) {
	withMailboxScope(t, func(f mailboxScopeEnv) {
		id := entityMailbox(t, f, "first@example.test")
		expectMailboxEntity(t, f, id, f.first)
		moveMailboxOwner(t, f)
		if same := entityMailbox(t, f, "first@example.test"); same != id {
			t.Fatal("reauth created another connection")
		}
		expectMailboxEntity(t, f, id, f.first)
		entityMailbox(t, f, "second@example.test")
		expectMailboxEntity(t, f, id, f.next)
		if _, err := f.tx.Exec(f.ctx, `UPDATE capture_connection SET legal_entity_id=NULL WHERE id=@id`,
			pgx.StrictNamedArgs{"id": id}); err != nil {
			t.Fatal(err)
		}
		entityMailbox(t, f, "second@example.test")
		expectMailboxEntity(t, f, id, ids.Nil)
	})
}

func scopedReceipt(t *testing.T, f mailboxScopeEnv, activity ids.UUID, message string) {
	t.Helper()
	when := time.Date(2020, time.March, 2, 9, 0, 0, 0, time.UTC)
	if err := recordImportTx(f.ctx, f.tx, ids.From[ids.ActivityKind](activity), f.seat, "gmail",
		birthDecision{}, when, when, nil, message); err != nil {
		t.Fatal(err)
	}
}

func expectReceiptEntity(t *testing.T, f mailboxScopeEnv, activity, want ids.UUID, deliveries int) {
	t.Helper()
	var got *ids.UUID
	var count int
	if err := f.tx.QueryRow(f.ctx, `SELECT legal_entity_id,cardinality(delivered_by)
 FROM capture_import WHERE activity_id=@activity AND user_id=@user`,
		pgx.StrictNamedArgs{"activity": activity, "user": f.seat}).Scan(&got, &count); err != nil {
		t.Fatal(err)
	}
	if got == nil || *got != want || count != deliveries {
		t.Fatalf("receipt entity = %v, deliveries = %d; want %s, %d", got, count, want, deliveries)
	}
}

func mailboxThread(t *testing.T, f mailboxScopeEnv) ids.UUID {
	t.Helper()
	contact := seedThread(f.ctx, t, f.tx, seatThread{
		kind: "email", audience: "participants", seatEmail: "mailbox-" + f.seat.String() + "@example.test",
	})
	var activity ids.UUID
	if err := f.tx.QueryRow(f.ctx, `SELECT activity_id FROM activity_participant WHERE contact_id=@contact`,
		pgx.StrictNamedArgs{"contact": contact}).Scan(&activity); err != nil {
		t.Fatal(err)
	}
	return activity
}

func TestBackfilledMailKeepsTheMailboxCompanyAfterItsOwnerMoves(t *testing.T) {
	withMailboxScope(t, func(f mailboxScopeEnv) {
		connection := entityMailbox(t, f, "first@example.test")
		if _, err := f.tx.Exec(f.ctx, `UPDATE capture_connection SET legal_entity_id=@entity WHERE id=@id`,
			pgx.StrictNamedArgs{"entity": f.first, "id": connection}); err != nil {
			t.Fatal(err)
		}
		moveMailboxOwner(t, f)
		first := mailboxThread(t, f)
		scopedReceipt(t, f, first, "first-receipt")
		expectReceiptEntity(t, f, first, f.first, 1)
		entityMailbox(t, f, "second@example.test")
		scopedReceipt(t, f, first, "replayed-receipt")
		expectReceiptEntity(t, f, first, f.first, 2)
		next := mailboxThread(t, f)
		scopedReceipt(t, f, next, "second-receipt")
		expectReceiptEntity(t, f, next, f.next, 1)
	})
}
