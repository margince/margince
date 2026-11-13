// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

func TestTaskTemplatesUseTheExplicitAuthorizerRecipient(t *testing.T) {
	now := time.Date(2026, 12, 20, 10, 0, 0, 0, time.UTC)
	for _, kind := range []string{renewalReminderName, listTaskName} {
		t.Run(kind, func(t *testing.T) {
			event := renewalEvent(t, now, now.AddDate(0, 0, 10), datasource.EntityRef{Type: "contact", ID: ids.NewV7()})
			event.OwnerID = ids.NewV7()
			event.Params = json.RawMessage(`{"recipient":"authorizer","task_title":"Call about renewal"}`)
			var handler workflow.Handler = renewalReminder{}
			if kind == listTaskName {
				var err error
				event.Payload, err = json.Marshal(listFiring{ListID: ids.NewV7(), Action: directionEntered, MemberEventID: ids.NewV7()})
				if err != nil {
					t.Fatal(err)
				}
				handler = listRule{name: listTaskName, action: ActionTypeCreateTask}
			}
			effect, err := handler.Plan(context.Background(), event)
			if err != nil || len(effect.Actions) != 1 {
				t.Fatalf("task plan = %+v, %v", effect, err)
			}
			var task struct {
				Assignee ids.UUID `json:"assignee_id"`
				Subject  string   `json:"subject"`
			}
			if err := json.Unmarshal(effect.Actions[0].Args, &task); err != nil {
				t.Fatal(err)
			}
			if task.Assignee != event.OwnerID || kind == listTaskName && task.Subject != "Call about renewal" {
				t.Fatalf("recipient or task title changed: %+v", task)
			}
		})
	}
}
