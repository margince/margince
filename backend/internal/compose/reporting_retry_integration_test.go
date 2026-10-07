// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type cancelledPublicationAuthority struct{ reportingAuthority }

func (a cancelledPublicationAuthority) PublicationHuman(ctx context.Context, tx pgx.Tx, owner ids.UUID) (context.Context, error) {
	human, err := a.reportingAuthority.PublicationHuman(ctx, tx, owner)
	if err != nil {
		return nil, err
	}
	cancelled, cancel := context.WithCancel(human)
	cancel()
	return cancelled, nil
}

func TestReportingRetriesBackOffExhaustAndKeepTheOriginalIntent(t *testing.T) {
	f := reportingBusiness(t)
	report, err := f.service.CreateReport(f.human, crmcontracts.ReportingReportInput{Name: "Retry proof", Audience: "workspace", Selection: f.selection()})
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.service.Freeze(f.human, ids.UUID(report.Id), report.Revision, "retry-intent")
	if err != nil {
		t.Fatal(err)
	}
	db := InstallationDB(f.env.Pool)
	failed := reporting.NewService(db, metricEvaluator{forecast: forecasting.NewStore(db)}, cancelledPublicationAuthority{reportingAuthority{users: identity.NewService(f.env.Pool)}}, reportingCalendar, func() time.Time { return f.at })
	system := principal.SystemActing(f.human, "system:retry-proof")
	for _, step := range []struct {
		advance time.Duration
		attempt int64
		status  string
	}{
		{0, 1, "pending"},
		{5*time.Minute - time.Second, 1, "pending"},
		{time.Second, 2, "pending"},
		{15*time.Minute - time.Second, 2, "pending"},
		{time.Second, 3, "failed"},
		{time.Hour, 3, "failed"},
	} {
		f.at = f.at.Add(step.advance)
		if err := failed.Sweep(system); err != nil {
			t.Fatal(err)
		}
		current, err := f.service.GetExecution(f.human, ids.UUID(run.Id))
		if err != nil {
			t.Fatal(err)
		}
		if current.Attempt != step.attempt || string(current.Status) != step.status || current.EditionId != nil {
			t.Fatalf("after %s: got %+v, want attempt %d/%s without edition", step.advance, current, step.attempt, step.status)
		}
	}
	retried, err := f.service.RetryExecution(f.human, ids.UUID(run.Id))
	if err != nil {
		t.Fatal(err)
	}
	if retried.Id != run.Id || !retried.IntendedDueAt.Equal(run.IntendedDueAt) {
		t.Fatal("retry replaced the original intent")
	}
	if err := f.service.Sweep(system); err != nil {
		t.Fatal(err)
	}
	current, err := f.service.GetExecution(f.human, ids.UUID(run.Id))
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != "succeeded" || current.Attempt != 4 || current.EditionId == nil {
		t.Fatalf("recovery did not publish: %+v", current)
	}
	editions, err := f.service.ListEditions(f.human, ids.UUID(report.Id), nil, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(editions.Data) != 1 {
		t.Fatalf("retry published %d editions", len(editions.Data))
	}
}

type leaseExpiringAuthority struct {
	reportingAuthority
	advance func()
}

func (a leaseExpiringAuthority) PublicationHuman(ctx context.Context, tx pgx.Tx, owner ids.UUID) (context.Context, error) {
	a.advance()
	return a.reportingAuthority.PublicationHuman(ctx, tx, owner)
}

func TestReportingLostLeaseRetriesWithoutSuspendingTheOwner(t *testing.T) {
	f := reportingBusiness(t)
	report, err := f.service.CreateReport(f.human, crmcontracts.ReportingReportInput{Name: "Lease proof", Audience: "private", Selection: f.selection()})
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.service.Freeze(f.human, ids.UUID(report.Id), 0, "lease-intent")
	if err != nil {
		t.Fatal(err)
	}
	db := InstallationDB(f.env.Pool)
	authority := leaseExpiringAuthority{reportingAuthority{users: identity.NewService(f.env.Pool)}, func() { f.at = f.at.Add(91 * time.Second) }}
	worker := reporting.NewService(db, metricEvaluator{forecast: forecasting.NewStore(db)}, authority, reportingCalendar, func() time.Time { return f.at })
	if err := worker.Sweep(principal.SystemActing(f.human, "system:lease-proof")); err != nil {
		t.Fatal(err)
	}
	current, err := f.service.GetExecution(f.human, ids.UUID(run.Id))
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != "pending" || current.Attempt != 1 || current.EditionId != nil {
		t.Fatalf("lost lease suspended owner: %+v", current)
	}
}

func TestReportingCrashedRunStopsAfterThreeExpiredClaims(t *testing.T) {
	f := reportingBusiness(t)
	report, err := f.service.CreateReport(f.human, crmcontracts.ReportingReportInput{Name: "Crash proof", Audience: "private", Selection: f.selection()})
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.service.Freeze(f.human, ids.UUID(report.Id), 0, "crash-intent")
	if err != nil {
		t.Fatal(err)
	}
	// Reconstruct the durable state left by a process dying after its third claim.
	if _, err := f.env.owner.Exec(f.human, "UPDATE report_execution SET status='running',attempt=3,fence=3,lease_until=$1 WHERE id=$2", f.at.Add(-time.Second), run.Id); err != nil {
		t.Fatal(err)
	}
	if err := f.service.Sweep(principal.SystemActing(f.human, "system:crash-proof")); err != nil {
		t.Fatal(err)
	}
	current, err := f.service.GetExecution(f.human, ids.UUID(run.Id))
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != "failed" || current.Attempt != 3 || current.EditionId != nil {
		t.Fatalf("crashed run was reclaimed forever: %+v", current)
	}
}
