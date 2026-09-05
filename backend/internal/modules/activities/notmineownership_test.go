// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Which firings reach the re-arm at all.
//
// The reconcile itself is a two-transaction claim and lives in the integration
// suite; what a unit test can hold is the filter in front of it, and that filter
// is the one place this handler can fail SILENTLY — an owner change it declines
// leaves a message hidden from the person who now owns it, with nothing on any
// screen to say why.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

func TestTheHandOffTriggerMatchesWhateverItCarries(t *testing.T) {
	t.Parallel()

	// deal.owner_changed IS the hand-off. Its payload names the owners rather
	// than a changed-field set, and reading it for one would decline every
	// firing of the only event that exists to announce this.
	handler := notMineRearm{trigger: handOffTriggers[0]}
	matched, err := handler.Match(context.Background(), workflow.Event{
		Payload: json.RawMessage(`{"to_owner_id":"01930000-0000-7000-8000-0000000000a1"}`),
	})
	if err != nil {
		t.Fatalf("matching the hand-off: %v", err)
	}
	if !matched {
		t.Error("the owner-changed trigger declined its own event")
	}
}

func TestAnUpdateMatchesOnlyWhenItNamesTheOwner(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name    string
		payload string
		want    bool
	}{
		{
			// The flat shape a patch produces.
			name:    "a patch that moved the owner",
			payload: `{"changed_fields":{"owner_id":"01930000-0000-7000-8000-0000000000a1"}}`,
			want:    true,
		}, {
			// The nested shape lead routing writes. Both are documented on the
			// envelope, and a handler that read one would work for hand-offs a
			// human made and not for the ones the router made.
			name:    "routing that chose an owner",
			payload: `{"changed_fields":{"delta":{"owner_id":"01930000-0000-7000-8000-0000000000a1"}}}`,
			want:    true,
		}, {
			// The case this filter exists for: a person's row is written by
			// every enrichment pass, and running a delete over that contact's
			// whole timeline behind each one is work nobody asked for.
			name:    "a patch that moved something else",
			payload: `{"changed_fields":{"title":"Head of Ops"}}`,
			want:    false,
		}, {
			name:    "routing that changed something else",
			payload: `{"changed_fields":{"delta":{"score":42}}}`,
			want:    false,
		}, {
			// Fails toward doing the work. Running the reconcile for nothing is
			// one statement that deletes no rows; skipping it is a message that
			// stays hidden from its new owner with nothing to say why.
			name:    "a payload this build cannot read",
			payload: `{"changed_fields":"the whole record"}`,
			want:    true,
		}, {
			name:    "an envelope carrying no changed fields at all",
			payload: `{}`,
			want:    true,
		}, {
			name:    "no payload",
			payload: ``,
			want:    true,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			handler := notMineRearm{trigger: "person.updated"}
			matched, err := handler.Match(context.Background(),
				workflow.Event{Payload: json.RawMessage(c.payload)})
			if err != nil {
				t.Fatalf("matching: %v", err)
			}
			if matched != c.want {
				t.Errorf("matched = %v, want %v", matched, c.want)
			}
		})
	}
}

// One handler per record a message can be filed under, each with a name of its
// own — the engine keys a run claim on the handler name, so two sharing one
// would have the first firing suppress the second.
func TestEveryHandOffTriggerGetsItsOwnHandler(t *testing.T) {
	t.Parallel()

	handlers := NotMineRearmWorkflows(nil)
	if len(handlers) != len(handOffTriggers) {
		t.Fatalf("registered %d handlers for %d triggers", len(handlers), len(handOffTriggers))
	}
	names, triggers := map[string]bool{}, map[string]bool{}
	for _, handler := range handlers {
		spec := handler.Spec()
		if names[spec.Name] {
			t.Errorf("two handlers are called %q", spec.Name)
		}
		names[spec.Name] = true
		triggers[spec.Trigger.EventType] = true
	}
	for _, trigger := range handOffTriggers {
		if !triggers[trigger] {
			t.Errorf("no handler watches %q, so a hand-off announced there re-arms nothing", trigger)
		}
	}
}
