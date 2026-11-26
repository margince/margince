// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package zalo

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type seen struct {
	method, path, query, contentType, body string
	header                                 http.Header
}

// stub answers every request with one canned status and body and records the
// request it got.
func stub(t *testing.T, status int, body string) (*Client, *seen) {
	t.Helper()
	got := &seen{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		*got = seen{r.Method, r.URL.Path, r.URL.Query().Get("data"), r.Header.Get("Content-Type"), string(raw), r.Header.Clone()}
		w.WriteHeader(status)
		if _, err := io.WriteString(w, body); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(srv.Close)
	return NewClient(srv.Client(), srv.URL+"/", srv.URL), got
}

func ctx() context.Context { return context.Background() }

func TestRefreshReturnsTheNewPair(t *testing.T) {
	t.Parallel()
	for _, file := range []string{"refresh_ok.json", "refresh_ok_number.json"} {
		t.Run(file, func(t *testing.T) {
			t.Parallel()
			c, got := stub(t, 200, string(fixture(t, file)))
			pair, err := c.Refresh(ctx(), App{ID: "app1", Secret: "app-secret"}, "old-refresh")
			if err != nil {
				t.Fatal(err)
			}
			if pair.AccessToken != "new-access" || pair.RefreshToken != "new-refresh" || pair.ExpiresIn != 90000*time.Second {
				t.Fatalf("pair %+v", pair)
			}
			if got.path != "/v4/oa/access_token" || got.method != http.MethodPost || got.header.Get("secret_key") != "app-secret" {
				t.Fatalf("request %+v", got)
			}
			for _, want := range []string{"refresh_token=old-refresh", "app_id=app1", "grant_type=refresh_token"} {
				if !strings.Contains(got.body, want) {
					t.Fatalf("form %q lacks %q", got.body, want)
				}
			}
		})
	}
}

func TestRefreshFailures(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"expired refresh token", 200, string(fixture(t, "refresh_expired.json")), ErrReauthRequired},
		{"expired on a 400", 400, string(fixture(t, "refresh_expired.json")), ErrReauthRequired},
		{"throttled", 429, `{"error":-32,"error_name":"rate"}`, ErrRateLimited},
		{"no new refresh token", 200, `{"access_token":"a","expires_in":10}`, ErrUnreachable},
		{"no expiry", 200, `{"access_token":"a","refresh_token":"r"}`, ErrUnreachable},
		{"server error", 502, `bad gateway`, ErrUnreachable},
		{"not json", 200, `oops`, ErrUnreachable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, _ := stub(t, tc.status, tc.body)
			pair, err := c.Refresh(ctx(), App{ID: "a", Secret: "s"}, "old")
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			if pair != (TokenPair{}) {
				t.Fatalf("a failed refresh returned %+v", pair)
			}
		})
	}
}

func TestSendTextSuccess(t *testing.T) {
	t.Parallel()
	c, got := stub(t, 200, string(fixture(t, "send_ok.json")))
	id, err := c.SendText(ctx(), "tok", "5550001", "Chào bạn")
	if err != nil {
		t.Fatal(err)
	}
	if id != "out-77" || got.path != "/v3.0/oa/message/cs" || got.header.Get("access_token") != "tok" {
		t.Fatalf("id %q request %+v", id, got)
	}
	var body struct {
		Recipient struct {
			UserID string `json:"user_id"`
		} `json:"recipient"`
		Message struct {
			Text string `json:"text"`
		} `json:"message"`
	}
	mustUnmarshal(t, []byte(got.body), &body)
	if body.Recipient.UserID != "5550001" || body.Message.Text != "Chào bạn" {
		t.Fatalf("body %s", got.body)
	}
}

func TestSendTextErrorMapping(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		status int
		body   string
		want   error
		code   int
	}{
		{"token invalid", 200, `{"error":-216,"message":"Access token is invalid"}`, ErrReauthRequired, -216},
		{"token expired", 200, `{"error":-124,"message":"expired"}`, ErrReauthRequired, -124},
		{"error as string", 200, `{"error":"-216","message":"x"}`, ErrReauthRequired, -216},
		{"rate limited", 200, `{"error":-32,"message":"limit"}`, ErrRateLimited, -32},
		{"http 429", 429, ``, ErrRateLimited, 429},
		{"outside window", 200, `{"error":-230,"message":"no interaction"}`, ErrOutsideReplyWindow, -230},
		{"not a follower", 200, `{"error":-213,"message":"not followed"}`, ErrOutsideReplyWindow, -213},
		{"unknown user", 200, `{"error":-204,"message":"no user"}`, ErrInvalidRecipient, -204},
		{"other code", 200, `{"error":-999,"message":"?"}`, ErrRejected, -999},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, _ := stub(t, tc.status, tc.body)
			_, err := c.SendText(ctx(), "tok-9f3", "u", "secret words")
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			var api *APIError
			if !errors.As(err, &api) || api.Code != tc.code {
				t.Fatalf("api error %+v, want code %d", api, tc.code)
			}
			if strings.Contains(err.Error(), "secret words") || strings.Contains(err.Error(), "tok-9f3") {
				t.Fatalf("error leaks request content: %v", err)
			}
		})
	}
}

func TestSendTextUnknownOutcomeIsUnreachable(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		status int
		body   string
	}{
		"5xx":              {500, `x`},
		"not json":         {200, `<html>`},
		"no error code":    {200, `{"data":{"message_id":"m"}}`},
		"success, no id":   {200, `{"error":0,"data":{}}`},
		"data wrong shape": {200, `{"error":0,"data":"x"}`},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c, _ := stub(t, tc.status, tc.body)
			if _, err := c.SendText(ctx(), "tok", "u", "hi"); !errors.Is(err, ErrUnreachable) {
				t.Fatalf("got %v, want ErrUnreachable", err)
			}
		})
	}
}

func TestSendTextTransportFailureDoesNotLeakTheToken(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	c := NewClient(srv.Client(), srv.URL, srv.URL)
	srv.Close()
	_, err := c.SendText(ctx(), "tok-secret", "u", "hi")
	if !errors.Is(err, ErrUnreachable) || strings.Contains(err.Error(), "tok-secret") {
		t.Fatalf("got %v", err)
	}
}

func TestUserDetail(t *testing.T) {
	t.Parallel()
	c, got := stub(t, 200, string(fixture(t, "user_detail.json")))
	u, err := c.UserDetail(ctx(), "tok", "5550001")
	if err != nil {
		t.Fatal(err)
	}
	if u.ID != "5550001" || u.UserIDByApp != "7777888899990000" || u.DisplayName != "Nguyễn Văn A" || u.AvatarURL == "" {
		t.Fatalf("user %+v", u)
	}
	var q struct {
		UserID string `json:"user_id"`
	}
	mustUnmarshal(t, []byte(got.query), &q)
	if got.method != http.MethodGet || got.path != "/v3.0/oa/user/detail" || q.UserID != "5550001" || got.header.Get("access_token") != "tok" {
		t.Fatalf("request %+v", got)
	}
}

func TestUserDetailMapsErrors(t *testing.T) {
	t.Parallel()
	c, _ := stub(t, 200, `{"error":-216,"message":"bad token"}`)
	if _, err := c.UserDetail(ctx(), "tok", "u"); !errors.Is(err, ErrReauthRequired) {
		t.Fatalf("got %v", err)
	}
}

func TestNewClientDefaults(t *testing.T) {
	t.Parallel()
	c := NewClient(nil, "", "")
	if c.oauth != defaultOAuthBase || c.openapi != defaultOpenAPIBase || c.http.Timeout != httpTimeout {
		t.Fatalf("defaults %+v", c)
	}
}

func mustUnmarshal[T any](t *testing.T, b []byte, v *T) {
	t.Helper()
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}
