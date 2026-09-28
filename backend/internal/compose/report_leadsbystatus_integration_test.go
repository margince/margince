// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The leads-by-status report, whose whole reason to exist is the population
// every other lead read hides.
//
// A lead is ARCHIVED the moment it is promoted or disqualified, and every list
// this product serves excludes archived rows by default. That is right for a
// work queue and it leaves the board's two terminal columns with no count to
// show — which is the gap this key fills.

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

// Terminal leads are counted, and counted under their own status.
//
// The mutation this guards against is the obvious one: giving the spec the
// `whereArchivedNull` base its neighbours all carry. The report would still
// answer, still name three statuses, and quietly report 0 for the two the
// board added these columns for.
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

// The column's count and the column's list answer over one population.
//
// A lead is readable by every seat holding the lead grant, whatever its row
// scope, so the terminal column lists a colleague's disqualified lead and an
// unowned one beside the reader's own. A count narrowed to the reader's own
// work printed 2 at the head of a column showing three rows.
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
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a rep's owner dial on a colleague got %d %s, want 403", rec.Code, rec.Body.String())
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

// seedLeadAt plants one lead at a status, archived or not, owned by the seat
// named or by nobody.
//
// archived_at is set from the status rather than passed independently: a
// promoted or disqualified lead IS an archived one, and a fixture that could
// spell a live disqualified lead would be proving the report against a row the
// product cannot produce.
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
