// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// jsonExportColumn reads one column of a JSON export's rows. A null cell reads
// as empty, as an empty CSV cell does.
func jsonExportColumn(t *testing.T, raw []byte, column string) []string {
	t.Helper()
	var doc struct {
		Rows []map[string]json.RawMessage `json:"rows"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing the json export: %v", err)
	}
	out := make([]string, 0, len(doc.Rows))
	for _, row := range doc.Rows {
		cell, ok := row[column]
		if !ok {
			t.Fatalf("a json export row has no %q column: %v", column, row)
		}
		var text string
		if string(cell) != "null" {
			if err := json.Unmarshal(cell, &text); err != nil {
				t.Fatalf("column %q holds %s, want text", column, cell)
			}
		}
		out = append(out, text)
	}
	return out
}

// The CSV export is the file the import reads back, so the linked company's
// current name travels in the JSON export only.
//
// A lead export keeps the company name typed on the lead, which an import
// writes back. Beside it, it names the linked company as it is called now.
func TestALeadExportKeepsTheTypedNameAndAddsTheLinkedCompanysCurrentOne(t *testing.T) {
	e := SetupSearch(t)
	company := e.SeedID(t, `INSERT INTO company (id, display_name, source, captured_by)
		VALUES ($1, 'Falkenberg Maschinenbau', 'manual', 'human:x')`)
	linked := e.SeedID(t, `INSERT INTO lead (id, full_name, company_name, company_id, status, source, captured_by)
		VALUES ($1, 'Lena Falk', 'Falkenberg', $2, 'new', 'manual', 'human:x')`, company)
	typedOnly := e.SeedID(t, `INSERT INTO lead (id, full_name, company_name, status, source, captured_by)
		VALUES ($1, 'No Company', 'Typed Only', 'new', 'manual', 'human:x')`)
	// The company is renamed after the lead typed its name.
	if _, err := e.Owner.Exec(context.Background(),
		`UPDATE company SET display_name = 'Falkenberg Antriebe GmbH' WHERE id = $1`, company); err != nil {
		t.Fatalf("renaming the company: %v", err)
	}

	ctx := e.exportAdmin()
	engine, ok, err := compose.NewCollectionsStore(e.Pool).SegmentEngine(ctx, "lead")
	if err != nil || !ok {
		t.Fatalf("resolving the lead engine: ok=%v %v", ok, err)
	}
	result, err := compose.NewFilteredExportWriter(e.Pool).WriteFiltered(
		ctx, engine, storekit.Predicate{Field: "status", Op: "eq", Value: "new"}, "json",
	)
	if err != nil {
		t.Fatalf("filtered export: %v", err)
	}
	exported, typed, current := jsonExportColumn(t, result.Body, "id"), jsonExportColumn(t, result.Body, "company_name"), jsonExportColumn(t, result.Body, "linked_company_name")
	got := map[string][2]string{}
	for i, id := range exported {
		got[id] = [2]string{typed[i], current[i]}
	}
	if want := [2]string{"Falkenberg", "Falkenberg Antriebe GmbH"}; got[linked.String()] != want {
		t.Errorf("the linked lead exports %v, want the typed name and the company's current one %v", got[linked.String()], want)
	}
	if want := [2]string{"Typed Only", ""}; got[typedOnly.String()] != want {
		t.Errorf("the lead with no linked company exports %v, want %v", got[typedOnly.String()], want)
	}
}

// The current name of a company the exporter may not open leaves with its id.
func TestALeadExportWithholdsTheNameOfACompanyTheExporterCannotOpen(t *testing.T) {
	e := SetupSearch(t)
	hidden := e.SeedID(t, `INSERT INTO company (id, display_name, owner_id, visibility, source, captured_by)
		VALUES ($1, 'Meridian Labs', $2, 'owner', 'manual', 'human:x')`, e.Rep3)
	lead := e.SeedID(t, `INSERT INTO lead (id, owner_id, full_name, company_name, company_id, status, source, captured_by)
		VALUES ($1, $2, 'Mara Meridian', 'Meridian', $3, 'new', 'manual', 'human:x')`, e.Rep1, hidden)

	reader := principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1,
		TeamIDs: []ids.UUID{e.Team1},
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"lead": {Read: true}, "company": {Read: true}, objInstallSettings: {Read: true},
			},
			RowScope: principal.RowScopeTeam,
		},
	}
	ctx := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.WS), reader)
	engine, ok, err := compose.NewCollectionsStore(e.Pool).SegmentEngine(ctx, "lead")
	if err != nil || !ok {
		t.Fatalf("resolving the lead engine: ok=%v %v", ok, err)
	}
	result, err := compose.NewFilteredExportWriter(e.Pool).WriteFiltered(
		ctx, engine, storekit.Predicate{Field: "status", Op: "eq", Value: "new"}, "json",
	)
	if err != nil {
		t.Fatalf("filtered export: %v", err)
	}
	if exported := jsonExportColumn(t, result.Body, "id"); len(exported) != 1 || exported[0] != lead.String() {
		t.Fatalf("exported ids = %v, want the one lead %s", exported, lead)
	}
	if typed := jsonExportColumn(t, result.Body, "company_name"); typed[0] != "Meridian" {
		t.Errorf("company_name = %q, want the name typed on the lead", typed[0])
	}
	if linked := jsonExportColumn(t, result.Body, "linked_company_name"); linked[0] != "" {
		t.Errorf("linked_company_name = %q, want empty for a company this exporter cannot open", linked[0])
	}
}
