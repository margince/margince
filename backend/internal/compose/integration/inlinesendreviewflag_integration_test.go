// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// The composer's switch reaches the session: on until an installation turns it
// off, then off for the next /me.
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
	if on := read(); on == nil || !*on {
		t.Fatalf("inline_send_review = %v before anyone chose, want true", on)
	}
	if _, err := e.Owner.Exec(t.Context(), `
		INSERT INTO setting (key, value, updated_at) VALUES ('activities.inline_send_review', 'false'::jsonb, now())
		ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = now()`); err != nil {
		t.Fatalf("turning the switch off: %v", err)
	}
	if on := read(); on == nil || *on {
		t.Fatalf("inline_send_review = %v after the switch was turned off, want false", on)
	}
}
