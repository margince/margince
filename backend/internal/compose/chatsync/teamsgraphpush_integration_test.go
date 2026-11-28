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

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/modules/chatlinks/teams"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestTeamsGraphNoticesPinScopeAndRecoverMissedCoverageWithoutContent(t *testing.T) {
	const app = "018f3a1b-0000-7000-8000-00000000a001"
	const tenant = "018f3a1b-0000-7000-8000-00000000a002"
	const team = "018f3a1b-0000-7000-8000-00000000a003"
	e := setupFlush(t)
	var connection ids.UUID
	if err := e.owner.QueryRow(context.Background(), `SELECT connection_id FROM chat_link WHERE id=$1`, e.link).Scan(&connection); err != nil {
		t.Fatal(err)
	}
	e.exec(t, `UPDATE channel_connection SET provider='teams',channel_id=$2,address=NULL WHERE id=$1`, connection, tenant+"/"+team)
	channel := team + "/19:channel@thread.tacv2"
	e.exec(t, `UPDATE chat_link SET provider='teams',channel_slug=$2 WHERE id=$1`, e.link, channel)
	ctx := principal.WithCorrelationID(principal.WithActor(principal.WithWorkspaceID(context.Background(), e.ws), chatLinksPrincipal), ids.NewV7())
	vault := keyvault.NewMemory()
	e.store.WithVault(func() keyvault.Vault { return vault })
	links, err := e.store.PullLinks(ctx, connection)
	if err != nil || len(links) != 1 {
		t.Fatalf("links: %#v %v", links, err)
	}
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	watch, err := e.store.PrepareChannelWatch(ctx, links[0], app, tenant, "https://crm.example.test/v1/channel-events/teams/graph", now)
	if err != nil {
		t.Fatal(err)
	}
	again, err := e.store.PrepareChannelWatch(ctx, links[0], app, tenant, watch.URL, now)
	if err != nil || again.ID != watch.ID {
		t.Fatalf("watch identity not stable: %#v %v", again, err)
	}
	secret, err := e.store.ChannelWatchSecret(ctx, watch)
	if err != nil || len(secret) < 32 {
		t.Fatalf("clientState not sealed: %v", err)
	}
	if err := e.store.CompleteChannelWatch(ctx, watch, "SUB", now.Add(48*time.Hour), ""); err != nil {
		t.Fatal(err)
	}
	queued := 0
	parent := &Receiver{Links: e.store, Roots: e.store, Replays: fakeReplays{}, Marks: fakeMarks{}, Now: func() time.Time { return now }, Log: e.flusher.Log, Pull: func(_ context.Context, ws, conn ids.UUID) error {
		if ws != e.ws || conn != connection {
			t.Error("notification escaped connection scope")
		}
		queued++
		return nil
	}}
	receiver := &TeamsGraphReceiver{Receiver: parent, Store: e.store, App: func(context.Context, string) (capture.ConnectorApp, error) {
		return capture.ConnectorApp{ClientID: app, Tenant: tenant}, nil
	}}
	delivery := func(state, resource, lifecycle string) chatlinks.Delivery {
		t.Helper()
		raw, err := json.Marshal(map[string]any{"value": []any{map[string]any{"subscriptionId": "SUB", "tenantId": tenant, "clientState": state, "resource": resource, "changeType": "updated", "lifecycleEvent": lifecycle, "resourceData": map[string]string{"body": "private source must never persist"}}}})
		if err != nil {
			t.Fatal(err)
		}
		return chatlinks.Delivery{Body: raw}
	}
	resource := "teams('" + team + "')/channels('19:channel@thread.tacv2')/messages('root')/replies('reply')"
	if _, err := receiver.Receive(ctx, delivery("forged", resource, "")); !errors.Is(err, chatlinks.ErrEventRejected) || queued != 0 {
		t.Fatalf("forged notice queued work: %v %d", err, queued)
	}
	if _, err := receiver.Receive(ctx, delivery(secret, strings.Replace(resource, "19:channel", "19:other", 1), "")); !errors.Is(err, chatlinks.ErrEventRejected) || queued != 0 {
		t.Fatalf("foreign resource queued work: %v %d", err, queued)
	}
	valid := delivery(secret, resource, "")
	if receipt, err := receiver.Receive(ctx, valid); err != nil || (receipt.Status == nil || *receipt.Status != crmcontracts.DispactChannelEventReceiptStatusPullQueued) || queued != 1 {
		t.Fatalf("verified notice not queued: %#v %v queued=%d", receipt, err, queued)
	}
	if receipt, err := receiver.Receive(ctx, valid); err != nil || (receipt.Status == nil || *receipt.Status != crmcontracts.DispactChannelEventReceiptStatusAlreadyTaken) || queued != 1 {
		t.Fatalf("replay queued work: %#v %v queued=%d", receipt, err, queued)
	}

	mixed, err := json.Marshal(map[string]any{"value": []any{map[string]string{"subscriptionId": "retired-subscription"}, map[string]string{"subscriptionId": "SUB", "tenantId": tenant, "clientState": secret, "resource": resource, "changeType": "updated"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := receiver.Receive(ctx, chatlinks.Delivery{Body: mixed}); err != nil || queued != 2 {
		t.Fatalf("retired subscription blocked a valid batch member: %v queued=%d", err, queued)
	}
	e.exec(t, `UPDATE chat_link SET native_cursor='old-page',native_coverage_at=now(),native_scan_started_at=now() WHERE id=$1`, e.link)
	if _, err := receiver.Receive(ctx, delivery(secret, "", "missed")); err != nil {
		t.Fatal(err)
	}
	current, err := e.store.WatchForNotification(ctx, "SUB")
	if err != nil || !current.Rescan {
		t.Fatalf("missed event lost recovery state: %#v %v", current, err)
	}
	if err := e.store.CompleteChannelWatch(ctx, current, "SUB", now.Add(48*time.Hour), ""); err != nil {
		t.Fatal(err)
	}
	var reset bool
	if err := e.owner.QueryRow(context.Background(), `SELECT native_cursor IS NULL AND native_coverage_at IS NULL AND native_scan_started_at IS NULL AND native_refresh_at IS NOT NULL FROM chat_link WHERE id=$1`, e.link).Scan(&reset); err != nil || !reset {
		t.Fatalf("missed event did not reset bounded recovery: %v %v", reset, err)
	}
	if err := e.store.InvalidateNativeRoot(ctx, connection, channel, chatwire.RootRef{ID: "other-root", Revision: "later"}); err != nil {
		t.Fatal(err)
	}
	var leaked int
	if err := e.owner.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE after::text LIKE $1 OR before::text LIKE $1`, "%private source must never persist%").Scan(&leaked); err != nil || leaked != 0 {
		t.Fatalf("notification persisted source content: %d %v", leaked, err)
	}
	assertWatchRenewalAndRetirement(ctx, t, e, receiver, watch, secret, now)
}

type teamWatchSessions struct{ session chatlinks.Session }

func (s teamWatchSessions) Open(context.Context) (chatlinks.Session, error) { return s.session, nil }

func (s teamWatchSessions) OpenProvider(_ context.Context, provider string) (chatlinks.Session, error) {
	if provider != "teams" {
		return chatlinks.Session{}, chatlinks.ErrBotNotConnected
	}
	return s.session, nil
}

func assertWatchRenewalAndRetirement(ctx context.Context, t *testing.T, e *flushEnv, receiver *TeamsGraphReceiver, watch chatlinks.ChannelWatch, secret string, now time.Time) {
	t.Helper()
	reauthorized, removed, created := 0, 0, 0
	subscription := teams.Subscription{ID: "SUB", Resource: "/teams/" + strings.Replace(watch.Channel, "/", "/channels/", 1) + "/messages", NotificationURL: watch.URL, LifecycleURL: watch.URL, ClientState: secret, ChangeType: "created,updated,deleted", Expires: now.Add(48 * time.Hour)}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != "Bearer graph-synthetic" {
			t.Error("watch used another identity")
		}
		switch r.Method + " " + r.URL.Path {
		case "GET /subscriptions/SUB", "GET /subscriptions/SUB-NEW":
			if err := json.NewEncoder(w).Encode(subscription); err != nil {
				t.Error(err)
			}
		case "POST /subscriptions/SUB/reauthorize":
			reauthorized++
			w.WriteHeader(http.StatusNoContent)
		case "PATCH /subscriptions/SUB":
			if err := json.NewEncoder(w).Encode(subscription); err != nil {
				t.Error(err)
			}
		case "DELETE /subscriptions/SUB", "DELETE /subscriptions/SUB-NEW":
			removed++
			w.WriteHeader(http.StatusNoContent)
		case "GET /subscriptions":
			if err := json.NewEncoder(w).Encode(map[string]any{"value": []any{}}); err != nil {
				t.Error(err)
			}
		case "POST /subscriptions":
			created++
			var incoming teams.Subscription
			if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
				t.Error(err)
			}
			incoming.ID = "SUB-NEW"
			if created > 1 {
				incoming.ID = "SUB-MOVED"
			}
			subscription = incoming
			if err := json.NewEncoder(w).Encode(incoming); err != nil {
				t.Error(err)
			}
		default:
			t.Errorf("unexpected watch boundary: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	base, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	team, _, _ := strings.Cut(watch.Channel, "/")
	bot := teams.NewBot(teamsProtocolHTTP{client: server.Client(), api: base}, "graph-synthetic", "connector-synthetic", chatwire.BotCredential{Workspace: watch.Tenant, Member: team, Credential: messaging.Credential{AppID: watch.App}})
	sessions := teamWatchSessions{session: chatlinks.Session{Provider: "teams", ConnectionID: watch.Connection, TeamID: watch.Tenant, Chat: bot}}
	puller := &Puller{Store: e.store, Chats: sessions, NotificationURL: watch.URL, Now: func() time.Time { return now }, Log: e.flusher.Log}
	raw, err := json.Marshal(map[string]any{"value": []any{map[string]string{"subscriptionId": "SUB", "tenantId": watch.Tenant, "clientState": secret, "lifecycleEvent": "reauthorizationRequired"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := receiver.Receive(ctx, chatlinks.Delivery{Body: raw}); err != nil {
		t.Fatal(err)
	}
	links, err := e.store.PullLinks(ctx, watch.Connection)
	if err != nil {
		t.Fatal(err)
	}
	if err := puller.maintainChannelWatch(ctx, sessions.session, links[0]); err != nil || reauthorized != 1 {
		t.Fatalf("durable lifecycle was not applied: %v %d", err, reauthorized)
	}
	read := false
	if _, err := e.store.SetSyncOptions(e.member(), "deal", e.deal, chatlinks.SyncOptions{Read: &read}); err != nil {
		t.Fatal(err)
	}
	if err := puller.retireChannelWatches(ctx, watch.Connection); err != nil || removed != 1 {
		t.Fatalf("unused watch was not removed: %v %d", err, removed)
	}
	if _, err := e.store.ChannelWatchSecret(ctx, watch); !errors.Is(err, keyvault.ErrNotFound) {
		t.Fatalf("retired watch retained clientState: %v", err)
	}
	read = true
	if _, err := e.store.SetSyncOptions(e.member(), "deal", e.deal, chatlinks.SyncOptions{Read: &read}); err != nil {
		t.Fatal(err)
	}
	links, err = e.store.PullLinks(ctx, watch.Connection)
	if err != nil {
		t.Fatal(err)
	}
	if err := puller.maintainChannelWatch(ctx, sessions.session, links[0]); err != nil || created != 1 {
		t.Fatalf("resumed watch was not renewed: %v created=%d", err, created)
	}
	resumed, err := e.store.WatchForNotification(ctx, "SUB-NEW")
	if err != nil || resumed.ID != watch.ID || resumed.Retired != nil {
		t.Fatalf("resumed watch lost identity: %#v %v", resumed, err)
	}
	freshSecret, err := e.store.ChannelWatchSecret(ctx, resumed)
	if err != nil || freshSecret == secret {
		t.Fatalf("resumed watch reused retired clientState: %v", err)
	}
	read = false
	if _, err := e.store.SetSyncOptions(e.member(), "deal", e.deal, chatlinks.SyncOptions{Read: &read}); err != nil {
		t.Fatal(err)
	}
	if err := bot.RemoveWatch(ctx, teams.WatchRequest{Channel: resumed.Channel, URL: resumed.URL, Secret: freshSecret, Subscription: resumed.Subscription, Now: now}); err != nil {
		t.Fatal(err)
	}
	read = true
	if _, err := e.store.SetSyncOptions(e.member(), "deal", e.deal, chatlinks.SyncOptions{Read: &read}); err != nil {
		t.Fatal(err)
	}
	if err := e.store.RetireChannelWatch(ctx, resumed, now, true); err != nil {
		t.Fatal(err)
	}
	raced, err := e.store.WatchForNotification(ctx, resumed.Subscription)
	if err != nil || raced.Retired != nil || raced.Expires != nil || !raced.Rescan {
		t.Fatalf("resume during removal left a deleted watch looking healthy: %#v %v", raced, err)
	}
	kept, err := e.store.ChannelWatchSecret(ctx, raced)
	if err != nil || kept != freshSecret {
		t.Fatalf("resume during removal destroyed active credential: %v", err)
	}
	puller.NotificationURL = "https://crm-new.example.test/v1/channel-events/teams/graph"
	links, err = e.store.PullLinks(ctx, watch.Connection)
	if err != nil {
		t.Fatal(err)
	}
	if err := puller.maintainChannelWatch(ctx, sessions.session, links[0]); err != nil || created != 2 {
		t.Fatalf("notification address did not move safely: %v creates=%d", err, created)
	}
	moved, err := e.store.WatchForNotification(ctx, "SUB-MOVED")
	if err != nil || moved.URL != puller.NotificationURL || moved.ID != watch.ID {
		t.Fatalf("notification address lost exact binding: %#v %v", moved, err)
	}
	if _, err := e.store.ChannelWatchSecret(ctx, raced); !errors.Is(err, keyvault.ErrNotFound) {
		t.Fatalf("old endpoint kept a valid notification credential: %v", err)
	}
	replacement, err := e.store.ChannelWatchSecret(ctx, moved)
	if err != nil || replacement == freshSecret {
		t.Fatalf("moved endpoint reused prior credential: %v", err)
	}
}
