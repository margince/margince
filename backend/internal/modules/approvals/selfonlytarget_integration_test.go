// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

// The self-only narrowing on the TWO target-filtered reads, against a real
// database — the property the unit suite cannot reach, because both reads open
// with a target-visibility probe that needs the row to exist.
//
// The inbox scan applies decidable() per row and always narrowed correctly. The
// target-filtered reads settle target visibility once for the record and then
// filter each row themselves, and both filtered on the decision grants alone: a
// colleague holding person.update and able to read the contact received another
// member's linkedin_match in full — the connection's real name and employer out
// of a private address book, third parties who never agreed to be in this CRM.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seatFor is the colleague seat the leak was read from: person grants outright
// and row scope over every row, which is what a manager, ops or management grid
// actually holds. If this seat cannot see the row, no weaker one can.
func seatFor(ws, user ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{
			RowScope: principal.RowScopeAll,
			Objects: map[string]principal.ObjectGrant{
				tablePerson: {Create: true, Read: true, Update: true, Delete: true},
			},
		},
	})
}

func TestASelfOnlyStagingIsAbsentFromAColleaguesTargetFilteredReads(t *testing.T) {
	e := setupStaging(t)
	ctx := context.Background()

	// The colleague: a real row, because on_behalf_of is a foreign key and a
	// fabricated id would be refused by the database before any assertion runs.
	colleague := ids.NewV7()
	if _, err := e.owner.Exec(ctx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Colleague')`,
		colleague, "colleague-"+colleague.String()+"@st.test"); err != nil {
		t.Fatalf("seeding the colleague: %v", err)
	}
	// The contact the match is staged against. Both reads open with a
	// target-visibility probe, so a target that does not exist would answer
	// empty for a reason that has nothing to do with the narrowing under test.
	target := ids.NewV7()
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO person (id, full_name, source, captured_by)
		VALUES ($1, 'Jan Dow', 'linkedin:seed', 'connector:linkedin')`, target); err != nil {
		t.Fatalf("seeding the contact: %v", err)
	}

	// Staged through the REAL writer, on the shape the importer produces: an
	// agent acting for e.rep, which is what stamps on_behalf_of with a person
	// rather than leaving it NULL. A test that wrote the row itself would be
	// asserting over a shape production does not make.
	staged, err := e.svc.Stage(e.asAgent(t), StageInput{
		Kind:           kindLinkedInMatch,
		ProposedChange: []byte(`{"connection_name":"Jane Doe","connection_company":"Contoso GmbH"}`),
		DiffHash:       "linkedin-match-" + target.String(),
		TargetType:     tablePerson,
		TargetID:       target,
		Summary:        "Jane Doe at Contoso GmbH looks like Jan Dow",
	})
	if err != nil {
		t.Fatalf("staging the match: %v", err)
	}

	targetType := tablePerson
	read := func(t *testing.T, as context.Context) (int, int) {
		t.Helper()
		listed, _, listErr := e.svc.List(as, ListInput{TargetType: &targetType, TargetID: &target})
		if listErr != nil {
			t.Fatalf("List(targeted): %v", listErr)
		}
		var panel int
		if err := e.svc.db.Tx(as, func(tx pgx.Tx) error {
			out, pendErr := e.svc.PendingForTarget(as, tx, targetType, target, PendingScanCap)
			panel = len(out)
			return pendErr
		}); err != nil {
			t.Fatalf("PendingForTarget: %v", err)
		}
		return len(listed), panel
	}

	// THE LEAK. The colleague holds every person grant and every row, so
	// requireDecisionGrants and targetVisible both pass — and the self-only
	// narrowing is the only thing standing between them and the row.
	if listed, panel := read(t, seatFor(e.ws, colleague)); listed != 0 || panel != 0 {
		t.Errorf("a colleague read %d row(s) from List(targeted) and %d from PendingForTarget for a "+
			"linkedin_match staged for somebody else — the staged change carries a third party's real "+
			"name and employer out of that member's private address book", listed, panel)
	}

	// THE POSITIVE CONTROL, and it is what makes the assertion above mean
	// anything: without it this test also passes when both reads are broken and
	// return nothing to anyone.
	listed, panel := read(t, seatFor(e.ws, e.rep))
	if listed != 1 || panel != 1 {
		t.Errorf("the member it was staged for read %d row(s) from List(targeted) and %d from "+
			"PendingForTarget, want 1 and 1 — the narrowing withholds the proposal from its own subject",
			listed, panel)
	}
	if listed == 1 {
		// The row is the one staged here rather than some other pending proposal
		// against the same contact, so "1" is this proposal and not a coincidence.
		rows, _, err := e.svc.List(seatFor(e.ws, e.rep), ListInput{TargetType: &targetType, TargetID: &target})
		if err != nil {
			t.Fatalf("re-reading the subject's own list: %v", err)
		}
		if rows[0].ID != staged {
			t.Errorf("the subject's list carries %v, want the staged match %v", rows[0].ID, staged)
		}
	}
}
