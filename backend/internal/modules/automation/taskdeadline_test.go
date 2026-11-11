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

func TestTaskDeadlinesFollowTheSavedCalendarDay(t *testing.T) {
	cases := []struct{ name, received, params, want string }{
		{"legacy default", "2026-03-28T10:00:00Z", `{}`, "2026-03-29T10:00:00Z"},
		{"legacy offset", "2026-03-28T10:00:00Z", `{"due_in_days":2}`, "2026-03-30T10:00:00Z"},
		{"spring forward", "2026-03-28T10:00:00Z", `{"timezone":"Europe/Berlin"}`, "2026-03-29T21:59:59.999999Z"},
		{"fall back", "2026-10-24T10:00:00Z", `{"timezone":"Europe/Berlin"}`, "2026-10-25T22:59:59.999999Z"},
		{"local receipt date", "2026-07-01T20:30:00Z", `{"timezone":"Asia/Ho_Chi_Minh"}`, "2026-07-03T16:59:59.999999Z"},
		{"two calendar days", "2026-03-27T10:00:00Z", `{"timezone":"Europe/Berlin","due_in_days":2}`, "2026-03-29T21:59:59.999999Z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			received, err := time.Parse(time.RFC3339, tc.received)
			if err != nil {
				t.Fatal(err)
			}
			got, err := taskDueAt(json.RawMessage(tc.params), received, 1)
			if err != nil {
				t.Fatal(err)
			}
			if got.Format(time.RFC3339Nano) != tc.want {
				t.Fatalf("deadline = %s, want %s", got.Format(time.RFC3339Nano), tc.want)
			}
		})
	}
}

func TestTaskDeadlineConfigurationRefusesHostDependentZones(t *testing.T) {
	for _, name := range []string{"", "Local", "Europe/Nowhere", " Europe/Berlin ", "+02:00"} {
		t.Run(name, func(t *testing.T) {
			if err := validateDueInDays(map[string]any{paramTaskTimezone: name}); err == nil {
				t.Fatal("invalid timezone accepted")
			}
		})
	}
	for _, key := range []string{routeLeadName, stageChangeCreateTaskName, listTaskName} {
		t.Run(key, func(t *testing.T) {
			entry, ok := CatalogEntryByKey(key)
			if !ok {
				t.Fatal("task template missing")
			}
			params := map[string]any{paramTaskTimezone: "Europe/Berlin"}
			if key == stageChangeCreateTaskName {
				params["pipeline_id"] = ids.NewV7().String()
				params["stage_ids"] = []string{ids.NewV7().String()}
				params["task_title"] = "Follow up"
				params[paramDueInDays] = float64(2)
			}
			if key == listTaskName {
				params[paramListID] = ids.NewV7().String()
			}
			if err := entry.Validate(params); err != nil {
				t.Fatal(err)
			}
			params[paramTaskTimezone] = "Local"
			if err := entry.Validate(params); err == nil {
				t.Fatal("task template accepted host timezone")
			}
		})
	}
}

func TestTheLeadTaskPlannerUsesTheSavedDeadline(t *testing.T) {
	leadID := ids.NewV7()
	ev := workflow.Event{
		Entity:     datasource.EntityRef{Type: datasource.EntityLead, ID: leadID},
		OccurredAt: time.Date(2026, time.March, 28, 10, 0, 0, 0, time.UTC),
		Params:     json.RawMessage(`{"timezone":"Europe/Berlin"}`),
	}
	effect, err := routeLeadPlanner(t, leadID, nil).Plan(context.Background(), ev)
	if err != nil {
		t.Fatal(err)
	}
	if len(effect.Actions) != 1 {
		t.Fatalf("actions = %d, want one", len(effect.Actions))
	}
	var task struct {
		DueAt time.Time `json:"due_at"`
	}
	if err := json.Unmarshal(effect.Actions[0].Args, &task); err != nil {
		t.Fatal(err)
	}
	if got := task.DueAt.Format(time.RFC3339Nano); got != "2026-03-29T21:59:59.999999Z" {
		t.Fatalf("planned deadline = %s", got)
	}
}
