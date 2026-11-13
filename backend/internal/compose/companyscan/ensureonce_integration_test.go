// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package companyscan

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Two opens of one account at once queue one read, however they overlap. A
// change after the read settled is seen by the next request.
func TestConcurrentOpensQueueOneReadAndAChangeIsSeenAfterwards(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Kranich Fördertechnik GmbH", &e.AdminUser))
	now := clocktest.Now(t)
	clock := func() time.Time { return now }
	view := company360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects, approvals.NewService(e.DB()), clock)
	var queued atomic.Int32
	enqueue := func(context.Context, pgx.Tx, Queued) error {
		queued.Add(1)
		return nil
	}
	svc := NewService(e.Pool, view, view, nil, enqueue, nil, clock, nil)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Go(func() {
			_, errs[i] = svc.Ensure(ctx, company, false)
		})
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
	}
	if got := queued.Load(); got != 1 {
		t.Fatalf("two opens at once queued %d reads, want one", got)
	}

	settleTheQueuedRead(t, e, svc, company)
	current, err := svc.Get(ctx, company)
	if err != nil || current.Stale != nil {
		t.Fatalf("read after the settled scan = %+v, %v; want it current", current, err)
	}

	industry := "Construction"
	if _, err := e.Contacts.UpdateCompany(ctx, company, contacts.UpdateCompanyInput{Industry: &industry}); err != nil {
		t.Fatal(err)
	}
	changed, err := svc.Get(ctx, company)
	if err != nil || changed.Stale == nil || !*changed.Stale {
		t.Fatalf("read after the account changed = %+v, %v; want the stored scan marked stale", changed, err)
	}
	if _, err := svc.Ensure(ctx, company, true); err != nil {
		t.Fatal(err)
	}
	if got := queued.Load(); got != 2 {
		t.Errorf("a forced open after the change queued %d reads in all, want a second one", got)
	}
}

// settleTheQueuedRead runs the queued read the way the worker does, with no
// lane bound, so the row settles on the rules' floor under a fingerprint.
func settleTheQueuedRead(t *testing.T, e *integration.Env, svc *Service, company ids.CompanyID) {
	t.Helper()
	ctx := e.Admin()
	var stored row
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		var err error
		stored, _, err = load(ctx, tx, ids.From[ids.UserKind](e.AdminUser), company)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Run(ctx, stored.ID, company); err != nil {
		t.Fatalf("running the queued read: %v", err)
	}
}
