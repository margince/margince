// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAcceptedRequestKeepsOneHumanOwnerWhenMailboxIdentityChanges(t *testing.T) {
	e := integration.Setup(t)
	at := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	first := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	identities := capture.NewOwnerIdentityStore(e.DB())
	alias := "requests@ourco.test"
	claim, err := identities.Add(first, "address", alias)
	if err != nil {
		t.Fatal(err)
	}
	contact := e.SeedContact(t, "Request buyer", &e.AdminUser)
	source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "email", Source: "manual", Subject: new("Please send the specification"), Direction: new("inbound"), OccurredAt: &at, EmailTo: []string{alias}, Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}}})
	if err != nil {
		t.Fatal(err)
	}
	sourceID := ids.UUID(source.Id)
	if _, err := e.Activities.SetOwedVerdict(first, sourceID, activities.OwedVerdictAsksUs, "fixture", at); err != nil {
		t.Fatal(err)
	}
	original, _, err := e.Activities.LogActivity(principal.SystemActing(e.Admin(), "system:owed_verdict"), activities.LogActivityInput{Kind: "task", Source: "manual", Subject: new("Reply to the request"), AssigneeID: new(ids.From[ids.UserKind](e.AdminUser)), SourceSystem: new(activities.EmailRequestTaskSource), SourceID: new(source.Id.String()), SourceActivityID: &sourceID})
	if err != nil {
		t.Fatal(err)
	}
	accepted, _, err := e.Activities.LogActivity(first, activities.LogActivityInput{Kind: "task", Source: "manual", RequestActivityID: &sourceID})
	if err != nil {
		t.Fatal(err)
	}
	if err := identities.Remove(first, claim.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := identities.Add(e.Admin(), "address", alias); err != nil {
		t.Fatal(err)
	}
	feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return at.Add(time.Hour) })
	wrong, err := feed.Worklist(e.Admin(), "mine", "all", ids.Nil, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range wrong.Queue {
		if row.Source == "task" && row.Id == original.Id.String() {
			t.Fatal("alias transfer revived the superseded automatic task")
		}
	}
	right, err := feed.Worklist(first, "mine", "all", ids.Nil, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	if right.Summary.Total != 1 || len(right.Queue) != 1 || right.Queue[0].Id != accepted.Id.String() {
		t.Fatalf("explicit acceptance lost ownership: %+v", right.Summary)
	}
}

func TestAcceptingAnOldOwnRequestRestoresPersonalAndContactWork(t *testing.T) {
	e := integration.Setup(t)
	at := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	old := at.AddDate(-2, 0, 0)
	contact := e.SeedContact(t, "Historical request buyer", &e.AdminUser)
	source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "email", Source: "manual", Subject: new("Please send the report"), Direction: new("inbound"), OccurredAt: &old, Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}}})
	if err != nil {
		t.Fatal(err)
	}
	sourceID := ids.UUID(source.Id)
	if _, err := e.Activities.SetOwedVerdict(e.Admin(), sourceID, activities.OwedVerdictAsksUs, "fixture", at); err != nil {
		t.Fatal(err)
	}
	original, _, err := e.Activities.LogActivity(principal.SystemActing(e.Admin(), "system:owed_verdict"), activities.LogActivityInput{Kind: "task", Source: "manual", Subject: new("Reply to the report request"), AssigneeID: new(ids.From[ids.UserKind](e.AdminUser)), SourceSystem: new(activities.EmailRequestTaskSource), SourceID: new(source.Id.String()), SourceActivityID: &sourceID, Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Activities.ArchiveActivity(principal.SystemActing(e.Admin(), "system:owed_verdict"), ids.From[ids.ActivityKind](ids.UUID(original.Id)), nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "task", Source: "manual", RequestActivityID: &sourceID}); err != nil {
		t.Fatal(err)
	}
	feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return at })
	page, err := feed.Worklist(e.Admin(), "mine", "all", ids.Nil, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	if page.Summary.Total != 1 || len(page.Queue) != 1 || page.Queue[0].Id != original.Id.String() {
		t.Fatalf("accepted historical task missing: %+v", page.Summary)
	}
	card, err := pageAsOf(e, at).Assemble(e.Admin(), ids.From[ids.ContactKind](contact))
	if err != nil {
		t.Fatal(err)
	}
	if card.NextSteps == nil || len(card.NextSteps.Data) != 1 || card.NextSteps.Data[0].Id != original.Id {
		t.Fatal("accepted historical task missing from contact next steps")
	}
}
