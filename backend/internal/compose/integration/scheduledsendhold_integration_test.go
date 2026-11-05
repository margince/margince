// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What a queued message does when its recipient answers before it is due. A
// stop holds marketing at once. A reply pauses a deal follow-up until the rep
// has read it. Both go through the real scheduling door, the real fire
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

// A stop holds the contact's queued marketing when it is recorded, not when
// the message comes due, and raises one card in the rep's inbox. Mail with
// another reason stays queued, and a stop delivered twice holds nothing twice
// and raises no second card.
//
// Mutation: make PendingMarketingToContact match no context, and the marketing
// message stays scheduled; build the consumer's store without the send
// assembly, and no card is raised.
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
	if _, found := p.heldCardFor(t, marketing); !found {
		t.Fatal("the stopped message raised no card in its rep's inbox")
	}
	if n := p.cardsFor(t, marketing); n != 1 {
		t.Fatalf("a stop delivered twice raised %d cards, want 1", n)
	}
}

// cardsFor counts the approval rows raised for one held message.
func (p *preflightEnv) cardsFor(t *testing.T, id ids.UUID) int {
	t.Helper()
	var n int
	if err := apptest.InWorkspace(p.AppEnv, t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM approval WHERE proposed_change->>'scheduled_send_id' = $1`, id.String()).Scan(&n)
	}); err != nil {
		t.Fatalf("counting the cards: %v", err)
	}
	return n
}

// A withdrawal reaches only the marketing it withdrew, and a lead's
// consent.changed is not about any contact's queue.
func TestAWithdrawalHoldsOnlyTheMarketingItCovers(t *testing.T) {
	p := setupPreflight(t)
	p.connect(t, gmailReadonlyScope, gmailSendScope)
	newsletter := p.scheduleClaiming(t, AnyMap{"communication_context": "marketing", "marketing_purpose": "newsletter"})
	events := p.scheduleClaiming(t, AnyMap{"communication_context": "marketing", "marketing_purpose": "events"})
	// The legacy purpose key names the purpose the engine judges, so a
	// withdrawal reaches it; a message naming no purpose is judged against it.
	legacy := p.scheduleClaiming(t, AnyMap{"communication_context": "marketing", "consent_purpose": "Newsletter"})
	purposeless := p.scheduleClaiming(t, AnyMap{"communication_context": "marketing"})
	legacyOther := p.scheduleClaiming(t, AnyMap{"communication_context": "marketing", "consent_purpose": "events"})

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
	if status, _ := p.scheduledStatus(t, legacyOther); status != activities.ScheduledStatusScheduled {
		t.Errorf("a legacy key for another purpose reads %q after a newsletter withdrawal, want it still scheduled", status)
	}
	for name, id := range map[string]ids.UUID{"the legacy purpose key": legacy, "no purpose at all": purposeless} {
		if status, reason := p.scheduledStatus(t, id); status != activities.ScheduledStatusHeld || reason != activities.HeldRecipientStopped {
			t.Errorf("%s: status %q reason %q, want held", name, status, reason)
		}
	}
}

// An address the stopped contact no longer holds is not theirs. After it moved
// to another contact, a stop on the first holds nothing sent to it. A stop on
// the contact who holds it now does.
//
// Mutation: drop `ce.archived_at IS NULL` from PendingMarketingToContact, and
// the first stop holds the message.
func TestAStopDoesNotReachAnAddressTheContactNoLongerHolds(t *testing.T) {
	p := setupPreflight(t)
	p.connect(t, gmailReadonlyScope, gmailSendScope)
	if err := apptest.InWorkspace(p.AppEnv, t, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE contact_email SET archived_at = now() WHERE contact_id = $1`, uuidOf(t, p.contactID))
		return err
	}); err != nil {
		t.Fatalf("taking the address off the first contact: %v", err)
	}
	var holder struct {
		ID string `json:"id"`
	}
	if status := p.Call(t, "POST", "/v1/contacts", AnyMap{
		"source": "manual", "full_name": "New Holder",
		"emails": []AnyMap{{"email": "buyer@preflight.test"}},
	}, nil, &holder); status != http.StatusCreated {
		t.Fatalf("giving the address to another contact → %d", status)
	}
	marketing := p.scheduleClaiming(t, AnyMap{"communication_context": "marketing"})

	stop := compose.NewScheduledSendStop(p.Pool, identity.NewService(p.Pool), slog.New(slog.NewTextHandler(io.Discard, nil)))
	suppressed := func(contact string) kevents.Envelope {
		return kevents.Envelope{
			EventID: ids.NewV7(), Type: "consent.suppressed", Version: 1, OccurredAt: time.Now().UTC(),
			Entity:  kevents.EntityRef{Type: "contact", ID: uuidOf(t, contact)},
			Payload: []byte(`{"kind":"objection","decided_by_level":"subject"}`),
		}
	}
	if err := stop.HandleEvent(context.Background(), suppressed(p.contactID)); err != nil {
		t.Fatalf("handling the first contact's stop: %v", err)
	}
	if status, _ := p.scheduledStatus(t, marketing); status != activities.ScheduledStatusScheduled {
		t.Fatalf("a stop on the address's former holder held the message: %q", status)
	}
	if err := stop.HandleEvent(context.Background(), suppressed(holder.ID)); err != nil {
		t.Fatalf("handling the holder's stop: %v", err)
	}
	if status, reason := p.scheduledStatus(t, marketing); status != activities.ScheduledStatusHeld || reason != activities.HeldRecipientStopped {
		t.Fatalf("a stop on the address's holder: status %q reason %q, want held", status, reason)
	}
}
