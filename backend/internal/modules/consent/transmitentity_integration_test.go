// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// The sending entity is settled once, at staging, from the sender: a human,
// or the user an agent works for. The worker that transmits runs as the
// system, which works for nobody, so transmit reads the entity staging stored.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func (e *resolveEnv) dispatcherCtx() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem, ID: "system:dispatcher"})
}

// agentFor is an agent on user's passport, holding the seat's grants.
func (e *resolveEnv) agentFor(t *testing.T, user ids.UUID) context.Context {
	t.Helper()
	seat := e.seatCtx(user)
	actor, ok := principal.Actor(seat)
	if !ok {
		t.Fatal("the seat context carries no actor")
	}
	passport := ids.NewV7()
	actor.Type, actor.ID = principal.PrincipalAgent, "agent:"+passport.String()
	actor.PassportID, actor.OnBehalfOf, actor.UserID = passport, user, user
	return principal.WithActor(seat, actor)
}

// sendThrough previews, stages and transmits one newsletter as sender, and
// answers whether staging and the worker's transmit allowed it.
func (e *resolveEnv) sendThrough(t *testing.T, sender context.Context) (staged, transmitted bool) {
	t.Helper()
	recipients := []connector.Recipient{{Email: e.address}}
	req := commsauthz.Request{Recipients: recipients, LegacyPurposeKey: "newsletter", Subject: "S", Body: "b"}
	preview, err := e.gate.Preview(sender, req)
	if err != nil {
		t.Fatalf("previewing: %v", err)
	}
	delivery := e.plantDelivery(t)
	var set commsauthz.DecisionSet
	if err := e.store.db.Tx(sender, func(tx pgx.Tx) error {
		set, err = e.gate.AuthorizeStagingTx(sender, tx, delivery, req)
		return err
	}); err != nil {
		t.Fatalf("staging: %v", err)
	}
	if preview.Decisions[0].Verdict != set.Decisions[0].Verdict {
		t.Fatalf("preview said %q and staging %q", preview.Decisions[0].Verdict, set.Decisions[0].Verdict)
	}
	ticket, err := e.gate.AuthorizeTransmit(e.dispatcherCtx(), commsauthz.TransmitRequest{
		DeliveryID: delivery, Attempt: 1, Recipients: recipients,
		PurposeKey: "newsletter", Subject: "S", Body: "b",
	})
	if err != nil {
		t.Fatalf("transmitting: %v", err)
	}
	return set.Decisions[0].Verdict == commsauthz.VerdictAllow, ticket.Allowed
}

func TestTransmitJudgesTheEntityStagingStoredForAHumanAndAnAgent(t *testing.T) {
	for name, sender := range map[string]func(t *testing.T, e *resolveEnv, user ids.UUID) context.Context{
		"a rep":                 func(_ *testing.T, e *resolveEnv, user ids.UUID) context.Context { return e.seatCtx(user) },
		"an agent for that rep": func(t *testing.T, e *resolveEnv, user ids.UUID) context.Context { return e.agentFor(t, user) },
	} {
		t.Run(name, func(t *testing.T) {
			e := setupResolve(t)
			e.seedPurpose(t, "newsletter", "marketing")
			branch := e.secondEntity(t)
			branchRep := e.colleague(t, branch)
			e.vouchAs(t, e.colleague(t, branch))

			staged, transmitted := e.sendThrough(t, sender(t, e, branchRep))
			if !staged || !transmitted {
				t.Fatalf("a branch vouch for a branch send: staged %v, transmitted %v, want both allowed", staged, transmitted)
			}
		})
	}
}

func TestTheWorkerNeverSendsOnTheDefaultEntitysVouchForABranchMessage(t *testing.T) {
	e := setupResolve(t)
	e.seedPurpose(t, "newsletter", "marketing")
	branchRep := e.colleague(t, e.secondEntity(t))
	e.vouchAs(t, e.colleague(t, ids.Nil))

	staged, transmitted := e.sendThrough(t, e.agentFor(t, branchRep))
	if staged || transmitted {
		t.Fatalf("the default entity's vouch carried a branch message: staged %v, transmitted %v", staged, transmitted)
	}
}

func TestTransmitRefusesADeliveryWhosePurposeDiffersFromTheStagedOne(t *testing.T) {
	e := setupResolve(t)
	e.seedPurpose(t, "newsletter", "marketing")
	e.seedPurpose(t, "events", "marketing")
	branchRep := e.colleague(t, ids.Nil)
	e.vouchAs(t, branchRep)
	recipients := []connector.Recipient{{Email: e.address}}
	delivery := e.plantDelivery(t)
	sender := e.seatCtx(branchRep)
	if err := e.store.db.Tx(sender, func(tx pgx.Tx) error {
		_, err := e.gate.AuthorizeStagingTx(sender, tx, delivery, commsauthz.Request{
			Recipients: recipients, LegacyPurposeKey: "newsletter", Subject: "S", Body: "b"})
		return err
	}); err != nil {
		t.Fatalf("staging: %v", err)
	}
	ticket, err := e.gate.AuthorizeTransmit(e.dispatcherCtx(), commsauthz.TransmitRequest{
		DeliveryID: delivery, Attempt: 1, Recipients: recipients,
		PurposeKey: "events", Subject: "S", Body: "b",
	})
	if err != nil {
		t.Fatalf("transmitting: %v", err)
	}
	if ticket.Allowed {
		t.Fatal("a delivery staged as newsletter went out as events")
	}
}
