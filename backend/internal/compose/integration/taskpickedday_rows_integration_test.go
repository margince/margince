// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration_test

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A lead's next task says whether its due is a date with no time, and the date
// and zone it was chosen in. A reader in any zone then prints the picked day.
func TestALeadsNextTaskCarriesItsPickedDate(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	assignee := ids.From[ids.UserKind](e.AdminUser)
	lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{
		FullName: new("Picked day prospect"), LinkedInURL: new("https://www.linkedin.com/in/picked-day-prospect"), Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	intent := &crmcontracts.IntentTime{WallClock: "2026-11-16T23:59:59", Timezone: "America/Los_Angeles"}
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: new("Call back"), DueIntent: intent, AssigneeID: &assignee, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "lead", EntityID: ids.UUID(lead.Id)}},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := e.Contacts.GetLead(e.Admin(), ids.From[ids.LeadKind](ids.UUID(lead.Id)), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if got.NextTaskDueHasTime == nil || *got.NextTaskDueHasTime {
		t.Errorf("next_task_due_has_time = %v, want false for a date with no time", got.NextTaskDueHasTime)
	}
	if got.NextTaskDueIntent == nil || got.NextTaskDueIntent.Timezone != intent.Timezone ||
		!strings.HasPrefix(got.NextTaskDueIntent.WallClock, intent.WallClock) {
		t.Errorf("next_task_due_intent = %+v, want %+v", got.NextTaskDueIntent, intent)
	}
}
