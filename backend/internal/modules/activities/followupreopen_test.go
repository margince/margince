// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Which activity updates ask the reopen question.

import (
	"context"
	"encoding/json"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// Every activity update fires this trigger, so Match is what keeps a note's
// edit from asking whether a reminder should reopen.
func TestOnlyACalledOffMeetingAsksTheReopenQuestion(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name   string
		status *crmcontracts.PublicEventActivityChangedFieldsMeetingStatus
		admits bool
	}{
		{"canceled", new(crmcontracts.MeetingWasCanceled), true},
		{"no_show", new(crmcontracts.MeetingWasNoShow), true},
		{"held", new(crmcontracts.MeetingWasHeld), false},
		{"booked", new(crmcontracts.MeetingWasBooked), false},
		{"no status at all", nil, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			payload, err := json.Marshal(crmcontracts.PublicEventActivityUpdated{
				ChangedFields: crmcontracts.PublicEventActivityChangedFields{MeetingStatus: c.status},
			})
			if err != nil {
				t.Fatalf("encoding the event: %v", err)
			}
			got, err := quietReminderReopen{}.Match(
				context.Background(), workflow.Event{Payload: payload},
			)
			if err != nil {
				t.Fatalf("Match: %v", err)
			}
			if got != c.admits {
				t.Errorf("a %s meeting is admitted %v, want %v", c.name, got, c.admits)
			}
		})
	}
}

// An update carrying no payload is not a cancellation. The envelope says which
// change happened.
//
// A handler admitting an empty one would run the reopen query on every
// activity write there is.
func TestAnEmptyPayloadAsksNothing(t *testing.T) {
	t.Parallel()
	admitted, err := quietReminderReopen{}.Match(context.Background(), workflow.Event{})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if admitted {
		t.Error("an update with no payload was read as a cancellation")
	}
}

// A payload that is not an activity update answers an error rather than a
// silent false.
//
// A trigger wired to the wrong event is a fault to see, not a handler that
// never runs and says nothing.
func TestAnUndecodablePayloadIsAnError(t *testing.T) {
	t.Parallel()
	_, err := quietReminderReopen{}.Match(
		context.Background(), workflow.Event{Payload: []byte("not json")},
	)
	if err == nil {
		t.Error("an undecodable payload was read as a decision")
	}
}

// Plan states the effect the reopen applies, and the key is per event.
func TestThePlanReopensAndTheKeyIsPerEvent(t *testing.T) {
	t.Parallel()
	handler := quietReminderReopen{}
	event := workflow.Event{ID: ids.NewV7()}

	effect, err := handler.Plan(context.Background(), event)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(effect.Actions) != 1 {
		t.Fatalf("Plan states %d actions, want 1", len(effect.Actions))
	}
	var args map[string]any
	if err := json.Unmarshal(effect.Actions[0].Args, &args); err != nil {
		t.Fatalf("decoding the planned args: %v", err)
	}
	if done, ok := args[auditFieldIsDone].(bool); !ok || done {
		t.Errorf("the plan sets %s to %v, want false", auditFieldIsDone, args[auditFieldIsDone])
	}
	if key := handler.IdempotencyKey(event); key == "" || key == handler.IdempotencyKey(workflow.Event{}) {
		t.Errorf("the key %q does not name this event", key)
	}
}

// The trigger and the redrive posture, which nothing else states.
//
// A handler registered on the wrong event never runs and reports nothing, so
// the one string that decides it is worth an assertion.
func TestTheReopenWatchesActivityUpdates(t *testing.T) {
	t.Parallel()
	handlers := QuietReminderReopenWorkflows(nil)
	if len(handlers) != 1 {
		t.Fatalf("the set holds %d handlers, want 1", len(handlers))
	}
	spec := handlers[0].Spec()
	if spec.Trigger.EventType != "activity.updated" {
		t.Errorf("the reopen watches %q, so a cancellation never reaches it", spec.Trigger.EventType)
	}
	// Redrivable because the query sees only reminders that are done and whose
	// record is quiet: a second pass over one cancellation finds nothing.
	if !spec.RedrivableWithoutDuplicating {
		t.Error("the reopen is not redrivable, so a redelivered cancellation is a failed run")
	}
	if spec.Name == "" {
		t.Error("the handler has no name, which is what a run is reported under")
	}
}
