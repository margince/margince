// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// Adopting a domain asks its authorization UNDER the lock.
//
// The order is the whole test. Asking first and locking second leaves a window
// under READ COMMITTED: a privatization committing inside it is invisible to
// the check that already ran, so a caller the record no longer admits is
// admitted anyway — the interleaving companyvisibility.go describes, where the
// guard passed and the write landed.
//
// The window is driven by hand rather than raced for. One transaction takes the
// company's row lock and privatizes it; the adoption is released only once a
// backend is provably waiting on that lock, so the adoption's authorization
// question is asked after the privatization committed, every run. No sleep, no
// clock, no guess.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAdoptingADomainAsksItsAuthorityUnderTheLock(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	// The incumbent: a company the FIRST rep owns, carrying the name a second
	// domain is about to resolve to.
	incumbent, err := e.store.CreateCompany(ctx, CreateCompanyInput{
		DisplayName: "Adopt Auth GmbH", Source: "manual",
		Domains: []CompanyDomainInput{{Domain: "adoptauth-first.test", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seeding the incumbent: %v", err)
	}
	if _, err := e.store.db.Pool().Exec(ctx,
		`UPDATE company SET owner_id = $2 WHERE id = $1`, incumbent.Id, e.rep); err != nil {
		t.Fatalf("giving the incumbent an owner: %v", err)
	}

	// A second domain of the same name, waiting on a verdict.
	e.openTriageFirst(ctx, t, "someone@adoptauth-second.test", "Some One", "adoptauth-second.test")
	readID := e.startTriageRead(ctx, t, "adoptauth-second.test")

	// A transaction holding the incumbent's row lock, which has privatized it —
	// frozen exactly where the defect lives, before its commit.
	holder, pid := e.beginHeldPrivatization(ctx, t, ids.UUID(incumbent.Id))

	// The adoption runs as a COLLEAGUE confined to their own rows, whom the
	// privatized record will not admit. It must block on the lock above rather
	// than deciding its authorization first.
	done := make(chan error, 1)
	go func() {
		_, err := e.store.ResolveDomainTriage(e.asConfinedColleague(), ResolveDomainTriageInput{
			Domain: "adoptauth-second.test", Status: DomainCompany, Source: DomainSourceSiteRead,
			ReadID: readID, DossierName: "Adopt Auth GmbH", SeedURL: "https://adoptauth-second.test",
		})
		done <- err
	}()
	mustBlockOn(t, holder, pid, done)

	if err := holder.Commit(ctx); err != nil {
		t.Fatalf("committing the privatization: %v", err)
	}
	// Whatever the adoption answers, it must not have written a domain onto a
	// record that, by the time it held the lock, no longer admitted it. A
	// refusal reads as not-found, which is how the row scope hides existence.
	if err := <-done; err != nil && !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("the adoption failed for an unexpected reason: %v", err)
	}

	var adopted bool
	if err := e.store.db.Pool().QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM company_domain
		   WHERE company_id = $1 AND domain = 'adoptauth-second.test' AND archived_at IS NULL)`,
		incumbent.Id).Scan(&adopted); err != nil {
		t.Fatalf("reading the incumbent's domains: %v", err)
	}
	if adopted {
		t.Error("the domain landed on a company the caller was no longer admitted to write: " +
			"the authorization was decided before the lock, and went stale inside the window")
	}
}

// beginHeldPrivatization opens a transaction that has taken one company's row
// lock and made the record owner-private, and hands it back still open — so the
// test owns the moment another writer can see it.
//
// Its rollback is registered here rather than left to the caller: a transaction
// still open on a failure path holds the row lock and a pooled connection with
// it, and the run that meant to fail loudly would hang instead.
func (e *dedupeEnv) beginHeldPrivatization(ctx context.Context, t *testing.T, companyID ids.UUID) (pgx.Tx, int) {
	t.Helper()
	tx, err := e.store.db.Pool().Begin(ctx)
	if err != nil {
		t.Fatalf("opening the privatization's transaction: %v", err)
	}
	t.Cleanup(func() {
		err := tx.Rollback(context.Background())
		if err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("releasing the privatization's transaction: %v", err)
		}
	})
	var pid int
	if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatalf("reading the privatization's backend pid: %v", err)
	}
	// Through the store's own lock, so a change to how this row is locked can
	// never leave this transaction holding something no adoption waits on.
	if _, err := storekit.LockRow(ctx, tx, entityCompany, companyID, storekit.LiveOnly); err != nil {
		t.Fatalf("taking the company's row lock: %v", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE company SET visibility = 'owner' WHERE id = $1`, companyID); err != nil {
		t.Fatalf("privatizing the company: %v", err)
	}
	return tx, pid
}

// asConfinedColleague is the second rep, confined to the rows they own.
//
// asOther() carries RowScopeAll, which admits every row and would pass the
// write check whatever the privatization did — so this test needs its own
// principal, or it would assert nothing.
func (e *dedupeEnv) asConfinedColleague() context.Context {
	other := e.otherRep
	ctx := principal.WithCorrelationID(
		principal.WithWorkspaceID(context.Background(), e.ws), ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + other.String(), UserID: other,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects: map[string]principal.ObjectGrant{
				"contact": {Create: true, Read: true, Update: true},
				"company": {Create: true, Read: true, Update: true},
			},
			RowScope: principal.RowScopeOwn,
		},
	})
}
