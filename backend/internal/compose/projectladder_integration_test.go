// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The project attribution ladder over a real migrated Postgres, through the
// composed capture sink a mail sync actually runs.
//
// The ladder files a captured message under a project on three signals, each
// following a link a human already made: a project key the sender wrote in the
// subject, a sibling in the same thread somebody already filed, or a deal the
// message is on that belongs to a project. It has no fourth signal. A message
// carrying none of them is filed under nothing AND asks nobody — the case with
// no assertion of its own until this file, because "nothing happened" is what a
// silently broken ladder also looks like.
//
// Every fixture is written by the thing that writes it in production: the
// account and contact through the people store, the project through the
// projects store, the message through the composed sink.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// ladderCounterparty writes the mail the ladder files.
const ladderCounterparty = "dana@ladder.example"

// ladderAccount is an account with a contact and a sink, seeded the way
// production seeds them.
type ladderAccount struct {
	e          *integration.Env
	orgID      ids.UUID
	sink       *capture.Sink
	captureCtx context.Context
}

func seedLadderAccount(t *testing.T, e *integration.Env) ladderAccount {
	t.Helper()
	orgID := e.SeedOrg(t, "Ladder Works", nil)
	person, err := e.People.CreatePerson(e.Admin(), people.CreatePersonInput{
		FullName: "Dana Ladder", Source: "manual",
		Emails: []people.PersonEmailInput{{Email: ladderCounterparty, EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seeding the contact: %v", err)
	}
	personID := ids.From[ids.PersonKind](ids.UUID(person.Id))
	employer := ids.From[ids.OrganizationKind](orgID)
	if _, err := e.People.CreateRelationship(e.Admin(), people.CreateRelationshipInput{
		Kind: "employment", PersonID: &personID, OrganizationID: &employer, Source: "manual",
	}); err != nil {
		t.Fatalf("seeding the employment: %v", err)
	}
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:gmail",
		UserID: e.Rep1, OnBehalfOf: e.Rep1,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"activity":     {Create: true, Read: true, Update: true},
				"person":       {Create: true, Read: true},
				"organization": {Create: true, Read: true},
				"project":      {Read: true},
				"deal":         {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
	return ladderAccount{e: e, orgID: orgID, sink: newCaptureSink(e.Pool, CaptureConfig{}), captureCtx: ctx}
}

// project opens one live project on the account and answers its id and key.
// The key is minted by the server, so the test reads back what it must write
// in a subject rather than choosing it.
func (a ladderAccount) project(t *testing.T, name string) (ids.UUID, string) {
	t.Helper()
	created, err := a.e.Projects.CreateProject(a.e.Admin(), projects.CreateProjectInput{
		Name: name, OrganizationID: ids.From[ids.OrganizationKind](a.orgID), Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating project %q: %v", name, err)
	}
	if created.Key == nil || *created.Key == "" {
		t.Fatalf("project %q was minted without a key, so no subject can name it", name)
	}
	return ids.UUID(created.Id), *created.Key
}

// capture lands one inbound mail through the composed sink, with the ladder
// running post-commit exactly as a sync runs it.
func (a ladderAccount) capture(t *testing.T, sourceID, threadKey, subject string) ids.UUID {
	t.Helper()
	if threadKey == "" {
		threadKey = sourceID
	}
	ref, err := a.sink.Upsert(a.captureCtx, connector.NormalizedRecord{
		EntityType: datasource.EntityActivity,
		NaturalKey: connector.NaturalKey{SourceSystem: "gmail", SourceID: sourceID},
		Fields: capture.ActivityFields{
			Kind: "email", Subject: subject, Body: "hello", Direction: connector.DirectionInbound,
		},
		Source:     "gmail:" + sourceID,
		CapturedBy: "connector:gmail",
		Raw:        []byte("From: " + ladderCounterparty + "\r\n\r\nhello"),
		ThreadKey:  threadKey,
		Counterparty: connector.Counterparty{
			Email: ladderCounterparty, Domain: "ladder.example", Direction: connector.DirectionInbound,
		},
	})
	if err != nil {
		t.Fatalf("capturing %s: %v", sourceID, err)
	}
	return ref.ID
}

// filedUnder answers the project the activity carries a link to, or the zero
// id when it carries none.
func filedUnder(t *testing.T, e *integration.Env, activityID ids.UUID) ids.UUID {
	t.Helper()
	var projectID ids.UUID
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT coalesce((SELECT project_id FROM activity_link
			                  WHERE activity_id = $1 AND entity_type = 'project'),
			                 '00000000-0000-0000-0000-000000000000'::uuid)`,
			activityID).Scan(&projectID)
	})
	if err != nil {
		t.Fatalf("reading the activity's project link: %v", err)
	}
	return projectID
}

// pendingApprovalsAbout counts every staged card naming this activity,
// whatever its kind. Kind-agnostic on purpose: the claim under test is that
// an unfilable message raises NO question, and a census naming only the kinds
// it expects would pass while a different kind asked one.
func pendingApprovalsAbout(t *testing.T, e *integration.Env, activityID ids.UUID) int {
	t.Helper()
	var n int
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM approval WHERE target_entity_id = $1 AND status = 'pending'`,
			activityID).Scan(&n)
	})
	if err != nil {
		t.Fatalf("counting the staged cards: %v", err)
	}
	return n
}

// The rung the founder decision rests on: the key in the subject files the
// message, and it is the only thing that has to be true.
func TestAProjectKeyInTheSubjectFilesTheMessage(t *testing.T) {
	e := integration.Setup(t)
	a := seedLadderAccount(t, e)
	projectID, key := a.project(t, "Ladder Rollout")

	activityID := a.capture(t, "key-1", "", "["+key+"] status this week")

	if got := filedUnder(t, e, activityID); got != projectID {
		t.Errorf("a subject naming %s filed the message under %v, want %v", key, got, projectID)
	}
}

// A reply carries the key its thread was opened with, so the same rung files
// it — no thread lookup required.
func TestAReplyKeepingTheKeyIsFiledByTheKey(t *testing.T) {
	e := integration.Setup(t)
	a := seedLadderAccount(t, e)
	projectID, key := a.project(t, "Ladder Rollout")

	activityID := a.capture(t, "key-reply-1", "", "Re: ["+key+"] status this week")

	if got := filedUnder(t, e, activityID); got != projectID {
		t.Errorf("a reply naming %s filed the message under %v, want %v", key, got, projectID)
	}
}

// The stickiness rung: a sender who trims the key off their reply is still
// answering a conversation somebody filed, and the sibling settles it.
func TestAReplyThatDroppedTheKeyInheritsItsThreadsProject(t *testing.T) {
	e := integration.Setup(t)
	a := seedLadderAccount(t, e)
	projectID, key := a.project(t, "Ladder Rollout")

	thread := "thread-root-1"
	opener := a.capture(t, thread, thread, "["+key+"] kickoff")
	if got := filedUnder(t, e, opener); got != projectID {
		t.Fatalf("the thread opener was not filed, so the reply proves nothing: got %v", got)
	}

	reply := a.capture(t, "reply-no-key-1", thread, "Re: kickoff")

	if got := filedUnder(t, e, reply); got != projectID {
		t.Errorf("a keyless reply on a filed thread landed under %v, want %v", got, projectID)
	}
}

// The founder decision, stated as a test: no key, no filed sibling, no deal —
// the message is filed under nothing and NOBODY is asked. The second assertion
// is the one that matters. Filing nothing was always the behaviour; asking
// nothing is what changed, and a card raised here would be the retired rung
// back from the dead.
func TestAMessageNamingNoProjectIsFiledNowhereAndAsksNobody(t *testing.T) {
	e := integration.Setup(t)
	a := seedLadderAccount(t, e)
	// One live project on the account, and the message names it nowhere. This
	// is exactly the shape the retired rung proposed on — sole_live_project,
	// confidence 1 — so the project's presence is what makes this test able to
	// fail rather than pass vacuously.
	a.project(t, "Ladder Rollout")

	activityID := a.capture(t, "no-key-1", "", "quick question about invoicing")

	if got := filedUnder(t, e, activityID); !got.IsZero() {
		t.Errorf("a message naming no project was filed under %v; the ladder guessed", got)
	}
	if n := pendingApprovalsAbout(t, e, activityID); n != 0 {
		t.Errorf("a message naming no project raised %d card(s); the ladder asked a question "+
			"the subject key was supposed to make unnecessary", n)
	}
}

// Ambiguity is answered the same way silence is. Two keys in one subject name
// nothing reliably, so the message is filed nowhere rather than under whichever
// key the tokenizer happened to read first.
func TestASubjectNamingTwoProjectsFilesTheMessageUnderNeither(t *testing.T) {
	e := integration.Setup(t)
	a := seedLadderAccount(t, e)
	_, first := a.project(t, "Ladder Rollout")
	_, second := a.project(t, "Ladder Migration")

	activityID := a.capture(t, "two-keys-1", "", "["+first+"] and ["+second+"] together")

	if got := filedUnder(t, e, activityID); !got.IsZero() {
		t.Errorf("a subject naming both %s and %s filed the message under %v; ambiguity was resolved by picking",
			first, second, got)
	}
}
