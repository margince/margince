// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package chatsync

import (
	"context"
	"errors"
	"testing"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/dispact"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// repCtx is the deal's owner, who may link and unlink every deal.
func (e *pullEnv) repCtx() context.Context {
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.ws), ids.NewV7())
	grant := principal.ObjectGrant{Read: true, Update: true}
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"}, RowScope: principal.RowScopeAll,
			Objects: map[string]principal.ObjectGrant{chatlinks.RecordDeal: grant, chatlinks.RecordProject: grant},
		},
	})
}

// secondDeal adds a deal like the test deal, under another name.
func (e *pullEnv) secondDeal(t *testing.T, name string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	e.exec(t, `INSERT INTO deal (id, name, status, owner_id, pipeline_id, stage_id, source, captured_by)
		SELECT $1, $2, status, owner_id, pipeline_id, stage_id, source, captured_by FROM deal WHERE id = $3`, id, name, e.deal)
	return id
}

func (e *pullEnv) linkStatus(t *testing.T) string {
	t.Helper()
	var status string
	if err := e.owner.QueryRow(context.Background(), `SELECT status FROM chat_link WHERE id = $1`, e.link).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func TestAChannelArchivedWithoutANoticePausesItsLinkOnThePull(t *testing.T) {
	e := setupPull(t)
	e.say("acme", 1, 60, "Acme wants 12% off")
	e.history.Archive("acme")
	e.pull(t)

	if got := e.linkStatus(t); got != chatlinks.StatusChannelGone {
		t.Fatalf("link status = %q, want %q: the chat archived the channel", got, chatlinks.StatusChannelGone)
	}
	if n := len(e.notes(t)); n != 0 {
		t.Errorf("an archived channel filed %d notes", n)
	}
	state, err := e.store.GetState(e.repCtx(), chatlinks.RecordDeal, e.deal)
	if err != nil || state.State != chatlinks.StatePaused {
		t.Fatalf("the deal's channel = %+v, %v; want it shown paused", state, err)
	}
}

func TestADealThatTakesTheChannelOverFilesItsOwnNoteOfTheTalkSoFar(t *testing.T) {
	e := setupPull(t)
	threeDaysBefore := -3 * 24 * 60
	e.say("acme", 1, threeDaysBefore, "Maya Brandt asked for 8 percent")
	e.say("acme", 2, threeDaysBefore+5, "We can do 6 percent if they sign this month")
	e.exec(t, `UPDATE chat_link SET backfill_requested = true WHERE id = $1`, e.link)
	e.pull(t)
	if n := len(e.notes(t)); n != 1 {
		t.Fatalf("the first deal has %d notes of the talk so far, want 1", n)
	}

	yes := true
	if err := e.store.Unlink(e.repCtx(), chatlinks.RecordDeal, e.deal); err != nil {
		t.Fatal(err)
	}
	taker := e.secondDeal(t, "Brandt renewal")
	if _, err := e.store.Link(e.repCtx(), chatlinks.RecordDeal, taker,
		chatlinks.LinkRequest{Mode: chatlinks.ModeExisting, Channel: "acme", SummarizeHistory: &yes}); err != nil {
		t.Fatal(err)
	}
	e.pull(t)

	var key string
	err := e.owner.QueryRow(context.Background(), `SELECT a.source_id FROM activity a
		JOIN activity_link l ON l.activity_id = a.id AND l.deal_id = $1
		WHERE a.source_system = 'dispact'`, taker).Scan(&key)
	if err != nil || key != "acme:1:deal:"+taker.String() {
		t.Fatalf("the taking deal's note = %q, %v; want its own note of the talk so far", key, err)
	}
	if n := len(e.notes(t)); n != 1 {
		t.Errorf("the first deal has %d notes, want its own one kept", n)
	}
}

// hookedChats runs before on the first history read of a pull.
type hookedChats struct {
	chatlinks.Chats
	before func()
}

type hookedChat struct {
	chatlinks.Chat
	before func()
}

func (h *hookedChats) Open(ctx context.Context) (chatlinks.Session, error) {
	session, err := h.Chats.Open(ctx)
	session.Chat = hookedChat{Chat: session.Chat, before: func() {
		if h.before != nil {
			before := h.before
			h.before = nil
			before()
		}
	}}
	return session, err
}

func (h hookedChat) History(ctx context.Context, slug string, after int64, limit int) (dispact.HistoryPage, error) {
	h.before()
	return h.Chat.History(ctx, slug, after, limit)
}

func TestALinkAskingForTheTalkSoFarDuringAPullRunsThePullOnceMore(t *testing.T) {
	e := setupPull(t)
	worker := &PullWorker[pullArgs]{Bind: bindAny, Puller: e.puller}
	job := &river.Job[pullArgs]{Args: pullArgs{conn: e.conn}}
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	if err := worker.Work(ctx, job); err != nil {
		t.Fatalf("a pull with no new link = %v, want it to end", err)
	}

	late := e.secondDeal(t, "Late renewal")
	e.chat.AddChannel("late")
	e.history.AddChannel("late")
	yes := true
	e.puller.Chats = &hookedChats{Chats: e.puller.Chats, before: func() {
		if _, err := e.store.Link(e.repCtx(), chatlinks.RecordDeal, late,
			chatlinks.LinkRequest{Mode: chatlinks.ModeExisting, Channel: "late", SummarizeHistory: &yes}); err != nil {
			t.Errorf("linking during the pull: %v", err)
		}
	}}
	var snooze *river.JobSnoozeError
	if err := worker.Work(ctx, job); !errors.As(err, &snooze) {
		t.Fatalf("a pull that a new link's own pull folded into = %v, want it to run once more", err)
	}
}
