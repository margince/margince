// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

func TestSlackTokenRefreshUsesEachCredentialExpiry(t *testing.T) {
	now := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	called := 0
	refresh := func(_ context.Context, previous messaging.Credential) (messaging.Credential, error) {
		called++
		return messaging.Credential{AccessToken: previous.AccessToken + "-fresh", ExpiresAt: now.Add(time.Hour)}, nil
	}
	for _, expiry := range []time.Time{{}, now.Add(time.Hour), now.Add(time.Minute)} {
		previous := messaging.Credential{AccessToken: "synthetic", ExpiresAt: expiry}
		got, err := refreshSlackToken(context.Background(), previous, refresh, now)
		if err != nil {
			t.Fatal(err)
		}
		if expiry.Equal(now.Add(time.Minute)) && got.AccessToken != "synthetic-fresh" {
			t.Fatal("near-expiry credential was not refreshed")
		}
	}
	if called != 1 {
		t.Fatalf("refreshed %d times", called)
	}
}
