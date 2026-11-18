// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A declared kind has lists, saved views and a filter builder like a core record.
// Its declared fields are the vocabulary, and a preview shows them as columns.
// A list takes members of its own kind only.
// A caller without the kind's read right is told nothing about it.

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func listingPerms(kindRead bool) principal.Permissions {
	full := principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}
	return principal.Permissions{
		RowScope: principal.RowScopeAll,
		Objects: map[string]principal.ObjectGrant{
			"contact": {Read: true}, "list": full, "saved_view": full,
			"ext_historydemo_visit": {Read: kindRead},
		},
	}
}

func TestAnExtensionKindsVocabularyIsItsDeclaredFields(t *testing.T) {
	e := integration.Setup(t)
	registerHistoryVisits(t)
	store := collections.NewStore(e.DB())

	fields, ok, err := store.FilterVocabulary(e.As(e.Rep1, nil, listingPerms(true)), historyDemoKind)
	if err != nil || !ok {
		t.Fatalf("vocabulary = ok %v, err %v; want the kind's fields", ok, err)
	}
	var names []string
	for _, field := range fields {
		names = append(names, field.Name)
	}
	if strings.Join(names, ",") != "code,site" {
		t.Errorf("fields = %v, want code and site", names)
	}
	if _, ok, _ := store.FilterVocabulary(e.As(e.Rep1, nil, listingPerms(false)), historyDemoKind); ok {
		t.Error("a caller without the kind's read right was served its vocabulary")
	}
	if _, ok, _ := store.FilterVocabulary(e.As(e.Rep1, nil, listingPerms(true)), "ext:nobody:ghost"); ok {
		t.Error("a kind no extension declared was served a vocabulary")
	}
}

func TestAPreviewOfAnExtensionKindShowsItsColumnsAndOnlyLiveMatches(t *testing.T) {
	e := integration.Setup(t)
	registerHistoryVisits(t)
	e.WsExec(t, `INSERT INTO ext.`+historyDemoTable+` (code, site, archived_at) VALUES
		('v1', 'Berlin site', NULL), ('v2', 'Berlin old', now()), ('v3', 'Munich site', NULL)`)
	preview := func(perms principal.Permissions) (int, crmcontracts.FilterPreview) {
		body := `{"resource":"` + historyDemoKind + `","filter":{"field":"site","op":"contains","value":"berlin"}}`
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/filters/preview", strings.NewReader(body)).
			WithContext(e.As(e.Rep1, nil, perms))
		req.Header.Set("Content-Type", "application/json")
		filterPreviewHandlers{pool: e.Pool, collections: collections.NewStore(e.DB())}.PreviewFilter(rec, req)
		var out crmcontracts.FilterPreview
		if rec.Code == http.StatusOK {
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
		}
		return rec.Code, out
	}

	code, got := preview(listingPerms(true))
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if got.MatchCount != 1 || len(got.Rows) != 1 || got.Rows[0]["site"] != "Berlin site" {
		t.Errorf("preview = %d matches, rows %v; want the one live Berlin visit", got.MatchCount, got.Rows)
	}
	if strings.Join(got.Columns, ",") != "id,code,site" {
		t.Errorf("columns = %v, want id, code, site", got.Columns)
	}
	if code, _ := preview(listingPerms(false)); code == http.StatusOK {
		t.Error("a caller without the kind's read right was served a preview")
	}
}

func TestAnExtensionListTakesItsOwnKindAndRefusesAnother(t *testing.T) {
	e := integration.Setup(t)
	registerHistoryVisits(t)
	e.WsExec(t, `INSERT INTO ext.`+historyDemoTable+` (code, site) VALUES ('v1', 'Berlin site')`)
	visit, err := ids.Parse(e.WsScalar(t, `SELECT id::text FROM ext.`+historyDemoTable+` WHERE code = 'v1'`))
	if err != nil {
		t.Fatal(err)
	}
	contact := e.SeedContact(t, "Ines Vogel", nil)
	store := collections.NewStore(e.DB())
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, listingPerms(true))

	list, err := store.CreateList(ctx, collections.CreateListInput{Name: "Visits", EntityType: historyDemoKind, ListType: "static"})
	if err != nil {
		t.Fatalf("create a list of the kind: %v", err)
	}
	if _, err := store.AddMember(ctx, list.ID, collections.MemberChange{
		EntityType: historyDemoKind, EntityID: visit, Reason: collections.ReasonChosen,
	}); err != nil {
		t.Fatalf("add a row of the kind: %v", err)
	}
	_, err = store.AddMember(ctx, list.ID, collections.MemberChange{
		EntityType: "contact", EntityID: contact, Reason: collections.ReasonChosen,
	})
	if bad, ok := errors.AsType[*collections.BadInputError](err); !ok || bad.Code != "entity_type_mismatch" {
		t.Errorf("adding a contact to a list of the kind = %v, want entity_type_mismatch", err)
	}
}

func TestALiveListOverAnExtensionKindFollowsItsFilter(t *testing.T) {
	e := integration.Setup(t)
	registerHistoryVisits(t)
	e.WsExec(t, `INSERT INTO ext.`+historyDemoTable+` (code, site, archived_at) VALUES
		('v1', 'Berlin site', NULL), ('v2', 'Berlin old', now()), ('v3', 'Munich site', NULL)`)
	store := collections.NewStore(e.DB())
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, listingPerms(true))

	list, err := store.CreateList(ctx, collections.CreateListInput{
		Name: "Berlin visits", EntityType: historyDemoKind, ListType: "dynamic",
		Definition: map[string]any{"field": "site", "op": "contains", "value": "berlin"},
	})
	if err != nil {
		t.Fatalf("create a Live List of the kind: %v", err)
	}
	members, _, err := store.ListMembers(ctx, list.ID, 50, "")
	if err != nil || len(members) != 1 {
		t.Fatalf("members = %d, err %v; want the one live Berlin visit", len(members), err)
	}
}

func TestASavedViewOverAnExtensionKindIsStoredAndAnUndeclaredKindRefused(t *testing.T) {
	e := integration.Setup(t)
	registerHistoryVisits(t)
	store := collections.NewStore(e.DB())
	ctx := e.As(e.Rep1, nil, listingPerms(true))
	query := map[string]any{
		"columns": []any{"code", "site"},
		"filter":  map[string]any{"field": "site", "op": "contains", "value": "berlin"},
	}

	created, err := store.CreateSavedView(ctx, collections.CreateSavedViewInput{Resource: historyDemoKind, Name: "Berlin visits", Query: query})
	if err != nil {
		t.Fatalf("create a view over the kind: %v", err)
	}
	got, err := store.GetSavedView(ctx, created.ID)
	if err != nil || got.Resource != historyDemoKind {
		t.Fatalf("read back = %+v, %v; want resource %s", got, err, historyDemoKind)
	}
	if _, err := store.CreateSavedView(ctx, collections.CreateSavedViewInput{Resource: "ext:nobody:ghost", Name: "x", Query: query}); err == nil {
		t.Error("a view over a kind no extension declared was stored")
	}
	badFilter := map[string]any{"filter": map[string]any{"field": "nope", "op": "eq", "value": "x"}}
	if _, err := store.CreateSavedView(ctx, collections.CreateSavedViewInput{Resource: historyDemoKind, Name: "x", Query: badFilter}); err == nil {
		t.Error("a view filtering on a field the kind does not declare was stored")
	}
}
