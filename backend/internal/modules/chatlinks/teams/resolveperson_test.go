// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package teams

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

type directoryDoer func(*http.Request) (*http.Response, error)

func (f directoryDoer) Do(r *http.Request) (*http.Response, error) { return f(r) }

func TestRecordPersonResolutionRefusesAmbiguousOrDifferentGraphAccounts(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		matched       bool
	}{
		{"exact account", `{"value":[{"id":"U2","displayName":"Jordan","mail":"jordan@example.test","userPrincipalName":"jordan@example.test","userType":"Member","accountEnabled":true}]}`, true},
		{"ambiguous account", `{"value":[{"id":"U2","mail":"jordan@example.test"},{"id":"U3","mail":"jordan@example.test"}]}`, false},
		{"different account", `{"value":[{"id":"U2","mail":"other@example.test"}]}`, false},
		{"additional page", `{"value":[{"id":"U2","mail":"jordan@example.test"}],"@odata.nextLink":"https://graph.microsoft.com/v1.0/users?$skip=2"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := New(directoryDoer(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path != "/v1.0/users" || r.URL.Query().Get("$filter") != "mail eq 'jordan@example.test' or userPrincipalName eq 'jordan@example.test'" || r.URL.Query().Get("$top") != "2" {
					t.Fatal("resolution did not bound an exact directory match")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.payload))}, nil
			}), "synthetic-token", "TENANT", "U1")
			member, err := client.ResolvePerson(context.Background(), "jordan@example.test")
			if !tc.matched {
				if !errors.Is(err, messaging.ErrUnavailable) {
					t.Fatalf("unproved Graph member accepted: %v", err)
				}
				return
			}
			if err != nil || member.ID != "U2" || member.External || !member.Active {
				t.Fatalf("exact member: %+v %v", member, err)
			}
		})
	}
}
