// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A task Margince wrote is named in the reader's language on every screen that
// shows it, as History names it.

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

const germanNewLeadTask = "Beim neuen Lead nachfassen"

func TestATaskMarginceWroteIsNamedInTheReadersLanguageEverywhere(t *testing.T) {
	t.Parallel()
	e := Setup(t)
	company := e.SeedCompany(t, "Initech", nil)
	deal := e.SeedOpenDealOn(t, "Initech rollout", company)
	profile := "https://www.linkedin.com/in/rena-quill"
	lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{FullName: new("Rena Quill"), LinkedInURL: &profile, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	subject := systemtext.Literal("automation.new_lead")
	overdue := roomFixedNow.AddDate(0, 0, -3)
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &subject, Source: "manual", DueAt: &overdue,
		SystemText: &systemtext.Message{Subject: systemtext.Sentence("automation.new_lead")},
		Links: []activities.ActivityLinkInput{
			{EntityType: "lead", EntityID: ids.UUID(lead.Id)},
			{EntityType: "company", EntityID: company},
			{EntityType: "deal", EntityID: deal},
		},
	}); err != nil {
		t.Fatal(err)
	}
	german := reading(e.Admin(), t, "de")
	named := func(where string, got *string) {
		t.Helper()
		if got == nil || *got != germanNewLeadTask {
			t.Errorf("%s names the task %v, want %q", where, got, germanNewLeadTask)
		}
	}

	got, err := e.Contacts.GetLead(german, leadIDOf(ids.UUID(lead.Id)), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	named("the lead's Next card", got.NextTaskSubject)
	found, err := e.Contacts.FindLeadByLinkedInURL(german, profile)
	if err != nil || found == nil {
		t.Fatalf("lookup by profile: %v %v", found, err)
	}
	named("the profile lookup", found.NextTaskSubject)
	for _, sort := range []*string{nil, new("full_name")} {
		rows, _, err := e.Contacts.ListLeads(german, contacts.ListLeadsInput{Sort: sort})
		if err != nil {
			t.Fatal(err)
		}
		listed := false
		for _, row := range rows {
			if row.Id == lead.Id {
				listed = true
				named("the lead list", row.NextTaskSubject)
			}
		}
		if !listed {
			t.Errorf("the lead list sorted by %v does not hold the lead", sort)
		}
	}

	page, err := companySurfaceService(e).Assemble(german, companyIDOf(company))
	if err != nil {
		t.Fatal(err)
	}
	if page.NextSteps == nil || len(page.NextSteps.Data) != 1 {
		t.Fatalf("the company's open tasks are %+v, want the one task", page.NextSteps)
	}
	named("the company's open tasks", &page.NextSteps.Data[0].Subject)
	if page.Deals == nil || len(page.Deals.Data) != 1 || page.Deals.Data[0].Attention == nil ||
		page.Deals.Data[0].Attention.Kind != crmcontracts.Company360WorkAttentionKindWorkAttentionOverdueTask {
		t.Fatalf("the deal row carries no overdue task: %+v", page.Deals)
	}
	named("the deal's overdue-task card", &page.Deals.Data[0].Attention.Title)

	hits, err := search.NewStore(e.DB()).Search(german, search.Input{Query: "new lead", Types: []string{"activity"}, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits.Hits) != 1 {
		t.Fatalf("search found %+v, want the one task", hits.Hits)
	}
	named("search", &hits.Hits[0].Title)
}
