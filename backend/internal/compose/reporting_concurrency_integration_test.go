// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"sync"
	"sync/atomic"
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

type heldPublicationAuthority struct {
	reportingAuthority
	once             sync.Once
	entered, release chan struct{}
}

func (a *heldPublicationAuthority) PublicationHuman(ctx context.Context, tx pgx.Tx, owner ids.UUID) (context.Context, error) {
	a.once.Do(func() {
		close(a.entered)
		select {
		case <-a.release:
		case <-ctx.Done():
		}
	})
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return a.reportingAuthority.PublicationHuman(ctx, tx, owner)
}

func TestReportingExpiredWorkerCannotReplaceTheSuccessfulRetry(t *testing.T) {
	f := reportingBusiness(t)
	report, err := f.service.CreateReport(f.human, crmcontracts.ReportingReportInput{Name: "Fenced publication", Audience: "workspace", Selection: f.selection()})
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.service.Freeze(f.human, ids.UUID(report.Id), report.Revision, "one-intent")
	if err != nil {
		t.Fatal(err)
	}
	var instant atomic.Int64
	instant.Store(f.at.UnixNano())
	now := func() time.Time { return time.Unix(0, instant.Load()).UTC() }
	authority := &heldPublicationAuthority{users: identity.NewService(f.env.Pool), entered: make(chan struct{}), release: make(chan struct{})}
	db := InstallationDB(f.env.Pool)
	stalled := reporting.NewService(db, metricEvaluator{forecast: forecasting.NewStore(db)}, authority, reportingCalendar, now)
	worker := newReportingService(f.env.Pool, now)
	ctx, cancel := context.WithTimeout(principal.SystemActing(f.human, "system:reporting-fence"), 2*time.Minute)
	defer cancel()
	finished := make(chan error, 1)
	go func() { finished <- stalled.Sweep(ctx) }()
	select {
	case <-authority.entered:
	case err := <-finished:
		t.Fatalf("worker stopped before publication barrier: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	instant.Add(int64(91 * time.Second))
	retryErr := worker.Sweep(ctx)
	close(authority.release)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if retryErr != nil {
		t.Fatal(retryErr)
	}
	completed, err := f.service.GetExecution(f.human, ids.UUID(run.Id))
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != "succeeded" || completed.Attempt != 2 || completed.EditionId == nil {
		t.Fatalf("stale worker replaced retry: %+v", completed)
	}
	if err := worker.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	editions, err := f.service.ListEditions(f.human, ids.UUID(report.Id), nil, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(editions.Data) != 1 || editions.Data[0].Id != *completed.EditionId {
		t.Fatalf("duplicate publication: %+v", editions)
	}
}
