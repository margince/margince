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
// filter each row themselves, and both once filtered on the decision grants
// alone: a colleague holding the target's grants and able to read the target
// received another member's self-only proposal in full.
//
// The kind here is a HELD DRAFT, and the choice is the assertion's substance
// rather than a fixture convenience: releasing one sends it, stamped with the
// approving human's own credential, display name and signature. A colleague who
// released a rep's draft would not have authorised the rep's message — they
// would have sent their own, into a customer thread they were never part of.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seatFor is the colleague seat the leak was read from: the target type's
// grants outright and row scope over every row, which is what a manager, ops or
// management grid actually holds. If this seat cannot see the row, no weaker
// one can.
func seatFor(ws, user ids.UUID, objects ...string) context.Context {
	if len(objects) == 0 {
		objects = []string{tableContact}
	}
	grants := make(map[string]principal.ObjectGrant, len(objects))
	for _, object := range objects {
		grants[object] = principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}
	}
	ctx := principal.WithWorkspaceID(context.Background(), ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll, Objects: grants},
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
	// The deal the draft answers. Both reads open with a target-visibility
	// probe, so a target that does not exist would answer empty for a reason
	// that has nothing to do with the narrowing under test.
	target := newDealSeeder(t, e).deal(t)

	// Staged through the REAL writer, on the shape the importer produces: an
	// agent acting for e.rep, which is what stamps on_behalf_of with a colleague
	// rather than leaving it NULL. A test that wrote the row itself would be
	// asserting over a shape production does not make.
	staged, err := e.svc.Stage(e.asAgent(t), StageInput{
		Kind:           kindHeldDraft,
		ProposedChange: []byte(`{"body":"Following up on our call."}`),
		DiffHash:       "held-draft-" + target.String(),
		TargetType:     tableDeal,
		TargetID:       target,
		Summary:        "A follow-up written for the rep who owns this deal",
	})
	if err != nil {
		t.Fatalf("staging the match: %v", err)
	}

	targetType := tableDeal
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

	// THE LEAK. The colleague holds every contact grant and every row, so
	// requireDecisionGrants and targetVisible both pass — and the self-only
	// narrowing is the only thing standing between them and the row.
	if listed, panel := read(t, seatFor(e.ws, colleague, tableDeal, objectActivity)); listed != 0 || panel != 0 {
		t.Errorf("a colleague read %d row(s) from List(targeted) and %d from PendingForTarget for a "+
			"held draft written for somebody else — releasing one sends it as the approver, so this "+
			"colleague could put their own name on a rep's message", listed, panel)
	}

	// THE POSITIVE CONTROL, and it is what makes the assertion above mean
	// anything: without it this test also passes when both reads are broken and
	// return nothing to anyone.
	listed, panel := read(t, seatFor(e.ws, e.rep, tableDeal, objectActivity))
	if listed != 1 || panel != 1 {
		t.Errorf("the member it was staged for read %d row(s) from List(targeted) and %d from "+
			"PendingForTarget, want 1 and 1 — the narrowing withholds the proposal from its own subject",
			listed, panel)
	}
	if listed == 1 {
		// The row is the one staged here rather than some other pending proposal
		// against the same contact, so "1" is this proposal and not a coincidence.
		rows, _, err := e.svc.List(seatFor(e.ws, e.rep, tableDeal, objectActivity), ListInput{TargetType: &targetType, TargetID: &target})
		if err != nil {
			t.Fatalf("re-reading the subject's own list: %v", err)
		}
		if rows[0].ID != staged {
			t.Errorf("the subject's list carries %v, want the staged draft %v", rows[0].ID, staged)
		}
	}
}

// The other side of the same two reads: a LinkedIn match IS a colleague's to
// see, and this is the read that has to prove it against a real database.
//
// It was narrowed here for years, on the reading that a match discloses the
// stager's private address book. It does not: the proposal's subject is a
// contact already on file, and both reads open with the target-visibility probe
// that settles whether this colleague may see that contact at all. So what is
// left after the probe is workspace-shared who-knows-whom, and the founder
// decision is that anyone with access decides it.
func TestALinkedInMatchIsReadableByAColleagueWhoCanSeeTheContact(t *testing.T) {
	e := setupStaging(t)
	ctx := context.Background()

	colleague := ids.NewV7()
	if _, err := e.owner.Exec(ctx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Colleague')`,
		colleague, "colleague-"+colleague.String()+"@st.test"); err != nil {
		t.Fatalf("seeding the colleague: %v", err)
	}
	target := ids.NewV7()
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO contact (id, full_name, source, captured_by)
		VALUES ($1, 'Jan Dow', 'linkedin:seed', 'connector:linkedin')`, target); err != nil {
		t.Fatalf("seeding the contact: %v", err)
	}

	staged, err := e.svc.Stage(e.asAgent(t), StageInput{
		Kind:           kindLinkedInMatch,
		ProposedChange: []byte(`{"connection_name":"Jane Doe","connection_company":"Contoso GmbH"}`),
		DiffHash:       "linkedin-match-" + target.String(),
		TargetType:     tableContact,
		TargetID:       target,
		Summary:        "Jane Doe at Contoso GmbH looks like Jan Dow",
	})
	if err != nil {
		t.Fatalf("staging the match: %v", err)
	}

	targetType := tableContact
	rows, _, err := e.svc.List(seatFor(e.ws, colleague), ListInput{TargetType: &targetType, TargetID: &target})
	if err != nil {
		t.Fatalf("List(targeted) as the colleague: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("a colleague read %d row(s), want the one match staged against a contact they can see", len(rows))
	}
	if rows[0].ID != staged {
		t.Errorf("the colleague's list carries %v, want the staged match %v", rows[0].ID, staged)
	}

	var panel int
	if err := e.svc.db.Tx(seatFor(e.ws, colleague), func(tx pgx.Tx) error {
		out, pendErr := e.svc.PendingForTarget(seatFor(e.ws, colleague), tx, targetType, target, PendingScanCap)
		panel = len(out)
		return pendErr
	}); err != nil {
		t.Fatalf("PendingForTarget as the colleague: %v", err)
	}
	if panel != 1 {
		t.Errorf("the contact's own panel showed the colleague %d row(s), want 1", panel)
	}
}
