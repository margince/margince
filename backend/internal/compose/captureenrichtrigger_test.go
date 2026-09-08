// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// enrichEvent builds an envelope of one type carrying one payload, occurring
// now. A payload marshalling failure is the test's own bug, so it fails here
// rather than travelling into the subject as an empty body that every arm
// happens to refuse.
//
// The payload is already JSON, so each caller marshals its own typed contract
// struct and this takes the bytes. A helper that marshalled for them would need
// to accept every payload type at once, and the one signature that does is the
// one that stops the compiler checking any of them.
func enrichEvent(t *testing.T, eventType, entityType string, payload json.RawMessage) events.Envelope {
	t.Helper()
	return events.Envelope{
		Type:       eventType,
		EventID:    ids.NewV7(),
		OccurredAt: time.Now().UTC(),
		Entity:     events.EntityRef{Type: entityType, ID: ids.NewV7()},
		Payload:    payload,
	}
}

// noPayload is the body of an event this consumer routes on its TYPE alone.
var noPayload = json.RawMessage(`{}`)

func capturedMail(t *testing.T, kind string) events.Envelope {
	t.Helper()
	body, err := json.Marshal(crmcontracts.PublicEventActivityCaptured{Kind: kind})
	if err != nil {
		t.Fatalf("marshalling a captured %s: %v", kind, err)
	}
	return enrichEvent(t, "activity.captured", "activity", body)
}

func audienceChange(t *testing.T, to crmcontracts.PublicEventActivityChangedFieldsAudience) events.Envelope {
	t.Helper()
	body, err := json.Marshal(crmcontracts.PublicEventActivityUpdated{
		ChangedFields: crmcontracts.PublicEventActivityChangedFields{Audience: &to},
	})
	if err != nil {
		t.Fatalf("marshalling an audience change to %s: %v", to, err)
	}
	return enrichEvent(t, "activity.updated", "activity", body)
}

// quietTrigger is the subject with no pool and no runner. Every case below
// stops at the envelope, so neither is ever reached — and a nil pool is the
// assertion that it was not: anything that queued or queried would panic.
func quietTrigger() *CaptureEnrichTrigger {
	return &CaptureEnrichTrigger{log: slog.New(slog.DiscardHandler)}
}

// The three doors, each named by what it is the only notice of. Without the
// person door a sender's FIRST mail — the one carrying their signature block —
// is read by nothing until the next day's reconciler, because the contact did
// not exist when their mail landed and no later event says it now does.
func TestTheSignaturePassIsQueuedByEveryDoorThatMakesSomebodyReadable(t *testing.T) {
	g := quietTrigger()
	ctx := context.Background()

	for name, env := range map[string]events.Envelope{
		"mail landing":           capturedMail(t, "email"),
		"a contact appearing":    enrichEvent(t, "person.created", "person", noPayload),
		"a held message opening": audienceChange(t, crmcontracts.Workspace),
	} {
		if !g.queues(ctx, env) {
			t.Errorf("%s: queued no pass, so nobody reads that signature until tomorrow", name)
		}
	}
}

// The refusals, and each one costs a model-backed pass when it is wrong.
func TestTheSignaturePassIsNotQueuedByWhatCannotMakeSomebodyReadable(t *testing.T) {
	g := quietTrigger()
	ctx := context.Background()

	for name, env := range map[string]events.Envelope{
		// Nothing but mail carries a signature block.
		"a captured meeting": capturedMail(t, "meeting"),
		"a captured call":    capturedMail(t, "call"),
		// The derivation emits the same event for a NARROWING, and the pass
		// reads open mail only — so this arm queues a paid pass per held
		// message if it stops reading the value.
		"a message being narrowed to its participants": audienceChange(t, crmcontracts.Participants),
		"a message narrowed to named seats":            audienceChange(t, crmcontracts.Selected),
		// An update about something else entirely.
		"an update naming no audience": enrichEvent(t, "activity.updated", "activity", noPayload),
		// A person who cannot have become newly readable.
		"a contact being archived": enrichEvent(t, "person.archived", "person", noPayload),
		"a contact being updated":  enrichEvent(t, "person.updated", "person", noPayload),
		// Streams the group also carries for its sibling consumers.
		"an unrelated entity's event": enrichEvent(t, "organization.created", "organization", noPayload),
	} {
		if g.queues(ctx, env) {
			t.Errorf("%s: queued a model-backed pass with no work in it", name)
		}
	}
}

// A body this consumer cannot read answers no rather than guessing, and answers
// it without an error — a group that wedged on one malformed payload would stop
// reading every later one.
func TestAnUnreadablePayloadQueuesNothingAndWedgesNothing(t *testing.T) {
	g := quietTrigger()
	ctx := context.Background()

	for _, eventType := range []string{"activity.captured", "activity.updated"} {
		env := events.Envelope{
			Type:       eventType,
			EventID:    ids.NewV7(),
			OccurredAt: time.Now().UTC(),
			Entity:     events.EntityRef{Type: "activity", ID: ids.NewV7()},
			Payload:    json.RawMessage(`{"kind":`),
		}
		if g.queues(ctx, env) {
			t.Errorf("%s: an unreadable payload queued a pass", eventType)
		}
		if err := g.HandleEvent(ctx, env); err != nil {
			t.Errorf("%s: HandleEvent returned %v, want a silent skip", eventType, err)
		}
	}
}

// A stale event is replayed backlog — a new consumer group starts at stream
// position 0 — and the reconciler owns everything that old. The nil pool is the
// assertion: reaching the enqueue would panic rather than fail.
func TestAStaleEventQueuesNothing(t *testing.T) {
	g := quietTrigger()
	ctx := context.Background()

	for name, env := range map[string]events.Envelope{
		"stale mail":      capturedMail(t, "email"),
		"a stale contact": enrichEvent(t, "person.created", "person", noPayload),
		"a stale opening": audienceChange(t, crmcontracts.Workspace),
	} {
		env.OccurredAt = time.Now().UTC().Add(-2 * captureEnrichFreshWindow)
		if err := g.HandleEvent(ctx, env); err != nil {
			t.Errorf("%s: HandleEvent returned %v, want a silent skip", name, err)
		}
	}
}
