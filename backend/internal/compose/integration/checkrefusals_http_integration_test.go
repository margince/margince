// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Deals answer only their own picklist CHECKs. Every other CHECK reaches
// httperr's constraint net, which tells an overlong custom field and a fault in
// a row the caller never wrote apart.

import (
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

type checkRefusal struct {
	Code    string `json:"code"`
	Detail  string `json:"detail"`
	Details struct {
		Errors []struct {
			Field string `json:"field"`
			Code  string `json:"code"`
			Max   int    `json:"max"`
		} `json:"errors"`
	} `json:"details"`
}

func (r checkRefusal) names(field, code string) bool {
	return len(r.Details.Errors) == 1 && r.Details.Errors[0].Field == field && r.Details.Errors[0].Code == code
}

func TestADealPicklistValueOutsideItsOptionsNamesTheField(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	deal := apptest.CreateOpenDeal(t, e, apptest.DiscoverSeededPipeline(t, e))

	var refused checkRefusal
	if status := e.Call(t, "PATCH", "/v1/deals/"+deal, AnyMap{"priority": "bogus"}, nil, &refused); status != http.StatusUnprocessableEntity {
		t.Fatalf("a bogus priority = %d %+v, want 422", status, refused)
	}
	if !refused.names("priority", "value_not_allowed") || !strings.Contains(refused.Detail, "check the picklist options") {
		t.Errorf("a bogus priority answered %+v, want value_not_allowed on priority with the picklist sentence", refused)
	}
}

func TestAnOverlongDealCustomTextFieldSaysWhichFieldIsTooLong(t *testing.T) {
	e := apptest.SetupAppWithOptions(t, compose.WithSchemaPool(SchemaPool(t)))
	e.BootstrapWorkspace(t)
	var field struct {
		ColumnName string `json:"column_name"`
	}
	if status := e.Call(t, "POST", "/v1/custom-fields", AnyMap{
		"object": "deal", "label": "Briefing", "type": "text", "source": "manual",
	}, nil, &field); status != http.StatusCreated {
		t.Fatalf("creating the deal text field = %d", status)
	}
	deal := apptest.CreateOpenDeal(t, e, apptest.DiscoverSeededPipeline(t, e))

	var refused checkRefusal
	overlong := strings.Repeat("a", values.TextDocumentCharacters+1)
	if status := e.Call(t, "PATCH", "/v1/deals/"+deal, AnyMap{field.ColumnName: overlong}, nil, &refused); status != http.StatusUnprocessableEntity {
		t.Fatalf("an overlong %s = %d %+v, want 422", field.ColumnName, status, refused)
	}
	if !refused.names(field.ColumnName, "too_long") || refused.Details.Errors[0].Max != values.TextDocumentCharacters {
		t.Errorf("an overlong %s answered %+v, want too_long on that field with max %d",
			field.ColumnName, refused, values.TextDocumentCharacters)
	}
}

// A CHECK on the outbox row is a fault in our write, never the caller's value.
func TestACheckOnARowTheCallerNeverWroteIsAServerFault(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	deal := apptest.CreateOpenDeal(t, e, apptest.DiscoverSeededPipeline(t, e))
	if _, err := e.Owner.Exec(t.Context(),
		`ALTER TABLE event_outbox ADD CONSTRAINT event_outbox_refuses_everything CHECK (false) NOT VALID`); err != nil {
		t.Fatalf("refusing every outbox row: %v", err)
	}
	var refused checkRefusal
	if status := e.Call(t, "PATCH", "/v1/deals/"+deal, AnyMap{"name": "Renamed"}, nil, &refused); status != http.StatusInternalServerError {
		t.Errorf("a write whose outbox row the schema refuses = %d %+v, want 500", status, refused)
	}
}
