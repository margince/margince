// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/assurance"
)

func TestACommittedInputEditIsUnknownUntilItsMarkerRuns(t *testing.T) {
	e := setupFreshness(t)
	env := e.editDeal(t, e.faulted, 3700000)
	if f := e.freshness(t); f.Known {
		t.Fatalf("an undelivered input change still has known freshness: %+v", f)
	}
	for _, finding := range e.findings(t) {
		if finding.InputsChanged != nil {
			t.Fatalf("an unread committed input leaves a finding marked current: %+v", finding)
		}
	}
	e.deliver(t, env)
	if f := e.freshness(t); !f.Known || !f.OutOfDate() || f.ChangedDeals != 1 {
		t.Fatalf("the delivered input change has freshness %+v", f)
	}
}

func TestACompleteCheckReadsCommittedInputsWithoutTheConsumer(t *testing.T) {
	e := setupFreshness(t)
	e.editDeal(t, e.faulted, 3800000)
	if err := e.scan(t); err != nil {
		t.Fatal(err)
	}
	if f := e.freshness(t); !f.Known || f.OutOfDate() {
		t.Fatalf("a complete check failed to read the committed edit: %+v", f)
	}
}

func TestAnUndeliveredEditDuringTheCheckCannotBeClearedByIt(t *testing.T) {
	e := setupFreshness(t)
	duringRead := func(ctx context.Context, tx pgx.Tx, at time.Time, coverage []assurance.SourceCoverage) ([]assurance.Subject, error) {
		subjects, err := AssuranceSubjects(ctx, tx, at, coverage)
		e.editDeal(t, e.faulted, 3900000)
		return subjects, err
	}
	if _, err := e.scanner(duringRead).Scan(e.passCtx(), e.at, nil); err != nil {
		t.Fatal(err)
	}
	if f := e.freshness(t); f.Known {
		t.Fatalf("an edit committed after the source read was declared checked: %+v", f)
	}
}

func TestWorkCreatedAfterTheCheckLeavesItsInputsUnread(t *testing.T) {
	e := setupFreshness(t)
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	if f := e.freshness(t); f.Known {
		t.Fatalf("work created after the scan was declared read by it: %+v", f)
	}
}

func TestAnIncompleteCheckLeavesCommittedInputsUnread(t *testing.T) {
	e := setupFreshness(t)
	e.editDeal(t, e.faulted, 4000000)
	unavailable := func(context.Context, pgx.Tx, time.Time) []assurance.SourceCoverage {
		return []assurance.SourceCoverage{{Source: sourceMail, State: assurance.CoverageUnavailable}, offerCoverage(e.at)}
	}
	scanner := assurance.NewScanner(e.store, AssuranceSubjects, unavailable, AssuranceConfig())
	result, err := scanner.Scan(e.passCtx(), e.at, nil)
	if err != nil || result.Status != assurance.StatusIncomplete {
		t.Fatalf("incomplete scan = %+v: %v", result, err)
	}
	if f := e.freshness(t); f.Known {
		t.Fatalf("an incomplete check cleared an unread input: %+v", f)
	}
}

func TestARunWithoutOutboxTrackingIsUnknownUntilTheNextCheck(t *testing.T) {
	e := setupFreshness(t)
	e.WsExec(t, `UPDATE assurance_run SET outbox_inputs_tracked = false`)
	if f := e.freshness(t); f.Known {
		t.Fatalf("a run without event tracking is still current: %+v", f)
	}
	if err := e.scan(t); err != nil {
		t.Fatal(err)
	}
	if f := e.freshness(t); !f.Known || f.OutOfDate() {
		t.Fatalf("a complete tracked run left freshness %+v", f)
	}
}
