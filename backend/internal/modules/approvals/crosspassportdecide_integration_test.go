// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

// A second agent passport, lent by a SECOND human, approving a
// confirmation-required action it never staged — against a real database.
//
// The self-approval rule bound the CREDENTIAL and nothing bound the HUMAN, so
// two passports lent by two contacts walked a confirm-first action through end to
// end — A's stages, B's approves, A's redeems — and the decide route is itself
// auto_execute, so B's approval needed no confirmation of its own. The whole
// "a human must look at this" guarantee of the tier was satisfied by a second
// autonomous agent instead of a contact.
//
// It lives here rather than in the unit suite because approval.passport_id and
// approval.on_behalf_of are foreign keys: the two credentials and the two
// contacts have to be real rows, and a fabricated id exercises a shape the
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
				tableCompany: {Create: true, Read: true, Update: true, Delete: true},
			},
		},
	})
}

func TestASecondContactsPassportDoesNotReleaseAConfirmationRequiredAction(t *testing.T) {
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
		INSERT INTO company (id, display_name, source, captured_by)
		VALUES ($1, 'Gitex', 'gmail:seed', 'connector:gmail')`, target); err != nil {
		t.Fatalf("seeding the target: %v", err)
	}

	// The three credentials the sequence needs, minted ONCE each: redemption is
	// bound to the passport that staged, so a fresh context per call would
	// exercise that binding instead of the rule under test.
	stager := e.lentPassport(t, e.rep)        // A's, which stages and redeems
	samecontact := e.lentPassport(t, e.rep)   // A's SECOND, the sanctioned decider
	othercontact := e.lentPassport(t, second) // B's, the attacker

	staging := func(name, hash string) StageInput {
		return StageInput{
			Kind:           "company_name_promotion",
			ProposedChange: []byte(`{"proposed_name":"` + name + `"}`),
			DiffHash:       hash + "-" + target.String(),
			TargetType:     tableCompany,
			TargetID:       target,
			Summary:        "Rename Gitex to " + name + "?",
		}
	}

	// Step 1: A's passport stages the confirm-first action.
	attacked := staging("Gitex Global", "cross-passport")
	staged, err := e.svc.Stage(stager, attacked)
	if err != nil {
		t.Fatalf("staging as the first contact's agent: %v", err)
	}

	// Step 2, and the defect: B's passport — a different token, a different
	// human, never having seen the proposal — approves it. The exact sentinel,
	// because the surface answers 403 on this refusal and a bare non-nil check
	// would stay green if it became an unrelated internal failure.
	if _, err := e.svc.Decide(othercontact, staged, true, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a second contact's passport approved an action staged for somebody else → %v, want "+
			"ErrPermissionDenied — two agents then push any confirmation-required mutation through "+
			"end to end with no human in the loop", err)
	}

	// A rejection is still allowed: it discards the proposal and cannot
	// escalate, and an agent unable to take a request off a desk is an obstacle
	// rather than a rule.
	if _, err := e.svc.Decide(othercontact, staged, false, nil); err != nil {
		t.Errorf("a second contact's passport could not REJECT the proposal: %v", err)
	}

	// THE POSITIVE CONTROL, and it runs all three steps to the
	// end. Without it this test also passes when no passport can decide anything
	// and the tier has simply stopped working: a SECOND credential of the SAME
	// contact releases it, and A's staging credential then redeems and the effect
	// runs — the path the product deliberately allows, because that contact could
	// have answered it in the CRM themselves.
	control := staging("Gitex Worldwide", "same-contact")
	sanctioned, err := e.svc.Stage(stager, control)
	if err != nil {
		t.Fatalf("staging the control proposal: %v", err)
	}
	if _, err := e.svc.Decide(samecontact, sanctioned, true, nil); err != nil {
		t.Fatalf("another credential of the SAME human was refused: %v — the rule binds the human, "+
			"not the credential, and this is the path the product deliberately allows", err)
	}
	// Redeeming is step 3, and it is also what keeps this test
	// out of the lapse sweep's count: that sweep is database-wide and counts
	// approved stagings nobody came back for, so a control stopping at the
	// decision would leave a row the sweep's own tests report as a second lapse.
	if _, _, err := e.svc.Redeem(stager, sanctioned, control.Kind, control.DiffHash); err != nil {
		t.Errorf("the staging credential could not redeem what its contact released: %v", err)
	}
}

// connection registers a client and the grant beneath it, which is the identity
// a credential keeps across its own rotations.
func (e *stagingEnv) connection(t *testing.T, human ids.UUID) ids.UUID {
	t.Helper()
	clientID := "client-" + ids.NewV7().String()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO oauth_client (client_id, client_name, redirect_uris)
		VALUES ($1, 'Rotating client', ARRAY['https://client.example/cb'])`, clientID); err != nil {
		t.Fatalf("registering the client: %v", err)
	}
	grant := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO oauth_grant (id, client_id, user_id, scopes, refresh_allowed)
		VALUES ($1, $2, $3, ARRAY['read','write'], true)`, grant, clientID, human); err != nil {
		t.Fatalf("seeding the connection: %v", err)
	}
	return grant
}

// connectedPassport mints a credential under an OAuth CONNECTION and returns
// the agent context asserting it. Calling it twice for one connection is what a
// refresh leaves behind: two passport rows, one grant, the same human and the
// same caps.
func (e *stagingEnv) connectedPassport(t *testing.T, human, grant ids.UUID) context.Context {
	t.Helper()
	ctx := e.lentPassport(t, human)
	actor, _ := principal.Actor(ctx)
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE passport SET oauth_grant_id = $2 WHERE id = $1`, actor.PassportID, grant); err != nil {
		t.Fatalf("binding the passport to its connection: %v", err)
	}
	actor.ConnectionID = grant
	return principal.WithActor(ctx, actor)
}

// A connected agent's passport id is not stable: refreshing its token retires
// the passport and mints a replacement under the same grant. Compared on the
// passport alone, the agent that staged a confirm-first call releases it itself
// after doing nothing but waiting for its own access token to expire — and the
// receipt names the human who never saw it.
//
// The sibling test above allows exactly this shape for two DIRECTLY minted
// passports of one human, on the grounds that a human had to be present to mint
// each. That reasoning is what fails here: nobody is present at a rotation.
func TestARotatedCredentialDoesNotReleaseWhatItStagedAgainstTheDatabase(t *testing.T) {
	e := setupStaging(t)
	ctx := context.Background()

	grant := e.connection(t, e.rep)
	target := ids.NewV7()
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO company (id, display_name, source, captured_by)
		VALUES ($1, 'Rotamer', 'gmail:seed', 'connector:gmail')`, target); err != nil {
		t.Fatalf("seeding the target: %v", err)
	}

	before := e.connectedPassport(t, e.rep, grant)
	after := e.connectedPassport(t, e.rep, grant)

	staged, err := e.svc.Stage(before, StageInput{
		Kind:           "company_name_promotion",
		ProposedChange: []byte(`{"proposed_name":"Rotamer Global"}`),
		DiffHash:       "rotated-" + target.String(),
		TargetType:     tableCompany,
		TargetID:       target,
		Summary:        "Rename Rotamer?",
	})
	if err != nil {
		t.Fatalf("staging on the connection's first credential: %v", err)
	}

	if _, err := e.svc.Decide(after, staged, true, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("the connection's renewed credential approved what the connection itself staged → %v, "+
			"want ErrPermissionDenied — an agent then walks any confirm-first call through by waiting "+
			"for its own token to refresh", err)
	}

	// The mirror, and the reason the fix is one comparison rather than a second
	// rule: the renewed credential must still REDEEM what a human released for
	// it. Refusing here would leave the proposer unable to finish its own work.
	human := e.asHumanWith(principal.Permissions{
		RowScope: principal.RowScopeAll,
		Objects: map[string]principal.ObjectGrant{
			tableCompany: {Create: true, Read: true, Update: true, Delete: true},
		},
	})
	if _, err := e.svc.Decide(human, staged, true, nil); err != nil {
		t.Fatalf("the human could not release the proposal: %v", err)
	}
	if _, _, err := e.svc.Redeem(after, staged, "company_name_promotion", "rotated-"+target.String()); err != nil {
		t.Errorf("the renewed credential could not redeem what its human released: %v — a rotation "+
			"must not cost an agent the authority it was granted", err)
	}
}

// The third reader of the same question. TaskState and Withdraw resolve the
// caller's own proposal through ownProposal, so before sameAgent a rotation
// told an agent that the proposal it was waiting on did not exist — and left it
// unable to take its own request off a human's desk.
//
// A second connection still sees nothing, which is what keeps this a fix to the
// identity rather than a widening of who may poll.
func TestARotatedCredentialStillPollsAndWithdrawsItsOwnProposal(t *testing.T) {
	e := setupStaging(t)
	ctx := context.Background()

	grant := e.connection(t, e.rep)
	stranger := e.connection(t, e.rep)
	target := ids.NewV7()
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO company (id, display_name, source, captured_by)
		VALUES ($1, 'Pollex', 'gmail:seed', 'connector:gmail')`, target); err != nil {
		t.Fatalf("seeding the target: %v", err)
	}

	before := e.connectedPassport(t, e.rep, grant)
	after := e.connectedPassport(t, e.rep, grant)
	other := e.connectedPassport(t, e.rep, stranger)

	staged, err := e.svc.Stage(before, StageInput{
		Kind:           "company_name_promotion",
		ProposedChange: []byte(`{"proposed_name":"Pollex Global"}`),
		DiffHash:       "polled-" + target.String(),
		TargetType:     tableCompany,
		TargetID:       target,
		Summary:        "Rename Pollex?",
	})
	if err != nil {
		t.Fatalf("staging on the connection's first credential: %v", err)
	}

	state, err := e.svc.TaskState(after, staged)
	if err != nil {
		t.Fatalf("the renewed credential could not poll its own staged proposal: %v — a rotation "+
			"must not make an agent's own work invisible to it", err)
	}
	if state.Status != StatusPending {
		t.Fatalf("the proposal polled as %q, want %q", state.Status, StatusPending)
	}
	if _, err := e.svc.ProposedChange(after, staged); err != nil {
		t.Errorf("the renewed credential could not read back what it staged: %v", err)
	}

	// A DIFFERENT connection is still nobody: not-found rather than a refusal,
	// so a poll cannot be used to discover that somebody else's proposal exists.
	if _, err := e.svc.TaskState(other, staged); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("another connection polled a proposal it did not stage → %v, want ErrNotFound", err)
	}

	retracted, err := e.svc.Withdraw(after, staged, "superseded")
	if err != nil {
		t.Fatalf("the renewed credential could not withdraw its own proposal: %v", err)
	}
	if !retracted {
		t.Error("withdrawing an undecided proposal reported nothing to retract")
	}
}
