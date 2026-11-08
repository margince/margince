// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

// A role taken away after the first check still stops the write. Each test's
// registry authority lets the member capture, as the check before the
// transaction saw it. Only the read inside the write sees the read seat.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// demotedInTx answers the read seat inside a transaction and capture rights
// outside one.
type demotedInTx struct{ fixtureAuthority }

func (a demotedInTx) EffectiveAuthority(ctx context.Context, ws, user ids.UUID) (authz.RBAC, principal.SeatType, error) {
	rbac, err := a.EffectiveRBAC(ctx, ws, user)
	return rbac, principal.SeatFull, err
}

func (demotedInTx) EffectiveAuthorityTx(context.Context, pgx.Tx, ids.UUID) (authz.RBAC, principal.SeatType, error) {
	return authz.RBAC{}, principal.SeatRead, nil
}

func (a demotedInTx) HeldAuthorityTx(ctx context.Context, tx pgx.Tx, user ids.UUID) (authz.RBAC, principal.SeatType, error) {
	return a.EffectiveAuthorityTx(ctx, tx, user)
}

// revokedSourceID keys the one message upsertingConnector syncs.
var revokedSourceID = "revoked-mid-run-" + ids.NewV7().String()

// upsertingConnector is the fixture connector whose sync lands one message.
type upsertingConnector struct{ fixtureConnector }

func (upsertingConnector) Sync(ctx context.Context, _ connector.Auth, cursor connector.Cursor, sink connector.Sink) (connector.Cursor, error) {
	rec := mailRecord(revokedSourceID, "buyer@customer.example", "buyer@customer.example")
	rec.CapturedBy, rec.Source = "connector:gmail", "gmail:"+revokedSourceID
	if _, err := sink.Upsert(ctx, rec); err != nil {
		return cursor, err
	}
	return cursor, nil
}

type countedVault interface {
	keyvault.Vault
	Count() int
}

func TestAConnectRefusedInsideItsTransactionLeavesNoRowAndNoCredential(t *testing.T) {
	ctx, _, _, ws := newCaptureRegistryFixture(t)
	_, pool := setupCaptureDB(t)
	db := database.BindTo(pool, ws)
	vault, ok := keyvault.NewMemory().(countedVault)
	if !ok {
		t.Fatal("the memory vault no longer counts its secrets")
	}
	reg := capture.NewRegistry(db, capture.NewSink(db).WithPartyAuthority(demotedInTx{}), fixtureAuthority{}, vault)
	reg.Register(fixtureConnector{})

	_, err := reg.Connect(ctx, "gmail", connector.Auth("fixture-token"))

	if _, refused := errors.AsType[*capture.RoleCannotCaptureError](err); !refused {
		t.Fatalf("Connect = %v, want the role refusal", err)
	}
	if n := vault.Count(); n != 0 {
		t.Errorf("the refused connect left %d sealed credentials, want none", n)
	}
	actor, _ := principal.Actor(ctx)
	owner, _ := setupCaptureDB(t)
	var rows int
	if err := owner.QueryRow(ctx, `SELECT count(*) FROM capture_connection WHERE user_id = $1`, actor.UserID).Scan(&rows); err != nil {
		t.Fatalf("counting connections: %v", err)
	}
	if rows != 0 {
		t.Errorf("the refused connect wrote %d connection rows, want none", rows)
	}
}

func TestASyncWhoseRoleIsTakenAwayMidRunWritesNoMessage(t *testing.T) {
	ctx, connecting, vault, ws := newCaptureRegistryFixture(t)
	id, err := connecting.Connect(ctx, "gmail", connector.Auth("fixture-token"))
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	_, pool := setupCaptureDB(t)
	db := database.BindTo(pool, ws)
	reg := capture.NewRegistry(db, capture.NewSink(db).WithPartyAuthority(demotedInTx{}), fixtureAuthority{}, vault)
	reg.Register(upsertingConnector{})

	err = reg.SyncOnce(principal.WithWorkspaceID(context.Background(), ws.UUID), id)

	if _, refused := errors.AsType[*capture.RoleCannotCaptureError](err); !refused {
		t.Fatalf("SyncOnce = %v, want the role refusal from the write", err)
	}
	owner, _ := setupCaptureDB(t)
	var activities int
	if err := owner.QueryRow(ctx, `SELECT count(*) FROM activity WHERE source_id = $1`, revokedSourceID).Scan(&activities); err != nil {
		t.Fatalf("counting activities: %v", err)
	}
	if activities != 0 {
		t.Errorf("the run wrote %d activities under the revoked role, want none", activities)
	}
}

// verbConnector is the fixture connector whose sync acts on a stored record
// instead of capturing one.
type verbConnector struct {
	fixtureConnector
	act func(context.Context, connector.Sink) error
}

func (c verbConnector) Sync(ctx context.Context, _ connector.Auth, cursor connector.Cursor, sink connector.Sink) (connector.Cursor, error) {
	return cursor, c.act(ctx, sink)
}

func TestARunWhoseRoleIsTakenAwayCancelsMovesAndRemovesNothing(t *testing.T) {
	key := connector.NaturalKey{SourceSystem: "gmail", SourceID: "stored-" + ids.NewV7().String()}
	for name, act := range map[string]func(context.Context, connector.Sink) error{
		"cancel a meeting": func(ctx context.Context, sink connector.Sink) error {
			canceller, ok := sink.(connector.MeetingCanceller)
			if !ok {
				return errors.New("the sink cannot cancel")
			}
			return canceller.CancelMeeting(ctx, key, clocktest.Now(t))
		},
		"move a meeting": func(ctx context.Context, sink connector.Sink) error {
			mover, ok := sink.(connector.MeetingMover)
			if !ok {
				return errors.New("the sink cannot move")
			}
			return mover.MoveMeeting(ctx, key, connector.MeetingSlot{Start: clocktest.Now(t)})
		},
		"remove a message": func(ctx context.Context, sink connector.Sink) error {
			remover, ok := sink.(connector.MessageRemover)
			if !ok {
				return errors.New("the sink cannot remove")
			}
			return remover.RemoveMessage(ctx, key)
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, connecting, vault, ws := newCaptureRegistryFixture(t)
			id, err := connecting.Connect(ctx, "gmail", connector.Auth("fixture-token"))
			if err != nil {
				t.Fatalf("Connect: %v", err)
			}
			_, pool := setupCaptureDB(t)
			db := database.BindTo(pool, ws)
			wrote := 0
			sink := capture.NewSink(db).WithPartyAuthority(demotedInTx{}).
				WithMeetingCloser(func(context.Context, pgx.Tx, connector.NaturalKey, time.Time) (ids.ActivityID, bool, error) {
					wrote++
					return ids.ActivityID{}, false, nil
				}, nil).
				WithMeetingMover(func(context.Context, pgx.Tx, ids.ActivityID, connector.MeetingSlot) (bool, error) {
					wrote++
					return false, nil
				}).
				WithMessagePurger(func(ctx context.Context, admit func(context.Context, pgx.Tx) error, _ ids.UUID, _, _ string) error {
					if err := db.Tx(ctx, func(tx pgx.Tx) error { return admit(ctx, tx) }); err != nil {
						return err
					}
					wrote++
					return nil
				})
			reg := capture.NewRegistry(db, sink, fixtureAuthority{}, vault)
			reg.Register(verbConnector{act: act})

			err = reg.SyncOnce(principal.WithWorkspaceID(context.Background(), ws.UUID), id)

			if _, refused := errors.AsType[*capture.RoleCannotCaptureError](err); !refused {
				t.Fatalf("SyncOnce = %v, want the role refusal", err)
			}
			if wrote != 0 {
				t.Errorf("the revoked run reached the writer %d times, want never", wrote)
			}
		})
	}
}
