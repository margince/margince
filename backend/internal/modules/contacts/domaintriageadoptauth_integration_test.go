// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A company archived while an adoption waits does not receive the domain.
//
// The dedupe ladder picks the twin from a scan that read its own snapshot, and
// the adoption writes to that row several statements later. An archive
// committing in between is invisible to the scan, and EnsureWritable does not
// see it either — that probe admits an archived row. LockRow(LiveOnly) is the
// only thing in the path that refuses one, which is what this holds.
//
// The caller owns the incumbent, so authorization is never in question. What is
// in question is whether the record survives long enough to receive the write.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAdoptingADomainDecidesTheRecordIsLiveUnderTheLock(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	// The incumbent, owned by the first rep and carrying the name a second
	// domain is about to resolve to. Seeded through the real writer, owner and
	// all.
	owner := ids.From[ids.UserKind](e.rep)
	incumbent, err := e.store.CreateCompany(ctx, CreateCompanyInput{
		DisplayName: "Adopt Auth GmbH", Source: "manual",
		OwnerID: &owner,
		Domains: []CompanyDomainInput{{Domain: "adoptauth-first.test", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seeding the incumbent: %v", err)
	}

	// A second domain of the same name, waiting on a verdict.
	e.openTriageFirst(ctx, t, "someone@adoptauth-second.test", "Some One", "adoptauth-second.test")
	readID := e.startTriageRead(ctx, t, "adoptauth-second.test")

	// A transaction holding the incumbent's row lock, which has archived it —
	// frozen before its commit, so the test owns the moment it becomes visible.
	holder, pid := e.holdArchivedCompany(ctx, t, ids.UUID(incumbent.Id))

	done := make(chan error, 1)
	go func() {
		_, err := e.store.ResolveDomainTriage(e.as(), ResolveDomainTriageInput{
			Domain: "adoptauth-second.test", Status: DomainCompany, Source: DomainSourceSiteRead,
			ReadID: readID, DossierName: "Adopt Auth GmbH", SeedURL: "https://adoptauth-second.test",
		})
		done <- err
	}()
	mustBlockOn(t, holder, pid, done)

	if err := holder.Commit(ctx); err != nil {
		t.Fatalf("committing the archive: %v", err)
	}
	// A retired record reads as not-found, which is the same answer every other
	// live probe gives for one.
	if err := mustFinishWithin(t, done); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("the adoption answered %v, want ErrNotFound — the company was "+
			"archived while this caller waited for the lock", err)
	}

	var adopted bool
	if err := e.store.db.Pool().QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM company_domain
		   WHERE company_id = $1 AND domain = 'adoptauth-second.test' AND archived_at IS NULL)`,
		incumbent.Id).Scan(&adopted); err != nil {
		t.Fatalf("reading the incumbent's domains: %v", err)
	}
	if adopted {
		t.Error("the domain landed on a company that had been archived: the record's " +
			"liveness was decided before the lock, and went stale inside the window")
	}
}

// holdArchivedCompany takes one company's row lock and retires the record, and
// hands the transaction back still open.
//
// Its rollback is registered here rather than left to the caller: a transaction
// still open on a failure path holds the row lock and a pooled connection with
// it, and the run that meant to fail loudly would hang instead.
func (e *dedupeEnv) holdArchivedCompany(ctx context.Context, t *testing.T, companyID ids.UUID) (pgx.Tx, int) {
	t.Helper()
	tx, err := e.store.db.Pool().Begin(ctx)
	if err != nil {
		t.Fatalf("opening the archive's transaction: %v", err)
	}
	t.Cleanup(func() {
		err := tx.Rollback(context.Background())
		if err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("releasing the archive's transaction: %v", err)
		}
	})
	var pid int
	if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatalf("reading the archive's backend pid: %v", err)
	}
	// Through the store's own lock, so a change to how this row is locked can
	// never leave this transaction holding something no adoption waits on.
	if _, err := storekit.LockRow(ctx, tx, entityCompany, companyID, storekit.LiveOnly); err != nil {
		t.Fatalf("taking the company's row lock: %v", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE company SET archived_at = now() WHERE id = $1`, companyID); err != nil {
		t.Fatalf("archiving the company: %v", err)
	}
	return tx, pid
}

// mustFinishWithin answers the waiting writer's result, or fails rather than
// letting a goroutine that never returns become a package-level timeout with a
// stack naming nothing.
func mustFinishWithin(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(30 * time.Second):
		t.Fatal("the adoption never finished after the lock was released")
		return nil
	}
}
