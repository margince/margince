// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture/gmail"
	"github.com/margince/margince/backend/internal/modules/capture/graph"
	"github.com/margince/margince/backend/internal/modules/capture/oauthflow"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type mailboxDeletionOAuth struct{}

func (mailboxDeletionOAuth) AccessToken(context.Context, string) (string, error) {
	return "test-access", nil
}

func (mailboxDeletionOAuth) Exchange(context.Context, string, string) (oauthflow.TokenGrant, error) {
	return oauthflow.TokenGrant{}, nil
}

func (mailboxDeletionOAuth) Refresh(context.Context, string, []string) (oauthflow.TokenRefresh, error) {
	return oauthflow.TokenRefresh{AccessToken: "test-access"}, nil
}

func mailboxJSON(t *testing.T, w http.ResponseWriter, value map[string]any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Error(err)
	}
}

func TestProviderSyncCapturesAndRemovesTheRealStoredEmailKey(t *testing.T) {
	for _, tc := range []struct {
		name, provider, outlookCase string
		kept                        bool
	}{
		{"Gmail deletion", "gmail", "", false}, {"Outlook ambiguous folder removal", "graph", "", true},
		{"Outlook hard delete after the hold", "graph", "hard delete", false},
		{"Outlook hard delete of a colleague's copy", "graph", "colleague", true},
		{"Outlook hard delete under a statutory hold", "graph", "hold", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := integration.Setup(t)
			mailbox := seatAddress(t, e, e.Rep1)
			if tc.provider == "graph" {
				e.WsExec(t, `INSERT INTO capture_connection (user_id,provider,status,credential_ref,account_label) VALUES ($1,'graph','connected','vault:test',$2)`, e.Rep1, mailbox)
			}
			raw := strings.ReplaceAll(string(threadMail(mailbox, "stored-rfc@example.test", "Tue, 02 Jun 2026 08:00:00 +0000", "")), "Re: Angebot", "Family note")
			var deleting atomic.Bool
			var server *httptest.Server
			handler := func(w http.ResponseWriter, r *http.Request) {
				if tc.provider == "gmail" {
					gmailDeletionResponse(t, w, r, mailbox, raw)
					return
				}
				graphDeletionResponse(t, w, r, server.URL, raw, outlookMail{removed: deleting.Load(), hardDeleted: tc.outlookCase != ""})
			}
			server = httptest.NewServer(http.HandlerFunc(handler))
			defer server.Close()
			auth, err := json.Marshal(map[string]any{"refresh_token": "test-refresh", "owner_email": mailbox, "scopes": []string{"read"}})
			if err != nil {
				t.Fatal(err)
			}
			ctx := threadConnectorCtx(e, e.Rep1)
			actor, ok := principal.Actor(ctx)
			if !ok {
				t.Fatal("no mailbox actor")
			}
			actor.ID = "connector:" + tc.provider
			ctx = principal.WithActor(ctx, actor)
			sink := newCaptureSink(e.Pool, CaptureConfig{}).WithMessagePurger(purgerFor(t, e).PurgeRemovedAdmitted)
			var pull func(connector.Cursor) (connector.Cursor, error)
			clock := time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)
			if tc.provider == "gmail" {
				provider := gmail.New(mailboxDeletionOAuth{}, gmail.NewAPI(server.Client(), server.URL))
				pull = func(cursor connector.Cursor) (connector.Cursor, error) { return provider.Sync(ctx, auth, cursor, sink) }
			} else {
				provider := graph.New(mailboxDeletionOAuth{}, graph.NewAPI(server.Client(), server.URL)).WithClock(func() time.Time { return clock })
				pull = func(cursor connector.Cursor) (connector.Cursor, error) {
					return provider.SyncFrom(ctx, auth, cursor, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), sink)
				}
			}
			cursor, err := pull(nil)
			if err != nil {
				t.Fatal(err)
			}
			activityID, err := ids.Parse(e.WsScalar(t, `SELECT id::text FROM activity WHERE source_system='email' AND source_id='stored-rfc@example.test'`))
			if err != nil {
				t.Fatal("connector did not write the normalized email key")
			}
			if count := e.WsCount(t, `SELECT count(*) FROM capture_import i JOIN capture_connection c ON c.user_id=i.user_id WHERE c.provider=$1 AND i.provider_message_ids @> ARRAY[c.provider || ':' || c.generation || ':provider-opaque-id']`, tc.provider); count != 1 {
				t.Fatal("connector did not persist its mailbox-specific provider id")
			}
			switch tc.outlookCase {
			case "colleague":
				addImporter(t, e, activityID, e.Rep2)
			case "hold":
				restrict(t, e, activityID)
			}
			deleting.Store(true)
			if cursor, err = pull(cursor); err != nil {
				t.Fatal(err)
			}
			if tc.outlookCase != "" {
				if e.WsCount(t, `SELECT count(*) FROM activity WHERE id=$1 AND body IS NULL`, activityID) != 0 {
					t.Fatal("an Outlook hard delete was purged before its hold ran out")
				}
				clock = clock.Add(15 * 24 * time.Hour)
				if _, err := pull(cursor); err != nil || !bytes.Contains(cursor, []byte(`"internet_message_id":"\u003cstored-rfc@example.test\u003e"`)) {
					t.Fatalf("the sync after the hold failed (%v), or the hold kept no identity: %s", err, cursor)
				}
			}
			kept := e.WsCount(t, `SELECT count(*) FROM activity WHERE source_id='stored-rfc@example.test' AND body IS NOT NULL`) > 0
			if kept != tc.kept {
				t.Errorf("kept message=%v for %s", kept, tc.name)
			}
		})
	}
}

func gmailDeletionResponse(t *testing.T, w http.ResponseWriter, r *http.Request, mailbox, raw string) {
	switch r.URL.Path {
	case "/profile":
		mailboxJSON(t, w, map[string]any{"emailAddress": mailbox, "historyId": "1"})
	case "/messages":
		mailboxJSON(t, w, map[string]any{"messages": []map[string]string{{"id": "provider-opaque-id"}}})
	case "/messages/provider-opaque-id":
		mailboxJSON(t, w, map[string]any{"raw": base64.RawURLEncoding.EncodeToString([]byte(raw)), "labelIds": []string{"INBOX"}, "internalDate": "1780358400000"})
	case "/history":
		mailboxJSON(t, w, map[string]any{"historyId": "2", "history": []map[string]any{{"messagesDeleted": []map[string]any{{"message": map[string]string{"id": "provider-opaque-id"}}}}}})
	default:
		t.Errorf("unexpected Gmail request %s", r.URL.Path)
		w.WriteHeader(http.StatusBadRequest)
	}
}

// outlookMail is what the fake Outlook mailbox shows for the one test message.
type outlookMail struct{ removed, hardDeleted bool }

func graphDeletionResponse(t *testing.T, w http.ResponseWriter, r *http.Request, base, raw string, mail outlookMail) {
	if r.Header.Get("Prefer") != `IdType="ImmutableId"` {
		t.Error("Outlook request omitted immutable ids")
	}
	switch r.URL.Path {
	case "/me/mailFolders/sentitems/messages/delta":
		mailboxJSON(t, w, map[string]any{"value": []string{}, "@odata.deltaLink": base + r.URL.Path + "?next=1"})
	case "/me/mailFolders/inbox/messages/delta":
		value := []map[string]any{{"id": "provider-opaque-id", "receivedDateTime": "2026-06-02T00:00:00Z"}}
		if mail.removed {
			value = []map[string]any{{"id": "provider-opaque-id", "@removed": map[string]string{"reason": "deleted"}}}
		}
		mailboxJSON(t, w, map[string]any{"value": value, "@odata.deltaLink": base + r.URL.Path + "?next=1"})
	case "/me/mailFolders/recoverableitemsdeletions/messages/delta":
		value := []map[string]any{}
		if mail.removed && mail.hardDeleted && r.URL.Query().Get("next") != "" {
			value = append(value, map[string]any{"id": "provider-opaque-id", "internetMessageId": "<stored-rfc@example.test>"})
		}
		mailboxJSON(t, w, map[string]any{"value": value, "@odata.deltaLink": base + r.URL.Path + "?next=1"})
	case "/me/mailFolders/recoverableitemsdeletions":
		mailboxJSON(t, w, map[string]any{"id": "deletions-folder"})
	case "/me/messages/provider-opaque-id":
		mailboxJSON(t, w, map[string]any{"parentFolderId": "deletions-folder"})
	case "/me/mailFolders/inbox/messages":
		mailboxJSON(t, w, map[string]any{"@odata.count": 1})
	case "/me/mailFolders/sentitems/messages":
		mailboxJSON(t, w, map[string]any{"@odata.count": 0})
	case "/me/messages/provider-opaque-id/$value":
		if _, err := w.Write([]byte(raw)); err != nil {
			t.Error(err)
		}
	default:
		t.Errorf("unexpected Graph request %s", r.URL.Path)
		w.WriteHeader(http.StatusBadRequest)
	}
}
