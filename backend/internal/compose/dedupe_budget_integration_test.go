// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Two composition seams: a captured lead colliding with a live lead
// stages a 🟡 merge proposal (never a second row, never an auto-merge),
// and the AI budget derives live from the workspace's full seats.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/compose/integration"
	"github.com/gradionhq/margince/backend/internal/modules/approvals"
	"github.com/gradionhq/margince/backend/internal/modules/capture"
	"github.com/gradionhq/margince/backend/internal/platform/database"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
	"github.com/gradionhq/margince/backend/internal/shared/ports/connector"
)

func connectorCtx(e *integration.Env) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:test",
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"lead": {Create: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}

func TestCaptureDedupeStagesMergeInsteadOfDuplicating(t *testing.T) {
	e := integration.Setup(t)
	sink := capture.NewSink(e.DB()).WithStager(mergeStager{svc: approvals.NewService(e.DB())})
	ctx := connectorCtx(e)

	first, err := sink.Upsert(ctx, connector.NormalizedRecord{
		EntityType: "lead",
		NaturalKey: connector.NaturalKey{SourceSystem: "apollo", SourceID: "a-1"},
		Fields:     capture.LeadFields{FullName: "Dana Dup", Email: "dana@example.test"},
		Source:     "apollo:a-1", CapturedBy: "connector:test",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Same address from ANOTHER source: no second row, the existing ref
	// answers, and a merge proposal lands in the inbox.
	second, err := sink.Upsert(ctx, connector.NormalizedRecord{
		EntityType: "lead",
		NaturalKey: connector.NaturalKey{SourceSystem: "hubspot", SourceID: "h-9"},
		Fields:     capture.LeadFields{FullName: "Dana Duplicate", Email: "DANA@example.test "},
		Source:     "hubspot:h-9", CapturedBy: "connector:test",
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = second
	err = database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var leads, proposals int
		if err := tx.QueryRow(context.Background(),
			`SELECT count(*) FROM lead WHERE email = 'dana@example.test'`).Scan(&leads); err != nil {
			return err
		}
		if err := tx.QueryRow(context.Background(),
			`SELECT count(*) FROM approval WHERE kind = 'merge_records' AND target_entity_id = $1 AND status = 'pending'`,
			first.ID).Scan(&proposals); err != nil {
			return err
		}
		if leads != 1 || proposals != 1 {
			t.Errorf("dedupe left %d leads and %d proposals, want 1/1", leads, proposals)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// A replay of the ORIGINAL natural key is idempotent, not a
	// self-collision.
	replay, err := sink.Upsert(ctx, connector.NormalizedRecord{
		EntityType: "lead",
		NaturalKey: connector.NaturalKey{SourceSystem: "apollo", SourceID: "a-1"},
		Fields:     capture.LeadFields{FullName: "Dana Dup", Email: "dana@example.test"},
		Source:     "apollo:a-1", CapturedBy: "connector:test",
	})
	if err != nil || replay.ID != first.ID {
		t.Fatalf("replay → %v / %v, want the original row", replay, err)
	}
}

func TestSeatDerivedBudget(t *testing.T) {
	e := integration.Setup(t)
	// The harness seeds four full-seat humans: three reps and the admin seat.
	const seats = 4
	budget, err := NewSeatBudget(e.Pool).MonthlyTokenBudget(context.Background(), ids.From[ids.WorkspaceKind](e.WS))
	if err != nil {
		t.Fatal(err)
	}
	if budget != seats*perSeatBaseTokens*budgetSafetyFactor {
		t.Fatalf("%d-seat budget = %d, want %d", seats, budget, seats*perSeatBaseTokens*budgetSafetyFactor)
	}
	// An installation with no live full seat floors at one rather than
	// refusing. Reached by deactivating the seats, which is how it actually
	// happens: the case this floor exists for is an onboarding flow calling the
	// model before the first seat settles. It used to be reached by pointing
	// the budget at a second, empty workspace — ADR-0091 §8 phase D took the
	// tenant column off app_user, so a seat count is the installation's and
	// there is no empty workspace to ask about.
	owner := integration.OwnerConn(t)
	if _, err := owner.Exec(context.Background(),
		`UPDATE app_user SET status = 'deactivated' WHERE seat_type = 'full'`); err != nil {
		t.Fatalf("deactivating the full seats: %v", err)
	}
	budget, err = NewSeatBudget(e.Pool).MonthlyTokenBudget(context.Background(), ids.From[ids.WorkspaceKind](e.WS))
	if err != nil {
		t.Fatal(err)
	}
	if budget != perSeatBaseTokens*budgetSafetyFactor {
		t.Fatalf("seatless-installation budget = %d, want the single-seat floor", budget)
	}
}

// Accepting a capture collision must CHANGE the lead.
//
// The card asked a real question and discarded the answer: `merge_records` had
// no entry in the effect registry, so approving it marked the approval approved,
// minted a token, and wrote nothing. The test beside this one proved "one lead,
// one pending approval" and passed throughout, because it never decided the
// approval it had just asserted the existence of.
//
// What makes this the honest shape: the proposal is staged by the real capture
// sink, and the verdict goes through the real approvals service with its
// registered effect. Nothing here supplies its own version of either.
func TestAcceptingACaptureCollisionFillsTheLeadsEmptyFields(t *testing.T) {
	e := integration.Setup(t)
	// The production wiring, not a service a test assembled: the registry is
	// exactly what decides whether accepting this card does anything, so a test
	// that registered its own effects would be proving its own setup.
	svc := approvalsServiceWithEffects(e.Pool)
	sink := capture.NewSink(e.DB()).WithStager(mergeStager{svc: svc})
	ctx := connectorCtx(e)

	// The incumbent knows a name and an address, and nothing else.
	first, err := sink.Upsert(ctx, connector.NormalizedRecord{
		EntityType: "lead",
		NaturalKey: connector.NaturalKey{SourceSystem: "apollo", SourceID: "c-1"},
		Fields:     capture.LeadFields{FullName: "Nils Vogel", Email: "nils@example.test"},
		Source:     "apollo:c-1", CapturedBy: "connector:test",
	})
	if err != nil {
		t.Fatalf("capturing the incumbent: %v", err)
	}

	// A second message about the same person, carrying what the first lacked.
	if _, err := sink.Upsert(ctx, connector.NormalizedRecord{
		EntityType: "lead",
		NaturalKey: connector.NaturalKey{SourceSystem: "apollo", SourceID: "c-2"},
		Fields: capture.LeadFields{
			FullName: "Nils Vogel", Email: "nils@example.test",
			CompanyName: "Nordwind Systeme", Title: "Head of Operations",
		},
		Source: "apollo:c-2", CapturedBy: "connector:test",
	}); err != nil {
		t.Fatalf("capturing the collision: %v", err)
	}

	staged := pendingCollisionFor(t, e, first.ID)
	if _, err := svc.Decide(e.Admin(), staged, true, nil); err != nil {
		t.Fatalf("approving the collision: %v", err)
	}

	var company, title *string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT company_name, title FROM lead WHERE id = $1`, first.ID).Scan(&company, &title)
	}); err != nil {
		t.Fatalf("reading the lead back: %v", err)
	}
	if company == nil || *company != "Nordwind Systeme" {
		t.Errorf("company_name is %q; approving the card wrote nothing", derefOr(company, "<unset>"))
	}
	if title == nil || *title != "Head of Operations" {
		t.Errorf("title is %q; approving the card wrote nothing", derefOr(title, "<unset>"))
	}
}

// derefOr reads a nullable column for a failure message. A test that printed
// the pointer would report an address where the reader needs the value.
func derefOr(value *string, absent string) string {
	if value == nil {
		return absent
	}
	return *value
}

// pendingCollisionFor reads the staged proposal's id straight from the table,
// because the id is what the test needs and the queue read is not what it is
// testing.
func pendingCollisionFor(t *testing.T, e *integration.Env, target ids.UUID) ids.ApprovalID {
	t.Helper()
	var id ids.ApprovalID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT id FROM approval WHERE kind = 'merge_records' AND target_entity_id = $1 AND status = 'pending'`,
			target).Scan(&id)
	}); err != nil {
		t.Fatalf("no pending capture-collision proposal for %s: %v", target, err)
	}
	return id
}

// A captured value never overwrites one that is already there.
//
// The incumbent's value may have been typed by a person, and an inbound message
// carries no evidence that it knows better. Without this rule the card would be
// a way for a connector to quietly rewrite a human's work, which is the reason
// it is gated behind a human decision in the first place.
func TestAcceptingACaptureCollisionNeverOverwritesWhatIsAlreadyThere(t *testing.T) {
	e := integration.Setup(t)
	svc := approvalsServiceWithEffects(e.Pool)
	sink := capture.NewSink(e.DB()).WithStager(mergeStager{svc: svc})
	ctx := connectorCtx(e)

	first, err := sink.Upsert(ctx, connector.NormalizedRecord{
		EntityType: "lead",
		NaturalKey: connector.NaturalKey{SourceSystem: "apollo", SourceID: "o-1"},
		Fields: capture.LeadFields{
			FullName: "Ida Brandt", Email: "ida@example.test",
			CompanyName: "Brandt Consulting", Title: "Managing Partner",
		},
		Source: "apollo:o-1", CapturedBy: "connector:test",
	})
	if err != nil {
		t.Fatalf("capturing the incumbent: %v", err)
	}
	// The same person, described differently by a second source.
	if _, err := sink.Upsert(ctx, connector.NormalizedRecord{
		EntityType: "lead",
		NaturalKey: connector.NaturalKey{SourceSystem: "apollo", SourceID: "o-2"},
		Fields: capture.LeadFields{
			FullName: "I. Brandt", Email: "ida@example.test",
			CompanyName: "Brandt GmbH", Title: "Partner",
		},
		Source: "apollo:o-2", CapturedBy: "connector:test",
	}); err != nil {
		t.Fatalf("capturing the collision: %v", err)
	}

	staged := pendingCollisionFor(t, e, first.ID)
	if _, err := svc.Decide(e.Admin(), staged, true, nil); err != nil {
		t.Fatalf("approving the collision: %v", err)
	}

	var name, company, title *string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT full_name, company_name, title FROM lead WHERE id = $1`, first.ID).
			Scan(&name, &company, &title)
	}); err != nil {
		t.Fatalf("reading the lead back: %v", err)
	}
	for _, held := range []struct {
		field string
		got   *string
		want  string
	}{
		{"full_name", name, "Ida Brandt"},
		{"company_name", company, "Brandt Consulting"},
		{"title", title, "Managing Partner"},
	} {
		if held.got == nil || *held.got != held.want {
			t.Errorf("%s is %q, want %q — a captured value overwrote one already there",
				held.field, derefOr(held.got, "<unset>"), held.want)
		}
	}
}
