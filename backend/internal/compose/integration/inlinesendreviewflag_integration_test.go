// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// The composer's pilot switch reaches the session: off until an installation
// turns it on, then on for the next /me.
func TestTheInlineSendReviewSwitchReachesTheSession(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Inline Pilot", "admin@inline-pilot.test", "Admin")
	read := func() *bool {
		var me struct {
			InlineSendReview *bool `json:"inline_send_review"`
		}
		if status := e.Call(t, "GET", "/v1/me", nil, nil, &me); status != http.StatusOK {
			t.Fatalf("GET /v1/me → %d", status)
		}
		return me.InlineSendReview
	}
	if on := read(); on == nil || *on {
		t.Fatalf("inline_send_review = %v before anyone chose, want false", on)
	}
	if _, err := e.Owner.Exec(t.Context(), `
		INSERT INTO setting (key, value, updated_at) VALUES ('activities.inline_send_review', 'true'::jsonb, now())
		ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = now()`); err != nil {
		t.Fatalf("turning the pilot on: %v", err)
	}
	if on := read(); on == nil || !*on {
		t.Fatalf("inline_send_review = %v after the pilot was turned on, want true", on)
	}
}
