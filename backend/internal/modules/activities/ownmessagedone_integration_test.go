// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func ownMessageDoneOf(ctx context.Context, t *testing.T, e *loadEnv, message ids.UUID) OwnMessageDone {
	t.Helper()
	var standing OwnMessageDone
	if err := database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		var err error
		standing, err = handledStore(e).OwnMessageDoneState(ctx, tx, ids.From[ids.ActivityKind](message), clocktest.Now(t))
		return err
	}); err != nil {
		t.Fatalf("reading the standing on our message: %v", err)
	}
	return standing
}

// The page's answer follows the writer's. Before Done the rep may mark the
// message. After it the message is set aside for the rep and for nobody else.
func TestOwnMessageDoneStateFollowsTheDoneWrite(t *testing.T) {
	e := setupLoad(t)
	contact := e.customerAt(t, "customer")
	sent, _ := e.sentTo(t, contact, "", 2*24*time.Hour)
	rep, colleague := e.asSeat(e.rep), e.asSeat(e.other)
	if got := ownMessageDoneOf(rep, t, e, sent); got != (OwnMessageDone{CanMark: true}) {
		t.Fatalf("before Done the rep's standing is %+v, want markable and not set aside", got)
	}

	if err := handledStore(e).MarkMessageHandled(rep, ids.From[ids.ActivityKind](sent)); err != nil {
		t.Fatalf("marking our message done: %v", err)
	}

	if got := ownMessageDoneOf(rep, t, e, sent); !got.SetAside {
		t.Errorf("after Done the rep's standing is %+v, want set aside", got)
	}
	if got := ownMessageDoneOf(colleague, t, e, sent); got.SetAside {
		t.Errorf("the rep's Done set the message aside for a colleague too: %+v", got)
	}
}

// A reader the writer would refuse is never offered Done. That is one who may
// not read the message, a pass with no colleague behind it, and a colleague who
// did not send it.
func TestOwnMessageDoneStateRefusesWhomTheWriterRefuses(t *testing.T) {
	e := setupLoad(t)
	contact := e.customerAt(t, "customer")
	sent, _ := e.sentTo(t, contact, "", 2*24*time.Hour)
	e.exec(t, `UPDATE activity SET audience = 'participants' WHERE id = $1`, sent)
	system := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.ws),
		principal.Principal{Type: principal.PrincipalSystem, ID: "system:test"})
	for name, ctx := range map[string]context.Context{"a colleague outside the audience": e.asSeat(e.other), "the system": system} {
		if got := ownMessageDoneOf(ctx, t, e, sent); got.CanMark {
			t.Errorf("%s may mark our message: %+v", name, got)
		}
	}
	e.exec(t, `UPDATE activity SET audience = 'workspace' WHERE id = $1`, sent)
	if got := ownMessageDoneOf(e.asSeat(e.other), t, e, sent); got.CanMark {
		t.Errorf("a colleague who may read our message but did not send it may mark it: %+v", got)
	}
	if got := ownMessageDoneOf(e.asSeat(e.rep), t, e, sent); !got.CanMark {
		t.Error("the sender may not mark their own message, so this proves nothing")
	}
}
