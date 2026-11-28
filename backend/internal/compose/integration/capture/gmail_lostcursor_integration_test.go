// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture/gmail"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// stalledGmail is a stubbed mailbox whose recent mail a test sets between
// syncs, and whose history Gmail no longer holds once a cursor is stored.
type stalledGmail struct {
	mu       sync.Mutex
	received map[string]time.Time
	// undated leaves Gmail's own receive time off each message.
	undated bool
}

func (g *stalledGmail) holds(first time.Time, count int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.received = map[string]time.Time{}
	for i := range count {
		g.received["g"+strconv.Itoa(i)+"-"+strconv.FormatInt(first.Unix(), 10)] = first.Add(time.Duration(i) * time.Minute)
	}
}

func (g *stalledGmail) serve(t *testing.T, owner string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		integration.WriteJSON(w, map[string]any{"access_token": "access-tok", "refresh_token": "refresh-tok", "expires_in": 3599})
	})
	mux.HandleFunc("/profile", func(w http.ResponseWriter, _ *http.Request) {
		integration.WriteJSON(w, map[string]any{"emailAddress": owner, "historyId": "1001"})
	})
	mux.HandleFunc("/history", func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) })
	mux.HandleFunc("/messages", func(w http.ResponseWriter, _ *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		listed := make([]map[string]string, 0, len(g.received))
		for id := range g.received {
			listed = append(listed, map[string]string{"id": id})
		}
		integration.WriteJSON(w, map[string]any{"messages": listed})
	})
	mux.HandleFunc("/messages/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/messages/")
		g.mu.Lock()
		at := g.received[id]
		g.mu.Unlock()
		rfc822 := "From: Buyer <buyer@acme.com>\r\nTo: " + owner + "\r\nSubject: Note " + id + "\r\n" +
			"Date: " + at.Format(time.RFC1123Z) + "\r\nMessage-ID: <" + id + "@acme.com>\r\n" +
			"Content-Type: text/plain; charset=utf-8\r\n\r\nHello."
		message := map[string]any{"id": id, "raw": base64.RawURLEncoding.EncodeToString([]byte(rfc822))}
		if !g.undated {
			message["internalDate"] = strconv.FormatInt(at.UnixMilli(), 10)
		}
		integration.WriteJSON(w, message)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// unreadPeriod is the span the connection records as read by no sync.
func unreadPeriod(t *testing.T, e *integration.SearchEnv, connection ids.UUID) (from, to *time.Time) {
	t.Helper()
	if err := database.WithTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT unread_from, unread_to FROM capture_connection WHERE id = $1`, connection).Scan(&from, &to)
	}); err != nil {
		t.Fatalf("reading the unread period: %v", err)
	}
	return from, to
}

// Gmail keeps a history cursor for about a week. A mailbox that synced nothing
// for longer comes back with its cursor gone, and reads only its newest mail.
func TestAGmailSyncThatLostItsCursorRecordsTheMailItLeftOut(t *testing.T) {
	const owner = "rep@ws.example"
	connected := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	resumed := connected.AddDate(0, 0, 14)
	dayBefore := resumed.Add(-24 * time.Hour)
	for name, tc := range map[string]struct {
		newest      int
		oldest      time.Time
		undated     bool
		wantUnread  bool
		wantReadsTo time.Time
	}{
		"more mail arrived than one round lists":  {newest: 50, oldest: dayBefore, wantUnread: true, wantReadsTo: dayBefore},
		"the mail read again states no date":      {newest: 50, oldest: dayBefore, undated: true, wantUnread: true, wantReadsTo: resumed},
		"the round lists everything that arrived": {newest: 3, oldest: dayBefore},
	} {
		t.Run(name, func(t *testing.T) {
			e := integration.SetupSearch(t)
			mailbox := &stalledGmail{}
			mailbox.holds(connected.Add(-time.Hour), 1)
			stub := mailbox.serve(t, owner)
			oauth := gmail.NewOAuth(gmail.OAuthConfig{ClientID: "cid", ClientSecret: "sec", TokenURL: stub.URL + "/token"})
			api := gmail.NewAPI(stub.Client(), stub.URL)
			now := connected
			registry := newTestCaptureRegistry(e, newTestKeyvault(t, e)).SetClock(func() time.Time { return now })
			registry.Register(gmail.New(oauth, api))
			authReq, err := gmail.AuthRequestFrom("the-code", "https://app.test/v1/connectors/gmail/callback")
			if err != nil {
				t.Fatal(err)
			}
			auth, err := gmail.New(oauth, api).Authenticate(context.Background(), authReq)
			if err != nil {
				t.Fatalf("Authenticate: %v", err)
			}
			grantCtx := humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})
			connection, err := registry.Connect(grantCtx, "gmail", auth)
			if err != nil {
				t.Fatalf("Connect: %v", err)
			}
			if err := registry.SyncOnce(grantCtx, connection); err != nil {
				t.Fatalf("the first sync: %v", err)
			}
			if from, _ := unreadPeriod(t, e, connection); from != nil {
				t.Fatalf("a first sync recorded an unread period from %v", from)
			}

			now = resumed
			mailbox.holds(tc.oldest, tc.newest)
			mailbox.undated = tc.undated
			if err := registry.SyncOnce(grantCtx, connection); err != nil {
				t.Fatalf("the sync after the stall: %v", err)
			}

			from, to := unreadPeriod(t, e, connection)
			if !tc.wantUnread {
				if from != nil || to != nil {
					t.Fatalf("unread period = %v..%v, want none: the round read all the mail there was", from, to)
				}
				return
			}
			if from == nil || to == nil || !from.Equal(connected) || !to.Equal(tc.wantReadsTo) {
				t.Fatalf("unread period = %s..%s, want the last good sync %s to where the round read again %s",
					fmt.Sprint(from), fmt.Sprint(to), connected, tc.wantReadsTo)
			}
		})
	}
}
