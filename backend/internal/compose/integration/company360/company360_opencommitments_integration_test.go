// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package company360

// The open_commitments section and the moment card read the same claim rows.
// A reader without the grant is told the section is withheld rather than
// shown an empty list.

import (
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func seedAccountClaim(t *testing.T, e *integration.Env, contact, company ids.UUID, kind, body string, due *time.Time) crmcontracts.ConversationClaim {
	t.Helper()
	ctx := e.Admin()
	subject, direction, at := "Re: "+body, "outbound", company360Clock.Add(-72*time.Hour)
	if kind == "commitment_theirs" {
		direction = "inbound"
	}
	activity, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: &direction, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return e.RecordClaim(ctx, t, contacts.ClaimInput{
		ContactID: ids.From[ids.ContactKind](contact), Kind: kind, Body: body, Quote: body,
		ActivityID: ids.UUID(activity.Id), DueAt: due, Source: "manual",
	})
}

func TestTheCompanyPageListsBothSidesOfTheOpenCommitmentsAndTheCardReadsTheSameRow(t *testing.T) {
	e := integration.Setup(t)
	svc := company360Service(e)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	contact := e.SeedContact(t, "Carol", &e.Rep1)
	e.WsExec(t, `INSERT INTO relationship(kind,contact_id,company_id,source,captured_by) VALUES('employment',$1,$2,'manual','human:test')`, contact, company)
	soon, late := company360Clock.Add(48*time.Hour), company360Clock.Add(-24*time.Hour)
	ours := seedAccountClaim(t, e, contact, company, "commitment_ours", "Send the signed order", &soon)
	theirs := seedAccountClaim(t, e, contact, company, "commitment_theirs", "Pay the deposit", &late)

	page, err := svc.Assemble(e.Admin(), ids.From[ids.CompanyKind](company))
	if err != nil {
		t.Fatal(err)
	}
	if page.OpenCommitments == nil || len(page.OpenCommitments.Data) != 2 {
		t.Fatalf("open_commitments = %+v, want both sides", page.OpenCommitments)
	}
	first, second := page.OpenCommitments.Data[0], page.OpenCommitments.Data[1]
	if first.Body != "Pay the deposit" || first.Kind != crmcontracts.Company360CommitmentKindTheirs || !first.Overdue || first.ContactName != "Carol" {
		t.Errorf("first = %+v, want their overdue promise leading", first)
	}
	if second.Body != "Send the signed order" || second.Kind != crmcontracts.Company360CommitmentKindOurs ||
		second.ClaimId.String() != ours.Id.String() {
		t.Errorf("second = %+v, want our promise", second)
	}
	if first.ClaimId.String() != theirs.Id.String() {
		t.Errorf("claim id = %s, want %s", first.ClaimId, theirs.Id)
	}
	// The card names the same claim the section lists for our side.
	if page.Moment == nil || len(page.Moment.Evidence) == 0 ||
		page.Moment.Evidence[0].Id.String() != second.ActivityId.String() {
		t.Errorf("moment = %+v, want evidence on the message of the section's own row", page.Moment)
	}
}

func TestACallerWithoutTheActivityGrantIsToldCommitmentsAreWithheld(t *testing.T) {
	e := integration.Setup(t)
	svc := company360Service(e)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	perms := integration.AccountRepPerms
	perms.Objects = maps.Clone(perms.Objects)
	delete(perms.Objects, "activity")
	page, err := svc.Assemble(e.As(e.Rep1, []ids.UUID{e.Team1}, perms), ids.From[ids.CompanyKind](company))
	if err != nil {
		t.Fatal(err)
	}
	if page.OpenCommitments != nil {
		t.Errorf("open_commitments = %+v, want the section absent", page.OpenCommitments)
	}
	if !slices.Contains(page.SectionsOmitted, crmcontracts.Company360SectionsOmitted("commitments")) {
		t.Errorf("sections_omitted = %v, want commitments named", page.SectionsOmitted)
	}
}

func TestATaskNamesItsLiveAssigneeInTheNextSteps(t *testing.T) {
	e := integration.Setup(t)
	svc := company360Service(e)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	subject, assignee := "Call the CFO", ids.From[ids.UserKind](e.Rep1)
	task, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &subject, Source: "manual", AssigneeID: &assignee,
		Links: []activities.ActivityLinkInput{{EntityType: "company", EntityID: company}},
	})
	if err != nil {
		t.Fatal(err)
	}
	page, err := svc.Assemble(e.Admin(), ids.From[ids.CompanyKind](company))
	if err != nil {
		t.Fatal(err)
	}
	if page.NextSteps == nil || len(page.NextSteps.Data) != 1 || page.NextSteps.Data[0].ActivityId.String() != task.Id.String() {
		t.Fatalf("next_steps = %+v", page.NextSteps)
	}
	if got := page.NextSteps.Data[0].AssigneeName; got == nil || *got == "" {
		t.Errorf("assignee_name = %v, want the assignee's name", got)
	}
}
