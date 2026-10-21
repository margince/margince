// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/csv"
	"net/http"
	"net/http/httptest"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// exportedQuestionRows downloads an edition's CSV as reader and returns its
// question card's rows.
func exportedQuestionRows(reader context.Context, t *testing.T, q *questionEditions, edition crmcontracts.ReportingEdition) [][]string {
	t.Helper()
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/analytics/editions/x/export.csv", nil).WithContext(reader)
	reportingExportHandlers{service: q.service}.ExportReportingEdition(response, request, crmcontracts.Id(edition.Id))
	if response.Code != http.StatusOK {
		t.Fatalf("export: status %d: %s", response.Code, response.Body.String())
	}
	records, err := csv.NewReader(response.Body).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	out := [][]string{}
	for _, record := range records {
		if record[1] == "question:open-deals" {
			out = append(out, record)
		}
	}
	return out
}

// The edition's export carries the kept answer, judged for whoever downloads
// it as the screen judges it: the sender gets the count, and a reader who may
// not ask the question gets only that it was withheld.
func TestAnEditionExportCarriesItsKeptAnswers(t *testing.T) {
	q := newQuestionEditions(t)
	q.keepForAYear(t)
	edition := q.send(t)

	sent := exportedQuestionRows(q.human, t, q, edition)
	if len(sent) != 1 || sent[0][3] != "7" || sent[0][9] != "deals" || sent[0][10] != "ok" {
		t.Fatalf("the sender's export carries %q, want the kept count of 7", sent)
	}

	reader := q.e.ownLensRepCtx(q.e.Rep3)
	actor, _ := principal.Actor(reader)
	for _, object := range []string{"report_definition", "report_edition"} {
		actor.Permissions.Objects[object] = principal.ObjectGrant{Read: true}
	}
	narrow := exportedQuestionRows(principal.WithActor(reader, actor), t, q, edition)
	if len(narrow) != 1 || narrow[0][3] != "" || narrow[0][10] != "withheld" {
		t.Fatalf("the narrower reader's export carries %q, want one withheld row and no value", narrow)
	}
}
