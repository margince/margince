// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The leads-by-status report counts what every other lead read hides. A
// promoted or disqualified lead is archived, and lists leave archived rows out.
// So the board's two terminal columns need this count.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Terminal leads are counted, under their own status. With the
// `whereArchivedNull` base its neighbours carry, the report would answer 0 for
// the two statuses the board needs.
func TestLeadsByStatusCountsTheTerminalLeadsEveryOtherReadHides(t *testing.T) {
	e := integration.Setup(t)

	// Live leads, which any list would also return.
	seedLeadAt(t, e, "new", false, nil)
	seedLeadAt(t, e, "new", false, nil)
	seedLeadAt(t, e, "engaged", false, nil)
	// Terminal leads: archived, which is what makes them invisible everywhere
	// else. A promoted lead and two disqualified ones.
	seedLeadAt(t, e, "promoted", true, nil)
	seedLeadAt(t, e, "disqualified", true, nil)
	seedLeadAt(t, e, "disqualified", true, nil)

	counts := leadStatusCounts(e.Admin(), t, e)
	for status, want := range map[string]int64{
		"new": 2, "engaged": 1, "promoted": 1, "disqualified": 2,
	} {
		if counts[status] != want {
			t.Errorf("leads-by-status counted %d %s, want %d — the two terminal statuses are the ones this report exists for",
				counts[status], status, want)
		}
	}
}

// The column's count and list answer over one population. Every seat with
// the lead grant reads every lead. A count narrowed to the reader's own work
// would print 2 above a column showing three rows.
func TestLeadsByStatusCountsEveryLeadTheTerminalColumnLists(t *testing.T) {
	e := integration.Setup(t)
	seedLeadAt(t, e, "new", false, nil)
	// The reader's own, a colleague's, and one nobody has claimed: the three
	// ownership shapes the opened column puts side by side.
	seedLeadAt(t, e, "disqualified", true, &e.Rep1)
	seedLeadAt(t, e, "disqualified", true, &e.Rep3)
	seedLeadAt(t, e, "disqualified", true, nil)

	rep := repReadingLeads(e)
	if listed := disqualifiedLeadsListed(rep, t, e); listed != 3 {
		t.Fatalf("the column listed %d leads, want the 3 an identity table serves every seat", listed)
	}
	if counted := leadStatusCounts(rep, t, e)["disqualified"]; counted != 3 {
		t.Errorf("the column's head counted %d disqualified leads over a list of 3 — "+
			"the count and the rows below it measure one population", counted)
	}
}

// The board's owner dial is an owner filter: naming a colleague is refused for
// a rep, as on every install-wide report.
func TestLeadsByStatusRefusesARepsOwnerDialOnAColleague(t *testing.T) {
	e := integration.Setup(t)
	seedLeadAt(t, e, "disqualified", true, &e.Rep3)

	req := httptest.NewRequest(http.MethodPost, "/v1/reports/leads-by-status",
		strings.NewReader(`{"filters":{"owner_id":"`+e.Rep3.String()+`"}}`)).WithContext(repReadingLeads(e))
	rec := httptest.NewRecorder()
	reportHandlers{engine: newReportEngine(e.Pool)}.RunReport(rec, req, "leads-by-status")
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"code":"owner_not_measurable"`) {
		t.Fatalf("a rep's owner dial on a colleague got %d %s, want 403 owner_not_measurable", rec.Code, rec.Body.String())
	}
}

// repReadingLeads is an own-scope rep holding the lead grant: the seat a
// population narrows, on a table whose row scope does not.
func repReadingLeads(e *integration.Env) context.Context {
	return e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		Objects: map[string]principal.ObjectGrant{
			"lead":                  {Read: true},
			"installation_settings": {Read: true},
		},
		RowScope: principal.RowScopeOwn,
	})
}

// disqualifiedLeadsListed is what the opened column shows: the same read the
// board makes, archived rows and all.
func disqualifiedLeadsListed(as context.Context, t *testing.T, e *integration.Env) int {
	t.Helper()
	status := crmcontracts.ListLeadsParamsStatus("disqualified")
	archived := true
	rec := httptest.NewRecorder()
	contacts.NewHandlers(InstallationDB(e.Pool)).ListLeads(rec,
		httptest.NewRequest(http.MethodGet, "/v1/leads", nil).WithContext(as),
		crmcontracts.ListLeadsParams{Status: &status, IncludeArchived: &archived})
	var page crmcontracts.LeadListResponse
	decodeWire(t, rec, http.StatusOK, &page)
	return len(page.Data)
}

// leadStatusCounts is the board's own report call: the figure at the head of
// each column, keyed by status.
func leadStatusCounts(as context.Context, t *testing.T, e *integration.Env) map[string]int64 {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/reports/leads-by-status",
		strings.NewReader(`{}`)).WithContext(as)
	rec := httptest.NewRecorder()
	reportHandlers{engine: newReportEngine(e.Pool)}.RunReport(rec, req, "leads-by-status")

	var result reportResultWire
	decodeWire(t, rec, http.StatusOK, &result)
	counts := map[string]int64{}
	for _, row := range result.Rows {
		status, ok := row["status"].(string)
		if !ok {
			t.Fatalf("row %v has no status", row)
		}
		counts[status] = wireInt(t, row, "leads")
	}
	return counts
}

// seedLeadAt plants one lead at a status, owned by the seat named or by
// nobody. archived_at follows the status, because a promoted or disqualified
// lead is always archived and a fixture must not spell a row the product cannot.
func seedLeadAt(t *testing.T, e *integration.Env, status string, terminal bool, owner *ids.UUID) {
	t.Helper()
	id := ids.NewV7()
	archived := "NULL"
	if terminal {
		archived = "now()"
	}
	// A promoted lead names the contact it became, which the table now requires
	// — the promotion writes both in one statement, so a row with the status
	// and no contact is one the product cannot produce. The contact is seeded
	// for the same reason: the pointer is a foreign key, and a fixture naming
	// nobody would be a different impossible row.
	promotedContact := "NULL"
	if status == "promoted" {
		contact := ids.NewV7()
		e.WsExec(t, `INSERT INTO contact (id, full_name, source, captured_by)
			VALUES ($1, 'Promoted Fixture', 'manual', 'human:x')`, contact)
		promotedContact = "'" + contact.String() + "'"
	}
	promotedAt := "NULL"
	if status == "promoted" {
		promotedAt = "now()"
	}
	e.WsExec(t, `INSERT INTO lead (id, full_name, status, source, captured_by, owner_id, archived_at, promoted_at, promoted_contact_id)
		VALUES ($1, 'Terminal Fixture', $2, 'inbound', 'human:x', $3, `+archived+`, `+promotedAt+`, `+promotedContact+`)`,
		id, status, owner)
}
