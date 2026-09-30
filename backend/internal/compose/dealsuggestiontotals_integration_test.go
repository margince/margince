// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A suggestion is never a deal. Every figure the product reports — each
// prebuilt report in the catalog, the forecast among them, and the account and
// contact pages — reads the same with open suggestions present as with none.
// The report list is the catalog itself, so a report added later is held to
// this without anybody remembering to add it here.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/contact360"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/extraction"
)

// figures is everything the product reports, read once.
type figures map[string]string

func (e *scoutEnv) figures(t *testing.T, company, contact ids.UUID, at time.Time) figures {
	t.Helper()
	out := figures{}
	clock := func() time.Time { return at }
	for _, key := range slices.Sorted(func(yield func(string) bool) {
		for key := range prebuiltReports {
			if !yield(key) {
				return
			}
		}
	}) {
		req := httptest.NewRequest(http.MethodPost, "/v1/reports/"+key, strings.NewReader(`{}`)).WithContext(e.Admin())
		rec := httptest.NewRecorder()
		reportHandlers{engine: newReportEngine(e.Pool)}.RunReport(rec, req, key)
		out["report "+key] = http.StatusText(rec.Code) + " " + reportedFigures(t, rec.Body.Bytes())
	}
	companyPage, err := company360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects,
		approvals.NewService(InstallationDB(e.Pool)), clock).Assemble(e.Admin(), ids.From[ids.CompanyKind](company))
	if err != nil {
		t.Fatalf("assembling the account page: %v", err)
	}
	contactPage, err := contact360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects,
		consent.NewStore(InstallationDB(e.Pool)),
		comms.NewStore(InstallationDB(e.Pool), clock, activities.NewStore(InstallationDB(e.Pool))),
		ai.NewFeedbackStore(InstallationDB(e.Pool)), clock).Assemble(e.Admin(), ids.From[ids.ContactKind](contact))
	if err != nil {
		t.Fatalf("assembling the contact page: %v", err)
	}
	out["account page"], out["contact page"] = encoded(t, companyPage), encoded(t, contactPage)
	return out
}

// reportFigureSet is the part of a report's answer that states figures.
type reportFigureSet struct {
	Columns   json.RawMessage              `json:"columns"`
	Rows      []map[string]json.RawMessage `json:"rows"`
	TotalRows json.RawMessage              `json:"total_rows"`
}

// reportedFigures keeps what a report reports — its columns and rows — and
// drops the stamps of when it was run, which each row's derivation link
// carries too.
func reportedFigures(t *testing.T, body []byte) string {
	t.Helper()
	var report reportFigureSet
	if err := json.Unmarshal(body, &report); err != nil || report.Rows == nil {
		return string(body)
	}
	for _, row := range report.Rows {
		delete(row, "derivation_url")
	}
	return encoded(t, report)
}

func encoded[T crmcontracts.Company360 | crmcontracts.Contact360 | reportFigureSet](t *testing.T, v T) string {
	t.Helper()
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		t.Fatalf("encoding: %v", err)
	}
	return buf.String()
}

func TestOpenSuggestionsChangeNoReportedFigure(t *testing.T) {
	e := setupScout(t)
	acme := e.SeedCompany(t, "Acme GmbH", nil)
	dana := e.employee(t, "Dana Buyer", acme)
	e.meeting(e.Admin(), t, "Scoping workshop", &dana, e.daysAgo(3))
	priced := e.SeedCompany(t, "Priced GmbH", nil)
	proposal := e.document(t, e.email(t, "Angebot", "outbound", priced, e.daysAgo(2)), "Angebot.pdf", "application/pdf")
	e.reading(t, proposal,
		extraction.ExtractedField{Field: "amount_minor", Value: "5000000", Confidence: "high"},
		extraction.ExtractedField{Field: "currency", Value: "EUR", Confidence: "high"})
	// A real deal elsewhere, so the reports have a figure to disagree about.
	other := ids.From[ids.CompanyKind](e.SeedCompany(t, "Other AG", nil))
	amount, currency := int64(100_000), "EUR"
	if _, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Real deal", PipelineID: e.pipeline, StageID: e.open, CompanyID: &other,
		AmountMinor: &amount, Currency: &currency, Source: "manual",
	}); err != nil {
		t.Fatalf("opening the real deal: %v", err)
	}
	e.pass(t)
	if n := e.WsCount(t, `SELECT count(*) FROM deal_suggestion WHERE state = 'open'`); n != 2 {
		t.Fatalf("%d open suggestions, want 2 to test against", n)
	}

	at := time.Now()
	with := e.figures(t, acme, dana, at)
	e.WsExec(t, `DELETE FROM deal_suggestion`)
	without := e.figures(t, acme, dana, at)

	answered := 0
	for key, reading := range with {
		if reading != without[key] {
			t.Errorf("%s differs with open suggestions present:\n with: %s\n without: %s", key, reading, without[key])
		}
		if strings.HasPrefix(reading, "OK ") {
			answered++
		}
	}
	// Six reports and two pages at the time of writing. A census that read
	// fewer answered than that has lost its subject, not proved anything.
	if answered < 8 {
		t.Fatalf("only %d readings answered 200, want every report and both pages", answered)
	}
}
