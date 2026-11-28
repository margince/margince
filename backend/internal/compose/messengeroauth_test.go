// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/messengeraccounts"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func messengerOAuthFixture() (Server, connectState, time.Time) {
	now := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	st := connectState{User: ids.MustParse("22222222-2222-2222-2222-222222222222"), Provider: "messenger:slack", Nonce: "browser-nonce", Version: stateVersionNamespacedCSRF}
	cfg := GraphConfig{StateKey: strings.Repeat("k", 32), PublicBaseURL: "https://app.example.test", APIBaseURL: "https://api.example.test"}
	return Server{messengerOAuthHandlers: messengerOAuthHandlers{cfg: cfg, now: func() time.Time { return now }}}, st, now
}

func TestMessengerCallbackRejectsForeignBrowserAndCrossProviderStateBeforeExchange(t *testing.T) {
	for _, tc := range []struct{ name, nonce, provider string }{
		{"no browser nonce", "", "slack"}, {"different browser", "another-browser", "slack"}, {"different provider", "browser-nonce", "teams"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, state, now := messengerOAuthFixture()
			server.resolve = func(context.Context, string) (messengeraccounts.App, error) {
				t.Fatal("untrusted callback resolved an app credential")
				return messengeraccounts.App{}, nil
			}
			token := newStateSigner([]byte(server.messengerOAuthHandlers.cfg.StateKey)).sign(state, now.Add(time.Minute))
			request := httptest.NewRequest(http.MethodGet, "/v1/messenger/"+tc.provider+"/callback", nil)
			if tc.nonce != "" {
				request.AddCookie(&http.Cookie{Name: "messenger_csrf_" + tc.provider, Value: tc.nonce})
			}
			reply := httptest.NewRecorder()
			server.MessengerOAuthCallback(reply, request, crmcontracts.MessengerProvider(tc.provider), crmcontracts.MessengerOAuthCallbackParams{State: token})
			if reply.Code != http.StatusBadRequest {
				t.Fatalf("untrusted callback status %d", reply.Code)
			}
		})
	}
}

func TestMessengerConsentDenialReturnsToSettingsWithoutOpeningCredentials(t *testing.T) {
	server, state, now := messengerOAuthFixture()
	server.resolve = func(context.Context, string) (messengeraccounts.App, error) {
		t.Fatal("a denial resolved credentials")
		return messengeraccounts.App{}, nil
	}
	token := newStateSigner([]byte(server.messengerOAuthHandlers.cfg.StateKey)).sign(state, now.Add(time.Minute))
	request := httptest.NewRequest(http.MethodGet, "/v1/messenger/slack/callback", nil)
	request.AddCookie(&http.Cookie{Name: "messenger_csrf_slack", Value: state.Nonce})
	denied := "access_denied"
	reply := httptest.NewRecorder()
	server.MessengerOAuthCallback(reply, request, "slack", crmcontracts.MessengerOAuthCallbackParams{State: token, Error: &denied})
	if reply.Code != http.StatusFound || !strings.Contains(reply.Header().Get("Location"), "messenger=denied") {
		t.Fatalf("denied callback: %d %s", reply.Code, reply.Header().Get("Location"))
	}
	cookies := reply.Result().Cookies()
	if len(cookies) != 1 || cookies[0].MaxAge != -1 {
		t.Fatal("completed callback retained browser nonce")
	}
}

func TestMessengerConnectBindsTheHumanBrowserAndReportsTheRegisteredCallback(t *testing.T) {
	server, state, now := messengerOAuthFixture()
	server.resolve = func(_ context.Context, provider string) (messengeraccounts.App, error) {
		if provider != "slack" {
			t.Fatal("wrong provider")
		}
		return messengeraccounts.App{Authorize: func(state, redirect string) string {
			return "https://slack.com/oauth/v2/authorize?" + url.Values{"state": {state}, "redirect_uri": {redirect}}.Encode()
		}}, nil
	}
	ctx := context.Background()
	ctx = principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + state.User.String(), UserID: state.User, SeatType: principal.SeatFull})
	reply := httptest.NewRecorder()
	server.ConnectMessenger(reply, httptest.NewRequest(http.MethodPost, "/v1/messenger/slack/connect", nil).WithContext(ctx), "slack")
	if reply.Code != http.StatusOK {
		t.Fatalf("start consent: %d %s", reply.Code, reply.Body.String())
	}
	cookies := reply.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Path != "/v1/messenger" || cookies[0].SameSite != http.SameSiteLaxMode || !cookies[0].HttpOnly || !cookies[0].Secure {
		t.Fatal("consent nonce is not browser bound")
	}
	var out struct {
		AuthorizeURL string `json:"authorize_url"`
	}
	if err := json.Unmarshal(reply.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	target, err := url.Parse(out.AuthorizeURL)
	if err != nil {
		t.Fatal(err)
	}
	if target.Query().Get("redirect_uri") != "https://api.example.test/v1/messenger/slack/callback" {
		t.Fatal("callback differs from registered API address")
	}
	signed, err := newStateSigner([]byte(server.messengerOAuthHandlers.cfg.StateKey)).verify(target.Query().Get("state"), now)
	if err != nil || signed.User != state.User || signed.Provider != "messenger:slack" || signed.Nonce != cookies[0].Value {
		t.Fatal("consent state did not bind browser, account and provider")
	}
}

func TestFreshMessengerCredentialsDoNotNeedAnotherProviderRoundTrip(t *testing.T) {
	now := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	raw := []byte(`{"access_token":"synthetic-access","expires_at":"2026-06-01T11:00:00Z"}`)
	got, err := messengeraccounts.RefreshCredential(context.Background(), chatlinks.MemberGrant{Provider: "slack"}, raw, func(context.Context, string) (messengeraccounts.App, error) {
		t.Fatal("a fresh credential refreshed")
		return messengeraccounts.App{}, nil
	}, func() time.Time { return now })
	if err != nil || string(got) != string(raw) {
		t.Fatal("fresh credential changed")
	}
}

func TestMessengerConsentPinsSlackWorkspaceAndReturnsToTheRecord(t *testing.T) {
	server, state, now := messengerOAuthFixture()
	record := ids.NewV7()
	server.resolve = func(context.Context, string) (messengeraccounts.App, error) {
		return messengeraccounts.App{Workspace: "T_ACCOUNT", Authorize: func(s, redirect string) string {
			return "https://slack.com/oauth/v2/authorize?" + url.Values{"state": {s}, "redirect_uri": {redirect}}.Encode()
		}}, nil
	}
	ctx := context.Background()
	ctx = principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + state.User.String(), UserID: state.User, SeatType: principal.SeatFull})
	body := `{"record_type":"deal","record_id":"` + record.String() + `"}`
	reply := httptest.NewRecorder()
	server.ConnectMessenger(reply, httptest.NewRequest(http.MethodPost, "/v1/messenger/slack/connect", strings.NewReader(body)).WithContext(ctx), "slack")
	if reply.Code != http.StatusOK {
		t.Fatalf("connect: %d %s", reply.Code, reply.Body.String())
	}
	var out struct {
		AuthorizeURL string `json:"authorize_url"`
	}
	if err := json.Unmarshal(reply.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	target, err := url.Parse(out.AuthorizeURL)
	if err != nil {
		t.Fatal(err)
	}
	st, err := newStateSigner([]byte(server.messengerOAuthHandlers.cfg.StateKey)).verify(target.Query().Get("state"), now)
	if err != nil || target.Query().Get("team") != "T_ACCOUNT" || st.MessagingWorkspace != "T_ACCOUNT" || st.MessengerRecord == nil || st.MessengerRecord.ID != record {
		t.Fatalf("consent lost trusted context: %s %v", out.AuthorizeURL, err)
	}
	request := httptest.NewRequest(http.MethodGet, "/v1/messenger/slack/callback", nil)
	request.AddCookie(&http.Cookie{Name: "messenger_csrf_slack", Value: st.Nonce})
	denied := "access_denied"
	result := httptest.NewRecorder()
	server.MessengerOAuthCallback(result, request, "slack", crmcontracts.MessengerOAuthCallbackParams{State: target.Query().Get("state"), Error: &denied})
	landing := result.Header().Get("Location")
	if result.Code != http.StatusFound || !strings.Contains(landing, "/#/deals/"+record.String()+"?messenger=denied") {
		t.Fatalf("lost record return: %d %s", result.Code, landing)
	}
}

func TestMessengerCallbackRefusesAnotherOrReplacedSlackWorkspaceBeforeSaving(t *testing.T) {
	for _, tc := range []struct{ name, started, installed, granted string }{
		{"wrong selected workspace", "T_ONE", "T_ONE", "T_TWO"},
		{"installation replaced during consent", "T_ONE", "T_TWO", "T_TWO"},
		{"installation added during consent", "", "T_ONE", "T_TWO"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, state, now := messengerOAuthFixture()
			state.MessagingWorkspace = tc.started
			server.authority = grantedAuthority{rbac: adminRBAC(), seat: principal.SeatFull}
			exchanged := false
			server.resolve = func(context.Context, string) (messengeraccounts.App, error) {
				return messengeraccounts.App{Workspace: tc.installed, Exchange: func(context.Context, string, string) (chatlinks.MemberGrant, messaging.Credential, error) {
					exchanged = true
					return chatlinks.MemberGrant{Provider: "slack", Workspace: tc.granted, Member: "U_REP"}, messaging.Credential{AccessToken: "synthetic"}, nil
				}}, nil
			}
			token := newStateSigner([]byte(server.messengerOAuthHandlers.cfg.StateKey)).sign(state, now.Add(time.Minute))
			request := httptest.NewRequest(http.MethodGet, "/v1/messenger/slack/callback", nil)
			request.AddCookie(&http.Cookie{Name: "messenger_csrf_slack", Value: state.Nonce})
			code := "synthetic-code"
			reply := httptest.NewRecorder()
			server.MessengerOAuthCallback(reply, request, "slack", crmcontracts.MessengerOAuthCallbackParams{State: token, Code: &code})
			if !exchanged || reply.Code != http.StatusFound || !strings.Contains(reply.Header().Get("Location"), "messenger=failed") {
				t.Fatalf("wrong workspace not refused before nil store: %d %s exchanged=%v", reply.Code, reply.Header().Get("Location"), exchanged)
			}
		})
	}
}

func TestSlackInstallationHistoryProfileSurvivesTheSignedConsentRoundTrip(t *testing.T) {
	server, state, now := messengerOAuthFixture()
	for _, class := range []string{"internal", "marketplace", "non_marketplace"} {
		profile, err := chatwire.SlackHistoryProfile(class, now)
		if err != nil {
			t.Fatal(err)
		}
		state.Provider, state.SlackHistory = "channel:slack", profile
		token := newStateSigner([]byte(server.messengerOAuthHandlers.cfg.StateKey)).sign(state, now.Add(time.Minute))
		got, err := newStateSigner([]byte(server.messengerOAuthHandlers.cfg.StateKey)).verify(token, now)
		if err != nil || got.SlackHistory == nil || *got.SlackHistory != *profile {
			t.Fatalf("lost declared installation profile %s: %+v %v", class, got.SlackHistory, err)
		}
	}
}

func TestEveryRequiredMessengerConnectBodyIDIsNamedWhenAbsent(t *testing.T) {
	t.Parallel()
	_, err := messengerReturnOf(crmcontracts.MessengerConnectRequest{RecordType: "deal"})
	if err == nil || !strings.Contains(err.Error(), "record_id") {
		t.Fatalf("missing record_id must be named before reading a record: %v", err)
	}
}
