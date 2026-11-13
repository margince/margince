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
)

func TestQuietPreviewIntersectsTheCallerAndCurrentAuthorizer(t *testing.T) {
	e := integration.Setup(t)
	e.GrantRole(t, e.AdminUser, "admin")
	e.GrantRole(t, e.Rep1, "admin")
	shared := e.SeedContact(t, "Shared customer", &e.AdminUser)
	callerOnly := e.SeedContact(t, "Caller's private customer", &e.AdminUser)
	ownerOnly := e.SeedContact(t, "Authorizer's private customer", &e.Rep1)
	quietTestMailbox(t, e)
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	lists := compose.NewCollectionsStore(e.Pool)
	list, err := lists.CreateList(e.Admin(), collections.CreateListInput{Name: "Preview customers", EntityType: "contact", ListType: "static", Sharing: "workspace"})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []ids.UUID{shared, callerOnly, ownerOnly} {
		quietTestTouch(t, e, "contact", id, now.AddDate(0, 0, -14))
		if _, err := lists.AddMember(e.Admin(), list.ID, collections.MemberChange{EntityType: "contact", EntityID: id, Reason: collections.ReasonChosen}); err != nil {
			t.Fatal(err)
		}
	}
	e.MakeCapturePrivate(t, "contact", callerOnly, e.AdminUser)
	e.MakeCapturePrivate(t, "contact", ownerOnly, e.Rep1)
	activityStore := activities.NewStore(e.DB()).WithOwnerMailbox(activities.OwnerMailbox{CaughtUp: capture.MailboxCaughtUpSQL, Providers: compose.MailProviders()})
	reader := quietreminders.NewReader(activityStore, lists, deals.NewStore(e.DB(), compose.DealsInstallation()))
	population := automation.QuietPopulation{ListID: list.ID.UUID}
	callerPage, err := reader.QuietCandidates(e.Admin(), population, now.AddDate(0, 0, -7), 5)
	if err != nil || callerPage.Total != 2 {
		t.Fatalf("caller-only candidates=%+v, %v", callerPage, err)
	}
	authority := identity.NewService(e.Pool)
	rbac, err := authority.EffectiveRBAC(e.Admin(), e.WS, e.Rep1)
	if err != nil {
		t.Fatal(err)
	}
	ownerPage, err := reader.QuietCandidates(e.As(e.Rep1, rbac.TeamIDs, rbac.Permissions), population, now.AddDate(0, 0, -7), 5)
	if err != nil || ownerPage.Total != 2 {
		t.Fatalf("authorizer-only candidates=%+v, %v", ownerPage, err)
	}
	callerRBAC, err := authority.EffectiveRBAC(e.Admin(), e.WS, e.AdminUser)
	if err != nil {
		t.Fatal(err)
	}
	caller := e.As(e.AdminUser, callerRBAC.TeamIDs, callerRBAC.Permissions)
	store := automation.NewAutomationStore(e.DB()).WithPreviewAuthority(authority).WithQuietPopulationScan(reader).SetClock(func() time.Time { return now })
	rule, err := store.Create(caller, automation.CreateAutomationInput{
		Key: "check_in_cadence", Name: "Reviewed customers",
		Params: map[string]any{"customer_list_id": list.ID.String(), "check_in_days": float64(7)},
	})
	if err != nil {
		t.Fatal(err)
	}
	owner := ids.From[ids.UserKind](e.Rep1)
	if _, err := store.Update(caller, rule.ID, automation.UpdateAutomationInput{OwnerID: &owner}); err != nil {
		t.Fatal(err)
	}
	preview, err := store.Preview(caller, rule.ID, automation.AutomationPreviewInput{})
	if err != nil {
		t.Fatal(err)
	}
	if preview.MatchesNow != 1 || len(preview.Sample) != 1 || preview.Sample[0] != shared {
		t.Fatalf("preview escaped the shared visibility: %+v", preview)
	}
}
