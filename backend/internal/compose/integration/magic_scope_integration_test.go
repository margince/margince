// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The machinery's receipt, scoped to the reader.
//
// THE HAZARD THIS SUITE EXISTS FOR. audit_log carries no workspace_id —
// migration 1787320004 dropped the tenant column from the append-only ledgers,
// and its own comment says no table carries one after it. So an audit row cannot
// be scoped by itself: it is placed by joining the table its entity_type names,
// under that table's own gate.
//
// Get that wrong and the failure is SILENT. A row about another rep's deal looks
// exactly like a row about your own — same shape, same sentence, a name you were
// never meant to read. Nothing fails, nothing logs, and the receipt reports it as
// yours.
//
// At the database rather than over a stub, because the predicate is SQL. A unit
// test can prove the query is assembled; only Postgres can prove that
// auth.ScopeClauseFor puts this reader outside that row.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/magic"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestTheReceiptCarriesTheSameReachTheRecordItselfDoes(t *testing.T) {
	e := Setup(t)
	since := time.Now().Add(-time.Hour)
	// Another team's deal, moved by a machine.
	theirs := seedMachineAction(t, e, e.Rep3, "agent", "agent:auto-apply", "advance_stage")

	svc := magic.NewService(e.Pool, nil, time.Now)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	receipt, err := svc.Read(ctx, &since, 20)
	if err != nil {
		t.Fatalf("reading the receipt: %v", err)
	}

	// A DEAL IS WORKSPACE-READABLE in this product: auth.UnboundedFor answers
	// true for a rep on `deal`, so ScopeClauseFor renders no predicate and every
	// seat holding deal.read sees every deal. The receipt must carry exactly
	// that reach — no wider, and no narrower.
	//
	// Narrower would be the tempting mistake. A receipt that quietly showed only
	// your own deals would look like good security and would be a lie about what
	// the machinery did to the pipeline you can already open. The gate this
	// suite protects is that the receipt inherits the RECORD's rule rather than
	// inventing one.
	var found bool
	for _, line := range receipt.Done {
		if line.Entity != nil && ids.UUID(line.Entity.Id) == theirs {
			found = true
		}
	}
	if !found {
		t.Fatal("a machine action on a deal this reader may open was withheld — the " +
			"receipt narrowed where the record does not, which hides what the " +
			"machinery did to pipeline the reader is answerable for")
	}
}

// The reach the receipt must NOT exceed: a record the reader cannot open at all.
//
// A seat with no deal grant reads no deal, so it reads no machine action about
// one either. This is the arm that would fail if the join were dropped and the
// audit rows served on their own — audit_log carries no workspace_id, so an
// unjoined read is scoped by nothing whatsoever.
func TestASeatThatCannotReadDealsSeesNoMachineActionOnOne(t *testing.T) {
	e := Setup(t)
	since := time.Now().Add(-time.Hour)
	mine := seedMachineAction(t, e, e.Rep1, "agent", "agent:auto-apply", "advance_stage")

	// The same rep, one grant fewer.
	noDeals := RepPerms
	noDeals.Objects = map[string]principal.ObjectGrant{"person": {Read: true}}
	svc := magic.NewService(e.Pool, nil, time.Now)
	receipt, err := svc.Read(e.As(e.Rep1, []ids.UUID{e.Team1}, noDeals), &since, 20)
	if err != nil {
		t.Fatalf("reading the receipt: %v", err)
	}

	for _, line := range receipt.Done {
		if line.Entity != nil && ids.UUID(line.Entity.Id) == mine {
			t.Fatalf("a seat that may not read deals was served a machine action on one: %+v", line)
		}
	}
}

// And the other half, without which the refusal above proves nothing: a machine
// action on the reader's OWN deal does reach them. A scoping bug that returned
// nothing at all would pass the test above and take the feature with it.
func TestARepSeesTheMachineActionsOnTheirOwnRecords(t *testing.T) {
	e := Setup(t)
	since := time.Now().Add(-time.Hour)
	mine := seedMachineAction(t, e, e.Rep1, "agent", "agent:auto-apply", "advance_stage")

	svc := magic.NewService(e.Pool, nil, time.Now)
	receipt, err := svc.Read(e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms), &since, 20)
	if err != nil {
		t.Fatalf("reading the receipt: %v", err)
	}

	var found bool
	for _, line := range receipt.Done {
		if line.Entity != nil && ids.UUID(line.Entity.Id) == mine {
			found = true
		}
	}
	if !found {
		t.Fatal("a rep was not shown a machine action on their own deal — the refusal " +
			"beside this proves nothing if the read returns nothing at all")
	}
}

// A HUMAN's own change is never reported back as machinery. This surface says
// what ran WITHOUT being asked, and handing a rep their own edit back under that
// heading is a lie about who did it.
func TestAHumansOwnChangeIsNotOnTheReceipt(t *testing.T) {
	e := Setup(t)
	since := time.Now().Add(-time.Hour)
	seedMachineAction(t, e, e.Rep1, "human", "human:someone", "advance_stage")

	svc := magic.NewService(e.Pool, nil, time.Now)
	receipt, err := svc.Read(e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms), &since, 20)
	if err != nil {
		t.Fatalf("reading the receipt: %v", err)
	}

	if len(receipt.Done) != 0 {
		t.Fatalf("a rep's own change was reported back as machinery: %+v", receipt.Done)
	}
}

// A machine write with no customer-facing meaning is not Magic. An export sweep
// is a machine write, and showing it would turn housekeeping into apparent
// value.
func TestAMachineHousekeepingWriteIsNotOnTheReceipt(t *testing.T) {
	e := Setup(t)
	since := time.Now().Add(-time.Hour)
	seedMachineAction(t, e, e.Rep1, "system", "system:export", "export")

	svc := magic.NewService(e.Pool, nil, time.Now)
	receipt, err := svc.Read(e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms), &since, 20)
	if err != nil {
		t.Fatalf("reading the receipt: %v", err)
	}

	if len(receipt.Done) != 0 {
		t.Fatalf("machine housekeeping was reported as Magic: %+v", receipt.Done)
	}
}

// seedMachineAction puts a deal and one audit row about it in place.
//
// Through WithWorkspaceTx under the admin context, which is how this harness
// seeds: the GUC binding is what every read below runs against, and a fixture
// written outside it would sit in a workspace the reads cannot see.
func seedMachineAction(
	t *testing.T, e *Env, owner ids.UUID, actorType, actorID, action string,
) ids.UUID {
	t.Helper()
	pipeline, stage, deal := ids.NewV7(), ids.NewV7(), ids.NewV7()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if _, err := tx.Exec(ctx, `INSERT INTO pipeline (id, name, is_default, position)
			VALUES ($1, 'Magic fixture', false, 91)`, pipeline); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
			VALUES ($1, $2, 'Qualify', 0, 'open', 10)`, stage, pipeline); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO deal (id, owner_id, name, pipeline_id, stage_id, source, captured_by)
			VALUES ($1, $2, 'Magic fixture deal', $3, $4, 'manual', 'human:x')`,
			deal, owner, pipeline, stage); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, occurred_at)
			VALUES ($1, $2, $3, 'deal', $4, now())`, actorType, actorID, action, deal)
		return err
	})
	if err != nil {
		t.Fatalf("seeding the machine action: %v", err)
	}
	return deal
}
