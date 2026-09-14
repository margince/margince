// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// Who may decide a refused message goes out anyway, and what that decision
// records.
//
// The authority is the point of this slice. Nothing executes on an instruction
// yet — the send path that consumes one is its own change — and landing the
// record first means the question "who may do this" is settled before anything
// can act on the answer.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// directorCtx is somebody holding the exception grant.
func directorCtx(ws, user ids.UUID) context.Context {
	return exceptionCtx(ws, user, principal.ObjectGrant{
		Create: true, Read: true, Update: true, Delete: true,
	})
}

// repCtx is an ordinary seat: they may write to a contact and may NOT direct a
// send against the engine's answer about them. The two authorities are
// deliberately separate, and this is the context that proves it.
func repCtx(ws, user ids.UUID) context.Context {
	return exceptionCtx(ws, user, principal.ObjectGrant{})
}

func exceptionCtx(ws, user ids.UUID, grant principal.ObjectGrant) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects: map[string]principal.ObjectGrant{
				"contact":                 {Read: true, Update: true},
				"communication_exception": grant,
			},
		},
	})
}

// connectorCtx is an integration running under a director's own grants and
// UserID. auth.RequireHuman ADMITS it — it refuses buyers and agents only — so
// without an explicit type check an integration would mint a row saying that
// contact decided to send a refused message.
func connectorCtx(ws, user ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:mailsync",
		UserID: user, OnBehalfOf: user,
		Permissions: principal.Permissions{
			RoleKeys: []string{"admin"},
			Objects: map[string]principal.ObjectGrant{
				"communication_exception": {Create: true, Read: true, Update: true, Delete: true},
			},
		},
	})
}

// agentCtx is an agent acting under a director's own grants. It must not be
// able to mint the record that says a HUMAN decided.
func agentCtx(ws, user ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:overnight", UserID: user,
		OnBehalfOf: user,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects: map[string]principal.ObjectGrant{
				"communication_exception": {Create: true, Read: true, Update: true, Delete: true},
			},
		},
	})
}

// seedOpenReview writes a refused send for these tests to direct.
func seedOpenReview(ctx context.Context, t *testing.T, e *channelConsentEnv) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := e.owner.QueryRow(ctx, `
		INSERT INTO communication_review (state, kind, reason_code, refusals)
		VALUES ('needs_repair', 'single', 'marketing_objection',
		        '[{"address":"anna@example.test","reason_code":"marketing_objection"}]'::jsonb)
		RETURNING id`).Scan(&id); err != nil {
		t.Fatalf("seeding the refused send: %v", err)
	}
	return id
}

// ONLY A HUMAN HOLDING THE GRANT DIRECTS A SEND. Three refusals and one
// success, because each arm answers a different question about who this
// authority belongs to.
func TestOnlyAHumanHoldingTheGrantDirectsASend(t *testing.T) {
	e := setupChannelConsent(t)
	ws, user := e.ws, e.user
	review := seedOpenReview(context.Background(), t, e)

	valid := DirectInput{
		ReasonCode:     ReasonContractualNecessity,
		Explanation:    "The framework agreement obliges us to send this notice.",
		WarningVersion: OverrideWarningVersion,
		Acknowledged:   true,
	}

	// A REP MAY NOT. Writing to a contact is not the authority to act against
	// the engine's answer about them, and an installation grants the second
	// deliberately or not at all.
	if _, err := e.store.DirectSend(repCtx(ws, user), review, valid); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a rep directed a send: %v — the exception grant is its own authority", err)
	}

	// AN AGENT MAY NOT, even carrying the grant. The row asserts that a CONTACT
	// took responsibility, and an agent minting it would make that assertion
	// false while looking entirely correct.
	if _, err := e.store.DirectSend(agentCtx(ws, user), review, valid); err == nil {
		t.Error("an agent minted the record that says a human decided")
	}

	// A CONNECTOR MAY NOT EITHER, and this is the arm auth.RequireHuman does
	// not cover: it runs with the granting human's UserID and permissions, so
	// the row it wrote would name a contact who was not there. The whole content
	// of this record is the claim that somebody took responsibility.
	if _, err := e.store.DirectSend(connectorCtx(ws, user), review, valid); err == nil {
		t.Error("a connector minted a decision attributed to the human who configured it")
	}

	// AN UNACKNOWLEDGED DECISION IS NOT ONE. The tick is the act; everything
	// else on the row merely describes it.
	unticked := valid
	unticked.Acknowledged = false
	if _, err := e.store.DirectSend(directorCtx(ws, user), review, unticked); err == nil {
		t.Error("an instruction was written with no acknowledgement behind it")
	}

	// And the director does.
	out, err := e.store.DirectSend(directorCtx(ws, user), review, valid)
	if err != nil {
		t.Fatalf("a director could not direct a send: %v", err)
	}
	if out.ID.IsZero() || out.Status != InstructionDirected {
		t.Errorf("the instruction came back %+v, want a directed row", out)
	}
}

// THE REFUSAL SURVIVES. This is the whole shape: an instruction sits BESIDE the
// refusal saying somebody overrode it, and never replaces it with a grant
// nobody made. A subject asking why they got the message must be shown both.
func TestDirectingASendRecordsNoConsentAndLeavesTheRefusal(t *testing.T) {
	e := setupChannelConsent(t)
	ws, user := e.ws, e.user
	review := seedOpenReview(context.Background(), t, e)

	if _, err := e.store.DirectSend(directorCtx(ws, user), review, DirectInput{
		ReasonCode:     ReasonLegalObligation,
		Explanation:    "Statutory notice, owed whatever they asked for.",
		WarningVersion: OverrideWarningVersion,
		Acknowledged:   true,
	}); err != nil {
		t.Fatalf("directing the send: %v", err)
	}

	var state, reason string
	if err := e.owner.QueryRow(context.Background(), `
		SELECT state, reason_code FROM communication_review WHERE id = $1`,
		review).Scan(&state, &reason); err != nil {
		t.Fatalf("reading the review: %v", err)
	}
	if state != "needs_repair" || reason != "marketing_objection" {
		t.Errorf("the review now reads state=%q reason=%q — directing a send must leave the "+
			"refusal exactly where it was", state, reason)
	}

	var grants int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM contact_consent WHERE state = 'granted'`).Scan(&grants); err != nil {
		t.Fatalf("counting the grants: %v", err)
	}
	if grants != 0 {
		t.Errorf("%d consent grant(s) written by an override, want 0 — an exception is not a "+
			"grant and must never be recorded as one", grants)
	}
}

// AN INSTRUCTION IS NOT REWRITABLE. It is the record of what one contact decided
// and what they were told when they decided it, and a reason edited afterwards
// would let the account of an override be improved by whoever gave it.
func TestAnInstructionCannotBeRewritten(t *testing.T) {
	e := setupChannelConsent(t)
	ws, user := e.ws, e.user
	review := seedOpenReview(context.Background(), t, e)

	out, err := e.store.DirectSend(directorCtx(ws, user), review, DirectInput{
		ReasonCode:     ReasonOtherException,
		Explanation:    "Agreed with the client on the call this morning.",
		WarningVersion: OverrideWarningVersion,
		Acknowledged:   true,
	})
	if err != nil {
		t.Fatalf("directing the send: %v", err)
	}

	if _, err := e.owner.Exec(context.Background(),
		`UPDATE communication_instruction SET explanation = 'something better' WHERE id = $1`,
		out.ID); err == nil {
		t.Error("the explanation was rewritten — the account of an override must not be " +
			"improvable by whoever gave it")
	}
	if _, err := e.owner.Exec(context.Background(),
		`DELETE FROM communication_instruction WHERE id = $1`, out.ID); err == nil {
		t.Error("the instruction was deleted — the record of a decision is not removable")
	}
	// The status half still moves, which is what revocation and consumption
	// need. Frozen everywhere else, movable here.
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE communication_instruction SET status = 'expired' WHERE id = $1`,
		out.ID); err != nil {
		t.Errorf("the status could not move: %v — revocation and consumption need it", err)
	}
}

// A REVOCATION NAMES WHO AND WHY, and a consumed instruction is not revocable:
// the message has gone, and taking the decision back would leave a sent message
// with no recorded authority behind it.
func TestARevocationNamesItselfAndSpareAConsumedInstruction(t *testing.T) {
	e := setupChannelConsent(t)
	ws, user := e.ws, e.user
	review := seedOpenReview(context.Background(), t, e)

	out, err := e.store.DirectSend(directorCtx(ws, user), review, DirectInput{
		ReasonCode:     ReasonOtherException,
		Explanation:    "Thought better of it.",
		WarningVersion: OverrideWarningVersion,
		Acknowledged:   true,
	})
	if err != nil {
		t.Fatalf("directing the send: %v", err)
	}

	if err := e.store.RevokeInstruction(directorCtx(ws, user), out.ID, ""); err == nil {
		t.Error("a decision was taken back with no reason given")
	}
	if err := e.store.RevokeInstruction(directorCtx(ws, user), out.ID, "sent by hand instead"); err != nil {
		t.Fatalf("revoking: %v", err)
	}

	var status, by, reason string
	if err := e.owner.QueryRow(context.Background(), `
		SELECT status, coalesce(revoked_by::text, ''), coalesce(revoked_reason, '')
		  FROM communication_instruction WHERE id = $1`, out.ID).Scan(&status, &by, &reason); err != nil {
		t.Fatalf("reading the instruction: %v", err)
	}
	if status != InstructionRevoked || by == "" || reason == "" {
		t.Errorf("revoked as status=%q by=%q reason=%q — a decision reversed by nobody is not "+
			"a thing that happened", status, by, reason)
	}

	// A second revocation finds nothing to revoke.
	if err := e.store.RevokeInstruction(directorCtx(ws, user), out.ID, "again"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("re-revoking answered %v, want not-found", err)
	}
}

// THE FREEZE HAS EXACTLY ONE DOOR, and it is Art. 17. The explanation is a rep's
// own sentence about a named contact, so it is personal data an erasure destroys
// — and a record that could not be scrubbed would put immutability above the
// right it exists inside.
//
// What the door admits is the TOMBSTONE and nothing else, so the account of the
// decision still cannot be improved. What survives is the accountable half:
// somebody decided, who, when, under which reason. None of it names the subject.
func TestAnErasureCanTombstoneTheWordsAndNothingElse(t *testing.T) {
	e := setupChannelConsent(t)
	ws, user := e.ws, e.user
	review := seedOpenReview(context.Background(), t, e)

	out, err := e.store.DirectSend(directorCtx(ws, user), review, DirectInput{
		ReasonCode:     ReasonOtherException,
		Explanation:    "Anna asked for this on the call this morning.",
		WarningVersion: OverrideWarningVersion,
		Acknowledged:   true,
	})
	if err != nil {
		t.Fatalf("directing the send: %v", err)
	}

	if _, err := e.owner.Exec(context.Background(),
		`UPDATE communication_instruction SET explanation = '[erased]' WHERE id = $1`,
		out.ID); err != nil {
		t.Fatalf("an erasure could not scrub the words: %v — a record that cannot be scrubbed "+
			"puts immutability above the right it exists inside", err)
	}

	// And no other value gets through the same door.
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE communication_instruction SET explanation = 'a better account' WHERE id = $1`,
		out.ID); err == nil {
		t.Error("the explanation was rewritten through the erasure door — the tombstone is the " +
			"whole of what the freeze admits")
	}

	// The accountable half survives.
	var reason, director string
	if err := e.owner.QueryRow(context.Background(), `
		SELECT reason_code, directed_by::text FROM communication_instruction WHERE id = $1`,
		out.ID).Scan(&reason, &director); err != nil {
		t.Fatalf("reading the instruction: %v", err)
	}
	if reason != ReasonOtherException || director == "" {
		t.Errorf("after the scrub the row reads reason=%q director=%q — who decided and under "+
			"what reason is the half that must survive", reason, director)
	}
}
