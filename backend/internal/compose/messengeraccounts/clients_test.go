// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package messengeraccounts

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

func TestSlackPersonalClientPinsAnyInstalledWorkspaceWithoutRequiringChannelSync(t *testing.T) {
	for _, tc := range []struct {
		name, installed  string
		missing, refused bool
	}{
		{name: "same workspace", installed: "T_ONE"},
		{name: "other workspace", installed: "T_TWO", refused: true},
		{name: "personal only", missing: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := Clients(false, func(_ context.Context, provider string) (capture.LiveBot, error) {
				if provider != "slack" {
					t.Fatalf("wrong installation %s", provider)
				}
				if tc.missing {
					return capture.LiveBot{}, capture.ErrNoConnection
				}
				return capture.LiveBot{TeamID: tc.installed}, nil
			})
			got, err := client(context.Background(), chatlinks.MemberGrant{Provider: "slack", Workspace: "T_ONE", Member: "U_REP"}, []byte(`{"access_token":"synthetic"}`))
			if tc.refused {
				if !errors.Is(err, messaging.ErrUnauthorized) || got != nil {
					t.Fatalf("opened wrong workspace: %v", err)
				}
			} else if err != nil || got == nil {
				t.Fatalf("refused authorized personal client: %v", err)
			}
		})
	}
}
