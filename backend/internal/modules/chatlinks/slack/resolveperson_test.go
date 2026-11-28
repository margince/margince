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

func TestRecordPersonResolutionRequiresAnExactProviderEmail(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		matched       bool
	}{
		{"exact account", `{"ok":true,"user":{"id":"U2","team_id":"T1","real_name":"Jordan","profile":{"email":"Jordan@Example.Test"}}}`, true},
		{"different account", `{"ok":true,"user":{"id":"U2","team_id":"T1","profile":{"email":"other@example.test"}}}`, false},
		{"unavailable email", `{"ok":false,"error":"users_not_found"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := New(oauthDoer(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path != "/api/users.lookupByEmail" || r.URL.Query().Get("email") != "jordan@example.test" {
					t.Fatal("resolution was not an exact email lookup")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.payload))}, nil
			}), "synthetic-token", "T1", "U1")
			member, err := client.ResolvePerson(context.Background(), "jordan@example.test")
			if !tc.matched {
				if !errors.Is(err, messaging.ErrUnavailable) {
					t.Fatalf("mismatched account accepted: %v", err)
				}
				return
			}
			if err != nil || member.ID != "U2" || member.External || !member.Active {
				t.Fatalf("exact directory member: %+v %v", member, err)
			}
		})
	}
}
