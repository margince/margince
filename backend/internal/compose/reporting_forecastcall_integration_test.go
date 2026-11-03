// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The company quarter has a standing call of 760k under the manager-call
// measure. A seat that may read the totals but not the call gets the landing
// GET /forecast gives it: the records' projection with the call_absent caveat.
// Neither the metric nor its CSV holds the call.
func TestAReaderWhoMayNotSeeTheCallGetsTheLandingWithoutIt(t *testing.T) {
	q := seedTargetQuarter(t)
	reader := reportingTieredReader(q.human, principal.RowScopeTeam)

	authorized, _ := q.comparisonFor(q.human, t, q.selection())
	if basis := authorized.Metrics[0].ForecastBasis; basis == nil || basis.CallMinor == nil || *basis.CallMinor != 76_000_000 || basis.Landing.Measure != "manager_call" {
		t.Fatalf("the admin's basis %+v, want the 760k call as the landing", basis)
	}
	evaluation, _ := q.comparisonFor(reader, t, q.selection())
	basis := evaluation.Metrics[0].ForecastBasis
	overHTTP := (&stageCohortFixture{env: q.env}).forecastOverHTTPAs(reader, t, q.at)
	if basis == nil || basis.CallMinor != nil || overHTTP.CurrentCall != nil || overHTTP.Landing == nil || !reflect.DeepEqual(basis.Landing, *overHTTP.Landing) {
		t.Fatalf("the team-lens basis %+v beside GET /forecast's call %v and landing %+v, want no call and one landing", basis, overHTTP.CurrentCall, overHTTP.Landing)
	}
	if basis.Landing.Measure != "commit_evidence" || basis.Landing.Caveat == nil || *basis.Landing.Caveat != "call_absent" {
		t.Fatalf("the team-lens landing %+v, want the records' projection with call_absent", basis.Landing)
	}
	assertNoCallInCSV(t, evaluation)

	// An edition is frozen under its publisher, and this seat may not publish
	// over the company, so no edition it publishes can hold the call.
	_, err := q.service.CreateReport(reader, crmcontracts.ReportingReportInput{Name: "Quarter review", Audience: "workspace", Selection: q.selection()})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("saving a company report as the team-lens seat: %v, want permission denied", err)
	}
}

func assertNoCallInCSV(t *testing.T, evaluation crmcontracts.ReportingEvaluation) {
	t.Helper()
	for _, row := range reportingCSVRows(evaluation) {
		if slices.Contains(row, "manager_call") || slices.Contains(row, "76000000") {
			t.Fatalf("the CSV holds the call: %v", row)
		}
	}
}
