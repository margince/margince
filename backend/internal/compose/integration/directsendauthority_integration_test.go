// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"slices"
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

// Only the author of a message held at Send may send it anyway, because the
// release goes from the caller's mailbox. A second admin holding the exception
// grant is refused before anything is recorded or sent; the author's own
// direction sends it once.
//
// Mutation: drop both author checks (directableReview.admits and
// refuseAnotherSeatsHold) and the colleague sends the message. Each alone has
// its own test in its module.
func TestOnlyTheAuthorReleasesAMessageHeldAtSend(t *testing.T) {
	c := setupConsent(t)
	c.useReviewableMarketingProfile(t)
	var refused struct {
		Details struct {
			AvailableActions []string `json:"available_actions"`
		} `json:"details"`
	}
	if status := c.Call(t, "POST", "/v1/activities/"+c.activityID+"/send-email", AnyMap{
		"subject": "Re: Inbound question", "body": "answer",
		"to": []string{"subject@consent.test"}, "consent_purpose": "marketing_email",
	}, nil, &refused); status != http.StatusConflict {
		t.Fatalf("a marketing send without consent → %d, want 409", status)
	}
	if !slices.Equal(refused.Details.AvailableActions, []string{"direct_send"}) {
		t.Fatalf("the author is offered %v, want [direct_send]", refused.Details.AvailableActions)
	}
	review := liveReviewID(t, c)
	path := "/v1/communication-reviews/" + review + "/direct-send"

	var denied struct {
		Code string `json:"code"`
	}
	if status := c.colleague(t).Call(t, "POST", path, directed(), nil, &denied); status != http.StatusForbidden {
		t.Fatalf("a second admin directing the author's message → %d, want 403", status)
	}
	if denied.Code != "permission_denied" {
		t.Errorf("the refusal reads code %q, want permission_denied", denied.Code)
	}
	if n := c.count(t, `SELECT count(*) FROM communication_instruction`); n != 0 {
		t.Fatalf("%d instruction(s) after the refused direction, want 0", n)
	}
	if n := c.count(t, `SELECT count(*) FROM comms_outbound`); n != 0 {
		t.Fatalf("%d deliveries after the refused direction, want 0", n)
	}
	if n := c.count(t, `SELECT count(*) FROM scheduled_send WHERE status = 'held' AND held_reason = 'send_refused'`); n != 1 {
		t.Fatalf("%d messages held at Send after the refused direction, want 1", n)
	}

	if status := c.Call(t, "POST", path, directed(), nil, nil); status != http.StatusCreated {
		t.Fatalf("the author directing their own message → %d, want 201", status)
	}
	if n := c.count(t, `SELECT count(*) FROM comms_outbound`); n != 1 {
		t.Fatalf("%d deliveries after the author directed it, want 1", n)
	}
}
