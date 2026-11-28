// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package slack

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

type oauthDoer func(*http.Request) (*http.Response, error)

func (f oauthDoer) Do(r *http.Request) (*http.Response, error) { return f(r) }

func TestConsentRequestsParticipantScopesAndKeepsTheBotOut(t *testing.T) {
	flow := OAuth{ClientID: "APP"}
	link, err := url.Parse(flow.AuthorizeURL("signed-state", "https://app.example.test/v1/messenger/slack/callback"))
	if err != nil {
		t.Fatal(err)
	}
	if link.Host != "slack.com" || link.Query().Get("state") != "signed-state" || link.Query().Get("scope") != "" {
		t.Fatal("consent did not preserve participant separation")
	}
	if link.Query().Get("user_scope") != strings.Join(UserScopes, ",") {
		t.Fatal("participant scopes missing")
	}
}

func TestConsentSealsOnlyTheVerifiedUserGrant(t *testing.T) {
	now := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, payload string
		valid         bool
	}{
		{"participant", `{"ok":true,"access_token":"bot-token","token_type":"bot","team":{"id":"T1"},"authed_user":{"id":"U1","access_token":"person-token","refresh_token":"refresh-one","expires_in":43200,"token_type":"user","scope":"` + strings.Join(UserScopes, ",") + `"}}`, true},
		{"bot only", `{"ok":true,"access_token":"bot-token","token_type":"bot","team":{"id":"T1"}}`, false},
		{"narrow consent", `{"ok":true,"team":{"id":"T1"},"authed_user":{"id":"U1","access_token":"person-token","token_type":"user","scope":"chat:write"}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			flow := OAuth{ClientID: "APP", ClientSecret: "synthetic-secret", Now: func() time.Time { return now }, HTTP: oauthDoer(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != "https://slack.com/api/oauth.v2.access" {
					t.Fatal("wrong token endpoint")
				}
				if err := r.ParseForm(); err != nil {
					t.Fatal(err)
				}
				if r.Form.Get("client_secret") != "synthetic-secret" || r.Form.Get("code") != "code-one" {
					t.Fatal("code exchange omitted confidential app binding")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.payload))}, nil
			})}
			grant, err := flow.Exchange(context.Background(), "code-one", "https://app.example.test/callback")
			if !tc.valid {
				if !errors.Is(err, messaging.ErrUnauthorized) {
					t.Fatalf("refused grant: %v", err)
				}
				return
			}
			if err != nil || grant.Credential.AccessToken != "person-token" || grant.Workspace != "T1" || grant.Member != "U1" || grant.Credential.AppID != "APP" || !grant.Credential.ExpiresAt.Equal(now.Add(12*time.Hour)) {
				t.Fatalf("wrong participant authorization: %v", err)
			}
		})
	}
}

func TestRefreshKeepsTheRotatedParticipantCredential(t *testing.T) {
	now := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	called := false
	flow := OAuth{ClientID: "APP", ClientSecret: "synthetic-secret", Now: func() time.Time { return now }, HTTP: oauthDoer(func(r *http.Request) (*http.Response, error) {
		called = true
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "refresh-one" {
			t.Fatal("wrong refresh grant")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ok":true,"token_type":"user","access_token":"next-person-token","refresh_token":"refresh-two","expires_in":43200,"scope":"` + strings.Join(UserScopes, ",") + `"}`))}, nil
	})}
	rotated, err := flow.Refresh(context.Background(), messaging.Credential{AppID: "APP", RefreshToken: "refresh-one"})
	if err != nil || rotated.RefreshToken != "refresh-two" || rotated.AccessToken != "next-person-token" {
		t.Fatalf("rotating user token: %v", err)
	}
	called = false
	if _, err := flow.Refresh(context.Background(), messaging.Credential{AppID: "OTHER", RefreshToken: "refresh-one"}); !errors.Is(err, messaging.ErrUnauthorized) || called {
		t.Fatal("spent old application's refresh token against a replacement app")
	}
}

func TestChannelConsentUsesBotHistoryWithoutDelegatedReader(t *testing.T) {
	flow := OAuth{ClientID: "APP", ClientSecret: "synthetic-secret", Now: func() time.Time { return time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC) }}
	link, err := url.Parse(flow.AuthorizeBotURL("state", "https://app.example.test/callback"))
	if err != nil || link.Query().Get("user_scope") != "" || link.Query().Get("scope") != strings.Join(BotScopes, ",") {
		t.Fatalf("bot consent mixed participant grants: %s %v", link, err)
	}
	for scope := range strings.SplitSeq(link.Query().Get("scope"), ",") {
		if strings.HasPrefix(scope, "im:") || strings.HasPrefix(scope, "mpim:") {
			t.Fatalf("channel install requested private chat scope %s", scope)
		}
	}
	flow.HTTP = oauthDoer(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ok":true,"access_token":"bot-token","token_type":"bot","bot_user_id":"BOTUSER","team":{"id":"TEAM"},"scope":"` + strings.Join(BotScopes, ",") + `","authed_user":{"id":"REP","access_token":"history-reader-token","token_type":"user","scope":"` + "groups:history,groups:read,users:read,users:read.email" + `"}}`))}, nil
	})
	grant, err := flow.ExchangeBot(context.Background(), "code", "https://app.example.test/callback")
	if err != nil || grant.Credential.AccessToken != "bot-token" || grant.Workspace != "TEAM" || grant.Member != "BOTUSER" {
		t.Fatalf("bot grant: %+v %v", grant, err)
	}
}

func TestChannelConsentRefusesAUserTokenEvenWhenItsScopesMatch(t *testing.T) {
	flow := OAuth{ClientID: "APP", HTTP: oauthDoer(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ok":true,"access_token":"personal-token","token_type":"user","bot_user_id":"BOTUSER","team":{"id":"TEAM"},"scope":"` + strings.Join(BotScopes, ",") + `"}`))}, nil
	})}
	if _, err := flow.ExchangeBot(context.Background(), "code", "https://app.example.test/callback"); !errors.Is(err, messaging.ErrUnauthorized) {
		t.Fatalf("a participant credential became an unattended bot: %v", err)
	}
}

func TestChannelInstallationDoesNotRequireAnInstallingUsersToken(t *testing.T) {
	flow := OAuth{ClientID: "APP", HTTP: oauthDoer(func(*http.Request) (*http.Response, error) {
		body := `{"ok":true,"access_token":"synthetic-bot","token_type":"bot","bot_user_id":"BOT","team":{"id":"TEAM"},"scope":"` + strings.Join(BotScopes, ",") + `"}`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	grant, err := flow.ExchangeBot(context.Background(), "synthetic", "https://app.example.test/callback")
	if err != nil || grant.Member != "BOT" {
		t.Fatalf("bot-only install: %+v %v", grant, err)
	}
}

func TestExistingPrivateBotConsentCanRefreshWithoutPublicChannelScopes(t *testing.T) {
	flow := OAuth{ClientID: "APP", ClientSecret: "synthetic", Now: func() time.Time { return time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC) }, HTTP: oauthDoer(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ok":true,"token_type":"bot","access_token":"renewed","refresh_token":"next","expires_in":43200,"scope":"` + strings.Join(RequiredBotScopes(), ",") + `"}`))}, nil
	})}
	credential, err := flow.RefreshBot(context.Background(), messaging.Credential{AppID: "APP", RefreshToken: "previous"})
	if err != nil || credential.AccessToken != "renewed" {
		t.Fatalf("existing bot forced to reconnect: %+v %v", credential, err)
	}
}
