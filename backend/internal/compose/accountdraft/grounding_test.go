// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package accountdraft

import (
	"slices"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// An account draft names every record its input was folded from — the
// company, the recipient, the deal, the project, the open task it leads with
// and each exchange — so a kept draft can re-prove each one.
func TestTheGroundingNamesEveryRecordTheInputHolds(t *testing.T) {
	company, contact, deal, project, task := ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7()
	view := viewWithActivities(t, activityWith(t, "Scope", "Can you send the scope?"))
	view.Company.Id = openapi_types.UUID(company)
	view.Contacts = &struct {
		Data []crmcontracts.Company360Contact `json:"data"`
		Page crmcontracts.PageInfo            `json:"page"`
	}{Data: []crmcontracts.Company360Contact{{ContactId: openapi_types.UUID(contact), FullName: "Priya Raman"}}}
	view.Deals = &crmcontracts.Company360Deals{Data: []crmcontracts.Company360Deal{{DealId: openapi_types.UUID(deal), Name: "Rollout"}}}
	view.Projects = &[]crmcontracts.Company360Project{{ProjectId: openapi_types.UUID(project), Name: "ERP"}}
	view.NextSteps = &struct {
		Data []crmcontracts.Company360NextStep `json:"data"`
		Page crmcontracts.PageInfo             `json:"page"`
	}{Data: []crmcontracts.Company360NextStep{{ActivityId: openapi_types.UUID(task), Subject: "Send the scope"}}}
	scoped := ids.From[ids.ProjectKind](project)

	in, err := FromView(view, Request{ContactID: contact.String(), DealID: deal.String(), ProjectID: &scoped})
	if err != nil {
		t.Fatalf("fold: %v", err)
	}
	got, err := in.Grounding()
	if err != nil {
		t.Fatalf("grounding: %v", err)
	}

	want := []datasource.EntityRef{
		{Type: datasource.EntityCompany, ID: company},
		{Type: datasource.EntityContact, ID: contact},
		{Type: datasource.EntityDeal, ID: deal},
		{Type: datasource.EntityProject, ID: project},
		{Type: datasource.EntityActivity, ID: task},
		{Type: datasource.EntityActivity, ID: ids.UUID(view.Activities.Data[0].Id)},
	}
	if !slices.Equal(got, want) {
		t.Errorf("grounding = %v\nwant       %v", got, want)
	}
}
