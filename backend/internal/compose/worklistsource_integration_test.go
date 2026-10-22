// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestWorklistCommitmentKeepsItsContactAndReadableSource(t *testing.T) {
	e := integration.Setup(t)
	now := time.Date(2026, 9, 15, 8, 0, 0, 0, time.UTC)
	due := now.Add(time.Hour)
	contact := e.SeedContact(t, "Dana Weiss", &e.Rep1)
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return now })
	for _, private := range []bool{false, true} {
		subject, body := "Scope discussion", "I will send the revised scope."
		input := activities.LogActivityInput{Kind: "meeting", Subject: &subject, Body: &body, Source: "manual", OccurredAt: &now}
		owner := reader
		if private {
			input.VisibleOnlyTo(e.Rep2)
			owner = e.As(e.Rep2, []ids.UUID{e.Team1}, integration.AdminPerms)
		}
		source, _, err := e.Activities.LogActivity(owner, input)
		if err != nil {
			t.Fatal(err)
		}
		assignee := ids.From[ids.UserKind](e.Rep1)
		if private {
			assignee = ids.From[ids.UserKind](e.Rep2)
		}
		task, _, err := e.Activities.LogActivity(owner, activities.LogActivityInput{Kind: "task", Subject: new("Send revised scope"), Source: "manual", DueAt: &due, AssigneeID: &assignee, SourceActivityID: new(ids.UUID(source.Id)), Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}}})
		if err != nil {
			t.Fatal(err)
		}

		day, err := feed.Worklist(reader, "mine", "all", ids.Nil, 100, "")
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, row := range day.Queue {
			if row.Id != task.Id.String() {
				continue
			}
			found = true
			if row.Subject == nil || ids.UUID(row.Subject.Id) != contact || row.Subject.Label == nil || *row.Subject.Label != "Dana Weiss" {
				t.Fatalf("commitment lost its contact: %+v", row.Subject)
			}
			if row.SourceRecord == nil || row.SourceRecord.Type != "activity" || ids.UUID(row.SourceRecord.Id) != ids.UUID(source.Id) || row.SourceRecord.Label == nil || *row.SourceRecord.Label != subject {
				t.Fatalf("commitment lost its named source: %+v", row.SourceRecord)
			}
			opened, err := e.Activities.GetActivity(reader, ids.From[ids.ActivityKind](ids.UUID(row.SourceRecord.Id)), storekit.LiveOnly)
			if err != nil || opened.Body == nil || *opened.Body != body {
				t.Fatalf("source cannot be read: %+v %v", opened, err)
			}
		}
		if found == private {
			t.Fatalf("private=%v: commitment present=%v", private, found)
		}
	}
}

func TestStandaloneAndCombinedFollowUpsInheritSourceFiling(t *testing.T) {
	for _, combined := range []bool{false, true} {
		t.Run(map[bool]string{false: "standalone", true: "combined"}[combined], func(t *testing.T) {
			e := integration.Setup(t)
			ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
			contact := e.SeedContact(t, "Private relationship", &e.Rep1)
			e.MakeCapturePrivate(t, "contact", contact, e.Rep1)
			task := activities.LogActivityInput{Kind: "task", Subject: new("Send pricing"), Source: "manual"}
			input := activities.LogActivityInput{Kind: "note", Subject: new("Pricing review"), Source: "manual", Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}}}
			if combined {
				input.FollowUpTask = &task
			}
			source, _, err := e.Activities.LogActivity(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			sourceID := ids.From[ids.ActivityKind](ids.UUID(source.Id))
			if !combined {
				task.SourceActivityID = &sourceID.UUID
				if _, _, err := e.Activities.LogActivity(ctx, task); err != nil {
					t.Fatal(err)
				}
			}
			tasks, _, err := e.Activities.ListActivities(ctx, activities.ListActivitiesInput{SourceActivityID: &sourceID})
			if err != nil || len(tasks) != 1 {
				t.Fatalf("follow-ups = %+v %v", tasks, err)
			}
			if tasks[0].Links == nil || len(*tasks[0].Links) != 1 || ids.UUID((*tasks[0].Links)[0].EntityId) != contact {
				t.Fatalf("source filing lost: %+v", tasks[0].Links)
			}
			other := e.As(e.Rep2, []ids.UUID{e.Team1}, integration.AdminPerms)
			if _, err := e.Activities.GetActivity(other, ids.From[ids.ActivityKind](ids.UUID(tasks[0].Id)), storekit.LiveOnly); !errors.Is(err, apperrors.ErrNotFound) {
				t.Fatalf("private follow-up discoverable: %v", err)
			}
		})
	}
}

func TestWorklistApprovalKeepsAnArchivedSourceReachable(t *testing.T) {
	e := integration.Setup(t)
	now := time.Date(2026, 9, 15, 8, 0, 0, 0, time.UTC)
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	source, _, err := e.Activities.LogActivity(reader, activities.LogActivityInput{
		Kind: "note", Subject: new("Scope agreement"), Body: new("Send revised scope."), Source: "manual", OccurredAt: &now,
	})
	if err != nil {
		t.Fatal(err)
	}
	pipeline, open, _ := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, "Scope renewal", pipeline, open, &e.Rep1)
	queue := approvalsServiceWithEffects(e.Pool)
	approval, err := queue.Stage(onBehalfOf(e.AgentCtx(), e.Rep1), approvals.StageInput{
		Kind: "commitment_task", ProposedChange: json.RawMessage(`{"summary":"Send revised scope","seat_id":"` + e.Rep1.String() + `","promiser_seat_id":"` + e.Rep1.String() + `"}`),
		DiffHash: "archived-scope-review", TargetType: "deal", TargetID: deal, Summary: "Send revised scope",
		Evidence: []approvals.Evidence{{SourceType: "activity", SourceID: ids.UUID(source.Id), Snippet: "Send revised scope."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	sourceID := ids.From[ids.ActivityKind](ids.UUID(source.Id))
	if _, err := e.Activities.ArchiveActivity(reader, sourceID, nil); err != nil {
		t.Fatal(err)
	}
	feed := newAttentionService(e.Pool, queue, func() time.Time { return now })
	work, err := feed.Worklist(reader, "mine", "all", ids.Nil, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range work.Queue {
		if row.Id != approval.String() {
			continue
		}
		if row.Subject == nil || row.Subject.Type != "deal" || ids.UUID(row.Subject.Id) != deal {
			t.Fatalf("approval lost its target context: %+v", row.Subject)
		}
		decision, err := queue.GetWire(reader, approval)
		if err != nil || decision.Evidence == nil || len(*decision.Evidence) != 1 || (*decision.Evidence)[0].SourceId == nil || ids.UUID(*(*decision.Evidence)[0].SourceId) != sourceID.UUID {
			t.Fatalf("approval lost its evidence: %+v %v", decision, err)
		}
		opened, err := e.Activities.GetActivity(reader, sourceID, storekit.IncludeArchived)
		if err != nil || opened.ArchivedAt == nil || opened.Body == nil || *opened.Body != "Send revised scope." {
			t.Fatalf("archived source cannot be inspected: %+v %v", opened, err)
		}
		return
	}
	t.Fatal("pending approval disappeared with its archived source")
}
