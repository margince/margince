// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What a queued message does when its recipient answers before it is due: a
// stop holds marketing at once, and a reply pauses a deal follow-up until the
// rep has read it. Both go through the real scheduling door, the real fire
// path and the real consumer.

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/identity"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// scheduleClaiming schedules a send through the reply door with the reason
// the rep claimed, and returns the scheduled record's id.
func (p *preflightEnv) scheduleClaiming(t *testing.T, claim AnyMap) ids.UUID {
	t.Helper()
	body := AnyMap{
		"subject": "Next steps", "body": "Written the night before.",
		"to":           []string{"buyer@preflight.test"},
		"scheduled_at": time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
		"scheduled_tz": "Europe/Berlin",
	}
	for key, value := range claim {
		body[key] = value
	}
	var scheduled struct {
		ID string `json:"id"`
	}
	if status := p.Call(t, "POST", "/v1/activities/"+p.activityID+"/send-email", body, nil, &scheduled); status != http.StatusCreated {
		t.Fatalf("scheduling %v → %d, want 201", claim, status)
	}
	return uuidOf(t, scheduled.ID)
}

// theyReply plants an authenticated inbound mail from the recipient, as
// capture writes one, dated now: after the message was scheduled.
func (p *preflightEnv) theyReply(t *testing.T, authenticated bool) {
	t.Helper()
	if err := apptest.InWorkspace(p.AppEnv, t, func(tx pgx.Tx) error {
		reply := ids.NewV7()
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, direction, subject, occurred_at, source, captured_by, inbound_sender_authenticated)
			VALUES ($1, 'email', 'inbound', 'Re: Next steps', now(), 'gmail', 'connector:gmail', $2)`,
			reply, authenticated); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO activity_participant (activity_id, address, role) VALUES ($1, 'buyer@preflight.test', 'from')`, reply)
		return err
	}); err != nil {
		t.Fatalf("planting the reply: %v", err)
	}
}

// A reply after a deal follow-up was scheduled holds the follow-up for the
// rep. A message with no deal claim is not pursuit and still goes, and a
// sender the mailbox could not authenticate pauses nothing.
//
// Mutation: drop the pursuitPausedByReply call from changedScheduledContext,
// and the deal follow-up is sent.
func TestAReplyPausesAScheduledDealFollowUp(t *testing.T) {
	for _, tc := range []struct {
		name          string
		claim         AnyMap
		authenticated bool
		wantHeld      bool
	}{
		{"a deal follow-up after a reply", AnyMap{"communication_context": "active_deal_followup"}, true, true},
		{"a deal follow-up after a forged reply", AnyMap{"communication_context": "active_deal_followup"}, false, false},
		{"a message with no deal claim", AnyMap{"consent_purpose": "transactional"}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := setupPreflight(t)
			p.connect(t, gmailReadonlyScope, gmailSendScope)
			id := p.scheduleClaiming(t, tc.claim)

			p.theyReply(t, tc.authenticated)
			p.makeDue(t, id)
			p.fire(t, id)

			status, reason := p.scheduledStatus(t, id)
			held := status == activities.ScheduledStatusHeld && reason == activities.HeldRecipientReplied
			if held != tc.wantHeld {
				t.Fatalf("status %q reason %q, want held for a reply = %v", status, reason, tc.wantHeld)
			}
		})
	}
}

// A stop holds the contact's queued marketing when it is recorded, not when
// the message comes due. Mail with another reason stays queued, and a stop
// delivered twice holds nothing twice.
//
// Mutation: make HoldMarketingToContact match no context, and the marketing
// message stays scheduled.
func TestAStopHoldsQueuedMarketingAtOnce(t *testing.T) {
	p := setupPreflight(t)
	p.connect(t, gmailReadonlyScope, gmailSendScope)
	marketing := p.scheduleClaiming(t, AnyMap{"communication_context": "marketing"})
	other := p.scheduleClaiming(t, AnyMap{"consent_purpose": "transactional"})

	stop := compose.NewScheduledSendStop(p.Pool, identity.NewService(p.Pool), slog.New(slog.NewTextHandler(io.Discard, nil)))
	env := kevents.Envelope{
		EventID: ids.NewV7(), Type: "consent.suppressed", Version: 1, OccurredAt: time.Now().UTC(),
		Entity:  kevents.EntityRef{Type: "contact", ID: uuidOf(t, p.contactID)},
		Payload: []byte(`{"kind":"objection","decided_by_level":"subject"}`),
	}
	for range 2 {
		if err := stop.HandleEvent(context.Background(), env); err != nil {
			t.Fatalf("handling the stop: %v", err)
		}
	}

	if status, reason := p.scheduledStatus(t, marketing); status != activities.ScheduledStatusHeld || reason != activities.HeldRecipientStopped {
		t.Fatalf("marketing after a stop: status %q reason %q, want held as %q", status, reason, activities.HeldRecipientStopped)
	}
	if status, _ := p.scheduledStatus(t, other); status != activities.ScheduledStatusScheduled {
		t.Fatalf("a message with another reason reads %q after a marketing stop, want it still scheduled", status)
	}
}

// A withdrawal reaches only the marketing it withdrew, and a lead's
// consent.changed is not about any contact's queue.
func TestAWithdrawalHoldsOnlyTheMarketingItCovers(t *testing.T) {
	p := setupPreflight(t)
	p.connect(t, gmailReadonlyScope, gmailSendScope)
	newsletter := p.scheduleClaiming(t, AnyMap{"communication_context": "marketing", "marketing_purpose": "newsletter"})
	events := p.scheduleClaiming(t, AnyMap{"communication_context": "marketing", "marketing_purpose": "events"})

	stop := compose.NewScheduledSendStop(p.Pool, identity.NewService(p.Pool), slog.New(slog.NewTextHandler(io.Discard, nil)))
	changed := func(entityType string) kevents.Envelope {
		return kevents.Envelope{
			EventID: ids.NewV7(), Type: "consent.changed", Version: 1, OccurredAt: time.Now().UTC(),
			Entity:  kevents.EntityRef{Type: entityType, ID: uuidOf(t, p.contactID)},
			Payload: []byte(`{"purpose_id":"` + ids.NewV7().String() + `","purpose":"newsletter","new_state":"withdrawn"}`),
		}
	}
	if err := stop.HandleEvent(context.Background(), changed("lead")); err != nil {
		t.Fatalf("handling a lead's withdrawal: %v", err)
	}
	if status, _ := p.scheduledStatus(t, newsletter); status != activities.ScheduledStatusScheduled {
		t.Fatalf("a lead's withdrawal held a contact's message: %q", status)
	}
	if err := stop.HandleEvent(context.Background(), changed("contact")); err != nil {
		t.Fatalf("handling the withdrawal: %v", err)
	}
	if status, reason := p.scheduledStatus(t, newsletter); status != activities.ScheduledStatusHeld || reason != activities.HeldRecipientStopped {
		t.Fatalf("the withdrawn purpose: status %q reason %q, want held", status, reason)
	}
	if status, _ := p.scheduledStatus(t, events); status != activities.ScheduledStatusScheduled {
		t.Fatalf("another purpose reads %q after a newsletter withdrawal, want it still scheduled", status)
	}
}
