// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
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
	column := apptest.CreateCustomField(t, e, AnyMap{"object": "company", "label": "Tier", "type": "picklist", "options": []string{"A", "B"}, "source": "manual"})
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
	column := apptest.CreateCustomField(t, e, AnyMap{"object": "contact", "label": "Segment", "type": "text", "source": "manual"})
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

func TestTheDryRunAndTheCommitSkipTheSameRowForTheSameCell(t *testing.T) {
	e := setupCustomFieldImportApp(t)
	column := apptest.CreateCustomField(t, e, AnyMap{"object": "company", "label": "Tier", "type": "picklist", "options": []string{"Gold", "Silver"}, "source": "manual"})
	profile, _ := uploadCSV(t, e, "company", "Name,Tier\nGood GmbH,Gold\nBad GmbH,Bronze\n")
	run, status := createRunWithMapping(t, e, "company", profile.SourceRef, map[string]string{"Name": "display_name", "Tier": column})
	if status != http.StatusAccepted {
		t.Fatalf("stage → %d, want 202", status)
	}
	assertOneBronzeIssue := func(stage string, report importReportDTO) {
		t.Helper()
		issue := issueOf(report, "invalid_value", "Bronze")
		if len(report.Issues) != 1 || issue == nil || issue.Column != "Tier" || report.Disposition.Created != 1 {
			t.Fatalf("%s report = %+v, want one created and one invalid_value issue on the Tier column", stage, report)
		}
	}
	assertOneBronzeIssue("dry-run", readImportReport(t, e, run.ID))
	approveAndRead(t, e, run.ID)
	assertOneBronzeIssue("finished", readImportReport(t, e, run.ID))
	if got := companyField(t, e, "Good GmbH", column); got != "Gold" {
		t.Fatalf("%s = %q, want Gold", column, got)
	}
	var page struct {
		Data []AnyMap `json:"data"`
	}
	if status := e.Call(t, http.MethodGet, "/v1/companies?limit=100", nil, nil, &page); status != http.StatusOK {
		t.Fatalf("GET /v1/companies → %d", status)
	}
	for _, company := range page.Data {
		if company["display_name"] == "Bad GmbH" {
			t.Fatal("Bad GmbH exists, but its Tier was refused")
		}
	}
}

// A typed value lands in its type, and an unedited re-import changes nothing.
func TestATypedCustomFieldLandsInItsTypeAndAReimportIsUnchanged(t *testing.T) {
	e := setupCustomFieldImportApp(t)
	active := apptest.CreateCustomField(t, e, AnyMap{"object": "company", "label": "Active", "type": "boolean", "source": "manual"})
	value := apptest.CreateCustomField(t, e, AnyMap{"object": "company", "label": "Value", "type": "currency", "currency": "EUR", "source": "manual"})
	body := "Name,Active,Value\nTyped GmbH,Yes,12000.50\n"
	mapping := map[string]string{"Name": "display_name", "Active": active, "Value": value}
	profile, _ := uploadCSV(t, e, "company", body)
	run, _ := createRunWithMapping(t, e, "company", profile.SourceRef, mapping)
	approveAndRead(t, e, run.ID)
	if got := companyField(t, e, "Typed GmbH", active); got != "true" {
		t.Fatalf("%s = %q, want true", active, got)
	}
	if got, err := strconv.ParseFloat(companyField(t, e, "Typed GmbH", value), 64); err != nil || got != 1200050 {
		t.Fatalf("%s = %v (%v), want 1200050 minor units", value, got, err)
	}
	again, _ := uploadCSV(t, e, "company", body)
	second, _ := createRunWithMapping(t, e, "company", again.SourceRef, mapping)
	approveAndRead(t, e, second.ID)
	if report := readImportReport(t, e, second.ID); report.Disposition.Unchanged != 1 {
		t.Fatalf("re-import report = %+v, want the row unchanged", report)
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
