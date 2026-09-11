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

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/magic"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
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
	noDeals.Objects = map[string]principal.ObjectGrant{"contact": {Read: true}}
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

// AN ERASED RECORD'S PRE-SCRUB IMAGES STAY BURIED.
//
// audit_log is append-only, so a contact erased under Art. 17 or anonymized by
// retention keeps every image written before the scrub — with their real name,
// email and phone still in it. The record row survives too: anonymize works IN
// PLACE and archives rather than deletes, so it still satisfies the scope clause
// for its original owner.
//
// Without the erasure boundary a receipt over a long window republishes exactly
// what the erasure certified destroyed. This read applies
// privacy.UnscrubbedImageSQL rather than a predicate of its own, because that
// helper's own doc comment asks every reader of the boundary to call it.
func TestAnErasedRecordsImagesAreNotRepublishedByTheReceipt(t *testing.T) {
	e := Setup(t)
	ctx := context.Background()
	since := time.Now().Add(-time.Hour)

	contact := ids.NewV7()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO contact (id, owner_id, full_name, source, captured_by)
			VALUES ($1, $2, 'Dana Buyer', 'manual', 'human:x')`, contact, e.Rep1); err != nil {
			return err
		}
		// A machine updated them while they were still a contact.
		if _, err := tx.Exec(ctx, `
			INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after, occurred_at)
			VALUES ('agent', 'agent:enrich', 'update', 'contact', $1,
			        '{"full_name":"D. Buyer"}', '{"full_name":"Dana Buyer"}', now() - interval '10 minutes')`,
			contact); err != nil {
			return err
		}
		// And then they were erased. The scrub is AFTER the row above.
		_, err := tx.Exec(ctx, `
			INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, occurred_at)
			VALUES ('human', 'human:dpo', 'erase', 'contact', $1, now() - interval '1 minute')`, contact)
		return err
	})
	if err != nil {
		t.Fatalf("seeding the erased contact: %v", err)
	}

	svc := magic.NewService(e.Pool, nil, time.Now)
	receipt, err := svc.Read(e.As(e.Rep1, []ids.UUID{e.Team1}, contactRepPerms()), &since, 20)
	if err != nil {
		t.Fatalf("reading the receipt: %v", err)
	}

	for _, line := range receipt.Done {
		if line.Entity != nil && ids.UUID(line.Entity.Id) == contact {
			t.Fatalf("an erased contact's pre-scrub image came back on the receipt: %+v", line)
		}
	}
}

// The other half: a record that was NOT erased still reports. Without this the
// fix above could be "serve nothing" and the test beside it would pass.
func TestAnUnerasedRecordsMachineActionStillReports(t *testing.T) {
	e := Setup(t)
	ctx := context.Background()
	since := time.Now().Add(-time.Hour)

	contact := ids.NewV7()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO contact (id, owner_id, full_name, source, captured_by)
			VALUES ($1, $2, 'Ines Bauer', 'manual', 'human:x')`, contact, e.Rep1); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, occurred_at)
			VALUES ('agent', 'agent:enrich', 'update', 'contact', $1, now() - interval '10 minutes')`, contact)
		return err
	})
	if err != nil {
		t.Fatalf("seeding the contact: %v", err)
	}

	svc := magic.NewService(e.Pool, nil, time.Now)
	receipt, err := svc.Read(e.As(e.Rep1, []ids.UUID{e.Team1}, contactRepPerms()), &since, 20)
	if err != nil {
		t.Fatalf("reading the receipt: %v", err)
	}

	var found bool
	for _, line := range receipt.Done {
		if line.Entity != nil && ids.UUID(line.Entity.Id) == contact {
			found = true
		}
	}
	if !found {
		t.Fatal("an unerased record's machine action was withheld — the erasure " +
			"boundary beside this proves nothing if the read returns nothing at all")
	}
}

// THE WINDOW HAS A FLOOR. Without one, any authenticated seat passes since=1970
// and pages the whole ledger back to installation — a morning receipt turned
// into an arbitrary historical audit read, which is what makes the erasure gap
// above reachable at all rather than a same-day race.
func TestTheWindowRefusesToReachBackToInstallation(t *testing.T) {
	e := Setup(t)
	ctx := context.Background()
	ancient := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)

	contact := ids.NewV7()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO contact (id, owner_id, full_name, source, captured_by)
			VALUES ($1, $2, 'Long ago', 'manual', 'human:x')`, contact, e.Rep1); err != nil {
			return err
		}
		// Older than any floor this surface would accept.
		_, err := tx.Exec(ctx, `
			INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, occurred_at)
			VALUES ('agent', 'agent:enrich', 'update', 'contact', $1, now() - interval '200 days')`, contact)
		return err
	})
	if err != nil {
		t.Fatalf("seeding the ancient action: %v", err)
	}

	svc := magic.NewService(e.Pool, nil, time.Now)
	receipt, err := svc.Read(e.As(e.Rep1, []ids.UUID{e.Team1}, contactRepPerms()), &ancient, 20)
	if err != nil {
		t.Fatalf("reading the receipt: %v", err)
	}

	if !receipt.Since.After(ancient) {
		t.Errorf("the window starts at %v, which is what the caller asked for — "+
			"a receipt that reaches back to installation is a historical audit read",
			receipt.Since)
	}
	for _, line := range receipt.Done {
		if line.Entity != nil && ids.UUID(line.Entity.Id) == contact {
			t.Fatal("an action from 200 days ago reached a morning receipt")
		}
	}
}

// contactRepPerms is a rep who reads contacts. RepPerms already does; this names
// the grant the cases above depend on rather than leaving it implied.
func contactRepPerms() principal.Permissions {
	return RepPerms
}

// judgeStub answers the undo question without the real evaluator behind it.
//
// The wiring is what this proves — that a bound judge's answer reaches the line
// — not the evaluator's judgment, which has its own suite. A stub keeps the two
// questions apart: a failure here means the seam is unbound or the answer is
// dropped, never that some refusal rule moved.
type judgeStub struct {
	undoable bool
	reason   string
	asked    int
}

func (j *judgeStub) JudgeUndoPage(
	_ context.Context, _ pgx.Tx, subjects []magic.UndoSubject,
) (map[ids.UUID]*crmcontracts.MagicUndo, error) {
	out := make(map[ids.UUID]*crmcontracts.MagicUndo, len(subjects))
	for _, subject := range subjects {
		j.asked++
		if j.undoable {
			id := openapi_types.UUID(subject.AuditID)
			out[subject.AuditID] = &crmcontracts.MagicUndo{Undoable: true, AuditId: &id}
			continue
		}
		reason := j.reason
		out[subject.AuditID] = &crmcontracts.MagicUndo{Undoable: false, Reason: &reason}
	}
	return out, nil
}

// The done lane used to hardcode Undoable: false on every row, which was true
// by accident — no path could reverse a sweep's correction, so a blanket no was
// not wrong. It is wrong now that corrections carry their own undo, and a
// receipt that says "the machine changed your deal" while greying out the only
// control answering that is worse than one that never mentioned the change.
func TestTheReceiptAsksWhetherEachChangeCanBeTakenBack(t *testing.T) {
	e := Setup(t)
	since := time.Now().Add(-time.Hour)
	seedMachineAction(t, e, e.Rep1, "agent", "agent:auto-apply", "advance_stage")
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)

	judge := &judgeStub{undoable: true}
	receipt, err := magic.NewService(e.Pool, nil, time.Now).
		WithUndoJudge(judge).Read(ctx, &since, 20)
	if err != nil {
		t.Fatalf("reading the receipt: %v", err)
	}
	if len(receipt.Done) == 0 {
		t.Fatal("the seeded machine action did not reach the done lane")
	}
	if judge.asked != len(receipt.Done) {
		t.Errorf("judged %d lines of %d — every drawn line is asked", judge.asked, len(receipt.Done))
	}
	line := receipt.Done[0]
	if line.Undo == nil || !line.Undo.Undoable {
		t.Fatalf("undo = %+v, want undoable", line.Undo)
	}
	// A client draws the control from audit_id; one without it has nothing to
	// send, which is the state the contract's own comment forbids.
	if line.Undo.AuditId == nil {
		t.Error("an undoable line carries no audit id for the control to name")
	}

	// A refusal carries its reason rather than a blank: the reason is what the
	// client renders beside the greyed control.
	refusing := &judgeStub{undoable: false, reason: "record_archived"}
	refused, err := magic.NewService(e.Pool, nil, time.Now).
		WithUndoJudge(refusing).Read(ctx, &since, 20)
	if err != nil {
		t.Fatal(err)
	}
	got := refused.Done[0].Undo
	if got == nil || got.Undoable || got.Reason == nil || *got.Reason != "record_archived" {
		t.Errorf("refused undo = %+v, want undoable=false carrying the reason", got)
	}
	if got != nil && got.AuditId != nil {
		t.Error("a refused line carries an audit id, so a client could draw a control that only fails")
	}
}

// An installation that has not wired the judge says so, rather than reading as
// "this cannot be undone" — the product did not look, which is a different
// thing and must not be presented as the same.
func TestAnUnwiredUndoJudgeSaysItDidNotLook(t *testing.T) {
	e := Setup(t)
	since := time.Now().Add(-time.Hour)
	seedMachineAction(t, e, e.Rep1, "agent", "agent:auto-apply", "advance_stage")
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)

	receipt, err := magic.NewService(e.Pool, nil, time.Now).Read(ctx, &since, 20)
	if err != nil {
		t.Fatal(err)
	}
	undo := receipt.Done[0].Undo
	if undo == nil || undo.Undoable || undo.Reason == nil || *undo.Reason != "undo_not_evaluated" {
		t.Errorf("unwired undo = %+v, want a stated not-evaluated reason", undo)
	}
}
