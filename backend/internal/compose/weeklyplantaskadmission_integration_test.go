// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestWeeklyPlanCapacityRequiresCurrentAutomaticResponsibility(t *testing.T) {
	for _, stale := range []bool{false, true} {
		t.Run(map[bool]string{false: "wrong recipient", true: "old source"}[stale], func(t *testing.T) {
			e := integration.Setup(t)
			week := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
			sourceAt := week.AddDate(0, 0, -1)
			recipient := e.Rep2
			if stale {
				sourceAt = week.AddDate(-2, 0, 0)
				recipient = e.Rep1
			}
			source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
				Kind: "email", Subject: new("Please send the plan"), Body: new("Please send the plan."), Direction: new("inbound"), Source: "manual", OccurredAt: &sourceAt,
				EmailFrom: "buyer@customer.test", EmailTo: []string{seatAddress(t, e, recipient)},
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := e.Activities.SetOwedVerdict(e.Admin(), ids.UUID(source.Id), activities.OwedVerdictAsksUs, "fixture", sourceAt); err != nil {
				t.Fatal(err)
			}
			due := week.AddDate(0, 0, 2)
			task, _, err := e.Activities.LogActivity(principal.SystemActing(e.Admin(), "system:email_request"), activities.LogActivityInput{
				Kind: "task", Subject: new("Send the plan"), Source: "manual", AssigneeID: new(ids.From[ids.UserKind](e.Rep1)), DueAt: &due,
				SourceSystem: new(activities.EmailRequestTaskSource), SourceID: new(source.Id.String()), SourceActivityID: new(ids.UUID(source.Id)),
			})
			if err != nil {
				t.Fatal(err)
			}
			check := func(want int) {
				t.Helper()
				got, err := (weeklyPlanCapacity{pool: e.Pool}).ForWeek(e.Admin(), e.Rep1, week)
				if err != nil {
					t.Fatal(err)
				}
				if got.Tasks != want {
					t.Fatalf("planned tasks=%d, want %d", got.Tasks, want)
				}
			}
			check(0)
			if _, err := e.Activities.UpdateActivity(e.Admin(), ids.From[ids.ActivityKind](ids.UUID(task.Id)), activities.UpdateActivityInput{Subject: new("I will send the plan")}); err != nil {
				t.Fatal(err)
			}
			check(1)
		})
	}
}
