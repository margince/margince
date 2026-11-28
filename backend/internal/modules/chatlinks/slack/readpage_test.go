// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package slack

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

func TestPersonalPageUsesNativeReadAccessWithoutDirectoryRequests(t *testing.T) {
	for _, denied := range []bool{false, true} {
		t.Run(map[bool]string{false: "readable", true: "denied"}[denied], func(t *testing.T) {
			client := New(oauthDoer(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path != "/api/conversations.history" {
					t.Fatalf("history requested unrelated roster: %s", r.URL.Path)
				}
				body := `{"ok":true,"messages":[{"ts":"1795756000.123456","user":"U2","attachments":[{"pretext":"Trial depends on legal approval","text":"Confirm Friday"}]}]}`
				if denied {
					body = `{"ok":false,"error":"not_in_channel"}`
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
			}), "participant", "TEAM", "REP")
			page, err := client.Messages(context.Background(), "PRIVATE", "", "")
			if denied {
				if !errors.Is(err, messaging.ErrUnavailable) {
					t.Fatalf("native denial bypassed: %v", err)
				}
				return
			}
			if err != nil || len(page.Messages) != 1 || !strings.Contains(page.Messages[0].Text, "legal approval") || !strings.Contains(page.Messages[0].Text, "Confirm Friday") {
				t.Fatalf("legacy attachment omitted: %+v %v", page, err)
			}
		})
	}
}

func TestPrivateBotDiscoveryFallsBackWhenPublicScopesNeedReinstall(t *testing.T) {
	requests := []string{}
	bot := NewBot(oauthDoer(func(r *http.Request) (*http.Response, error) {
		requested := r.URL.Query().Get("types")
		requests = append(requests, requested)
		body := `{"ok":false,"error":"missing_scope"}`
		if requested == "private_channel" {
			body = `{"ok":true,"channels":[{"id":"PRIVATE","name":"Pilot","is_private":true,"is_member":true}]}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	}), "bot", "TEAM", "BOT")
	channels, err := bot.Channels(context.Background())
	if err != nil || len(channels) != 1 || channels[0].Slug != "PRIVATE" || len(requests) != 2 {
		t.Fatalf("private installation regressed: %+v %v requests=%v", channels, err, requests)
	}
}
