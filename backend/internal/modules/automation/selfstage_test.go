// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

func TestStageNotificationNeedsAChangeTheOwnerDidNotMake(t *testing.T) {
	owner := ids.NewV7()
	fields, err := json.Marshal(dealOwnerFields{OwnerID: &owner, Name: "Renewal"})
	if err != nil {
		t.Fatal(err)
	}
	handler := stageChangeNotify{ex: Executors{Provider: &fakeReadProvider{record: datasource.Record{Fields: fields}}}}
	for _, tc := range []struct {
		name  string
		actor events.Actor
		skip  bool
	}{
		{"owner", events.Actor{Type: "human", ID: "human:" + owner.String()}, true},
		{"bare owner", events.Actor{Type: "human", ID: owner.String()}, true},
		{"uppercase UUID", events.Actor{Type: "human", ID: "human:" + strings.ToUpper(owner.String())}, true},
		{"colleague", events.Actor{Type: "human", ID: "human:" + ids.NewV7().String()}, false},
		{"agent with owner authority", events.Actor{Type: "agent", ID: "agent:planner", OnBehalfOf: &owner}, false},
		{"system with matching id", events.Actor{Type: "system", ID: "human:" + owner.String()}, false},
		{"unknown actor", events.Actor{}, false},
		{"invalid human id", events.Actor{Type: "human", ID: "unknown"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			effect, err := handler.Plan(context.Background(), workflow.Event{ID: ids.NewV7(), Actor: tc.actor})
			var declined declinedFiring
			if tc.skip {
				if !errors.As(err, &declined) || len(effect.Actions) != 0 {
					t.Fatalf("self-made move must decline without delivery: %+v, %v", effect, err)
				}
			} else if err != nil || len(effect.Actions) != 1 {
				t.Fatalf("a move not known to be self-made must notify: %+v, %v", effect, err)
			}
		})
	}
}
