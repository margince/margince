// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package company360

// Signal-derived advice end to end, over a real database: the conflict between
// what the record says and what its own correspondence says, and the one signal
// the state strip has room to state.
//
// Every fixture sets its timestamps explicitly. The read's clock is pinned to
// company360Clock while the database's now() is not, so a fixture on now() would
// land on the wrong side of a stale-thread window by accident.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAnUnprovenContractEndingStaysASignalWithoutAnAccountConflict(t *testing.T) {
	e := integration.Setup(t)
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Acme", &e.Rep1))
	e.WsExec(t, `UPDATE company SET lifecycle='customer' WHERE id=$1`, company.UUID)
	seedSignal(t, company.UUID, "contract_ended", "warn", "They wrote that one contract ends.", "2026-05-20T09:00:00Z")
	view, err := company360Service(e).Assemble(e.As(e.Rep1, []ids.UUID{e.Team1}, company360SignalPerms), company)
	if err != nil {
		t.Fatal(err)
	}
	for _, suggestion := range *view.Suggestions {
		if string(suggestion.Kind) == "lifecycle_conflict" {
			t.Fatal("unproven scope was presented as an account conflict")
		}
	}
	if view.StateStrip == nil || view.StateStrip.Signal == nil || view.StateStrip.Signal.Kind != "contract_ended" {
		t.Fatal("uncertain evidence lost its reviewable signal")
	}
}

// A stage that already reads as over is not in conflict with the mail that
// says so; it is that mail's conclusion. Firing there would hand every closed
// account a permanent card nobody can clear.
func TestAnEndedContractDoesNotContradictAnEndedRelationship(t *testing.T) {
	e := integration.Setup(t)
	svc := company360Service(e)
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Acme", &e.Rep1))
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, company360SignalPerms)

	e.WsExec(t, `UPDATE company SET lifecycle = 'former_customer' WHERE id = $1`, company.UUID)
	seedSignal(t, company.UUID, "contract_ended", "warn",
		"They wrote that the contract ends on 31 July.", "2026-05-20T09:00:00Z")

	view, err := svc.Assemble(rep, company)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	for _, suggestion := range *view.Suggestions {
		if string(suggestion.Kind) == "lifecycle_conflict" {
			t.Fatalf("the card claims a conflict on an account already filed as former_customer")
		}
	}
}

// kindsOf names what a card actually offered, so a failure says what was there
// instead of only what was missing.
func kindsOf(found []crmcontracts.Company360Suggestion) []string {
	kinds := make([]string, 0, len(found))
	for _, suggestion := range found {
		kinds = append(kinds, string(suggestion.Kind))
	}
	return kinds
}

// A caller who may not read signals is told nothing about them, not told there
// is nothing. The rule reads a record the reader has no right to, so it stays
// silent rather than leaking its existence through advice.
func TestTheConflictStaysSilentWithoutTheSignalGrant(t *testing.T) {
	e := integration.Setup(t)
	svc := company360Service(e)
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Acme", &e.Rep1))

	e.WsExec(t, `UPDATE company SET lifecycle = 'customer' WHERE id = $1`, company.UUID)
	seedSignal(t, company.UUID, "contract_ended", "warn",
		"They wrote that the contract ends on 31 July.", "2026-05-20T09:00:00Z")

	// integration.AccountRepPerms carries no signal grant, which is the point.
	view, err := svc.Assemble(e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms), company)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	for _, suggestion := range *view.Suggestions {
		if string(suggestion.Kind) == "lifecycle_conflict" {
			t.Fatalf("a caller without the signal grant was shown advice derived from one")
		}
	}
}

// seedSignal writes one open derived signal at a severity and an instant.
func seedSignal(t *testing.T, company ids.UUID, kind, severity, summary, at string) {
	t.Helper()
	if _, err := integration.OwnerConn(t).Exec(context.Background(), `INSERT INTO signal
		(id, kind, source_channel, entity_type, entity_id, resolved_company_id,
		 resolution_state, severity, summary, status, detected_at, source, captured_by)
		VALUES ($1, '`+kind+`', 'derived', 'company', '`+company.String()+`',
		        '`+company.String()+`', 'resolved', '`+severity+`', '`+summary+`', 'open',
		        '`+at+`', 'signal-scan', 'agent:`+kind+`')`, ids.NewV7()); err != nil {
		t.Fatalf("seeding a signal: %v", err)
	}
}

// The strip has room for one signal and the account may have several. It
// states the most serious, and among equals the newest — an old warning that
// has been sitting there is less news than the one that just arrived.
func TestTheStripStatesTheWorstOpenSignal(t *testing.T) {
	e := integration.Setup(t)
	svc := company360Service(e)
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Acme", &e.Rep1))
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, company360SignalPerms)

	seedSignal(t, company.UUID, "commitment_made", "info",
		"They promised volumes by Friday.", "2026-05-20T09:00:00Z")
	seedSignal(t, company.UUID, "ghosted_thread", "warn",
		"We wrote 20 days ago and nobody has answered.", "2026-05-01T09:00:00Z")
	seedSignal(t, company.UUID, "contract_ended", "warn",
		"They wrote that the contract ends on 31 July.", "2026-05-21T09:00:00Z")

	view, err := svc.Assemble(rep, company)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if view.StateStrip == nil || view.StateStrip.Signal == nil {
		t.Fatal("the strip states no signal on an account carrying three")
	}
	if view.StateStrip.Signal.Kind != "contract_ended" {
		t.Errorf("the strip leads with %q, want the newest of the two warnings",
			view.StateStrip.Signal.Kind)
	}
	if view.StateStrip.Signal.Summary != "They wrote that the contract ends on 31 July." {
		t.Errorf("the strip says %q, want the producer's own sentence",
			view.StateStrip.Signal.Summary)
	}
}

// A caller who may not read signals is told nothing about them — not told
// there is nothing. The tile is absent, and the difference is what the signals
// card is for.
func TestTheStripStatesNoSignalWithoutTheGrant(t *testing.T) {
	e := integration.Setup(t)
	svc := company360Service(e)
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Acme", &e.Rep1))

	seedSignal(t, company.UUID, "contract_ended", "warn",
		"They wrote that the contract ends on 31 July.", "2026-05-21T09:00:00Z")

	// integration.AccountRepPerms carries no signal grant, which is the point.
	view, err := svc.Assemble(e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms), company)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if view.StateStrip != nil && view.StateStrip.Signal != nil {
		t.Fatalf("the strip stated a signal to a caller with no grant to read one")
	}
}
