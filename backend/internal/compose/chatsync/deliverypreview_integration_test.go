// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package chatsync

import (
	"context"
	"errors"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/modules/chatlinks/slack"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func assertNativeOutboundPreview(t *testing.T, e *flushEnv, lane *outboundModelBoundary, server *httptest.Server) {
	t.Helper()
	base, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	vault := keyvault.NewMemory()
	e.store.WithMemberAuthority(func(ctx context.Context, _ pgx.Tx) (context.Context, error) { return ctx, nil })
	e.store.WithMessenger(func() keyvault.Vault { return vault }, func(_ context.Context, grant chatlinks.MemberGrant, raw []byte) (messaging.Client, error) {
		return slack.New(routedSlackHTTP{server.Client(), base}, string(raw), grant.Workspace, grant.Member), nil
	})
	human := e.member()
	actor, _ := principal.Actor(human)
	actor.SeatType = principal.SeatFull
	human = principal.WithActor(human, actor)
	if _, err := e.store.ConnectMember(human, chatlinks.MemberGrant{Provider: "slack", Workspace: "T_NATIVE", Member: "U_REP"}, []byte("synthetic-personal")); err != nil {
		t.Fatal(err)
	}
	filer := Filer{Pool: e.pool, Store: e.store, Chats: e.flusher.Chats, Lane: lane, Activities: activities.NewStore(e.db())}
	preview, err := filer.PreviewDelivery(human, "deal", e.deal)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Audience) != 1 || preview.Audience[0].Id != "U_REP" || ids.UUID(preview.LinkId) != e.link || !strings.Contains(preview.OverviewText, "Acme renewal") || !strings.Contains(preview.UpdateText, "Legal has not approved") || preview.PendingUpdates != 1 {
		t.Fatalf("preview omitted real audience/content: %+v", preview)
	}
	var changed int
	if err := e.owner.QueryRow(context.Background(), `SELECT count(*) FROM chat_post_item WHERE posted_at IS NOT NULL OR withdrawn_at IS NOT NULL OR retry_at IS NOT NULL`).Scan(&changed); err != nil || changed != 0 {
		t.Fatalf("preview changed delivery queue: %d %v", changed, err)
	}
	stale := ids.NewV7()
	yes := true
	if _, err := e.store.SetSyncOptions(human, "deal", e.deal, chatlinks.SyncOptions{Post: &yes, ExpectedLink: &stale, ExpectedVersion: &preview.Version}); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("preview enabled a replacement link: %v", err)
	}
	lane.beforeReply = func() { e.exec(t, `UPDATE chat_link SET message_detail='brief' WHERE id=$1`, e.link) }
	if _, err := filer.PreviewDelivery(human, "deal", e.deal); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("preview returned stale settings after model: %v", err)
	}
	lane.beforeReply = nil
	e.exec(t, `UPDATE chat_link SET message_detail='full' WHERE id=$1`, e.link)
}
