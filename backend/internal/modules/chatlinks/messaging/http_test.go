// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package messaging

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProviderAccessDenialDoesNotRevokeTheCredential(t *testing.T) {
	for _, test := range []struct {
		status int
		want   error
	}{{401, ErrUnauthorized}, {403, ErrForbidden}, {404, ErrNotFound}} {
		t.Run(http.StatusText(test.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(test.status) }))
			defer server.Close()
			client := HTTP{Client: server.Client(), Base: server.URL, Token: "fixture-token"}
			_, err := client.Call(context.Background(), http.MethodGet, "/conversation", nil)
			if !errors.Is(err, test.want) {
				t.Fatalf("status %d: %v", test.status, err)
			}
			if test.status == 403 && errors.Is(err, ErrUnauthorized) {
				t.Fatal("one conversation denial invalidated the whole credential")
			}
		})
	}
}

func TestRichContentKeepsCardFactsAndRejectsUnreadableContent(t *testing.T) {
	text, complete := RichContent([]byte(`{"type":"AdaptiveCard","body":[{"type":"TextBlock","text":"Approval is conditional"},{"type":"FactSet","facts":[{"title":"Owner","value":"Jordan"}]}]}`))
	if !complete || text != "Approval is conditional\nOwner\nJordan" {
		t.Fatalf("card: %q %v", text, complete)
	}
	_, complete = RichContent([]byte(`{"type":"AdaptiveCard","body":[{"type":"Image","url":"https://example.test/image"}]}`))
	if complete {
		t.Fatal("an unread image became complete source text")
	}
}
