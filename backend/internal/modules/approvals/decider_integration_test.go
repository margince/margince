// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// forRep is a background job staging on the env rep's behalf.
func (e *stagingEnv) forRep() context.Context {
	return principal.WithActor(e.sweeping(), principal.Principal{
		Type: principal.PrincipalSystem, ID: ExpiryActor, OnBehalfOf: e.rep,
	})
}

func (s *dealSeeder) dealOwnedBy(t *testing.T, owner *ids.UUID) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := s.env.owner.Exec(context.Background(),
		`INSERT INTO deal (id, name, owner_id, pipeline_id, stage_id, source, captured_by)
		 VALUES ($1, 'Owned', $2, $3, $4, 'manual', 'system')`,
		id, owner, s.pipeline, s.stage); err != nil {
		t.Fatalf("seeding a deal: %v", err)
	}
	return id
}

func (e *stagingEnv) deciderOf(t *testing.T, id ids.ApprovalID) (*ids.UUID, *string) {
	t.Helper()
	var decider *ids.UUID
	var source *string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT requested_decider_id, requested_decider_source FROM approval WHERE id = $1`, id).Scan(&decider, &source); err != nil {
		t.Fatal(err)
	}
	return decider, source
}

func advanceDeal(deal ids.UUID, hash string) StageInput {
	return StageInput{
		Kind: "advance_deal", ProposedChange: []byte(`{"stage":"lost"}`), DiffHash: hash,
		TargetType: tableDeal, TargetID: deal, Summary: "Close the deal as lost",
	}
}

func TestABackgroundRequestIsForTheOwnerOfItsRecord(t *testing.T) {
	e := setupStaging(t)
	other := e.secondRep(t)
	deal := newDealSeeder(t, e).dealOwnedBy(t, &other)
	id, err := e.svc.Stage(e.sweeping(), advanceDeal(deal, "owned"))
	if err != nil {
		t.Fatal(err)
	}
	decider, source := e.deciderOf(t, id)
	if decider == nil || *decider != other || source == nil || *source != string(deciderRecordOwner) {
		t.Fatalf("decider = %v (%v), want the deal owner %s", decider, source, other)
	}
}

func TestARequestOnARecordNobodyOwnsIsNotStaged(t *testing.T) {
	e := setupStaging(t)
	deal := newDealSeeder(t, e).dealOwnedBy(t, nil)
	_, err := e.svc.Stage(e.sweeping(), advanceDeal(deal, "unowned"))
	if !errors.Is(err, ErrNoDecider) {
		t.Fatalf("err = %v, want ErrNoDecider", err)
	}
	if n := e.count(t, `SELECT count(*) FROM approval`); n != 0 {
		t.Fatalf("%d approvals staged for nobody", n)
	}
}

func TestTheHumanAnAgentWorksForOutranksTheRecordOwner(t *testing.T) {
	e := setupStaging(t)
	other := e.secondRep(t)
	deal := newDealSeeder(t, e).dealOwnedBy(t, &other)
	id, err := e.svc.Stage(e.asAgent(t), advanceDeal(deal, "delegated"))
	if err != nil {
		t.Fatal(err)
	}
	if decider, _ := e.deciderOf(t, id); decider == nil || *decider != e.rep {
		t.Fatalf("decider = %v, want the human the agent works for %s", decider, e.rep)
	}
}

func TestADeactivatedOwnerIsNotNamed(t *testing.T) {
	e := setupStaging(t)
	other := e.secondRep(t)
	if _, err := e.owner.Exec(context.Background(), `UPDATE app_user SET status = 'deactivated' WHERE id = $1`, other); err != nil {
		t.Fatal(err)
	}
	deal := newDealSeeder(t, e).dealOwnedBy(t, &other)
	if _, err := e.svc.Stage(e.sweeping(), advanceDeal(deal, "gone")); !errors.Is(err, ErrNoDecider) {
		t.Fatalf("err = %v, want ErrNoDecider for a deactivated owner", err)
	}
}

func TestOnlyTheNamedColleagueIsAskedToDecide(t *testing.T) {
	e := setupStaging(t)
	admin := e.secondRep(t)
	deal := newDealSeeder(t, e).dealOwnedBy(t, &e.rep)
	id, err := e.svc.Stage(e.asAgent(t), advanceDeal(deal, "asked"))
	if err != nil {
		t.Fatal(err)
	}
	askedOf := func(seat ids.UUID) bool {
		t.Helper()
		var asked bool
		ctx := seatFor(e.ws, seat, tableDeal, objectActivity)
		if err := e.svc.db.Tx(ctx, func(tx pgx.Tx) error {
			var err error
			asked, err = e.svc.PendingAskedOfInTx(ctx, tx, id)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return asked
	}
	if !askedOf(e.rep) {
		t.Fatal("the member the request is for was not asked")
	}
	if askedOf(admin) {
		t.Fatal("a colleague who may decide everything was asked about a request that is not theirs")
	}
}

func TestADealsNewOwnerInheritsItsOwnerNamedRequests(t *testing.T) {
	e := setupStaging(t)
	other := e.secondRep(t)
	deal := newDealSeeder(t, e).dealOwnedBy(t, &e.rep)
	id, err := e.svc.Stage(e.sweeping(), advanceDeal(deal, "reassigned"))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.db.Tx(e.as(), func(tx pgx.Tx) error {
		return FollowDealOwnerInTx(e.as(), tx, deal, &e.rep, &other)
	}); err != nil {
		t.Fatal(err)
	}
	if decider, _ := e.deciderOf(t, id); decider == nil || *decider != other {
		t.Fatalf("decider = %v after reassignment, want the new owner %s", decider, other)
	}
	if err := e.svc.db.Tx(e.as(), func(tx pgx.Tx) error {
		return FollowDealOwnerInTx(e.as(), tx, deal, &other, nil)
	}); err != nil {
		t.Fatal(err)
	}
	if status := e.statusOf(t, id); status != StatusExpired {
		t.Fatalf("status = %s after the deal lost its owner, want expired", status)
	}
}

// A promise is put only to the colleague who made it. When they cannot be
// asked, nobody is, not even the owner of the record it is staged on.
func TestAPromiseWhosePromiserCannotBeAskedIsPutToNobody(t *testing.T) {
	e := setupStaging(t)
	promiser := e.secondRep(t)
	if _, err := e.owner.Exec(context.Background(), `UPDATE app_user SET archived_at = now() WHERE id = $1`, promiser); err != nil {
		t.Fatal(err)
	}
	deal := newDealSeeder(t, e).dealOwnedBy(t, &e.rep)
	_, err := e.svc.Stage(e.sweeping(), StageInput{
		Kind: approvalsubject.KindCommitmentTask, ProposedChange: []byte(`{"promiser_seat_id":"` + promiser.String() + `"}`),
		DiffHash: "archived-promiser", TargetType: tableDeal, TargetID: deal, Summary: "Send the pricing",
	})
	if !errors.Is(err, ErrNoDecider) {
		t.Fatalf("err = %v, want ErrNoDecider for a promise whose promiser is archived", err)
	}
	if n := e.count(t, `SELECT count(*) FROM approval`); n != 0 {
		t.Fatalf("%d approvals staged for somebody other than the promiser", n)
	}
}

func counterpartyFor(owner ids.UUID, deal ids.UUID) StageInput {
	return StageInput{
		Kind: approvalsubject.KindCounterparty, ProposedChange: []byte(`{"owner_id":"` + owner.String() + `"}`),
		DiffHash: "sender-" + owner.String(), TargetType: tableDeal, TargetID: deal, Summary: "Add a contact from mail",
	}
}

// A sender question is put only to the colleague whose mail it came from. When
// they cannot be asked, nobody is: not the record's owner, who could not open it.
func TestASenderQuestionWhoseMailboxOwnerCannotBeAskedIsPutToNobody(t *testing.T) {
	for _, tc := range []struct{ name, unaskable string }{
		{"archived", `UPDATE app_user SET archived_at = now() WHERE id = $1`},
		{"deactivated", `UPDATE app_user SET status = 'deactivated' WHERE id = $1`},
		{"an agent", `UPDATE app_user SET is_agent = true WHERE id = $1`},
		{"unknown", `DELETE FROM app_user WHERE id = $1`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := setupStaging(t)
			mailboxOwner := e.secondRep(t)
			if _, err := e.owner.Exec(context.Background(), tc.unaskable, mailboxOwner); err != nil {
				t.Fatal(err)
			}
			deal := newDealSeeder(t, e).dealOwnedBy(t, &e.rep)
			_, err := e.svc.Stage(e.sweeping(), counterpartyFor(mailboxOwner, deal))
			if !errors.Is(err, ErrNoDecider) {
				t.Fatalf("err = %v, want ErrNoDecider for a sender question whose mailbox owner is %s", err, tc.name)
			}
			if n := e.count(t, `SELECT count(*) FROM approval`); n != 0 {
				t.Fatalf("%d approvals staged for somebody other than the mailbox owner", n)
			}
		})
	}
}

func TestASenderQuestionIsPutToItsMailboxOwner(t *testing.T) {
	e := setupStaging(t)
	mailboxOwner := e.secondRep(t)
	deal := newDealSeeder(t, e).dealOwnedBy(t, &e.rep)
	id, err := e.svc.Stage(e.sweeping(), counterpartyFor(mailboxOwner, deal))
	if err != nil {
		t.Fatal(err)
	}
	if decider, source := e.deciderOf(t, id); decider == nil || *decider != mailboxOwner || source == nil || *source != string(deciderPayloadOwner) {
		t.Fatalf("decider = %v (%v), want the mailbox owner %s named by the payload", decider, source, mailboxOwner)
	}
}

// A sender question staged on behalf of somebody else would show one
// colleague's mail to another, so it is refused before anything is read.
func TestASenderQuestionStagedForAnotherMemberIsRefused(t *testing.T) {
	e := setupStaging(t)
	mailboxOwner := e.secondRep(t)
	deal := newDealSeeder(t, e).dealOwnedBy(t, &e.rep)
	if _, err := e.svc.Stage(e.forRep(), counterpartyFor(mailboxOwner, deal)); !errors.Is(err, ErrNoDecider) {
		t.Fatalf("err = %v, want ErrNoDecider for a sender question staged on behalf of another member", err)
	}
}
