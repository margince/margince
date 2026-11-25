// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Removing a unit leaves every list, saved view and export that does not name
// its kinds working. What names them reads as unavailable or empty, never as a
// fault, and a member without a kind's read right sees the same.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/collections"
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

func seedKindLists(t *testing.T, e *integration.Env, store *collections.Store, ctx context.Context) kindLists {
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

// assertKindListsUnavailable checks every list and view surface a reader who
// cannot read the kind reaches.
func assertKindListsUnavailable(t *testing.T, e *integration.Env, store *collections.Store, ctx context.Context, seeded kindLists) {
	t.Helper()
	page, err := store.ListsPage(ctx, collections.ListFilter{})
	if err != nil {
		t.Fatalf("list library = %v, want it to load", err)
	}
	health := map[string]crmcontracts.ListHealth{}
	for _, l := range page.Data {
		health[l.Name] = l.Health
	}
	want := map[string]crmcontracts.ListHealth{"Berlin visits": "unavailable", "Chosen visits": "unavailable", "Contacts": crmcontracts.ListHealthOk}
	for name, h := range want {
		if health[name] != h {
			t.Errorf("health of %s = %q, want %q", name, health[name], h)
		}
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
		if code := exportListStatus(e, store, ctx, id); code != http.StatusUnprocessableEntity {
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

func exportListStatus(e *integration.Env, store *collections.Store, ctx context.Context, id ids.ListID) int {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/exports/filtered",
		strings.NewReader(`{"list_id":"`+id.String()+`","format":"csv"}`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	filteredExportHandlers{writer: NewFilteredExportWriter(e.Pool), collections: store, listsOn: true}.CreateFilteredExport(rec, req)
	return rec.Code
}

func TestRemovingAUnitLeavesItsListsUnavailableAndTheRestWorking(t *testing.T) {
	e := integration.Setup(t)
	registerHistoryVisits(t)
	store := collections.NewStore(e.DB())
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, listingPerms(true))
	seeded := seedKindLists(t, e, store, ctx)
	if _, err := CheckLiveLists(context.Background(), e.Pool, e.WS, time.Now); err != nil {
		t.Fatalf("the first check: %v", err)
	}

	recordkinds.Reset()

	assertKindListsUnavailable(t, e, store, ctx, seeded)
	checks, err := CheckLiveLists(context.Background(), e.Pool, e.WS, time.Now)
	if err != nil {
		t.Fatalf("the check after the removal: %v", err)
	}
	for _, check := range checks {
		if check.ListID == seeded.live && (check.Outcome != collections.CheckInvalid || check.Err != nil) {
			t.Errorf("check of the Live List = %s, %v; want invalid and no failure", check.Outcome, check.Err)
		}
	}
	if left := e.WsScalar(t, `SELECT count(*)::text FROM list_member_event WHERE list_id = '`+seeded.live.String()+`' AND action = 'left'`); left != "0" {
		t.Errorf("%s members recorded as left, want none: the unit's removal is not the records leaving", left)
	}
}

func TestAMemberWithoutTheKindsReadRightSeesItsListsUnavailable(t *testing.T) {
	e := integration.Setup(t)
	registerHistoryVisits(t)
	store := collections.NewStore(e.DB())
	seeded := seedKindLists(t, e, store, e.As(e.Rep1, []ids.UUID{e.Team1}, listingPerms(true)))

	assertKindListsUnavailable(t, e, store, e.As(e.Rep1, []ids.UUID{e.Team1}, listingPerms(false)), seeded)
}
