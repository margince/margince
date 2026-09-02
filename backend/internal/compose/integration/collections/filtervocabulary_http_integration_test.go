// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package collections

// The HTTP half of the segment-vocabulary proof. The store-level suite
// (segmentvocabulary_integration_test.go) proves the engine and the
// merge; this one proves compose's OWN wiring — the exact regression a
// review found: a cf_* filter that filtered export accepted was refused
// 422 by the stored-filter write path, because collections' catalog was
// wired into only one of its two handler stores. A reflection test
// (compose/collectionswiring_test.go) checks the one constructor both
// surfaces are now built through, so neither can lose the seam alone;
// this drives the endpoints over the real composed server and proves
// that wiring actually PRODUCES an accepted, evaluated filter rather
// than merely passing a structural check.

import (
	"net/http"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// TestAFilterOverHTTPAcceptsAndEvaluatesACustomFieldFilter is the point of
// the task: the same two endpoints a review found disagreeing (the filter
// write path refusing 422 what GET-via-export accepted) must now both accept
// a cf_* predicate AND evaluate it to the right rows, through the server
// compose actually assembles — no hand-built store.
func TestAFilterOverHTTPAcceptsAndEvaluatesACustomFieldFilter(t *testing.T) {
	e := apptest.SetupAppWithOptions(t, compose.WithSchemaPool(integration.SchemaPool(t)))
	e.BootstrapWorkspace(t)

	var field integration.AnyMap
	if status := e.Call(t, "POST", "/v1/custom-fields", integration.AnyMap{
		"object": "person", "label": "Loyalty Tier HTTP", "type": "text", "source": "ui",
	}, nil, &field); status != http.StatusCreated {
		t.Fatalf("create custom field: status=%d body=%v", status, field)
	}
	column, ok := field["column_name"].(string)
	if !ok || column == "" {
		t.Fatalf("created field carries no column_name: %v", field)
	}

	var matching integration.AnyMap
	if status := e.Call(t, "POST", "/v1/people", integration.AnyMap{
		"full_name": "Match", "source": "ui",
	}, nil, &matching); status != http.StatusCreated {
		t.Fatalf("create matching person: status=%d body=%v", status, matching)
	}
	matchID, ok := matching["id"].(string)
	if !ok || matchID == "" {
		t.Fatalf("created matching person carries no id: %v", matching)
	}

	var other integration.AnyMap
	if status := e.Call(t, "POST", "/v1/people", integration.AnyMap{
		"full_name": "Other", "source": "ui",
	}, nil, &other); status != http.StatusCreated {
		t.Fatalf("create non-matching person: status=%d body=%v", status, other)
	}

	// Set through the update path, exactly like the store-level scenario:
	// a value a customer fills in after the fact must filter the same way.
	var updated integration.AnyMap
	if status := e.Call(t, "PATCH", "/v1/people/"+matchID, integration.AnyMap{column: "gold"}, nil, &updated); status != http.StatusOK {
		t.Fatalf("set the custom field through the update path: status=%d body=%v", status, updated)
	}

	// The regression's own repro: this exact predicate used to answer 422 at
	// the endpoint filtered export already accepted the same tree on.
	var preview struct {
		MatchCount int                  `json:"match_count"`
		Rows       []integration.AnyMap `json:"rows"`
	}
	if status := e.Call(t, "POST", "/v1/filters/preview", integration.AnyMap{
		"resource": "person",
		"filter":   integration.AnyMap{"field": column, "op": "eq", "value": "gold"},
	}, nil, &preview); status != http.StatusOK {
		t.Fatalf("a filter on a custom field was refused over HTTP: status=%d body=%v", status, preview)
	}

	// It EVALUATES, not merely parses: the person whose value was set through
	// the update path is the one row it selects.
	if preview.MatchCount != 1 || len(preview.Rows) != 1 || preview.Rows[0]["id"] != matchID {
		t.Fatalf("preview = %d match(es) %v, want exactly [%s]", preview.MatchCount, preview.Rows, matchID)
	}
}

// The vocabulary READ (LVS-EXT-8), over the composed server, against the
// endpoint that consumes the same vocabulary.
//
// Two things only an HTTP test can prove here. First, that the operation is
// actually served: every contract operation gets a generated 501 stub in
// compose, and a module handler wins only by being embedded one level
// shallower — a mechanism the build cannot fail on, because an unimplemented
// operation compiles perfectly and answers 501 at runtime.
//
// Second, the equivalence the extension is defined by: a field this operation
// lists must be one a filter may name. Asserting that against the preview
// endpoint rather than against the engine's map closes the loop the
// store-level suite cannot — the two surfaces resolving the same vocabulary
// is exactly what the regression above proved is not automatic.
func TestTheFilterVocabularyOverHTTPOffersWhatAFilterAccepts(t *testing.T) {
	e := apptest.SetupAppWithOptions(t, compose.WithSchemaPool(integration.SchemaPool(t)))
	e.BootstrapWorkspace(t)

	var field integration.AnyMap
	if status := e.Call(t, "POST", "/v1/custom-fields", integration.AnyMap{
		"object": "person", "label": "Vocabulary Probe", "type": "picklist", "source": "ui",
		"options": []string{"gold", "silver"},
	}, nil, &field); status != http.StatusCreated {
		t.Fatalf("create custom field: status=%d body=%v", status, field)
	}
	column, ok := field["column_name"].(string)
	if !ok || column == "" {
		t.Fatalf("created field carries no column_name: %v", field)
	}

	var vocab struct {
		Resource string `json:"resource"`
		Fields   []struct {
			Name      string   `json:"name"`
			Type      string   `json:"type"`
			Operators []string `json:"operators"`
			Custom    bool     `json:"custom"`
			// A POINTER, so this test can tell an absent key from an empty
			// string. A plain string would decode both as "" and the
			// omitted-for-a-non-id-field guarantee would be unassertable.
			References *string `json:"references,omitempty"`
			// A pointer for the same reason: an absent key and an empty array are
			// different answers, and only one of them is right for a field with no
			// closed set.
			Options *[]string `json:"options,omitempty"`
		} `json:"fields"`
	}
	status := e.Call(t, "GET", "/v1/filters/vocabulary?resource=person", nil, nil, &vocab)
	if status == http.StatusNotImplemented {
		t.Fatal("the operation answered 501: the generated stub is being served, so the module handler is not shadowing it")
	}
	if status != http.StatusOK {
		t.Fatalf("read the filter vocabulary: status=%d body=%v", status, vocab)
	}
	if vocab.Resource != "person" {
		t.Errorf("resource = %q, want the one asked for", vocab.Resource)
	}

	reported := map[string]bool{}
	for _, f := range vocab.Fields {
		reported[f.Name] = true
		switch f.Name {
		case column:
			if !f.Custom {
				t.Errorf("%s is a workspace-defined column and is not reported custom", column)
			}
			if f.Type != "picklist" {
				t.Errorf("%s type = %q, want the picklist the admin created", column, f.Type)
			}
			// The values the admin authored, over the wire and through the real
			// catalogue write — the one path that proves the jsonb column, the
			// decode and the response mapping agree. A picklist that arrived
			// without them is the whole defect: the builder falls back to a free
			// text box over a closed set.
			switch {
			case f.Options == nil:
				t.Errorf("%s is a picklist and the response carries no options key, so a builder can only ask a reader to type a value", column)
			case !slices.Equal(*f.Options, []string{"gold", "silver"}):
				t.Errorf("%s offers %v, want the two values the admin created", column, *f.Options)
			}
		case "owner_id":
			if f.Custom {
				t.Error("owner_id is a core field and is reported custom")
			}
			// The wire value, which no unit test can reach: an id field names
			// the record type its values point at, in the contract's own
			// record-type word.
			if f.References == nil {
				t.Error("owner_id is an id field and the response carries no references key, so a builder can only ask for a uuid")
			} else if *f.References != "app_user" {
				t.Errorf("owner_id references %q, want app_user", *f.References)
			}
		}
		// And the other arm, for every non-id field in the same response: the key
		// is ABSENT rather than blank. "" is not a member of the contract's enum,
		// so a strict client rejects a response that sends it.
		if f.Type != "id" && f.References != nil {
			t.Errorf("%s is typed %s and the response carries references=%q; the key belongs only to an id field",
				f.Name, f.Type, *f.References)
		}
		// Same shape for the closed set: only a picklist has one, so any other
		// type carrying an options key promises a picker for a free-text column.
		if f.Type != "picklist" && f.Options != nil {
			t.Errorf("%s is typed %s and the response carries options=%v; only a picklist has a closed set",
				f.Name, f.Type, *f.Options)
		}
		if len(f.Operators) == 0 {
			t.Errorf("%s reports no operators, so a builder could offer no clause on it", f.Name)
		}
	}
	if !reported[column] {
		t.Fatalf("the vocabulary omits %s, a column a filter may name", column)
	}
	if !reported["owner_id"] {
		t.Error("the vocabulary omits owner_id, a core field every person filter may name")
	}

	// The equivalence, forwards: a listed field is one a filter accepts.
	var accepted integration.AnyMap
	if status := e.Call(t, "POST", "/v1/filters/preview", integration.AnyMap{
		"resource": "person",
		"filter":   integration.AnyMap{"field": column, "op": "eq", "value": "gold"},
	}, nil, &accepted); status != http.StatusOK {
		t.Fatalf("the vocabulary listed %s but a filter on it was refused: status=%d body=%v", column, status, accepted)
	}

	// And backwards: a field it does not list is one the same endpoint refuses,
	// so the omission is a real answer rather than an incomplete one.
	var refused integration.AnyMap
	unlisted := column + "_not_in_the_catalog"
	if reported[unlisted] {
		t.Fatalf("%s was meant to be absent from the vocabulary", unlisted)
	}
	if status := e.Call(t, "POST", "/v1/filters/preview", integration.AnyMap{
		"resource": "person",
		"filter":   integration.AnyMap{"field": unlisted, "op": "eq", "value": "gold"},
	}, nil, &refused); status != http.StatusUnprocessableEntity {
		t.Fatalf("the vocabulary omits %s but a filter on it was not refused 422: status=%d body=%v", unlisted, status, refused)
	}

	// A resource the enum does not admit is a 422 naming the parameter, not a
	// bare 404: the generated wrapper binds this as a plain string and never
	// calls the Valid() it also generates, so the handler is the only thing that
	// can tell a typo from a real absence.
	var badResource integration.AnyMap
	if status := e.Call(t, "GET", "/v1/filters/vocabulary?resource=peron", nil, nil, &badResource); status != http.StatusUnprocessableEntity {
		t.Fatalf("a misspelled resource: status=%d body=%v, want 422", status, badResource)
	}
}

// The gap between what a filter may SAY and what a builder may OFFER, end to end
// through the real retire operation.
//
// This is the half I had backwards before review. CUSTOM-FIELDS-AC-13 makes
// retire "hidden from API + filtering", and AC-14 names the admin catalog read as
// the one surface that still shows retired fields — so a vocabulary read that
// listed them would put a field in a picker that an admin retired precisely to get
// it out of there. Dropping it from the ENGINE would be the opposite mistake: every
// saved segment naming it becomes a read-time error. Both are asserted here,
// against a field retired through the operation that really retires it.
func TestARetiredCustomFieldLeavesTheVocabularyAndKeepsEvaluating(t *testing.T) {
	e := apptest.SetupAppWithOptions(t, compose.WithSchemaPool(integration.SchemaPool(t)))
	e.BootstrapWorkspace(t)

	var field integration.AnyMap
	if status := e.Call(t, "POST", "/v1/custom-fields", integration.AnyMap{
		"object": "person", "label": "Retiring Tier", "type": "text", "source": "ui",
	}, nil, &field); status != http.StatusCreated {
		t.Fatalf("create custom field: status=%d body=%v", status, field)
	}
	column, _ := field["column_name"].(string)
	fieldID, _ := field["id"].(string)
	if column == "" || fieldID == "" {
		t.Fatalf("created field carries no column_name/id: %v", field)
	}

	// A saved view built while the field is live — the one that has to keep
	// working after the field leaves the vocabulary.
	var view integration.AnyMap
	if status := e.Call(t, "POST", "/v1/views", integration.AnyMap{
		"resource": "people", "name": "Built before retirement",
		"query": integration.AnyMap{
			"filter": integration.AnyMap{"field": column, "op": "eq", "value": "gold"},
		},
	}, nil, &view); status != http.StatusCreated {
		t.Fatalf("create the view while the field is live: status=%d body=%v", status, view)
	}
	viewID, _ := view["id"].(string)

	if offered := vocabularyOffers(t, e, column); !offered {
		t.Fatalf("%s is active and the vocabulary does not offer it", column)
	}

	var retired integration.AnyMap
	if status := e.Call(t, "POST", "/v1/custom-fields/"+fieldID+"/retire", integration.AnyMap{}, nil, &retired); status != http.StatusOK {
		t.Fatalf("retire the field: status=%d body=%v", status, retired)
	}

	if offered := vocabularyOffers(t, e, column); offered {
		t.Errorf("%s was retired and the vocabulary still offers it for a new clause", column)
	}

	// And the stored filter written against it still evaluates rather than
	// erroring: the export path resolves the view's predicate through the same
	// engine a live clause runs on.
	// json, not csv: this asserts the predicate still RESOLVES, and a decoder
	// pointed at a CSV body fails on the header row for a reason that has
	// nothing to do with the filter.
	var exported struct {
		Object   string `json:"object"`
		RowCount int    `json:"row_count"`
	}
	if status := e.Call(t, "POST", "/v1/exports", integration.AnyMap{
		"view_id": viewID, "format": "json",
	}, nil, &exported); status != http.StatusOK {
		t.Errorf("the stored filter naming the retired %s no longer evaluates: status=%d", column, status)
	}
	// It resolved rather than erroring, and it resolved to the RIGHT object:
	// a filter that silently evaluated against the wrong resource would also
	// answer 200.
	if exported.Object != "person" {
		t.Errorf("the stored filter exported %q, want person", exported.Object)
	}
}

// vocabularyOffers answers whether the filter vocabulary currently lists a field.
func vocabularyOffers(t *testing.T, e *apptest.AppEnv, name string) bool {
	t.Helper()
	var vocab struct {
		Fields []struct {
			Name string `json:"name"`
		} `json:"fields"`
	}
	if status := e.Call(t, "GET", "/v1/filters/vocabulary?resource=person", nil, nil, &vocab); status != http.StatusOK {
		t.Fatalf("read the filter vocabulary: status=%d", status)
	}
	for _, f := range vocab.Fields {
		if f.Name == name {
			return true
		}
	}
	return false
}
