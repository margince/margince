// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestExistingRequestTaskRequiresItsAssigneeToBeAddressed(t *testing.T) {
	for _, scenario := range []string{"copied importer", "addressed assignee", "old addressed assignee", "withdrawn request"} {
		t.Run(scenario, func(t *testing.T) {
			e := integration.Setup(t)
			at := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
			sourceAt := at
			if scenario == "old addressed assignee" {
				sourceAt = at.AddDate(-2, 0, 0)
			}
			addressed := e.Rep1
			if scenario != "copied importer" {
				addressed = e.AdminUser
			}
			var recipient string
			if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
				return tx.QueryRow(context.Background(), "SELECT email FROM app_user WHERE id=$1", addressed).Scan(&recipient)
			}); err != nil {
				t.Fatal(err)
			}
			contact := e.SeedContact(t, "Request buyer", &e.AdminUser)
			e.Contacts.WithClock(func() time.Time { return at.Add(time.Hour) })
			lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{FullName: new("Request prospect"), Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
				Kind: "email", Subject: new("Please send the specification"), Body: new("Please send the specification for review."), Direction: new("inbound"), OccurredAt: &sourceAt, Source: "manual", EmailFrom: "buyer@customer.test", EmailTo: []string{recipient}, Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
			})
			if err != nil {
				t.Fatal(err)
			}
			verdict := activities.OwedVerdictAsksUs
			if scenario == "withdrawn request" {
				verdict = activities.OwedVerdictInformsUs
			}
			classifier := principal.WithActor(e.Admin(), principal.Principal{Type: principal.PrincipalSystem, ID: "system:owed_verdict", Permissions: principal.Permissions{RowScope: principal.RowScopeAll}})
			if _, err := e.Activities.SetOwedVerdict(classifier, ids.UUID(source.Id), verdict, "fixture", at); err != nil {
				t.Fatal(err)
			}
			assignee := ids.From[ids.UserKind](e.AdminUser)
			task, _, err := e.Activities.LogActivity(principal.SystemActing(e.Admin(), "system:email_request"), activities.LogActivityInput{
				Kind: "task", Subject: new("Reply to the specification request"), Source: "manual", AssigneeID: &assignee,
				SourceSystem: new(activities.EmailRequestTaskSource), SourceID: new(source.Id.String()), SourceActivityID: new(ids.UUID(source.Id)), Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}, {EntityType: "lead", EntityID: ids.UUID(lead.Id)}},
			})
			if err != nil {
				t.Fatal(err)
			}
			taskID := ids.From[ids.ActivityKind](ids.UUID(task.Id))
			feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return at.Add(time.Hour) })
			company := ids.Nil
			assertTask := func(want bool) {
				t.Helper()
				for _, scope := range []string{"mine", "all"} {
					owner := ids.Nil
					if scope == "all" {
						owner = e.AdminUser
					}
					page, err := feed.Worklist(e.Admin(), scope, "all", owner, 100, "")
					if err != nil {
						t.Fatal(err)
					}
					found := false
					for _, row := range page.Queue {
						if row.Source == "task" && row.Id == taskID.String() {
							found = true
						}
					}
					if found != want {
						t.Errorf("%s automatic task visible=%v, want %v", scope, found, want)
					}
					expected := 0
					if want {
						expected = 1
					}
					if page.Summary.Total != expected {
						t.Errorf("unowned task counted: %+v", page.Summary)
					}
				}
				contactPage, err := pageAsOf(e, at.Add(time.Hour)).Assemble(e.Admin(), ids.From[ids.ContactKind](contact))
				if err != nil {
					t.Fatal(err)
				}
				expected := 0
				if want {
					expected = 1
				}
				assertLeadRequestWork(t, e, ids.From[ids.LeadKind](ids.UUID(lead.Id)), expected)
				if contactPage.NextSteps == nil || len(contactPage.NextSteps.Data) != expected {
					t.Errorf("contact current tasks=%+v want %d", contactPage.NextSteps, expected)
				}
				if company != ids.Nil {
					companyPage, err := company360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects, approvals.NewService(e.DB()), func() time.Time { return at.Add(time.Hour) }).Assemble(e.Admin(), ids.From[ids.CompanyKind](company))
					if err != nil {
						t.Fatal(err)
					}
					if companyPage.NextSteps == nil || len(companyPage.NextSteps.Data) != expected {
						t.Errorf("company current tasks=%+v want %d", companyPage.NextSteps, expected)
					}
				}
			}
			assertTask(scenario == "addressed assignee")
			company = e.SeedCompany(t, "Filed account", &e.AdminUser)
			if _, err := e.Activities.RelinkActivity(e.Admin(), taskID, activities.RelinkActivityInput{EntityType: "company", EntityID: company}); err != nil {
				t.Fatal(err)
			}
			assertTask(scenario == "addressed assignee")
			stored, err := e.Activities.GetActivity(e.Admin(), taskID, storekit.LiveOnly)
			if err != nil {
				t.Fatal(err)
			}
			if stored.IsDone != nil && *stored.IsDone {
				t.Fatal("admission must not complete the historical task")
			}
			if scenario != "addressed assignee" {
				ref := activities.WorklistRowRef{Source: "task", RowID: taskID.String()}
				if err := e.Activities.PinWorklistRow(e.Admin(), ref); err != nil {
					t.Fatal(err)
				}
				assertTask(true)
				if err := e.Activities.UnpinWorklistRow(e.Admin(), ref); err != nil {
					t.Fatal(err)
				}
				assertTask(false)
			}
			if _, err := e.Activities.UpdateActivity(e.Admin(), taskID, activities.UpdateActivityInput{Subject: new("I will take this request")}); err != nil {
				t.Fatal(err)
			}
			assertTask(true)
		})
	}
}

func assertLeadRequestWork(t *testing.T, e *integration.Env, leadID ids.LeadID, want int) {
	t.Helper()
	lead, err := e.Contacts.GetLead(e.Admin(), leadID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if lead.OpenTaskCount == nil || *lead.OpenTaskCount != want {
		t.Errorf("lead open tasks=%v want %d", lead.OpenTaskCount, want)
	}
	if (lead.NextTaskSubject != nil) != (want > 0) {
		t.Errorf("lead next task=%v want visible=%v", lead.NextTaskSubject, want > 0)
	}
}
