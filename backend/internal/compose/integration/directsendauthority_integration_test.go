// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"
)

// Only a caller who may direct a held message, and who confirmed the refusals
// it holds, may release it. Each is refused before the message is tried again,
// even when a consent granted since would now let it go.
func TestOnlyAnAdmittedDirectorReleasesAHeldMessage(t *testing.T) {
	unacknowledged := directed()
	unacknowledged["acknowledged"] = false
	misread := directedShowing("no_compatible_evidence")
	for name, c := range map[string]struct {
		body     AnyMap
		readOnly bool
		want     int
	}{
		"a reader without the exception grant": {directed(), true, http.StatusForbidden},
		"a director who did not acknowledge":   {unacknowledged, false, http.StatusUnprocessableEntity},
		"a director shown other refusals":      {misread, false, http.StatusUnprocessableEntity},
	} {
		t.Run(name, func(t *testing.T) {
			env := setupConsent(t)
			if status, _ := env.send(t, "marketing_email"); status != http.StatusConflict {
				t.Fatalf("a marketing send without consent → %d, want 409", status)
			}
			review := liveReviewID(t, env)
			env.grantMarketingByConfirmLink(t)
			if c.readOnly {
				if _, err := env.Owner.Exec(t.Context(), `
					UPDATE role SET permissions = jsonb_set(
						permissions, '{objects,communication_exception}',
						'{"create":false,"read":true,"update":false,"delete":false}'::jsonb, true)`); err != nil {
					t.Fatalf("leaving only the read grant: %v", err)
				}
			}
			before := env.count(t, `SELECT count(*) FROM comms_outbound`)

			if status := env.Call(t, "POST", "/v1/communication-reviews/"+review+"/direct-send", c.body, nil, nil); status != c.want {
				t.Fatalf("directing the send → %d, want %d", status, c.want)
			}
			if after := env.count(t, `SELECT count(*) FROM comms_outbound`); after != before {
				t.Fatalf("%d deliveries after the refused direction, want %d", after, before)
			}
			if held := env.count(t, `SELECT count(*) FROM scheduled_send WHERE status = 'held'`); held != 1 {
				t.Fatalf("%d held messages after the refused direction, want 1", held)
			}
		})
	}
}
