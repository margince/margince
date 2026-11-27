// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package whatsapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestVerifyChallenge(t *testing.T) {
	cases := []struct {
		name    string
		query   url.Values
		token   string
		want    string
		wantErr error
	}{
		{"ok", url.Values{"hub.mode": {"subscribe"}, "hub.verify_token": {"tok"}, "hub.challenge": {"1158201444"}}, "tok", "1158201444", nil},
		{"wrong token", url.Values{"hub.mode": {"subscribe"}, "hub.verify_token": {"nope"}, "hub.challenge": {"1"}}, "tok", "", ErrBadVerifyToken},
		{"wrong mode", url.Values{"hub.mode": {"unsubscribe"}, "hub.verify_token": {"tok"}, "hub.challenge": {"1"}}, "tok", "", ErrBadVerifyToken},
		{"no token sent", url.Values{"hub.mode": {"subscribe"}, "hub.challenge": {"1"}}, "tok", "", ErrBadVerifyToken},
		{"no token configured", url.Values{"hub.mode": {"subscribe"}, "hub.verify_token": {""}, "hub.challenge": {"1"}}, "", "", ErrBadVerifyToken},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := VerifyChallenge(tc.query, tc.token)
			if !errors.Is(err, tc.wantErr) || got != tc.want {
				t.Fatalf("VerifyChallenge = %q, %v; want %q, %v", got, err, tc.want, tc.wantErr)
			}
		})
	}
}

func TestVerifySignature(t *testing.T) {
	body := []byte(`{"object":"whatsapp_business_account"}`)
	good := sign("secret", body)
	cases := []struct {
		name, secret, header string
		wantErr              error
	}{
		{"valid", "secret", good, nil},
		{"wrong secret", "other", good, ErrBadSignature},
		{"missing header", "secret", "", ErrBadSignature},
		{"wrong prefix", "secret", "sha1=" + strings.TrimPrefix(good, "sha256="), ErrBadSignature},
		{"no prefix", "secret", strings.TrimPrefix(good, "sha256="), ErrBadSignature},
		{"not hex", "secret", "sha256=zzzz", ErrBadSignature},
		{"truncated", "secret", good[:len(good)-2], ErrBadSignature},
		{"empty secret", "", sign("", body), ErrBadSignature},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := VerifySignature(tc.secret, body, tc.header); !errors.Is(err, tc.wantErr) {
				t.Fatalf("VerifySignature = %v, want %v", err, tc.wantErr)
			}
		})
	}
	if err := VerifySignature("secret", append([]byte(nil), body[1:]...), good); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("a tampered body verified: %v", err)
	}
}

func TestReadBodyStopsAtTheLimit(t *testing.T) {
	post := func(body []byte) ([]byte, error) {
		req := httptest.NewRequest(http.MethodPost, "/hook", strings.NewReader(string(body)))
		return ReadBody(httptest.NewRecorder(), req)
	}
	body := []byte(`{"a":1}`)
	if got, err := post(body); err != nil || string(got) != string(body) {
		t.Fatalf("body = %q, %v", got, err)
	}
	if _, err := post([]byte(strings.Repeat("x", MaxWebhookBodyBytes+1))); !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("oversize body err = %v, want ErrBodyTooLarge", err)
	}
}
