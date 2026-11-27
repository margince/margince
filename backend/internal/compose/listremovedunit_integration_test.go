// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Removing a unit leaves every list, saved view and export that does not name
// its kinds working. What names them reads as unavailable or empty, never as a
// fault, and composing the unit again picks up where it left off. A member
// without a kind's read right is told nothing about its lists.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
)

// kindLists are a Live List, a Shortlist with one member and a saved view over
// the visit kind. The contact list beside them never names it.
type kindLists struct {
	live, short, contacts ids.ListID
	view                  ids.SavedViewID
	visit                 ids.UUID
}

func seedKindLists(ctx context.Context, t *testing.T, e *integration.Env, store *collections.Store) kindLists {
	t.Helper()
	e.WsExec(t, `INSERT INTO ext.`+historyDemoTable+` (code, site) VALUES ('v1', 'Berlin site')`)
	visit, err := ids.Parse(e.WsScalar(t, `SELECT id::text FROM ext.`+historyDemoTable+` WHERE code = 'v1'`))
	if err != nil {
		t.Fatal(err)
	}
	berlin := map[string]any{"field": "site", "op": "contains", "value": "berlin"}
	live, err := store.CreateList(ctx, collections.CreateListInput{Name: "Berlin visits", EntityType: historyDemoKind, ListType: "dynamic", Definition: berlin})
	if err != nil {
		t.Fatalf("create the Live List: %v", err)
	}
	short, err := store.CreateList(ctx, collections.CreateListInput{Name: "Chosen visits", EntityType: historyDemoKind, ListType: "static"})
	if err != nil {
		t.Fatalf("create the Shortlist: %v", err)
	}
	if _, err := store.AddMember(ctx, short.ID, collections.MemberChange{EntityType: historyDemoKind, EntityID: visit, Reason: collections.ReasonChosen}); err != nil {
		t.Fatalf("add the visit: %v", err)
	}
	contacts, err := store.CreateList(ctx, collections.CreateListInput{Name: "Contacts", EntityType: "contact", ListType: "static"})
	if err != nil {
		t.Fatalf("create the contact list: %v", err)
	}
	view, err := store.CreateSavedView(ctx, collections.CreateSavedViewInput{Resource: historyDemoKind, Name: "Berlin", Query: map[string]any{"filter": berlin}})
	if err != nil {
		t.Fatalf("create the saved view: %v", err)
	}
	return kindLists{live: live.ID, short: short.ID, contacts: contacts.ID, view: view.ID, visit: visit}
}

// assertRemovedKindListsUnavailable checks every list and view surface once the
// unit is gone.
func assertRemovedKindListsUnavailable(ctx context.Context, t *testing.T, e *integration.Env, store *collections.Store, seeded kindLists) {
	t.Helper()
	page, err := store.ListsPage(ctx, collections.ListFilter{})
	if err != nil {
		t.Fatalf("list library = %v, want it to load", err)
	}
	byName := map[string]crmcontracts.List{}
	for _, l := range page.Data {
		byName[l.Name] = l
	}
	want := map[string]crmcontracts.ListHealth{"Berlin visits": "unavailable", "Chosen visits": "unavailable", "Contacts": crmcontracts.ListHealthOk}
	for name, h := range want {
		if byName[name].Health != h {
			t.Errorf("health of %s = %q, want %q", name, byName[name].Health, h)
		}
	}
	if byName["Berlin visits"].Definition != nil {
		t.Error("the library returned the filter of a removed kind's list")
	}
	for _, id := range []ids.ListID{seeded.live, seeded.short} {
		if _, err := store.ListView(ctx, id); err != nil {
			t.Errorf("list %s = %v, want it to load", id, err)
		}
		members, err := store.MembersPage(ctx, id, collections.MemberRead{})
		if err != nil || len(members.Data) != 0 {
			t.Errorf("members of %s = %d, %v; want an empty page", id, len(members.Data), err)
		}
		if _, err := store.ExplainView(ctx, id, seeded.visit); !errors.Is(err, apperrors.ErrNotFound) {
			t.Errorf("why the visit is on %s = %v, want not found", id, err)
		}
		history, _, err := store.History(ctx, id, 50, "")
		if err != nil || len(history) == 0 {
			t.Errorf("history of %s = %d entries, %v; want it to load", id, len(history), err)
		}
		for _, entry := range history {
			if entry.Definition != nil || entry.PreviousDefinition != nil {
				t.Errorf("history of %s returned the filter of a removed kind", id)
			}
		}
		if code := exportListStatus(ctx, e, store, id); code != http.StatusUnprocessableEntity {
			t.Errorf("export of %s = %d, want 422", id, code)
		}
	}
	filter := map[string]any{"filter": map[string]any{"field": "site", "op": "contains", "value": "munich"}}
	_, err = store.UpdateSavedView(ctx, seeded.view, collections.UpdateSavedViewInput{Query: &filter})
	if _, ok := errors.AsType[*collections.BadInputError](err); !ok {
		t.Errorf("changing the view's filter = %v, want a 422", err)
	}
	name := "Renamed"
	if _, err := store.UpdateSavedView(ctx, seeded.view, collections.UpdateSavedViewInput{Name: &name}); err != nil {
		t.Errorf("renaming the view = %v, want it to work", err)
	}
}

func exportListStatus(ctx context.Context, e *integration.Env, store *collections.Store, id ids.ListID) int {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/exports/filtered",
		strings.NewReader(`{"list_id":"`+id.String()+`","format":"csv"}`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	filteredExportHandlers{writer: NewFilteredExportWriter(e.Pool), collections: store, listsOn: true}.CreateFilteredExport(rec, req)
	return rec.Code
}

// checkOf runs one Live List check pass and answers the given list's check,
// failing on any error the pass or that list reports.
func checkOf(t *testing.T, e *integration.Env, list ids.ListID) collections.LiveCheck {
	t.Helper()
	checks, err := CheckLiveLists(context.Background(), e.Pool, e.WS, func() time.Time { return clocktest.Now(t) })
	if err != nil {
		t.Fatalf("check pass: %v", err)
	}
	for _, check := range checks {
		if check.ListID == list {
			if check.Err != nil {
				t.Fatalf("check of %s: %v", list, check.Err)
			}
			return check
		}
	}
	t.Fatalf("the pass did not check %s", list)
	return collections.LiveCheck{}
}

func TestRemovingAUnitLeavesItsListsUnavailableAndComposingItAgainResumes(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	registerHistoryVisits(t)
	store := collections.NewStore(e.DB())
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, listingPerms(true))
	seeded := seedKindLists(ctx, t, e, store)
	if first := checkOf(t, e, seeded.live); first.Outcome != collections.CheckComplete || first.Members != 1 {
		t.Fatalf("first check = %+v, want complete with the one Berlin visit", first)
	}
	rule := ids.NewV7()
	e.WsExec(t, `INSERT INTO automation (id, key, name, trigger, action, params, owner_id, enabled, tier)
		VALUES ($1, 'list_membership_notify', 'Tell me', '{"event_type":"list.evaluated"}', '{"kind":"notify"}',
		jsonb_build_object('list_id', $2::text), $3, true, 'auto_execute')`, rule, seeded.live.String(), e.Rep1)

	recordkinds.Reset()

	assertRemovedKindListsUnavailable(ctx, t, e, store, seeded)
	if removed := checkOf(t, e, seeded.live); removed.Outcome != collections.CheckInvalid || !removed.KindRemoved {
		t.Errorf("check after the removal = %+v, want invalid with the kind removed", removed)
	}
	if reason := e.WsScalar(t, `SELECT coalesce(paused_reason, '') FROM automation WHERE id = $1`, rule); reason != "list_unavailable" {
		t.Errorf("the rule on the list paused as %q, want list_unavailable", reason)
	}

	registerHistoryVisitKind()
	e.WsExec(t, `UPDATE ext.`+historyDemoTable+` SET site = 'Munich site' WHERE code = 'v1'`)
	e.WsExec(t, `INSERT INTO ext.`+historyDemoTable+` (code, site) VALUES ('v2', 'Berlin new')`)
	back := checkOf(t, e, seeded.live)
	if back.Outcome != collections.CheckComplete || back.Members != 1 || back.Entered != 1 || back.Left != 1 {
		t.Errorf("check after composing the unit again = %+v, want complete with v2 entered and v1 left", back)
	}
	left := e.WsScalar(t, `SELECT string_agg(entity_id::text, ',') FROM list_member_event WHERE list_id = $1::uuid AND action = 'left'`, seeded.live.String())
	if left != seeded.visit.String() {
		t.Errorf("left events = %q, want only v1 (%s): the removal itself is not the records leaving", left, seeded.visit)
	}
	members, err := store.MembersPage(ctx, seeded.live, collections.MemberRead{})
	if err != nil || len(members.Data) != 1 {
		t.Errorf("members after the unit is back = %d, %v; want v2", len(members.Data), err)
	}
}

func TestAMemberWithoutTheKindsReadRightIsToldNothingAboutItsLists(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	registerHistoryVisits(t)
	store := collections.NewStore(e.DB())
	seeded := seedKindLists(e.As(e.Rep1, []ids.UUID{e.Team1}, listingPerms(true)), t, e, store)
	colleague := e.As(e.Rep2, []ids.UUID{e.Team1}, listingPerms(false))

	page, err := store.ListsPage(colleague, collections.ListFilter{})
	if err != nil {
		t.Fatalf("list library = %v, want it to load", err)
	}
	var names []string
	for _, l := range page.Data {
		names = append(names, l.Name)
		if l.EntityType == historyDemoKind {
			t.Errorf("the library showed %q, a team list over a kind the member may not read, with filter %v", l.Name, l.Definition)
		}
	}
	if !slices.Contains(names, "Contacts") {
		t.Errorf("library = %v, want the team's contact list still there", names)
	}
	for _, id := range []ids.ListID{seeded.live, seeded.short} {
		if _, err := store.ListView(colleague, id); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("list %s = %v, want permission denied", id, err)
		}
		if _, _, err := store.History(colleague, id, 50, ""); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("history of %s = %v, want permission denied, as it carries the filter", id, err)
		}
		if _, err := store.MembersPage(colleague, id, collections.MemberRead{}); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("members of %s = %v, want permission denied", id, err)
		}
		if code := exportListStatus(colleague, e, store, id); code != http.StatusForbidden {
			t.Errorf("export of %s = %d, want 403", id, code)
		}
	}
}

func TestCreatingOverAnUnreadableKindAnswersAsOverARemovedOne(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	registerHistoryVisits(t)
	store := collections.NewStore(e.DB())
	colleague := e.As(e.Rep2, []ids.UUID{e.Team1}, listingPerms(false))
	refusal := func(kind string) (string, string) {
		_, listErr := store.CreateList(colleague, collections.CreateListInput{Name: "Probe", EntityType: kind, ListType: "static"})
		_, viewErr := store.CreateSavedView(colleague, collections.CreateSavedViewInput{Resource: kind, Name: "Probe", Query: map[string]any{}})
		for _, err := range []error{listErr, viewErr} {
			if _, ok := errors.AsType[*collections.BadInputError](err); !ok {
				t.Errorf("creating over %s = %v, want a 422", kind, err)
			}
		}
		return fmt.Sprint(listErr), fmt.Sprint(viewErr)
	}
	unreadableList, unreadableView := refusal(historyDemoKind)
	removedList, removedView := refusal("ext:nobody:ghost")
	if strings.ReplaceAll(unreadableList, historyDemoKind, "") != strings.ReplaceAll(removedList, "ext:nobody:ghost", "") ||
		strings.ReplaceAll(unreadableView, historyDemoKind, "") != strings.ReplaceAll(removedView, "ext:nobody:ghost", "") {
		t.Errorf("the refusals differ, so they tell the kinds apart: %q vs %q, %q vs %q", unreadableList, removedList, unreadableView, removedView)
	}
}
