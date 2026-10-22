// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Accepting a promise read out of a meeting writes the task as the human who
// accepts it. Every record the meeting is filed under must be one they can
// see, because reaching a meeting through one link says nothing of its others.

import (
	"context"
	"errors"
	"maps"
	"testing"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// stagedMeetingPromise is one staged proposal read from a meeting filed under
// a shared contact, a contact private to privateOwner, and a project when
// underProject.
type stagedMeetingPromise struct {
	*transcriptEnv
	project, private ids.UUID
	approval         ids.ApprovalID
}

func stageMeetingPromise(
	t *testing.T, privateOwner func(*transcriptEnv) ids.UUID, underProject bool,
) stagedMeetingPromise {
	t.Helper()
	e := setupTranscript(t)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	shared := e.SeedContact(t, "Dana Weiss", &e.Rep1)
	private := e.SeedContact(t, "Jonas Brandt", nil)
	e.MakeCapturePrivate(t, "contact", private, privateOwner(e))
	project, err := ids.Parse(e.WsScalar(t, `INSERT INTO project (name, company_id, owner_id, source, captured_by)
		VALUES ('Rollout', $1, $2, 'manual', 'human:x') RETURNING id::text`, company, e.Rep1))
	if err != nil {
		t.Fatalf("seeding the project: %v", err)
	}
	for _, contact := range []ids.UUID{shared, private} {
		e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`,
			e.activity, contact)
	}
	if underProject {
		e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, project_id) VALUES ($1, 'project', $2)`,
			e.activity, project)
	}
	read := e.read(t, cannedBrain{reply: groundedReply(t, 3, 0.9, e.Rep1)})
	if len(read.ProposalIDs) != 1 {
		t.Fatalf("want one staged proposal, got %d (%v)", len(read.ProposalIDs), read.StatusDetail)
	}
	return stagedMeetingPromise{
		transcriptEnv: e, project: project, private: private,
		approval: ids.From[ids.ApprovalKind](read.ProposalIDs[0]),
	}
}

// The contact is private to a rep in another team: the acceptance is refused
// whole, no task is written, and the card still waits.
func TestAcceptingAMeetingPromiseFiledUnderAContactTheDeciderCannotSeeIsRefused(t *testing.T) {
	s := stageMeetingPromise(t, func(e *transcriptEnv) ids.UUID { return e.Rep3 }, true)

	if _, err := s.svc.Decide(s.ctx, s.approval, true, nil); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("accepting with a link the decider cannot see: err = %v, want not found", err)
	}
	if n := s.taskCount(t); n != 0 {
		t.Errorf("a refused acceptance wrote %d task(s)", n)
	}
	if status := s.wsString(t, `SELECT status FROM approval WHERE id = $1`, s.approval.UUID); status != "pending" {
		t.Errorf("the refused card is %q, want it still pending", status)
	}
}

// The write asks again, for a record hidden after the precheck answered: run
// with no precheck in front of it, the effect alone writes nothing.
func TestTheAcceptedWriteRefusesALinkTheDeciderCannotSee(t *testing.T) {
	s := stageMeetingPromise(t, func(e *transcriptEnv) ids.UUID { return e.Rep3 }, true)
	writeOnly := approvals.NewService(s.DB())
	writeOnly.WithEffect(CommitmentTaskKind, commitmentTaskEffect(writeOnly, s.Activities, s.Contacts))

	if _, err := writeOnly.Decide(s.ctx, s.approval, true, nil); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("writing with a link the decider cannot see: err = %v, want not found", err)
	}
	if n := s.taskCount(t); n != 0 {
		t.Errorf("a refused write left %d task(s)", n)
	}
}

// A card whose write was refused stays owed its task. Once the decider can see
// the contact, deciding again re-drives the write, past the precheck's guard.
func TestARefusedWriteIsRedrivenOnceTheDeciderCanSeeTheLink(t *testing.T) {
	s := stageMeetingPromise(t, func(e *transcriptEnv) ids.UUID { return e.Rep3 }, true)
	writeOnly := approvals.NewService(s.DB())
	writeOnly.WithEffect(CommitmentTaskKind, commitmentTaskEffect(writeOnly, s.Activities, s.Contacts))
	if _, err := writeOnly.Decide(s.ctx, s.approval, true, nil); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("writing with a link the decider cannot see: err = %v, want not found", err)
	}
	s.MakeCapturePrivate(t, "contact", s.private, s.Rep1)

	if _, err := s.svc.Decide(s.ctx, s.approval, true, nil); err != nil {
		t.Fatalf("deciding again once the contact is visible: %v", err)
	}
	if n := s.taskCount(t); n != 1 {
		t.Errorf("the re-driven write left %d task(s), want one", n)
	}
	var decided *approvals.AlreadyDecidedError
	if _, err := s.svc.Decide(s.ctx, s.approval, true, nil); !errors.As(err, &decided) {
		t.Errorf("deciding a spent card again: err = %v, want already decided", err)
	}
}

// The private contact is the decider's own, so they see every record the
// meeting is filed under, and the task carries all of them.
func TestAcceptingAMeetingPromiseTheDeciderCanSeeWholeCarriesItsLinks(t *testing.T) {
	s := stageMeetingPromise(t, func(e *transcriptEnv) ids.UUID { return e.Rep1 }, true)

	if _, err := s.svc.Decide(s.ctx, s.approval, true, nil); err != nil {
		t.Fatalf("accepting: %v", err)
	}
	task := s.wsString(t, `SELECT id::text FROM activity WHERE kind = 'task'`)
	if n := s.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1`, task); n != 3 {
		t.Errorf("the task carries %d link(s), want both contacts and the project", n)
	}
	if n := s.WsCount(t, `SELECT count(*) FROM activity_link
		WHERE activity_id = $1 AND (project_id = $2 OR contact_id = $3)`, task, s.project, s.private); n != 2 {
		t.Errorf("the task carries %d of the project and the private contact, want both", n)
	}
	if by := s.wsString(t, `SELECT captured_by FROM activity WHERE id = $1`, task); by != "human:"+s.Rep1.String() {
		t.Errorf("the task is captured by %q, want the human who accepted it", by)
	}
}

// A decider who may create activities but not change them cannot file one
// under a project. The precheck asks what the write would, so the card waits.
func TestADeciderWhoMayNotFileUnderAProjectLeavesTheCardPending(t *testing.T) {
	s := stageMeetingPromise(t, func(e *transcriptEnv) ids.UUID { return e.Rep1 }, true)
	createOnly := transcriptPerms
	createOnly.Objects = maps.Clone(transcriptPerms.Objects)
	createOnly.Objects["activity"] = principal.ObjectGrant{Create: true, Read: true}
	decider := s.As(s.Rep1, []ids.UUID{s.Team1}, createOnly)

	if _, err := s.svc.Decide(decider, s.approval, true, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("accepting a project filing without activity update: err = %v, want permission denied", err)
	}
	if status := s.wsString(t, `SELECT status FROM approval WHERE id = $1`, s.approval.UUID); status != "pending" {
		t.Errorf("the refused card is %q, want it still pending", status)
	}
}

// An agent deciding for its human through the tool surface spends the card the
// server staged. The task is written under the agent's own bounds.
func TestAnAgentAcceptingForItsHumanWritesTheTask(t *testing.T) {
	s := stageMeetingPromise(t, func(e *transcriptEnv) ids.UUID { return e.Rep1 }, false)
	passport := ids.NewV7()
	if _, err := s.owner.Exec(context.Background(), `
		INSERT INTO passport (id, on_behalf_of, granted_by, token_hash, scopes, expires_at)
		VALUES ($1, $2, $2, $3, ARRAY['read','write'], now() + interval '30 days')`,
		passport, s.Rep1, "hash-"+passport.String()); err != nil {
		t.Fatalf("seeding the passport: %v", err)
	}
	agent := principal.WithActor(principal.WithCorrelationID(
		principal.WithWorkspaceID(context.Background(), s.WS), ids.NewV7()), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:" + s.Rep1.String(), SeatType: principal.SeatFull,
		PassportID: passport, OnBehalfOf: s.Rep1, UserID: s.Rep1, TeamIDs: []ids.UUID{s.Team1},
		Scopes: principal.NewScopeSet(principal.ScopeWrite), Permissions: transcriptPerms,
	})

	if _, err := (approvalInbox{svc: s.svc}).DecideApproval(agent, s.approval.UUID, true, ""); err != nil {
		t.Fatalf("the agent accepting: %v", err)
	}
	if n := s.taskCount(t); n != 1 {
		t.Fatalf("the agent's acceptance wrote %d task(s), want one", n)
	}
	if consumed := s.wsString(t, `SELECT (consumed_at IS NOT NULL)::text FROM approval WHERE id = $1`,
		s.approval.UUID); consumed != "true" {
		t.Error("the card the agent accepted was not spent")
	}
}
