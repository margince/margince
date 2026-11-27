// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

type slackReaderHTTP func(*http.Request) (*http.Response, error)

func (f slackReaderHTTP) Do(r *http.Request) (*http.Response, error) { return f(r) }

func TestSlackInstallationReaderNeedsItsOwnLiveInternalIdentity(t *testing.T) {
	for _, mode := range []string{"valid", "wrong workspace", "bot", "guest", "revoked"} {
		t.Run(mode, func(t *testing.T) {
			credential := chatwire.BotCredential{Workspace: "T_ONE", Member: "U_BOT", Credential: messaging.Credential{AppID: "APP"}, SlackReaderMember: "U_READER", SlackReader: &messaging.Credential{AccessToken: "synthetic-reader", AppID: "APP"}}
			chats := LiveChats{HTTP: slackReaderHTTP(func(r *http.Request) (*http.Response, error) {
				if r.Header.Get("Authorization") != "Bearer synthetic-reader" {
					t.Fatal("used bot or personal Messenger credential")
				}
				body := `{"ok":false,"error":"token_revoked"}`
				if mode != "revoked" {
					switch r.URL.Path {
					case "/api/auth.test":
						team := "T_ONE"
						if mode == "wrong workspace" {
							team = "T_OTHER"
						}
						body = `{"ok":true,"user_id":"U_READER","team_id":"` + team + `"}`
					case "/api/users.info":
						flags := ""
						if mode == "bot" {
							flags = `,"is_bot":true`
						}
						if mode == "guest" {
							flags = `,"is_restricted":true`
						}
						body = `{"ok":true,"user":{"id":"U_READER","team_id":"T_ONE","real_name":"Installation reader"` + flags + `}}`
					default:
						t.Fatalf("unexpected reader API %s", r.URL.Path)
					}
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
			})}
			reader, err := chats.slackThreadReader(context.Background(), credential, nil)
			if mode == "valid" {
				if err != nil || reader == nil {
					t.Fatalf("valid reader refused: %v", err)
				}
			} else if reader != nil || !errors.Is(err, chatwire.ErrHistoryProfileNeedsReview) {
				t.Fatalf("unsafe reader accepted: %v", err)
			}
		})
	}
}

func TestSlackTokenRefreshUsesEachCredentialExpiryAndRetainsReaderFailures(t *testing.T) {
	now := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	called := 0
	refresh := func(_ context.Context, previous messaging.Credential) (messaging.Credential, error) {
		called++
		return messaging.Credential{AccessToken: previous.AccessToken + "-fresh", ExpiresAt: now.Add(time.Hour)}, nil
	}
	for _, expiry := range []time.Time{{}, now.Add(time.Hour), now.Add(time.Minute)} {
		previous := messaging.Credential{AccessToken: "synthetic", ExpiresAt: expiry}
		got, err := refreshSlackToken(context.Background(), previous, refresh, now)
		if err != nil {
			t.Fatal(err)
		}
		if expiry.Equal(now.Add(time.Minute)) && got.AccessToken != "synthetic-fresh" {
			t.Fatal("near-expiry reader was not refreshed")
		}
	}
	if called != 1 {
		t.Fatalf("refreshed %d times", called)
	}
	failure := errors.New("synthetic transient boundary failure")
	_, err := (LiveChats{}).slackThreadReader(context.Background(), chatwire.BotCredential{}, failure)
	if !errors.Is(err, failure) {
		t.Fatal("reader failure disappeared")
	}
}
