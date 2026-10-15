// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// importedFromOf reads one record over the wire and answers imported_from, or
// nil when the field is null.
func importedFromOf(t *testing.T, e *apptest.AppEnv, path string) *string {
	t.Helper()
	var record struct {
		ImportedFrom *string `json:"imported_from"`
	}
	if status := e.Call(t, http.MethodGet, path, nil, nil, &record); status != http.StatusOK {
		t.Fatalf("GET %s → %d, want 200", path, status)
	}
	return record.ImportedFrom
}

func idByQuery(t *testing.T, e *apptest.AppEnv, query string, args ...any) string {
	t.Helper()
	var id string
	if err := e.Owner.QueryRow(context.Background(), query, args...).Scan(&id); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return id
}

func TestEveryRecordKindAnImportWritesReadsAsImportedOnTheWire(t *testing.T) {
	e := setupImportApp(t)
	pipeline, open, _ := seededStageNames(t, e)
	parts := append(bundleParts(), dealImportParts("ref,name,pipeline,stage,company_ref")...)
	parts = append(parts, activityImportParts()...)
	importActivities(t, e, map[string]string{
		"companies.csv": "ref,name\nCO-1,Wire Origin GmbH\n",
		"contacts.csv":  "ref,email,name\nP-1,jo@wire-origin.example,Jo Origin\n",
		"deals.csv":     fmt.Sprintf("ref,name,pipeline,stage,company_ref\nD-1,Origin deal,%s,%s,CO-1\n", pipeline, open),
		"activities.csv": activityHeader + "\n" +
			"A-1,note,Origin note,Met,2025-03-02,contact:P-1,,,\n",
	}, parts)

	paths := map[string]string{
		"company":  "/v1/companies/" + idByQuery(t, e, `SELECT id::text FROM company WHERE display_name = 'Wire Origin GmbH'`),
		"contact":  "/v1/contacts/" + idByQuery(t, e, `SELECT id::text FROM contact WHERE full_name = 'Jo Origin'`),
		"deal":     "/v1/deals/" + idByQuery(t, e, `SELECT id::text FROM deal WHERE name = 'Origin deal'`),
		"activity": "/v1/activities/" + idByQuery(t, e, `SELECT id::text FROM activity WHERE subject = 'Origin note'`),
	}
	for kind, path := range paths {
		got := importedFromOf(t, e, path)
		if got == nil || *got != "crm" {
			t.Errorf("%s imported_from = %v, want the source name crm", kind, got)
		}
	}

	typed := createdID(t, e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Typed Tess"})
	if got := importedFromOf(t, e, "/v1/contacts/"+typed); got != nil {
		t.Errorf("a typed contact reads imported_from = %q, want null", *got)
	}
}

func TestAnImportedLeadReadsAsImportedOnTheWire(t *testing.T) {
	e := setupImportApp(t)
	importAndApprove(t, e, "lead", "Email,Name\norigin@lead-wire.example,Lead Origin\n",
		map[string]string{"Email": "email", "Name": "full_name"})
	id := idByQuery(t, e, `SELECT id::text FROM lead WHERE full_name = 'Lead Origin'`)
	if got := importedFromOf(t, e, "/v1/leads/"+id); got == nil {
		t.Fatal("an imported lead reads imported_from = null, want the import's source")
	}
}
