// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package quietreminders_test

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/quietreminders"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func TestQuietPipelineSelectionRunsBeforeThePageLimit(t *testing.T) {
	e := integration.Setup(t)
	pipeline, open, _ := integration.DealFixture(t, e)
	other, err := e.Deals.CreatePipeline(e.Admin(), deals.CreatePipelineInput{Name: "Other sales", Stages: []deals.StageInput{
		{Name: "Open", Semantic: "open", WinProbability: 20},
		{Name: "Won", Semantic: "won", Position: 1, WinProbability: 100},
		{Name: "Lost", Semantic: "lost", Position: 2, WinProbability: 0},
	}})
	if err != nil {
		t.Fatal(err)
	}
	var otherOpen ids.StageID
	for _, stage := range *other.Stages {
		if stage.Semantic == "open" {
			otherOpen = ids.From[ids.StageKind](ids.UUID(stage.Id))
		}
	}
	outside := e.SeedDeal(t, "Earlier outside the chosen pipeline", ids.From[ids.PipelineKind](ids.UUID(other.Id)), otherOpen, nil)
	want := []ids.UUID{e.SeedDeal(t, "First selected deal", pipeline, open, nil), e.SeedDeal(t, "Second selected deal", pipeline, open, nil)}
	quietTestMailbox(t, e)
	quietTestTouch(t, e, "deal", outside, time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC))
	for _, id := range want {
		quietTestTouch(t, e, "deal", id, time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC))
	}
	reader := quietreminders.NewReader(activities.NewStore(e.DB()).WithOwnerMailbox(activities.OwnerMailbox{CaughtUp: capture.MailboxCaughtUpSQL, Providers: compose.MailProviders()}), compose.NewCollectionsStore(e.Pool), deals.NewStore(e.DB(), compose.DealsInstallation()))
	page, err := reader.QuietCandidates(e.Admin(), automation.QuietPopulation{PipelineID: pipeline.UUID}, time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC), 1)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Records) != 1 || page.Records[0].Ref.ID == outside {
		t.Fatalf("selected pipeline page=%+v", page)
	}
}

func TestQuietCustomerListNeedsNoOpenDealAndSharesReminderSuppression(t *testing.T) {
	e := integration.Setup(t)
	visible := e.SeedContact(t, "Selected customer", nil)
	private := e.SeedContact(t, "Private selected customer", &e.Rep2)
	outside := e.SeedContact(t, "Customer outside the list", nil)
	quietTestMailbox(t, e)
	for _, id := range []ids.UUID{visible, private, outside} {
		quietTestTouch(t, e, "contact", id, time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC))
	}
	e.MakeCapturePrivate(t, "contact", private, e.Rep2)
	lists := compose.NewCollectionsStore(e.Pool)
	list, err := lists.CreateList(e.Admin(), collections.CreateListInput{Name: "Chosen customers", EntityType: "contact", ListType: "static", Sharing: "workspace"})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []ids.UUID{visible, private} {
		if _, err := lists.AddMember(e.Admin(), list.ID, collections.MemberChange{EntityType: "contact", EntityID: id, Reason: collections.ReasonChosen}); err != nil {
			t.Fatal(err)
		}
	}
	rbac, err := identity.NewService(e.Pool).EffectiveRBAC(e.Admin(), e.WS, e.Rep1)
	if err != nil {
		t.Fatal(err)
	}
	ctx := e.As(e.Rep1, rbac.TeamIDs, rbac.Permissions)
	reader := quietreminders.NewReader(activities.NewStore(e.DB()).WithOwnerMailbox(activities.OwnerMailbox{CaughtUp: capture.MailboxCaughtUpSQL, Providers: compose.MailProviders()}), compose.NewCollectionsStore(e.Pool), deals.NewStore(e.DB(), compose.DealsInstallation()))
	population := automation.QuietPopulation{ListID: list.ID.UUID}
	cutoff := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	page, err := reader.QuietCandidates(ctx, population, cutoff, 1)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Records) != 1 || page.Records[0].Ref.ID != visible {
		t.Fatalf("visible customer page=%+v", page)
	}
	_, err = activities.NewProvider(e.DB()).Create(principal.SystemActing(e.Admin(), "time-scan"), datasource.CreateInput{
		EntityType: datasource.EntityActivity, Source: "system", Fields: map[string]any{
			"kind": "task", "subject": "Check in", "source_system": provenance.NoActivityReminderSource, "source_id": "selected-customer-reminder",
			"links": []map[string]any{{"entity_type": "contact", "entity_id": visible}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	page, err = reader.QuietCandidates(ctx, population, cutoff, 1)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 0 || len(page.Records) != 0 {
		t.Fatalf("equivalent open reminder was ignored: %+v", page)
	}
}

func quietTestTouch(t *testing.T, e *integration.Env, kind string, id ids.UUID, at time.Time) {
	t.Helper()
	host := ids.From[ids.UserKind](e.AdminUser)
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "call", Source: "manual",
		Subject: new("Customer conversation"), OccurredAt: &at, CallOutcome: new("reached"), HostUserID: &host,
		Links: []activities.ActivityLinkInput{{EntityType: kind, EntityID: id}},
	}); err != nil {
		t.Fatal(err)
	}
}

func quietTestMailbox(t *testing.T, e *integration.Env) {
	t.Helper()
	for _, user := range []ids.UUID{e.AdminUser, e.Rep2} {
		e.WsExec(t, `WITH connection AS (
 INSERT INTO capture_connection(user_id,provider,status,credential_ref) VALUES($1,'gmail','connected','vault:test') RETURNING id)
 INSERT INTO capture_backfill(connection_id,window_months,after_date,status)
 SELECT id,12,DATE '2025-07-01','done' FROM connection`, user)
	}
}
