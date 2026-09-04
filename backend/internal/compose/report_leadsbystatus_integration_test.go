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
// show — which is the gap (#1886) this key fills.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
	seedLeadAt(t, e, "new", false)
	seedLeadAt(t, e, "new", false)
	seedLeadAt(t, e, "engaged", false)
	// Terminal leads: archived, which is what makes them invisible everywhere
	// else. A promoted lead and two disqualified ones.
	seedLeadAt(t, e, "promoted", true)
	seedLeadAt(t, e, "disqualified", true)
	seedLeadAt(t, e, "disqualified", true)

	handlers := reportHandlers{engine: newReportEngine(e.Pool)}
	req := httptest.NewRequest(http.MethodPost, "/v1/reports/leads-by-status",
		strings.NewReader(`{}`)).WithContext(e.Admin())
	rec := httptest.NewRecorder()
	handlers.RunReport(rec, req, "leads-by-status")

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

	for status, want := range map[string]int64{
		"new": 2, "engaged": 1, "promoted": 1, "disqualified": 2,
	} {
		if counts[status] != want {
			t.Errorf("leads-by-status counted %d %s, want %d — the two terminal statuses are the ones this report exists for",
				counts[status], status, want)
		}
	}
}

// seedLeadAt plants one lead at a status, archived or not.
//
// archived_at is set from the status rather than passed independently: a
// promoted or disqualified lead IS an archived one, and a fixture that could
// spell a live disqualified lead would be proving the report against a row the
// product cannot produce.
func seedLeadAt(t *testing.T, e *integration.Env, status string, terminal bool) {
	t.Helper()
	id := ids.NewV7()
	archived := "NULL"
	if terminal {
		archived = "now()"
	}
	e.WsExec(t, `INSERT INTO lead (id, full_name, status, source, captured_by, archived_at)
		VALUES ($1, 'Terminal Fixture', $2, 'inbound', 'human:x', `+archived+`)`, id, status)
}
