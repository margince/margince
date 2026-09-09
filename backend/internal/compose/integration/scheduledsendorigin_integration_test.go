// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The origin a scheduled message's unsubscribe link is built on.
//
// Split from the scheduled-send suite because it is about a different thing:
// that suite asks when a message fires and who it is attributed to, and this
// asks what the worker was composed WITH. The distinction is what the lane was
// missing — every case there claims the one purpose that needs no link, so the
// worker's send path was never asked for an origin and a missing one passed.

import (
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A scheduled message that CARRIES an unsubscribe link fires.
//
// This is the case the whole lane was missing. Every other scheduled-send
// fixture claims the transactional purpose, which needs no link — so the lane
// exercised the one shape that cannot reach the origin, and the worker composed
// with none passed everything. A fixture that always picks the cheap option is
// a census of one.
//
// The link is built from the installation's public base URL, and an empty one
// refuses rather than emitting a forgeable address. Without the origin on the
// worker's send path that refusal is what a marketing or correspondence message
// meets at fire time, in production, having been accepted at schedule time.
func TestAScheduledMessageCarryingAnUnsubscribeLinkStillFires(t *testing.T) {
	p := setupPreflight(t)
	// Without the send grant the schedule refuses at the pre-flight, and this
	// case would fail before it reached the origin it is about.
	p.connect(t, gmailReadonlyScope, gmailSendScope)
	p.seedConsentedRecipient(t, "Link Recipient", "linked@preflight.test")
	// The recipient wrote to us first, which is the qualifying event a
	// correspondence send derives its basis from. The purpose matters here only
	// because business_correspondence CARRIES a one-click unsubscribe link,
	// which is the whole subject: the link is what needs an origin to point at.
	var person struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if status := p.Call(t, "GET", "/v1/people?q=linked@preflight.test", nil, nil, &person); status != http.StatusOK || len(person.Data) == 0 {
		t.Fatalf("finding the seeded recipient → %d (%d rows)", status, len(person.Data))
	}
	if status := p.Call(t, "POST", "/v1/activities", AnyMap{
		"kind": "email", "subject": "Inbound question", "direction": "inbound",
		"links": []AnyMap{{"entity_type": "person", "entity_id": person.Data[0].ID}},
	}, nil, nil); status != http.StatusCreated {
		t.Fatalf("logging the inbound the reply answers → %d", status)
	}

	var scheduled struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	status := p.Call(t, "POST", "/v1/activities/"+p.activityID+"/send-email", AnyMap{
		"subject": "Re: your question", "body": "Written the night before.",
		"to": []string{"linked@preflight.test"}, "consent_purpose": "business_correspondence",
		"scheduled_at": time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
		"scheduled_tz": "Europe/Berlin",
	}, nil, &scheduled)
	if status != http.StatusCreated {
		t.Fatalf("scheduling a link-carrying message → %d, want 201", status)
	}
	id, err := ids.Parse(scheduled.ID)
	if err != nil {
		t.Fatalf("scheduling returned no id: %v", err)
	}

	p.makeDue(t, id)
	p.fire(t, id)

	if state, reason := p.scheduledStatus(t, id); state != activities.ScheduledStatusReleased {
		t.Fatalf("a link-carrying message fired to %q (%q), want released — the worker's send path carries no public origin, so the link it must build has nowhere to point",
			state, reason)
	}
}
