// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package customfields

// Removing a choice from a picklist or multiselect field clears it from the
// records that hold it, in the options edit itself. The admin reads how many
// records hold each choice before confirming, and the edit answers how many it
// changed. Records of other fields and other objects that hold the same text
// are not touched, and a Live List whose filter names the choice keeps working.

import (
	"maps"
	"net/http"
	"reflect"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	customfieldsmod "github.com/margince/margince/backend/internal/modules/customfields"
)

type choiceUseWire struct {
	Choices []struct {
		Choice  string `json:"choice"`
		Records int    `json:"records"`
	} `json:"choices"`
}

type optionsEditWire struct {
	Options        []string `json:"options"`
	RecordsChanged *int     `json:"records_changed"`
}

func choiceUse(t *testing.T, e *apptest.AppEnv, fieldID string) map[string]int {
	t.Helper()
	var use choiceUseWire
	if status := e.Call(t, "GET", "/v1/custom-fields/"+fieldID+"/choices", nil, nil, &use); status != http.StatusOK {
		t.Fatalf("read choice use → %d", status)
	}
	counts := map[string]int{}
	for _, c := range use.Choices {
		counts[c.Choice] = c.Records
	}
	return counts
}

func editOptions(t *testing.T, e *apptest.AppEnv, fieldID string, options []string) optionsEditWire {
	t.Helper()
	var out optionsEditWire
	if status := e.Call(t, "PATCH", "/v1/custom-fields/"+fieldID+"/options", integration.AnyMap{"options": options}, nil, &out); status != http.StatusOK {
		t.Fatalf("edit options → %d (%+v)", status, out)
	}
	if out.RecordsChanged == nil {
		t.Fatal("the options edit answer carries no records_changed")
	}
	return out
}

func newField(t *testing.T, e *apptest.AppEnv, body integration.AnyMap) customFieldWire {
	t.Helper()
	status, field, problem := createCustomField(t, e, body)
	if status != http.StatusCreated {
		t.Fatalf("create field %v → %d %+v", body["label"], status, problem)
	}
	return field
}

func readRecord(t *testing.T, e *apptest.AppEnv, path string) integration.AnyMap {
	t.Helper()
	var got integration.AnyMap
	if status := e.Call(t, "GET", path, nil, nil, &got); status != http.StatusOK {
		t.Fatalf("GET %s → %d", path, status)
	}
	return got
}

func TestEveryFieldObjectHasAChoiceRemover(t *testing.T) {
	wired := slices.Sorted(maps.Keys(compose.CustomFieldChoiceRemovers()))
	objects := slices.Sorted(slices.Values(customfieldsmod.FieldObjects))
	if !slices.Equal(wired, objects) {
		t.Fatalf("choice removers are wired for %v, custom fields attach to %v; "+
			"an object without one refuses every options edit its records hold a removed choice of", wired, objects)
	}
}

func TestRemovingAPicklistChoiceClearsItFromTheDealsThatHoldIt(t *testing.T) {
	e := apptest.SetupAppWithOptions(t, compose.WithSchemaPool(integration.SchemaPool(t)), compose.WithListsEnabled(true))
	e.BootstrapWorkspace(t)
	region := newField(t, e, integration.AnyMap{"object": "deal", "label": "Sales region", "type": "picklist", "options": []string{"North", "West", "South"}, "source": "manual"})
	twin := newField(t, e, integration.AnyMap{"object": "deal", "label": "Delivery region", "type": "picklist", "options": []string{"North", "West"}, "source": "manual"})
	home := newField(t, e, integration.AnyMap{"object": "company", "label": "Home region", "type": "picklist", "options": []string{"North"}, "source": "manual"})
	col := region.ColumnName

	stages := apptest.DiscoverSeededPipeline(t, e)
	deal := func(name, value string) string {
		_, id := createWithCF(t, e, "/v1/deals", integration.AnyMap{
			"name": name, "pipeline_id": stages.PipelineID, "stage_id": stages.Open, "source": "manual",
			col: value, twin.ColumnName: "North",
		})
		return id
	}
	north, west, archived := deal("Northern renewal", "North"), deal("Western renewal", "West"), deal("Old northern deal", "North")
	if status := e.Call(t, "DELETE", "/v1/deals/"+archived, nil, nil, nil); status != http.StatusNoContent && status != http.StatusOK {
		t.Fatalf("archive deal → %d", status)
	}
	_, company := createWithCF(t, e, "/v1/companies", integration.AnyMap{"display_name": "Northern Holdings", "source": "manual", home.ColumnName: "North"})

	var list listWire
	if status := e.Call(t, "POST", "/v1/lists", integration.AnyMap{
		"name": "Northern deals", "entity_type": "deal", "list_type": "dynamic",
		"definition": integration.AnyMap{"field": col, "op": "eq", "value": "North"},
	}, nil, &list); status != http.StatusCreated {
		t.Fatalf("create list → %d", status)
	}

	if got := choiceUse(t, e, region.ID); !reflect.DeepEqual(got, map[string]int{"North": 2, "West": 1, "South": 0}) {
		t.Fatalf("choice use before the edit = %v, want North 2 (one archived), West 1, South 0", got)
	}
	edited := editOptions(t, e, region.ID, []string{"West", "South"})
	if *edited.RecordsChanged != 2 {
		t.Fatalf("records_changed = %d, want the two deals that held North", *edited.RecordsChanged)
	}

	if got := readRecord(t, e, "/v1/deals/"+north); got[col] != nil || got[twin.ColumnName] != "North" {
		t.Fatalf("the northern deal reads %v / %v, want its region cleared and the other field's North kept", got[col], got[twin.ColumnName])
	}
	if got := readRecord(t, e, "/v1/deals/"+west); got[col] != "West" {
		t.Fatalf("the western deal reads %v, want West kept", got[col])
	}
	if got := readRecord(t, e, "/v1/companies/"+company); got[home.ColumnName] != "North" {
		t.Fatalf("the company reads %v, want another object's North kept", got[home.ColumnName])
	}
	var archivedValue *string
	if err := e.Owner.QueryRow(t.Context(), `SELECT `+col+` FROM deal WHERE id = $1`, archived).Scan(&archivedValue); err != nil || archivedValue != nil {
		t.Fatalf("the archived deal holds %v (%v), want it cleared too", archivedValue, err)
	}

	var audited, announced int
	if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM audit_log
		WHERE entity_type = 'deal' AND action = 'update' AND before->>$1 = 'North' AND after ? $1 AND after->>$1 IS NULL`, col).Scan(&audited); err != nil {
		t.Fatal(err)
	}
	if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM event_outbox
		WHERE envelope->>'type' = 'deal.updated' AND envelope->'payload'->'changed_fields' ? $1`, col).Scan(&announced); err != nil {
		t.Fatal(err)
	}
	if audited != 2 || announced != 2 {
		t.Fatalf("audited %d and announced %d cleared deals, want each of the two", audited, announced)
	}

	var after listWire
	if status := e.Call(t, "GET", "/v1/lists/"+list.ID, nil, nil, &after); status != http.StatusOK {
		t.Fatalf("read the list naming the removed choice → %d", status)
	}
	if after.VisibleCount == nil || *after.VisibleCount != 0 {
		t.Fatalf("the list counts %v, want it still evaluating and matching no deal", after.VisibleCount)
	}
}

func TestRemovingAMultiselectChoiceKeepsTheOtherChoices(t *testing.T) {
	e := schemaWiredEnv(t)
	field := newField(t, e, integration.AnyMap{"object": "company", "label": "Services", "type": "multiselect", "options": []string{"Audit", "Build", "Care"}, "source": "manual"})
	col := field.ColumnName
	_, both := createWithCF(t, e, "/v1/companies", integration.AnyMap{"display_name": "Two Services", "source": "manual", col: []string{"Audit", "Build"}})
	_, only := createWithCF(t, e, "/v1/companies", integration.AnyMap{"display_name": "Only Audit", "source": "manual", col: []string{"Audit"}})
	_, other := createWithCF(t, e, "/v1/companies", integration.AnyMap{"display_name": "Only Care", "source": "manual", col: []string{"Care"}})

	if got := choiceUse(t, e, field.ID); !reflect.DeepEqual(got, map[string]int{"Audit": 2, "Build": 1, "Care": 1}) {
		t.Fatalf("choice use = %v, want Audit 2, Build 1, Care 1", got)
	}
	if edited := editOptions(t, e, field.ID, []string{"Build", "Care"}); *edited.RecordsChanged != 2 {
		t.Fatalf("records_changed = %d, want the two companies that held Audit", *edited.RecordsChanged)
	}
	if got := readRecord(t, e, "/v1/companies/"+both)[col]; !reflect.DeepEqual(got, []any{"Build"}) {
		t.Fatalf("the company with two services reads %v, want [Build]", got)
	}
	if got := readRecord(t, e, "/v1/companies/"+only)[col]; got != nil {
		t.Fatalf("the company whose only service was removed reads %v, want no value", got)
	}
	if got := readRecord(t, e, "/v1/companies/"+other)[col]; !reflect.DeepEqual(got, []any{"Care"}) {
		t.Fatalf("the company without Audit reads %v, want [Care] untouched", got)
	}
}
