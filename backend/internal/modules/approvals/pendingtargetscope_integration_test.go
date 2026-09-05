// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

// The record panel and the inbox answer ONE question about a target.
//
// decidable exists so they cannot drift: it backs List, Get and Decide alike,
// "so triage visibility and the decision gate can never drift apart — you see
// exactly what you could act on, and what you cannot see you cannot decide (in
// either direction)". The panel was a fourth caller answering differently — it
// hoisted targetVisible, the READ half, where the inbox takes targetDecidable,
// the WRITE one.
//
// The two differ in exactly the arms a manual grant can widen, so a read-only
// SHARE was enough. And the panel renders the proposal — summary and proposed
// change, not merely that one exists — so what reached that seat was the staged
// content itself.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// sharedSeat is a colleague whose reach over ONE record comes from a grant
// rather than from row scope: the object grants a decision needs, and no scope
// of their own. What the share says — read or write — is then the whole
// difference between the two predicates.
func sharedSeat(ws, user ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{
			RowScope: principal.RowScopeOwn,
			Objects: map[string]principal.ObjectGrant{
				tableOrganization: {Create: true, Read: true, Update: true, Delete: true},
			},
		},
	})
}

func TestAReadOnlyShareIsNotEnoughToSeeAPendingDecision(t *testing.T) {
	e := setupStaging(t)
	ctx := context.Background()

	sharee := ids.NewV7()
	if _, err := e.owner.Exec(ctx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Sharee')`,
		sharee, "sharee-"+sharee.String()+"@st.test"); err != nil {
		t.Fatalf("seeding the sharee: %v", err)
	}
	target := ids.NewV7()
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO organization (id, display_name, source, captured_by, owner_id)
		VALUES ($1, 'Gitex', 'test', 'human:seed', $2)`, target, e.rep); err != nil {
		t.Fatalf("seeding the account: %v", err)
	}
	// A shared kind, so the self-only narrowing is not what answers this: the
	// only thing between the sharee and the row is which half of the target
	// predicate the panel hoists.
	if _, err := e.svc.Stage(e.asAgent(t), StageInput{
		Kind:           "org_name_promotion",
		ProposedChange: []byte(`{"proposed_name":"Gitex Global"}`),
		DiffHash:       "promotion-" + target.String(),
		TargetType:     tableOrganization,
		TargetID:       target,
		Summary:        "Rename Gitex?",
	}); err != nil {
		t.Fatalf("staging the proposal: %v", err)
	}

	share := func(t *testing.T, access string) {
		t.Helper()
		// Replaced rather than inserted-if-absent: the two calls below are the
		// SAME share widened, and an ON CONFLICT DO NOTHING would silently keep
		// the read grant — which passes the first assertion and then fails the
		// control for a reason that has nothing to do with the panel.
		if _, err := e.owner.Exec(ctx,
			`DELETE FROM record_grant WHERE record_id = $1 AND subject_id = $2`, target, sharee); err != nil {
			t.Fatalf("clearing the previous share: %v", err)
		}
		if _, err := e.owner.Exec(ctx, `
			INSERT INTO record_grant (record_type, record_id, subject_type, subject_id, access, granted_by)
			VALUES ('organization', $1, 'user', $2, $3, $4)`, target, sharee, access, e.rep); err != nil {
			t.Fatalf("granting %s: %v", access, err)
		}
	}
	panelRows := func(t *testing.T, as context.Context) int {
		t.Helper()
		var n int
		if err := e.svc.db.Tx(as, func(tx pgx.Tx) error {
			out, err := e.svc.PendingForTarget(as, tx, tableOrganization, target, PendingScanCap)
			n = len(out)
			return err
		}); err != nil {
			t.Fatalf("PendingForTarget: %v", err)
		}
		return n
	}

	share(t, "read")
	if got := panelRows(t, sharedSeat(e.ws, sharee)); got != 0 {
		t.Errorf("a read-only share put %d pending proposal(s) on the record panel — the panel renders "+
			"the summary and the proposed change, so this seat received a staged change the inbox "+
			"withholds from them", got)
	}

	// THE POSITIVE CONTROL, and it is what makes the assertion above mean
	// anything: without it this test passes just as well when the panel is
	// broken and shows nobody anything.
	share(t, "write")
	if got := panelRows(t, sharedSeat(e.ws, sharee)); got != 1 {
		t.Errorf("a write share read %d pending proposal(s), want 1 — narrowing the panel to the "+
			"deciding half has taken it from the seat that CAN decide", got)
	}
}
