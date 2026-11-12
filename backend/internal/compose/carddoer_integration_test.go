// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// stagedPromiseCard reads a mailed promise put to Rep2 as a card, and returns
// it with its staged payload.
func stagedPromiseCard(t *testing.T) (*integration.Env, ids.ApprovalID, map[string]any) {
	t.Helper()
	e, _ := seedMailedPromise(t, "active")
	message := ids.MustParse(e.WsScalar(t, `SELECT activity_id::text FROM activity_identity
		WHERE identity_kind = 'mail' AND identity_key = 'thread-seat@buyer.example'`))
	if err := runSignalPass(t, e, mailedCommitmentReply(t, message, e.Rep2, 0.75)); err != nil {
		t.Fatalf("the signal pass: %v", err)
	}
	id := ids.MustParse(e.WsScalar(t, `SELECT id::text FROM approval WHERE kind = 'commitment_task' AND status = 'pending'`))
	var change map[string]any
	if err := json.Unmarshal([]byte(e.WsScalar(t, `SELECT proposed_change::text FROM approval WHERE id = $1`, id)), &change); err != nil {
		t.Fatal(err)
	}
	if doer, ok := change[approvals.DoerField]; !ok || doer != "" {
		t.Fatalf("the card stages %s = %v, want an empty doer the accepter can fill", approvals.DoerField, doer)
	}
	return e, ids.From[ids.ApprovalKind](id), change
}

// The colleague who made a promise may hand it to a teammate as they accept
// it. The task is the teammate's, and the card is spent.
func TestTheAccepterHandsAPromiseToAColleague(t *testing.T) {
	e, id, change := stagedPromiseCard(t)
	change[approvals.DoerField] = e.Rep1.String()
	edited, err := json.Marshal(change)
	if err != nil {
		t.Fatal(err)
	}
	promiser := e.As(e.Rep2, []ids.UUID{e.Team1}, integration.AdminPerms)
	if _, err := NewApprovalsServiceWithEffects(e.Pool).DecideEdited(promiser, id, edited); err != nil {
		t.Fatalf("accepting with a chosen colleague: %v", err)
	}
	if got := e.WsScalar(t, `SELECT coalesce(string_agg(assignee_id::text, ','), '') FROM activity WHERE kind = 'task'`); got != e.Rep1.String() {
		t.Errorf("the accepted promise is held by %q, want the chosen colleague %s", got, e.Rep1)
	}
}

// A colleague who cannot hold work is refused before the decision commits, so
// the card stays pending and nothing is written.
func TestAPromiseCannotBeHandedToAColleagueWhoLeft(t *testing.T) {
	e, id, change := stagedPromiseCard(t)
	e.WsExec(t, `UPDATE app_user SET status = 'deactivated' WHERE id = $1`, e.Rep1)
	change[approvals.DoerField] = e.Rep1.String()
	edited, err := json.Marshal(change)
	if err != nil {
		t.Fatal(err)
	}
	promiser := e.As(e.Rep2, []ids.UUID{e.Team1}, integration.AdminPerms)
	_, err = NewApprovalsServiceWithEffects(e.Pool).DecideEdited(promiser, id, edited)
	if _, ok := errors.AsType[*approvals.InvalidEditError](err); !ok {
		t.Fatalf("handing a promise to a departed colleague answered %v, want a field refusal", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM approval WHERE id = $1 AND status = 'pending'`, id); n != 1 {
		t.Error("the refused hand-over decided the card; it must stay pending")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task'`); n != 0 {
		t.Errorf("the refused hand-over wrote %d task(s)", n)
	}
}

// The deal owner may hand an overnight follow-up to a teammate as they approve.
func TestTheDealOwnerHandsAFollowUpToAColleague(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Handed on", e.pipeline, e.open, &e.Rep1)
	e.seedInteraction(t, deal, "call", "Discovery", 1)
	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	id, proposal := e.followUpApproval(t, deal)
	proposal.DoerID = e.Rep2.String()
	edited, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.DecideEdited(e.As(e.Rep1, []ids.UUID{e.Team1}, reconcilePerms), id, edited); err != nil {
		t.Fatalf("approving with a chosen colleague: %v", err)
	}
	if got := e.WsScalar(t, `SELECT coalesce(a.assignee_id::text, '') FROM activity a
		JOIN activity_link l ON l.activity_id = a.id AND l.deal_id = $1
		WHERE a.kind = 'task' AND a.source = 'overnight-reconcile'`, deal); got != e.Rep2.String() {
		t.Errorf("the follow-up is held by %q, want the chosen colleague %s", got, e.Rep2)
	}
}

// Both card payloads name their doer in the one member approvals leaves free.
func TestBothCardsNameTheirDoerInTheMemberApprovalsFrees(t *testing.T) {
	for name, payload := range map[string]any{
		"promise":   CommitmentTaskProposal{},
		"follow-up": deals.FollowUpProposal{},
	} {
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		var members map[string]json.RawMessage
		if err := json.Unmarshal(raw, &members); err != nil {
			t.Fatal(err)
		}
		if _, ok := members[approvals.DoerField]; !ok {
			t.Errorf("the %s card does not stage %s, so its accepter cannot choose a doer", name, approvals.DoerField)
		}
	}
}
