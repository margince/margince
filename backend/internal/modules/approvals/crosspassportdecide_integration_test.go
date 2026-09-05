// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

// A second agent passport, lent by a SECOND human, approving a
// confirmation-required action it never staged — against a real database.
//
// The self-approval rule bound the CREDENTIAL and nothing bound the PERSON, so
// two passports lent by two people walked a confirm-first action through end to
// end — A's stages, B's approves, A's redeems — and the decide route is itself
// auto_execute, so B's approval needed no confirmation of its own. The whole
// "a human must look at this" guarantee of the tier was satisfied by a second
// autonomous agent instead of a person.
//
// It lives here rather than in the unit suite because approval.passport_id and
// approval.on_behalf_of are foreign keys: the two credentials and the two
// people have to be real rows, and a fabricated id exercises a shape the
// database refuses rather than the rule under test.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// lentPassport mints a real credential for one human and returns the agent
// context that asserts it, carrying the write cap a decision spends.
func (e *stagingEnv) lentPassport(t *testing.T, human ids.UUID) context.Context {
	t.Helper()
	passport := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO passport (id, on_behalf_of, granted_by, token_hash, scopes, expires_at)
		VALUES ($1, $2, $2, $3, ARRAY['read','write'], now() + interval '30 days')`,
		passport, human, "hash-"+passport.String()); err != nil {
		t.Fatalf("seeding the lent passport: %v", err)
	}
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:" + human.String(),
		PassportID: passport, OnBehalfOf: human, UserID: human,
		Scopes: principal.NewScopeSet(principal.ScopeWrite),
		Permissions: principal.Permissions{
			RowScope: principal.RowScopeAll,
			Objects: map[string]principal.ObjectGrant{
				tableOrganization: {Create: true, Read: true, Update: true, Delete: true},
			},
		},
	})
}

func TestASecondPersonsPassportDoesNotReleaseAConfirmationRequiredAction(t *testing.T) {
	e := setupStaging(t)
	ctx := context.Background()

	// The second human. A real row: on_behalf_of is a foreign key.
	second := ids.NewV7()
	if _, err := e.owner.Exec(ctx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Second')`,
		second, "second-"+second.String()+"@st.test"); err != nil {
		t.Fatalf("seeding the second human: %v", err)
	}
	target := ids.NewV7()
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO organization (id, display_name, source, captured_by)
		VALUES ($1, 'Gitex', 'gmail:seed', 'connector:gmail')`, target); err != nil {
		t.Fatalf("seeding the target: %v", err)
	}

	// The three credentials the sequence needs, minted ONCE each: redemption is
	// bound to the passport that staged, so a fresh context per call would
	// exercise that binding instead of the rule under test.
	stager := e.lentPassport(t, e.rep)       // A's, which stages and redeems
	sameperson := e.lentPassport(t, e.rep)   // A's SECOND, the sanctioned decider
	otherperson := e.lentPassport(t, second) // B's, the attacker

	staging := func(name, hash string) StageInput {
		return StageInput{
			Kind:           "org_name_promotion",
			ProposedChange: []byte(`{"proposed_name":"` + name + `"}`),
			DiffHash:       hash + "-" + target.String(),
			TargetType:     tableOrganization,
			TargetID:       target,
			Summary:        "Rename Gitex to " + name + "?",
		}
	}

	// Step 1: A's passport stages the confirm-first action.
	attacked := staging("Gitex Global", "cross-passport")
	staged, err := e.svc.Stage(stager, attacked)
	if err != nil {
		t.Fatalf("staging as the first person's agent: %v", err)
	}

	// Step 2, and the defect: B's passport — a different token, a different
	// human, never having seen the proposal — approves it. The exact sentinel,
	// because the surface answers 403 on this refusal and a bare non-nil check
	// would stay green if it became an unrelated internal failure.
	if _, err := e.svc.Decide(otherperson, staged, true, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a second person's passport approved an action staged for somebody else → %v, want "+
			"ErrPermissionDenied — two agents then push any confirmation-required mutation through "+
			"end to end with no human in the loop", err)
	}

	// A rejection is still allowed: it discards the proposal and cannot
	// escalate, and an agent unable to take a request off a desk is an obstacle
	// rather than a rule.
	if _, err := e.svc.Decide(otherperson, staged, false, nil); err != nil {
		t.Errorf("a second person's passport could not REJECT the proposal: %v", err)
	}

	// THE POSITIVE CONTROL, and it runs all three steps to the
	// end. Without it this test also passes when no passport can decide anything
	// and the tier has simply stopped working: a SECOND credential of the SAME
	// person releases it, and A's staging credential then redeems and the effect
	// runs — the path the product deliberately allows, because that person could
	// have answered it in the CRM themselves.
	control := staging("Gitex Worldwide", "same-person")
	sanctioned, err := e.svc.Stage(stager, control)
	if err != nil {
		t.Fatalf("staging the control proposal: %v", err)
	}
	if _, err := e.svc.Decide(sameperson, sanctioned, true, nil); err != nil {
		t.Fatalf("another credential of the SAME person was refused: %v — the rule binds the person, "+
			"not the credential, and this is the path the product deliberately allows", err)
	}
	// Redeeming is step 3, and it is also what keeps this test
	// out of the lapse sweep's count: that sweep is database-wide and counts
	// approved stagings nobody came back for, so a control stopping at the
	// decision would leave a row the sweep's own tests report as a second lapse.
	if _, _, err := e.svc.Redeem(stager, sanctioned, control.Kind, control.DiffHash); err != nil {
		t.Errorf("the staging credential could not redeem what its person released: %v", err)
	}
}
