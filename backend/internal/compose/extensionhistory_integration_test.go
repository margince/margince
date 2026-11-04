// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A core record's History carries the rows of an extension kind that reference
// it, and the signals of an extension kind about it. Both leave out what the
// caller may not read.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
	"github.com/margince/margince/backend/pkg/extension"
)

const historyDemoTable = "ext_historydemo_visit"

func registerHistoryVisits(t *testing.T) {
	t.Helper()
	owner := integration.OwnerConn(t)
	for _, stmt := range []string{
		`CREATE SCHEMA IF NOT EXISTS ext`,
		`GRANT USAGE ON SCHEMA ext TO margince_app`,
		`DROP TABLE IF EXISTS ext.` + historyDemoTable,
		`CREATE TABLE ext.` + historyDemoTable + ` (id uuid PRIMARY KEY DEFAULT uuidv7(), archived_at timestamptz, code text NOT NULL UNIQUE, site text, visited_at timestamptz, contact_id uuid)`,
		`GRANT SELECT, INSERT, UPDATE, DELETE ON ext.` + historyDemoTable + ` TO margince_app`,
	} {
		if _, err := owner.Exec(context.Background(), stmt); err != nil {
			t.Fatalf("preparing the visit table (%s): %v", stmt, err)
		}
	}
	recordkinds.Register("historydemo", []extension.RecordKindDeclaration{{
		Kind: "visit", DisplayName: "Visit", Table: historyDemoTable,
		Fields:     []extension.RecordKindField{{Name: "code", Label: "Code"}, {Name: "site", Label: "Site"}},
		NaturalKey: []string{"code"},
		Timeline: &extension.RecordKindTimeline{
			LabelField: "site", AtColumn: "visited_at",
			Subjects: []extension.TimelineSubject{{Entity: "contact", Column: "contact_id"}},
		},
	}})
	t.Cleanup(func() {
		recordkinds.Reset()
		if _, err := owner.Exec(context.Background(), `DROP TABLE IF EXISTS ext.`+historyDemoTable); err != nil {
			t.Errorf("dropping the visit table: %v", err)
		}
	})
}

func readExtensionHistory(t *testing.T, e *integration.Env, ctx context.Context, contact ids.UUID) (int, []crmcontracts.ExtensionHistoryEntry) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/extension-history", nil).WithContext(ctx)
	analyticsQueryHandlers{db: e.DB()}.ListExtensionHistory(rec, req, crmcontracts.ListExtensionHistoryParams{
		EntityType: crmcontracts.ListExtensionHistoryParamsEntityTypeContact, EntityId: crmcontracts.Id(contact),
	})
	var body crmcontracts.ExtensionHistoryResponse
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
	}
	return rec.Code, body.Data
}

func TestAContactsHistoryCarriesExtensionRowsAndSignalsNewestFirst(t *testing.T) {
	e := integration.Setup(t)
	registerHistoryVisits(t)
	contact := e.SeedContact(t, "Ines Vogel", nil)
	other := e.SeedContact(t, "Marc Dubois", nil)
	for _, row := range []struct {
		code, site string
		at, who    string
		archived   bool
	}{
		{"v1", "Berlin site", "2026-03-01T10:00:00Z", "mine", false},
		{"v2", "Munich site", "2026-05-01T10:00:00Z", "mine", false},
		{"v3", "Hamburg site", "2026-04-01T10:00:00Z", "other", false},
		{"v4", "Old site", "2026-06-01T10:00:00Z", "mine", true},
	} {
		who := contact
		if row.who == "other" {
			who = other
		}
		e.WsExec(t, `INSERT INTO ext.`+historyDemoTable+` (code, site, visited_at, contact_id, archived_at)
			VALUES ($1, $2, $3::timestamptz, $4, CASE WHEN $5 THEN now() END)`, row.code, row.site, row.at, who, row.archived)
	}
	e.WsExec(t, `INSERT INTO signal (kind, entity_type, entity_id, summary, detected_at, source, captured_by)
		VALUES ('ext:historydemo:late', 'contact', $1, 'Paid late', '2026-04-15T10:00:00Z', 'test', 'human:seed'),
		       ('risk', 'contact', $1, 'A core signal', '2026-04-16T10:00:00Z', 'test', 'human:seed')`, contact)

	reader := e.As(e.Rep1, nil, principal.Permissions{
		RowScope: principal.RowScopeAll,
		Objects: map[string]principal.ObjectGrant{
			"contact": {Read: true}, "signal": {Read: true}, "ext_historydemo_visit": {Read: true},
		},
	})
	code, got := readExtensionHistory(t, e, reader, contact)
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	var titles []string
	for _, entry := range got {
		titles = append(titles, entry.Title)
	}
	want := []string{"Munich site", "Paid late", "Berlin site"}
	if len(titles) != len(want) {
		t.Fatalf("entries = %v, want %v (another contact's row, an archived row and a core signal stay out)", titles, want)
	}
	for i := range want {
		if titles[i] != want[i] {
			t.Fatalf("entries = %v, want %v", titles, want)
		}
	}
	if got[0].Source != crmcontracts.ExtensionHistoryEntrySourceRecord || got[1].Source != crmcontracts.ExtensionHistoryEntrySourceSignal {
		t.Errorf("sources = %s, %s; want record then signal", got[0].Source, got[1].Source)
	}

	noKind := e.As(e.Rep1, nil, principal.Permissions{
		RowScope: principal.RowScopeAll,
		Objects:  map[string]principal.ObjectGrant{"contact": {Read: true}, "signal": {Read: true}},
	})
	if _, got := readExtensionHistory(t, e, noKind, contact); len(got) != 1 || got[0].Title != "Paid late" {
		t.Errorf("without the kind's read right = %+v, want the signal alone", got)
	}
	noContact := e.As(e.Rep1, nil, principal.Permissions{
		RowScope: principal.RowScopeAll,
		Objects:  map[string]principal.ObjectGrant{"ext_historydemo_visit": {Read: true}},
	})
	if code, _ := readExtensionHistory(t, e, noContact, contact); code == http.StatusOK {
		t.Error("a caller who may not read contacts was served the History")
	}
	if code, _ := readExtensionHistory(t, e, reader, ids.NewV7()); code != http.StatusNotFound {
		t.Errorf("a contact nobody holds = %d, want 404", code)
	}
}
