// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A buyer's audit row names the CONTACT, not their kind.
//
// audit_log records a buyer's action with actor_type='buyer' and
// actor_id='buyer:<participant uuid>'. The read path resolved actor_name from
// app_user alone, which a buyer is not in — they hold no seat and appear in no
// member directory — so the row arrived nameless and the audit screen rendered
// the kind. The actor kind exists so a disputed negotiation can answer "who
// confirmed v5?"; it answered "a Deal Room participant did" (#2235).
//
// Driven end to end rather than against the join, because the two halves are
// what makes it true: the write has to record the participant's id in the
// shape the read builds its key from, and a test that seeded the row by hand
// would agree with itself about a spelling neither end uses.

import (
	"context"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// auditReaderContext is the compliance reader the log is written for: a human
// with audit_log:read and an unbounded row scope, which is what the read
// demands and nothing more.
func auditReaderContext(t *testing.T, e *apptest.AppEnv) context.Context {
	t.Helper()
	wsID := apptest.InstallationWorkspaceUUID(context.Background(), t, e.Pool)
	ctx := principal.WithWorkspaceID(context.Background(), wsID)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	user := ids.NewV7()
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"audit_log": {Read: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}

func TestABuyersAuditRowNamesTheParticipant(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	room := openRoomWithABuyer(t, e)

	var session AnyMap
	if status := publicCall(t, e, "POST", "/v1/public/rooms/exchange",
		AnyMap{"credential": room.credential}, nil, &session); status != http.StatusOK {
		t.Fatalf("exchange = %d %v", status, session)
	}
	token, _ := session["session_token"].(string)

	// The buyer acts. Opening a thread is audited, and it is the buyer's own
	// act rather than the seller's — which is the row this is about.
	if status := publicCall(t, e, "POST", "/v1/public/rooms/threads",
		AnyMap{"body": "Can we move the start date?"}, bearer(token), nil); status != http.StatusCreated {
		t.Fatalf("the buyer opens a thread = %d", status)
	}

	entries, err := privacy.ListAuditLog(auditReaderContext(t, e), e.DB(), privacy.AuditFilter{})
	if err != nil {
		t.Fatalf("reading the audit log: %v", err)
	}
	var buyerRows int
	for _, entry := range entries.Entries {
		if entry.ActorType != "buyer" {
			continue
		}
		buyerRows++
		if entry.ActorName == nil {
			t.Errorf("a buyer's audit row (%s %s) carries no name — the trail answers \"who did this?\" "+
				"with the kind, which is what the actor kind was added to stop", entry.Action, entry.ActorID)
			continue
		}
		if *entry.ActorName != "Laura Buyer" {
			t.Errorf("the buyer's audit row names %q, want \"Laura Buyer\"", *entry.ActorName)
		}
	}
	if buyerRows == 0 {
		t.Fatal("no audit row carries actor_type='buyer' — the buyer acted and the trail did not " +
			"record it as theirs, so this test is passing on an empty set")
	}
}

// An audit row naming a participant the tree no longer holds keeps the row and
// loses the name, which is the LEFT half of the join doing its job.
//
// A participant cannot simply be deleted — a thread they opened holds them by
// foreign key, which is itself the trail refusing to lose its author. What can
// happen is a row whose participant is gone another way: a room dropped before
// its audit rows, a restore that brought back less than it took. An invented
// name would be worse than none, and the human join already takes that posture.
func TestAnAuditRowForAnUnknownParticipantKeepsTheRowAndLosesTheName(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	openRoomWithABuyer(t, e)

	stranger := ids.NewV7()
	if _, err := e.Owner.Exec(t.Context(),
		`INSERT INTO audit_log (id, actor_type, actor_id, action, entity_type, entity_id, occurred_at)
		 VALUES ($1, 'buyer', $2, 'create', 'deal_room_thread', $3, now())`,
		ids.NewV7(), "buyer:"+stranger.String(), ids.NewV7()); err != nil {
		t.Fatalf("seeding the orphan row: %v", err)
	}

	entries, err := privacy.ListAuditLog(auditReaderContext(t, e), e.DB(), privacy.AuditFilter{})
	if err != nil {
		t.Fatalf("reading the audit log: %v", err)
	}
	var found bool
	for _, entry := range entries.Entries {
		if entry.ActorID != "buyer:"+stranger.String() {
			continue
		}
		found = true
		if entry.ActorName != nil {
			t.Errorf("a row naming a participant the tree does not hold resolved %q", *entry.ActorName)
		}
	}
	if !found {
		t.Error("the orphan row did not come back at all — the join dropped an audit row, which is " +
			"the one thing a LEFT join is there to prevent")
	}
}
