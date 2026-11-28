// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package chatsync

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/modules/chatlinks/slack"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestOutcomeDraftRechecksPrivateMessagesAndNeverFilesThem(t *testing.T) {
	e := setupFlush(t)
	e.exec(t, `UPDATE channel_connection SET provider='slack',address=NULL`)
	e.exec(t, `UPDATE chat_link SET provider='slack'`)
	changed := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := ""
		switch r.URL.Path {
		case "/auth.test":
			response = `{"ok":true,"team_id":"T_NATIVE","user_id":"U_REP"}`
		case "/conversations.info":
			response = `{"ok":true,"channel":{"id":"C_PRIVATE","name":"Pilot","is_private":true,"is_member":true}}`
		case "/conversations.members":
			response = `{"ok":true,"members":["U_REP"]}`
		case "/users.info":
			response = `{"ok":true,"user":{"id":"U_REP","team_id":"T_NATIVE","real_name":"Rita Rep","profile":{"email":"rep@chat.test"}}}`
		case "/conversations.history":
			text := "Legal has not approved the pilot."
			if changed {
				text = "Legal approved the amended agreement."
			}
			response = `{"ok":true,"messages":[{"ts":"1794301200.000001","user":"U_REP","text":"` + text + `"}]}`
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if _, err := w.Write([]byte(response)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	base, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	vault := keyvault.NewMemory()
	e.store.WithMemberAuthority(func(ctx context.Context, _ pgx.Tx) (context.Context, error) { return ctx, nil })
	e.store.WithMessenger(func() keyvault.Vault { return vault }, func(_ context.Context, g chatlinks.MemberGrant, secret []byte) (messaging.Client, error) {
		return slack.New(routedSlackHTTP{server.Client(), base}, string(secret), g.Workspace, g.Member), nil
	})
	ctx := e.member()
	actor, _ := principal.Actor(ctx)
	actor.SeatType = principal.SeatFull
	ctx = principal.WithActor(ctx, actor)
	if _, err := e.store.ConnectMember(ctx, chatlinks.MemberGrant{Provider: "slack", Workspace: "T_NATIVE", Member: "U_REP"}, []byte("fixture")); err != nil {
		t.Fatal(err)
	}
	lane := &chosenSummaryBoundary{}
	lane.summary = "Legal has not approved the pilot."
	filer := NewFiler(e.pool, e.store, &LiveChats{DB: e.db()}, lane)
	selection := []string{"1794301200.000001"}
	draft, err := filer.DraftMessengerOutcome(ctx, "deal", e.deal, "slack", "C_PRIVATE", "", selection)
	if err != nil || !strings.Contains(draft.Body, "not approved") {
		t.Fatalf("draft=%+v error=%v", draft, err)
	}
	lane.after = func() { changed = true }
	if _, err := filer.DraftMessengerOutcome(ctx, "deal", e.deal, "slack", "C_PRIVATE", "", selection); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("changed private source accepted: %v", err)
	}
	var saved int
	if err := e.owner.QueryRow(context.Background(), `SELECT count(*) FROM activity WHERE subject NOT IN ('Shared plan','Private aside')`).Scan(&saved); err != nil || saved != 0 {
		t.Fatalf("draft filed private content: %d %v", saved, err)
	}
	if len(lane.requests) != 2 || !strings.Contains(lane.requests[0].Messages[0].Content, "not approved") {
		t.Fatalf("model did not receive the selected source: %+v", lane.requests)
	}
}
