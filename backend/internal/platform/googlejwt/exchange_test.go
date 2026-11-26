// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package googlejwt

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestExchangePostsTheAssertionUnderTheJWTBearerGrant(t *testing.T) {
	var grant, assertion string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		grant, assertion = r.PostForm.Get("grant_type"), r.PostForm.Get("assertion")
		_, _ = w.Write([]byte(`{"access_token":"tok-1","expires_in":3599}`))
	}))
	defer srv.Close()

	got, err := Exchange(context.Background(), srv.Client(), srv.URL, "signed.assertion")
	if err != nil {
		t.Fatalf("AccessToken: %v", err)
	}
	if got.AccessToken != "tok-1" || got.ExpiresIn != 3599*time.Second || grant != JWTBearerGrant || assertion != "signed.assertion" {
		t.Fatalf("token=%q grant=%q assertion=%q", got, grant, assertion)
	}
}

func TestExchangeKeepsOnlyTheStatusAndTheOAuthCodeOfARefusal(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		body      string
		code      string
		permanent bool
	}{
		{"delegation refused", http.StatusBadRequest, `{"error":"unauthorized_client","error_description":"Client is unauthorized"}`, "unauthorized_client", true},
		{"prose in the code field is dropped", http.StatusUnauthorized, `{"error":"Some words with the key in them"}`, "", true},
		{"throttled", http.StatusTooManyRequests, `{"error":"rate_limited"}`, "rate_limited", false},
		{"server error", http.StatusBadGateway, `<html>proxy</html>`, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			_, err := Exchange(context.Background(), srv.Client(), srv.URL, "a")
			var refused ErrExchangeRefused
			if !errors.As(err, &refused) {
				t.Fatalf("err = %v, want ErrExchangeRefused", err)
			}
			if refused.Status != tc.status || refused.Code != tc.code || refused.Permanent() != tc.permanent {
				t.Fatalf("refused = %+v permanent=%v", refused, refused.Permanent())
			}
		})
	}
}

func TestExchangeRefusesAnAnswerWithNoToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{}`)) }))
	defer srv.Close()
	if _, err := Exchange(context.Background(), srv.Client(), srv.URL, "a"); err == nil {
		t.Fatal("an answer without an access token must fail")
	}
}
