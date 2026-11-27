// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Deals answer only their own picklist CHECKs. Every other CHECK reaches
// httperr's constraint net, which tells an overlong custom field and a fault in
// a row the caller never wrote apart.

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
	t.Parallel()
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
	t.Parallel()
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
//
// The package's tests share one database, so the CHECK refuses only events that
// name this test's deal, and it is dropped when the test ends.
func TestACheckOnARowTheCallerNeverWroteIsAServerFault(t *testing.T) {
	t.Parallel()
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	deal := ids.MustParse(apptest.CreateOpenDeal(t, e, apptest.DiscoverSeededPipeline(t, e)))
	refuses := pgx.Identifier{"event_outbox_refuses_" + strings.ReplaceAll(deal.String(), "-", "")}.Sanitize()
	if _, err := e.Owner.Exec(t.Context(), `ALTER TABLE event_outbox ADD CONSTRAINT `+refuses+
		` CHECK (strpos(envelope::text, '`+deal.String()+`') = 0) NOT VALID`); err != nil {
		t.Fatalf("refusing the deal's outbox rows: %v", err)
	}
	t.Cleanup(func() {
		if _, err := e.Owner.Exec(context.Background(), `ALTER TABLE event_outbox DROP CONSTRAINT `+refuses); err != nil {
			t.Errorf("dropping the outbox refusal: %v", err)
		}
	})
	var refused checkRefusal
	if status := e.Call(t, "PATCH", "/v1/deals/"+deal.String(), AnyMap{"name": "Renamed"}, nil, &refused); status != http.StatusInternalServerError {
		t.Errorf("a write whose outbox row the schema refuses = %d %+v, want 500", status, refused)
	}
}
