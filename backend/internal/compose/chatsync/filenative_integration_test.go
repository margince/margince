// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package chatsync

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

type chosenSummaryBoundary struct {
	nativeSummaryBoundary
	after func()
}

func (s *chosenSummaryBoundary) Complete(ctx context.Context, request model.Request) (model.Response, error) {
	response, err := s.nativeSummaryBoundary.Complete(ctx, request)
	if s.after != nil {
		s.after()
	}
	return response, err
}

func TestNativeOneOffFilingKeepsTheSelectedScopeAndRefusesChangedSources(t *testing.T) {
	e := setupFlush(t)
	const root = "1794301200.000001"
	const reply = "1794301200.000002"
	changed, denied := false, false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var response any
		switch r.URL.Path {
		case "/conversations.info":
			response = map[string]any{"ok": true, "channel": map[string]any{"id": "C_ONE", "name": "Pilot", "is_member": true, "is_private": true}}
		case "/conversations.members":
			response = map[string]any{"ok": true, "members": []string{"U_REP", "U_BOT"}}
		case "/conversations.replies":
			text := "Legal has not approved the pilot."
			if changed {
				text = "Legal approved the amended agreement."
			}
			response = map[string]any{"ok": true, "messages": []any{map[string]any{"ts": root, "user": "U_REP", "text": "The pilot requires Legal approval."}, map[string]any{"ts": reply, "thread_ts": root, "user": "U_REP", "text": text}}}
		case "/users.info":
			response = map[string]any{"ok": true, "user": map[string]any{"id": r.URL.Query().Get("user"), "team_id": "T_ONE", "is_bot": r.URL.Query().Get("user") == "U_BOT", "real_name": "Rita Rep", "profile": map[string]string{"email": "rep@chat.test"}}}
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	base, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	var connection ids.UUID
	if err := e.owner.QueryRow(context.Background(), `SELECT connection_id FROM chat_link WHERE id=$1`, e.link).Scan(&connection); err != nil {
		t.Fatal(err)
	}
	e.exec(t, `UPDATE channel_connection SET provider='slack',channel_id='T_ONE',address=NULL WHERE id=$1`, connection)
	e.exec(t, `UPDATE chat_link SET provider='slack',channel_slug='C_ONE',root_id=$2,read_disabled=true WHERE id=$1`, e.link, root)
	now := linkedAt.Add(48 * time.Hour)
	bot := pacedSlackFixture(t, routedSlackHTTP{server.Client(), base}, "synthetic", "T_ONE", "U_BOT", &now)
	sessions := nativeRootSessions{chatlinks.Session{Provider: "slack", ConnectionID: connection, TeamID: "T_ONE", Chat: bot}}
	lane := &chosenSummaryBoundary{summary: "Legal has not approved the pilot; starting it remains conditional on approval."}
	filer := &Filer{Pool: e.pool, Store: e.store, Chats: sessions, Activities: activities.NewStore(e.db()), Contacts: contacts.NewStore(e.db()), Lane: lane, Reauthorize: func(ctx context.Context) (context.Context, error) {
		if denied {
			return nil, apperrors.ErrPermissionDenied
		}
		return ctx, nil
	}}
	member := e.member()
	actor, _ := principal.Actor(member)
	actor.SeatType = principal.SeatFull
	actor.Permissions.Objects["activity"] = principal.ObjectGrant{Create: true, Read: true}
	member = principal.WithActor(member, actor)
	reference := "https://fixture.slack.com/archives/C_ONE/p1794301200000002?thread_ts=" + root
	selected, err := filer.FileMessage(member, "deal", e.deal, reference, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(lane.requests) != 1 || strings.Contains(lane.requests[0].Messages[0].Content, "The pilot requires Legal approval.") || !strings.Contains(lane.requests[0].Messages[0].Content, "Legal has not approved") {
		t.Fatalf("saved the wrong scope: %+v", lane.requests)
	}
	whole, err := filer.FileMessage(member, "deal", e.deal, reference, true)
	if err != nil || whole.Id == selected.Id {
		t.Fatalf("whole thread replaced selected message: %v", err)
	}
	if !strings.Contains(lane.requests[1].Messages[0].Content, "The pilot requires Legal approval.") {
		t.Fatal("thread root omitted")
	}
	again, err := filer.FileMessage(member, "deal", e.deal, reference, false)
	if err != nil || again.Id != selected.Id {
		t.Fatalf("selection replay duplicated note: %v", err)
	}
	lane.after = func() { changed = true }
	if _, err := filer.FileMessage(member, "deal", e.deal, reference, false); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("filed stale source: %v", err)
	}
	changed = false
	lane.after = func() { denied = true }
	if _, err := filer.FileMessage(member, "deal", e.deal, reference, false); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("returned a receipt after authority narrowed: %v", err)
	}
	var count int
	if err := e.owner.QueryRow(context.Background(), `SELECT count(*) FROM activity WHERE source_system=$1 AND source=$2`, provenance.ChatSummarySource, provenance.RecordSourceAutomation).Scan(&count); err != nil || count != 2 {
		t.Fatalf("unexpected saved selections: %d %v", count, err)
	}
	if err := e.store.Unlink(member, "deal", e.deal); err != nil {
		t.Fatal(err)
	}
	state, err := e.store.GetState(member, "deal", e.deal)
	if err != nil || state.Link != nil || state.LastSync == nil || state.LastSync.Provider != "slack" {
		t.Fatalf("unlink lost its ended association: %+v %v", state, err)
	}
	if _, err := filer.Activities.GetActivity(member, ids.From[ids.ActivityKind](ids.UUID(selected.Id)), storekit.LiveOnly); err != nil {
		t.Fatalf("unlink removed eligible saved history: %v", err)
	}
}
