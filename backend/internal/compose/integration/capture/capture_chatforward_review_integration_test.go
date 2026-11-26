// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/pendingforward"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// heldForwardOlderThanItsWindow files a held forward for the member behind
// Telegram account 5001 and ages it past its window.
func heldForwardOlderThanItsWindow(t *testing.T, e *integration.SearchEnv, id string) string {
	t.Helper()
	verifyTelegram(t, e, e.Rep1, "5001")
	sink := compose.NewCaptureSink(e.Pool, compose.CaptureConfig{})
	ref, err := capturemod.FileChatMessage(workspaceConnectorCtx(e), sink, forwardBinding, forwardedMessage(id, &connector.ChatForward{}))
	if err != nil {
		t.Fatal(err)
	}
	wsExec(t, e, `UPDATE activity SET created_at = now() - $2::interval WHERE id = $1`, ref.ID, (pendingforward.Window + time.Hour).String())
	return ref.ID.String()
}

func TestTheTraceSweepAsWiredExpiresHeldForwardsWithAndWithoutAnObjectStore(t *testing.T) {
	for name, store := range map[string]blobstore.Store{"no object store": nil, "object store": blobstore.NewMemory()} {
		t.Run(name, func(t *testing.T) {
			e := newCaptureEnv(t).e
			old := heldForwardOlderThanItsWindow(t, e, "9")
			integration.ApplyRiverSchema(t)
			runner, err := compose.NewJobRunner(e.Pool, slog.New(slog.NewTextHandler(io.Discard, nil)), compose.JobRunnerConfig{Blobstore: store})
			if err != nil {
				t.Fatal(err)
			}
			done, cancelSub := runner.SubscribeCompleted()
			defer cancelSub()
			if err := runner.Start(context.Background()); err != nil {
				t.Fatal(err)
			}
			defer func() {
				stop, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if err := runner.Stop(stop); err != nil {
					t.Errorf("Stop: %v", err)
				}
			}()
			if err := runner.Enqueue(context.Background(), compose.CaptureTraceSweepArgs{}, nil); err != nil {
				t.Fatal(err)
			}
			wait, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			integration.AwaitKindCompleted(wait, t, done, compose.CaptureTraceSweepArgs{}.Kind())
			if n := wsCount(t, e, `SELECT count(*) FROM activity WHERE id = $1 AND body IS NOT NULL`, old); n != 0 {
				t.Fatal("the worker left an expired held forward readable")
			}
		})
	}
}

func TestAForwardFiledOnAContactAfterItWasSelectedForExpiryIsKept(t *testing.T) {
	e := newCaptureEnv(t).e
	held := heldForwardOlderThanItsWindow(t, e, "10")
	sink := compose.NewCaptureSink(e.Pool, compose.CaptureConfig{})
	author := &connector.ChatForward{Author: &connector.ChatParty{UserID: "6002", DisplayName: "Anna Weber"}}
	filed, err := capturemod.FileChatMessage(workspaceConnectorCtx(e), sink, forwardBinding, forwardedMessage("11", author))
	if err != nil {
		t.Fatal(err)
	}
	var subject capturemod.PurgeSubject
	if err := database.WithWorkspaceTx(workspaceConnectorCtx(e), e.Pool, func(tx pgx.Tx) (err error) {
		subject, err = capturemod.SelectExpiredForwardsTx(context.Background(), tx)
		return err
	}); err != nil || len(subject.SoleImports) != 1 {
		t.Fatalf("selected %d forwards, %v; want the held one", len(subject.SoleImports), err)
	}
	wsExec(t, e, `INSERT INTO activity_link (activity_id, entity_type, contact_id)
		SELECT $1, 'contact', contact_id FROM activity_link WHERE activity_id = $2`, held, filed.ID)

	purger := compose.NewCapturePurger(e.Pool, compose.NewRetentionServiceFor(compose.InstallationDB(e.Pool), nil, slog.Default()))
	if err := purger.DestroyExpiredForwards(principal.SystemActing(workspaceConnectorCtx(e), "system:test-forward-expiry"), subject); err != nil {
		t.Fatal(err)
	}
	if n := wsCount(t, e, `SELECT count(*) FROM activity WHERE id = $1 AND body IS NOT NULL`, held); n != 1 {
		t.Fatal("a forward the member filed on a contact was erased")
	}
}

func TestAForwardOfAColleaguesMessageIsHeldAndMakesNoContactForTheColleague(t *testing.T) {
	e := newCaptureEnv(t).e
	verifyTelegram(t, e, e.Rep1, "5001")
	wsExec(t, e, `UPDATE app_user SET chat_telegram_user_id = '7001', chat_telegram_verified_at = now() WHERE id = $1`, e.Rep3)
	sink := compose.NewCaptureSink(e.Pool, compose.CaptureConfig{})
	colleague := &connector.ChatForward{Author: &connector.ChatParty{UserID: "7001", DisplayName: "Rep Three"}}
	ref, err := capturemod.FileChatMessage(workspaceConnectorCtx(e), sink, forwardBinding, forwardedMessage("12", colleague))
	if err != nil {
		t.Fatal(err)
	}
	if n := wsCount(t, e, `SELECT count(*) FROM contact_channel_identity WHERE channel_user_id = '7001'`); n != 0 {
		t.Fatalf("the colleague was bound to %d contacts", n)
	}
	if got := wsScalar(t, e, `SELECT audience || '/' || audience_reason FROM activity WHERE id = $1`, ref.ID); got != "participants/no_counterparty" {
		t.Fatalf("hold = %s", got)
	}
}
