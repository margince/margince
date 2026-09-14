// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func TestLiveAndRedrivenStageNoticesPreserveOriginalAuthority(t *testing.T) {
	owner, authority, workspace := ids.NewV7(), ids.NewV7(), ids.NewV7()
	env := kevents.Envelope{EventID: ids.NewV7(), Type: "deal.stage_changed", Actor: kevents.Actor{Type: "agent", ID: "agent:import", OnBehalfOf: &authority}, OccurredAt: time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC)}
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := eventFromEnvelope(raw, workspace)
	if err != nil {
		t.Fatal(err)
	}
	live := workflowEvent(env, workspace)
	if !reflect.DeepEqual(live, retry) {
		t.Fatalf("retry lost original event: %+v != %+v", retry, live)
	}
	fields, err := json.Marshal(dealOwnerFields{OwnerID: &owner})
	if err != nil {
		t.Fatal(err)
	}
	notifier := &fakeNotifier{}
	w := stageChangeNotify{ex: Executors{Provider: &fakeReadProvider{record: datasource.Record{Fields: fields}}, Notifier: notifier}}
	effect, err := w.Plan(context.Background(), retry)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Apply(context.Background(), retry, effect, nil); err != nil {
		t.Fatal(err)
	}
	if len(notifier.calls) != 1 {
		t.Fatalf("notifications: %d", len(notifier.calls))
	}
	call := notifier.calls[0]
	if call.origin == nil || call.origin.ActorId != env.Actor.ID || call.origin.ActorType != "agent" || call.origin.OnBehalfOf == nil || ids.UUID(*call.origin.OnBehalfOf) != authority || !call.origin.OccurredAt.Equal(env.OccurredAt) || ids.UUID(call.origin.EventId) != env.EventID {
		t.Fatalf("notification lost authority: %+v", call.origin)
	}
	if call.dedupe != w.IdempotencyKey(live) || call.recipient != owner {
		t.Fatalf("wrong recipient or replay key: %+v", call)
	}
}
