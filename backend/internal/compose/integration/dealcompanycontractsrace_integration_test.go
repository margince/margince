// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A deal company move and a contract filing on the same deal, at once. The
// move locks the deal row FOR UPDATE and the filing FOR SHARE, so the second
// one waits, sees what the first committed, and refuses.
//
// Each ordering is pinned by a row this file holds. Each step waits until
// pg_stat_activity shows the racer blocked on the expected lock.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contracts"
	"github.com/margince/margince/backend/internal/platform/testdb"
)

// The statements each racer blocks on, as pg_stat_activity shows them.
// Patterns rather than full texts, because the row-scope clause in the middle
// varies by caller.
const (
	moveWaitsOnTheNewCompany = `SELECT 1 FROM company WHERE id = %FOR SHARE`
	filingWaitsOnTheDeal     = `SELECT 1 FROM deal WHERE id = %FOR SHARE`
	filingWaitsToInsert      = `%INSERT INTO contract %`
	moveWaitsOnTheDeal       = `SELECT id FROM deal WHERE id = %FOR UPDATE`
	holdTheNewCompany        = `SELECT 1 FROM company WHERE id = $1 FOR UPDATE`
	holdContractInserts      = `LOCK TABLE contract IN SHARE ROW EXCLUSIVE MODE`
)

// The move locks the deal first. A filing for Acme that arrives meanwhile
// waits on the deal, then reads the company the move committed, and is refused.
func TestAContractFiledDuringADealCompanyMoveSeesTheNewCompany(t *testing.T) {
	f := newCompanyMoveFixture(t)
	admin := f.e.Admin()
	deal := f.dealFor(t, nil, nil)
	release := holdRow(t, holdTheNewCompany, f.contoso.UUID)

	moved := make(chan error, 1)
	moveReturned := make(chan struct{})
	go func() {
		defer close(moveReturned)
		moved <- f.moveTo(admin, deal, f.contoso)
	}()
	waitUntilAStatementWaits(t, moveWaitsOnTheNewCompany, moveReturned,
		"the move returned before it reached the company it moves to",
		"the move never queued on the held company, so it was not inside its transaction yet")

	filed := make(chan error, 1)
	filingReturned := make(chan struct{})
	go func() {
		defer close(filingReturned)
		_, err := f.e.Contracts.CreateContract(admin, contracts.CreateContractInput{
			CompanyID: f.acme, DealID: &deal, Title: "Acme MSA",
			ValueBasis: contracts.BasisTotal, Source: "manual",
		})
		filed <- err
	}()
	waitUntilAStatementWaits(t, filingWaitsOnTheDeal, filingReturned,
		"the filing finished while the move held the deal, so the move did not lock the deal row",
		"the filing never queued on the deal row the move holds")
	release()

	if err := <-moved; err != nil {
		t.Fatalf("the move with nothing filed yet was refused: %v", err)
	}
	var crossed *contracts.CrossCompanyLinkError
	if err := <-filed; !errors.As(err, &crossed) || crossed.Field != "deal_id" {
		t.Fatalf("the filing after the move answered %v, want the cross-company refusal on deal_id", err)
	}
	if n := f.e.WsCount(t, `SELECT count(*) FROM contract WHERE deal_id = $1`, deal); n != 0 {
		t.Errorf("%d contract(s) for Acme landed on a deal that now names Contoso", n)
	}
}

// The filing locks the deal first. A move that arrives meanwhile waits on the
// deal, then reads the contract the filing committed, and is refused.
func TestADealCompanyMoveDuringAContractFilingSeesTheContract(t *testing.T) {
	f := newCompanyMoveFixture(t)
	admin := f.e.Admin()
	deal := f.dealFor(t, nil, nil)
	release := holdRow(t, holdContractInserts)

	filed := make(chan error, 1)
	filingReturned := make(chan struct{})
	go func() {
		defer close(filingReturned)
		_, err := f.e.Contracts.CreateContract(admin, contracts.CreateContractInput{
			CompanyID: f.acme, DealID: &deal, Title: "Acme MSA",
			ValueBasis: contracts.BasisTotal, Source: "manual",
		})
		filed <- err
	}()
	waitUntilAStatementWaits(t, filingWaitsToInsert, filingReturned,
		"the filing returned before it reached its insert",
		"the filing never queued on the held contract table")

	moved := make(chan error, 1)
	moveReturned := make(chan struct{})
	go func() {
		defer close(moveReturned)
		moved <- f.moveTo(admin, deal, f.contoso)
	}()
	waitUntilAStatementWaits(t, moveWaitsOnTheDeal, moveReturned,
		"the move finished while the filing held the deal, so the filing did not lock the deal row",
		"the move never queued on the deal row the filing holds")
	release()

	if err := <-filed; err != nil {
		t.Fatalf("the filing on a deal with no company was refused: %v", err)
	}
	if _, ok := errors.AsType[*contracts.DealContractsCrossCompanyError](<-moved); !ok {
		t.Fatal("the move after the filing was admitted, so Acme's contract now reads through Contoso's deal")
	}
	f.dealStillNames(t, deal, nil)
}

// waitUntilAStatementWaits waits until a backend of this test's database is
// blocked on a lock while running a statement that matches pattern.
func waitUntilAStatementWaits(t *testing.T, pattern string, racerReturned <-chan struct{}, finishedEarly, missed string) {
	t.Helper()
	conn := freshConnection(t)
	testdb.WaitForContention(t, racerReturned, finishedEarly, missed, func(ctx context.Context) (bool, error) {
		// pg_stat_activity is cached per transaction until cleared, so each
		// look clears it first.
		if _, err := conn.Exec(ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
			return false, err
		}
		var waiting bool
		err := conn.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM pg_stat_activity
			                WHERE datname = current_database() AND wait_event_type = 'Lock'
			                  AND query LIKE $1)`, pattern).Scan(&waiting)
		return waiting, err
	})
}
