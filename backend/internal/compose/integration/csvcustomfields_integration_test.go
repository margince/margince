// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/blobstore"
)

// setupCustomFieldImportApp is setupImportApp with the schema pool a custom
// field's column needs.
func setupCustomFieldImportApp(t *testing.T) *apptest.AppEnv {
	t.Helper()
	e := apptest.SetupAppWithOptions(t, compose.WithBlobstore(blobstore.NewMemory()), compose.WithSchemaPool(SchemaPool(t)))
	e.BootstrapWorkspace(t)
	return e
}

// createCustomField defines a field and answers its cf_* key.
func createCustomField(t *testing.T, e *apptest.AppEnv, body AnyMap) string {
	t.Helper()
	var field struct {
		ColumnName string `json:"column_name"`
	}
	if status := e.Call(t, http.MethodPost, "/v1/custom-fields", body, nil, &field); status != http.StatusCreated {
		t.Fatalf("POST /v1/custom-fields → %d, want 201", status)
	}
	if field.ColumnName == "" {
		t.Fatal("the created field carries no column_name")
	}
	return field.ColumnName
}

// recordField reads one top-level field of the listed record whose nameKey is
// name, as text.
func recordField(t *testing.T, e *apptest.AppEnv, list, nameKey, name, column string) string {
	t.Helper()
	var page struct {
		Data []AnyMap `json:"data"`
	}
	if status := e.Call(t, http.MethodGet, list+"?limit=100", nil, nil, &page); status != http.StatusOK {
		t.Fatalf("GET %s → %d, want 200", list, status)
	}
	for _, record := range page.Data {
		if record[nameKey] == name {
			if value, ok := record[column]; ok && value != nil {
				return fmt.Sprint(value)
			}
			return ""
		}
	}
	t.Fatalf("no record in %s is named %q", list, name)
	return ""
}

func companyField(t *testing.T, e *apptest.AppEnv, displayName, column string) string {
	t.Helper()
	return recordField(t, e, "/v1/companies", "display_name", displayName, column)
}

func TestACustomFieldColumnLandsOnCreateAndOnUpdate(t *testing.T) {
	e := setupCustomFieldImportApp(t)
	column := createCustomField(t, e, AnyMap{"object": "company", "label": "Tier", "type": "picklist", "options": []string{"A", "B"}, "source": "manual"})
	profile, _ := uploadCSV(t, e, "company", "Name,Tier\nCustom One GmbH,A\n")
	if profile.SuggestedMapping["Tier"] != column {
		t.Errorf("suggested mapping = %v, want Tier onto %s by its label", profile.SuggestedMapping, column)
	}
	run, status := createRunWithMapping(t, e, "company", profile.SourceRef, map[string]string{"Name": "display_name", "Tier": column})
	if status != http.StatusAccepted {
		t.Fatalf("stage → %d, want 202", status)
	}
	approveAndRead(t, e, run.ID)
	if got := companyField(t, e, "Custom One GmbH", column); got != "A" {
		t.Fatalf("%s = %q after create, want A", column, got)
	}
	again, _ := uploadCSV(t, e, "company", "Name,Tier\nCustom One GmbH,B\n")
	second, _ := createRunWithMapping(t, e, "company", again.SourceRef, map[string]string{"Name": "display_name", "Tier": column})
	approveAndRead(t, e, second.ID)
	if got := companyField(t, e, "Custom One GmbH", column); got != "B" {
		t.Fatalf("%s = %q after update, want B", column, got)
	}
}

func TestAZipPartSuggestsAndLandsAContactCustomFieldByItsLabel(t *testing.T) {
	e := setupCustomFieldImportApp(t)
	column := createCustomField(t, e, AnyMap{"object": "contact", "label": "Segment", "type": "text", "source": "manual"})
	profile := uploadZip(t, e, "crm.zip", zipBytes(t, map[string]string{
		"contacts.csv": "ref,email,name,Segment\nC1,ada@segment.test,Ada Segment,Enterprise\n",
	}))
	if len(profile.Parts) != 1 || profile.Parts[0].SuggestedMappings["contact"]["Segment"] != column {
		t.Fatalf("parts = %+v, want Segment suggested onto %s", profile.Parts, column)
	}
	run := stageBundle(t, e, profile.SourceRef, "crm", []AnyMap{{"file": "contacts.csv", "object": "contact", "mapping": AnyMap{
		"ref": "ref", "email": "email", "name": "full_name", "Segment": column,
	}}})
	approveAndRead(t, e, run.ID)
	if got := recordField(t, e, "/v1/contacts", "full_name", "Ada Segment", column); got != "Enterprise" {
		t.Fatalf("%s = %q, want Enterprise", column, got)
	}
}

// A caller-opened create that sends values with no catalog is refused before
// it writes, so a value is never dropped silently.
func TestACallerOpenedCreateWithValuesAndNoCatalogIsStillRefused(t *testing.T) {
	f := setupGates(t)
	values := map[string]any{"cf_x": "y"}
	creates := map[string]func(tx pgx.Tx) error{
		"contact": func(tx pgx.Tx) error {
			_, err := f.contacts.CreateContactTx(f.granted, tx, contacts.CreateContactInput{FullName: "X", Source: "manual", CustomFields: values}, contacts.CustomColumns{})
			return err
		},
		"company": func(tx pgx.Tx) error {
			_, err := f.contacts.CreateCompanyTx(f.granted, tx, contacts.CreateCompanyInput{DisplayName: "X", Source: "manual", CustomFields: values}, contacts.CustomColumns{})
			return err
		},
		"lead": func(tx pgx.Tx) error {
			email := "x@refused.test"
			_, _, err := f.contacts.CreateLeadTx(f.granted, tx, contacts.CreateLeadInput{Email: &email, Status: "new", Source: "manual", CustomFields: values}, contacts.CustomColumns{})
			return err
		},
	}
	for table, create := range creates {
		probe := f.refusedInTx(f.granted, t, table, create)
		if !errors.Is(probe.err, contacts.ErrCustomFieldsNeedTheStoresOwnTransaction) || probe.rows != 0 {
			t.Errorf("%s: err = %v with %d rows, want the custom-field refusal before any write", table, probe.err, probe.rows)
		}
	}
	probe := f.refusedInTx(f.granted, t, "deal", func(tx pgx.Tx) error {
		_, err := f.deals.CreateDealTx(f.granted, tx, deals.CreateDealInput{
			Name: "X", PipelineID: f.pipeline, StageID: f.stage, Source: "manual", CustomFields: values,
		}, deals.CustomColumns{})
		return err
	})
	if !errors.Is(probe.err, deals.ErrCustomFieldsNeedTheStoresOwnTransaction) || probe.rows != 0 {
		t.Errorf("deal: err = %v with %d rows, want the custom-field refusal before any write", probe.err, probe.rows)
	}
}
