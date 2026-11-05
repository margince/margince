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
	"github.com/margince/margince/backend/internal/compose/project360"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestLegacyClaimsRemainHistoryWithoutAssigningTheContactOwner(t *testing.T) {
	e := integration.Setup(t)
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(-2, 0, 0)
	contact := e.SeedContact(t, "Meeting buyer", &e.AdminUser)
	company := e.SeedCompany(t, "Account example", &e.AdminUser)
	contactID, companyID := ids.From[ids.ContactKind](contact), ids.From[ids.CompanyKind](company)
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{Kind: "employment", ContactID: &contactID, CompanyID: &companyID, IsCurrentPrimary: new(true), Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	quote := "Morgan will send the account plan."
	source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "meeting", Body: &quote, OccurredAt: &old, MeetingStatus: new("held"), Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatal(err)
	}
	automatic := principal.SystemActing(e.Admin(), "system:signal_scan")
	claim := e.RecordClaim(automatic, t, contacts.ClaimInput{
		ContactID: ids.From[ids.ContactKind](contact), Kind: "commitment_ours", Body: quote,
		ActivityID: ids.UUID(source.Id), Quote: quote, DueAt: &old, Source: "manual",
	})
	feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return now })
	page, err := feed.Worklist(e.Admin(), "mine", "all", ids.Nil, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range page.Queue {
		if row.Source == "conversation_claim" {
			t.Errorf("contact ownership manufactured personal work: %+v", row)
		}
	}
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		history, err := e.Contacts.ClaimsForContact(e.Admin(), tx, ids.From[ids.ContactKind](contact), nil, 20)
		if err != nil {
			return err
		}
		if len(history) != 1 || history[0].Id != claim.Id || !history[0].NeedsReview || history[0].SourceQuote != quote {
			t.Errorf("legacy evidence must remain reviewable: %+v", history)
		}
		companyClaims, complete, err := e.Contacts.OpenCommitmentsForCompany(e.Admin(), tx, company, 20)
		if err != nil {
			return err
		}
		if len(companyClaims) != 0 || !complete {
			t.Errorf("legacy claim became account obligation: %+v complete=%v", companyClaims, complete)
		}
		current, err := e.Contacts.OpenCommitmentsForContact(e.Admin(), tx, ids.From[ids.ContactKind](contact), nil, 20)
		if err == nil && len(current) != 0 {
			t.Errorf("unverified historical promise became current: %+v", current)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestHistoricalAutomaticTasksRequireCurrentEvidenceOrHumanAdoption(t *testing.T) {
	for _, state := range []string{"automatic", "undated automatic", "imported", "manual", "edited", "reopened", "pinned", "reassigned"} {
		t.Run(state, func(t *testing.T) {
			e := integration.Setup(t)
			now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
			old := now.AddDate(-2, 0, 0)
			contact := e.SeedContact(t, "Task buyer", &e.AdminUser)
			company := e.SeedCompany(t, "Task account", &e.AdminUser)
			project, err := e.Projects.CreateProject(e.Admin(), projects.CreateProjectInput{Name: "Account delivery", CompanyID: ids.From[ids.CompanyKind](company), Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "meeting", Subject: new("Completed account meeting"), OccurredAt: &old, MeetingStatus: new("held"), Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			actor := principal.SystemActing(e.Admin(), "system:signal_scan")
			if state == "manual" {
				actor = e.Admin()
			}
			if state == "imported" {
				actor = legacyImportContext(t, e)
			}
			assignee := ids.From[ids.UserKind](e.AdminUser)
			due := &old
			if state == "manual" || state == "edited" || state == "reassigned" {
				due = new(now.Add(-time.Hour))
			}
			if state == "undated automatic" {
				due = nil
			}
			task, _, err := e.Activities.LogActivity(actor, activities.LogActivityInput{
				Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}, {EntityType: "company", EntityID: company}, {EntityType: "project", EntityID: ids.UUID(project.Id)}},
				Kind:  "task", Subject: new("Send the account plan"), DueAt: due, Source: "manual", AssigneeID: &assignee,
				SourceSystem: new(activities.CommitmentTaskSource), SourceID: new(ids.NewV7().String()), SourceActivityID: new(ids.UUID(source.Id)),
			})
			if err != nil {
				t.Fatal(err)
			}
			id := ids.From[ids.ActivityKind](ids.UUID(task.Id))
			switch state {
			case "edited":
				_, err = e.Activities.UpdateActivity(e.Admin(), id, activities.UpdateActivityInput{Subject: new("My active account plan")})
			case "reopened":
				for _, done := range []bool{true, false} {
					if _, err := e.Activities.UpdateActivity(e.Admin(), id, activities.UpdateActivityInput{IsDone: &done}); err != nil {
						t.Fatal(err)
					}
				}
			case "pinned":
				err = e.Activities.PinWorklistRow(e.Admin(), activities.WorklistRowRef{Source: "task", RowID: id.String()})
			case "reassigned":
				for _, owner := range []ids.UUID{e.Rep1, e.AdminUser} {
					assigned := ids.From[ids.UserKind](owner)
					if _, err := e.Activities.UpdateActivity(e.Admin(), id, activities.UpdateActivityInput{AssigneeID: &assigned}); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return now })
			page, err := feed.Worklist(e.Admin(), "mine", "all", ids.Nil, 100, "")
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, row := range page.Queue {
				if row.Source == "task" && row.Id == id.String() {
					found = true
				}
			}
			if found != (state != "automatic" && state != "undated automatic" && state != "imported") {
				t.Errorf("task in personal queue=%v for %s: %+v", found, state, page.Queue)
			}
			contactPage, err := pageAsOf(e, now).Assemble(e.Admin(), ids.From[ids.ContactKind](contact))
			if err != nil {
				t.Fatal(err)
			}
			companyPage, err := company360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects, approvals.NewService(e.DB()), func() time.Time { return now }).Assemble(e.Admin(), ids.From[ids.CompanyKind](company))
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if state != "automatic" && state != "undated automatic" && state != "imported" {
				want = 1
			}
			if page.Summary.Total != want {
				t.Errorf("Worklist total=%d, want %d", page.Summary.Total, want)
			}
			if contactPage.NextSteps == nil || len(contactPage.NextSteps.Data) != want {
				t.Errorf("contact next steps=%+v, want %d", contactPage.NextSteps, want)
			}
			if companyPage.NextSteps == nil || len(companyPage.NextSteps.Data) != want {
				t.Errorf("company next steps=%+v, want %d", companyPage.NextSteps, want)
			}
			projectPage, err := project360.NewService(e.Pool, e.Deals, e.Projects, e.Contacts, e.Contracts, e.Activities, func() time.Time { return now }).Assemble(e.Admin(), ids.From[ids.ProjectKind](ids.UUID(project.Id)))
			if err != nil {
				t.Fatal(err)
			}
			if projectPage.Commitments == nil || len(projectPage.Commitments.Data) != want {
				t.Errorf("project commitments=%+v, want %d", projectPage.Commitments, want)
			}
			if projectPage.Rollups == nil || projectPage.Rollups.OpenCommitments != want {
				t.Errorf("project commitment count=%+v, want %d", projectPage.Rollups, want)
			}
			stored, err := e.Activities.GetActivity(e.Admin(), id, storekit.LiveOnly)
			if err != nil {
				t.Fatal(err)
			}
			if stored.IsDone != nil && *stored.IsDone {
				t.Fatal("read admission pretended the task was fulfilled")
			}
		})
	}
}

func TestHistoricalClaimBatchUndoStillRestoresItsRecord(t *testing.T) {
	e := integration.Setup(t)
	contact := e.SeedContact(t, "Account buyer", &e.Rep1)
	promise := seedWorklistPromise(t, e, contact, "A colleague will send the plan")
	batch := ids.NewV7()
	// A stored completion predates personal admission. Use the native record
	// writer and batch recorder to preserve its original undo contract.
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		ctx := storekit.WithBatch(e.Admin(), batch)
		version, err := e.Contacts.SetClaimDoneTx(ctx, tx, ids.UUID(promise.Id), true)
		if err != nil {
			return err
		}
		change := completeItems(promise)
		return recordBulkOperation(ctx, tx, batch, change, bulkRun{
			changed:  []crmcontracts.BulkSampleRow{{Id: promise.Id, Label: "A colleague will send the plan"}},
			outcomes: []bulkOutcome{{ID: promise.Id, Version: version}},
		})
	}); err != nil {
		t.Fatal(err)
	}
	result, err := bulkEngineFor(e).Undo(e.Admin(), batch, "")
	if err != nil || result.Changed != 1 {
		t.Fatalf("historical undo=%+v, error=%v", result, err)
	}
	if got := e.WsScalar(t, `SELECT status FROM conversation_claim WHERE id=$1`, promise.Id); got != "open" {
		t.Fatalf("undo left claim %s, want open", got)
	}
}

func TestCurrentNativePromiseLeavesWorkWhenItsMeaningIsCompleted(t *testing.T) {
	e := integration.Setup(t)
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	at, quote := now.Add(-time.Hour), "I will send the account plan."
	source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "meeting", Body: &quote, OccurredAt: &at, MeetingStatus: new("held"), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	sourceID := ids.UUID(source.Id)
	commitment := Commitment{Meaning: openCommitmentMeaning("", quote), AsOf: now, ReadMessages: []ids.UUID{sourceID}, Extractor: signalScanActor, SourceActivityID: sourceID, Summary: "Send the account plan", Quote: quote, Seat: &e.Rep1, Confidence: 0.95, Locator: commitmentLocator(sourceID, claimKindOurs, e.Rep1.String(), quote), Body: quote}
	ctx := withTranscriptReader(e.Admin(), "human:"+e.Rep1.String(), ids.NewV7())
	dispatcher := NewCommitmentDispatcher(e.Pool, approvals.NewService(e.DB()))
	var taskID ids.UUID
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		outcome, task, err := dispatcher.DispatchTx(ctx, tx, commitment, ids.NewV7())
		if err == nil && outcome != CommitmentTaskWritten {
			t.Errorf("native current promise=%s, want task", outcome)
		}
		taskID = task
		return err
	}); err != nil {
		t.Fatal(err)
	}
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return now })
	for _, completed := range []bool{false, true} {
		if completed {
			read, err := e.Activities.GetActivity(e.Admin(), ids.From[ids.ActivityKind](sourceID), storekit.LiveOnly)
			if err != nil {
				t.Fatal(err)
			}
			if read.CommitmentAdmissions == nil || len(*read.CommitmentAdmissions) != 1 {
				t.Fatal("native dispatcher omitted admission")
			}
			decision := (*read.CommitmentAdmissions)[0]
			decision.CurrentWork = false
			decision.Meaning.State = "completed"
			decision.EvaluatedAt = now.Add(time.Minute)
			if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
				return activities.RecordCommitmentAdmissionTx(ctx, tx, ids.From[ids.ActivityKind](sourceID), decision)
			}); err != nil {
				t.Fatal(err)
			}
		}
		page, err := feed.Worklist(reader, "mine", "all", ids.Nil, 100, "")
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, row := range page.Queue {
			if row.Source == "task" && row.Id == taskID.String() {
				found = true
			}
		}
		if found == completed {
			t.Errorf("current task=%v with completed=%v", found, completed)
		}
	}
}

func legacyImportContext(t *testing.T, e *integration.Env) context.Context {
	t.Helper()
	actor := e.As(e.AdminUser, nil, principal.Permissions{RowScope: principal.RowScopeAll, Objects: map[string]principal.ObjectGrant{"import_run": {Create: true}}})
	run, err := migration.NewRunStore(e.DB()).Create(actor, migration.CreateRunInput{Connector: "csv", SourceRef: "history.csv", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	return storekit.WithBatch(storekit.WithImportRun(e.Admin(), run.ID), run.ID)
}

func TestImportedTeamClaimsDoNotBecomeCurrentBecauseAHumanRanTheImport(t *testing.T) {
	e := integration.Setup(t)
	contact := e.SeedContact(t, "Imported account contact", &e.AdminUser)
	old := time.Date(2024, 10, 21, 12, 0, 0, 0, time.UTC)
	quote := "Morgan will send the account plan."
	actor := legacyImportContext(t, e)
	source, _, err := e.Activities.LogActivity(actor, activities.LogActivityInput{Kind: "meeting", Body: &quote, OccurredAt: &old, MeetingStatus: new("held"), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	claim := e.RecordClaim(actor, t, contacts.ClaimInput{ContactID: ids.From[ids.ContactKind](contact), Kind: "commitment_ours", Body: quote, ActivityID: ids.UUID(source.Id), Quote: quote, DueAt: &old, Source: "manual"})
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		history, err := e.Contacts.ClaimsForContact(e.Admin(), tx, ids.From[ids.ContactKind](contact), nil, 20)
		if err != nil {
			return err
		}
		if len(history) != 1 || history[0].Id != claim.Id || !history[0].NeedsReview {
			t.Errorf("human import treated as fresh confirmation: %+v", history)
		}
		current, err := e.Contacts.OpenCommitmentsForContact(e.Admin(), tx, ids.From[ids.ContactKind](contact), nil, 20)
		if err == nil && len(current) != 0 {
			t.Errorf("imported claim admitted as current: %+v", current)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
