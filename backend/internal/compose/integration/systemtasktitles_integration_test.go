// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A task Margince wrote is named in the reader's language on every screen that
// shows it, as History names it.

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

const germanNewLeadTask = "Beim neuen Lead nachfassen"

func TestATaskMarginceWroteIsNamedInTheReadersLanguageEverywhere(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Initech", nil)
	lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{FullName: new("Rena Quill"), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	subject := systemtext.Literal("automation.new_lead")
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &subject, Source: "manual",
		SystemText: &systemtext.Message{Subject: systemtext.Sentence("automation.new_lead")},
		Links: []activities.ActivityLinkInput{
			{EntityType: "lead", EntityID: ids.UUID(lead.Id)},
			{EntityType: "company", EntityID: company},
		},
	}); err != nil {
		t.Fatal(err)
	}
	german := reading(e.Admin(), t, "de")

	got, err := e.Contacts.GetLead(german, leadIDOf(ids.UUID(lead.Id)), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if got.NextTaskSubject == nil || *got.NextTaskSubject != germanNewLeadTask {
		t.Errorf("the lead's Next card names the task %v, want %q", got.NextTaskSubject, germanNewLeadTask)
	}
	rows, _, err := e.Contacts.ListLeads(german, contacts.ListLeadsInput{})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Id == lead.Id && (row.NextTaskSubject == nil || *row.NextTaskSubject != germanNewLeadTask) {
			t.Errorf("the lead list names the task %v, want %q", row.NextTaskSubject, germanNewLeadTask)
		}
	}

	page, err := companySurfaceService(e).Assemble(german, companyIDOf(company))
	if err != nil {
		t.Fatal(err)
	}
	if page.NextSteps == nil || len(page.NextSteps.Data) != 1 || page.NextSteps.Data[0].Subject != germanNewLeadTask {
		t.Errorf("the company's open tasks are %+v, want the one task named %q", page.NextSteps, germanNewLeadTask)
	}

	hits, err := search.NewStore(e.DB()).Search(german, search.Input{Query: "new lead", Types: []string{"activity"}, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits.Hits) != 1 || hits.Hits[0].Title != germanNewLeadTask {
		t.Errorf("search found %+v, want the one task titled %q", hits.Hits, germanNewLeadTask)
	}
}
