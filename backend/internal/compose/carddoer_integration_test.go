// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"errors"
	"maps"
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
	e.GrantRole(t, e.Rep1, "rep")
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
	// The decided card keeps the payload it was accepted with, so its history
	// names the colleague the task went to.
	if got := e.WsScalar(t, `SELECT proposed_change->>'doer_id' FROM approval WHERE id = $1`, id); got != e.Rep1.String() {
		t.Errorf("the decided card names doer %q, want the chosen colleague %s", got, e.Rep1)
	}
}

// The card names the colleague it will go to, read from the doer member: the
// chosen one once the accepter picked somebody, else the promiser.
func TestThePromiseCardNamesTheColleagueChosenToDoIt(t *testing.T) {
	e, id, _ := stagedPromiseCard(t)
	promiser := e.As(e.Rep2, []ids.UUID{e.Team1}, integration.AdminPerms)
	card, err := NewApprovalsServiceWithEffects(e.Pool).GetWire(promiser, id)
	if err != nil {
		t.Fatal(err)
	}
	ownerOf := func(change map[string]any) string {
		card.ProposedChange = &change
		preview, err := newApprovalPreviewReader(e.DB()).read(promiser, card)
		if err != nil || preview == nil {
			t.Fatalf("reading the card's preview: %v", err)
		}
		for _, row := range preview.Changes {
			if row.Field == bulkParamOwner && row.After.EntityId != nil {
				return ids.UUID(*row.After.EntityId).String()
			}
		}
		return ""
	}
	staged := *card.ProposedChange
	if got := ownerOf(staged); got != e.Rep2.String() {
		t.Errorf("the staged card names %q, want the promiser %s", got, e.Rep2)
	}
	chosen := maps.Clone(staged)
	chosen[approvals.DoerField] = e.Rep1.String()
	if got := ownerOf(chosen); got != e.Rep1.String() {
		t.Errorf("the card with a chosen doer names %q, want the chosen colleague %s", got, e.Rep1)
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

// teamDealPolicy is a colleague whose stored role reads deals of their own
// team only.
const teamDealPolicy = `{"objects":{"activity":{"create":true,"read":true,"update":true},
	  "deal":{"read":true},"contact":{"read":true},"company":{"read":true}},"row_scope":"team"}`

// The deal owner may hand an overnight follow-up to a teammate as they approve.
// A colleague who cannot open the deal is refused. The task carries the deal's
// name and its latest conversation. One not signed in yet is refused too.
func TestTheDealOwnerHandsAFollowUpToAColleague(t *testing.T) {
	for name, c := range map[string]struct {
		prepare func(e *reconcileEnv) ids.UUID
		refused bool
	}{
		"a teammate": {prepare: func(e *reconcileEnv) ids.UUID {
			e.grantOwner(t, e.Rep2, teamDealPolicy)
			return e.Rep2
		}},
		"an invited teammate": {refused: true, prepare: func(e *reconcileEnv) ids.UUID {
			e.grantOwner(t, e.Rep2, teamDealPolicy)
			e.WsExec(t, `UPDATE app_user SET status = 'invited' WHERE id = $1`, e.Rep2)
			return e.Rep2
		}},
		"a colleague with no grant to read deals": {refused: true, prepare: func(e *reconcileEnv) ids.UUID {
			e.WsExec(t, `DELETE FROM role_assignment WHERE user_id = $1`, e.Rep3)
			e.grantOwner(t, e.Rep3, `{"objects":{"activity":{"create":true,"read":true}},"row_scope":"own"}`)
			return e.Rep3
		}},
	} {
		t.Run(name, func(t *testing.T) {
			e := setupReconcile(t)
			deal := e.SeedDeal(t, "Handed on", e.pipeline, e.open, &e.Rep1)
			e.seedInteraction(t, deal, "call", "Discovery", 1)
			if err := e.reconcile(); err != nil {
				t.Fatal(err)
			}
			doer := c.prepare(e)
			evidence, _, _ := e.followUpEvidence(t, deal)
			id, proposal := e.stageLegacyFollowUp(t, deal, evidence)
			proposal.DoerID = doer.String()
			edited, err := json.Marshal(proposal)
			if err != nil {
				t.Fatal(err)
			}
			_, err = e.svc.DecideEdited(e.As(e.Rep1, []ids.UUID{e.Team1}, reconcilePerms), id, edited)
			got := e.WsScalar(t, `SELECT coalesce(string_agg(a.assignee_id::text, ','), '') FROM activity a
				JOIN activity_link l ON l.activity_id = a.id AND l.deal_id = $1
				WHERE a.kind = 'task' AND a.source = 'overnight-reconcile'`, deal)
			if c.refused {
				if _, ok := errors.AsType[*approvals.InvalidEditError](err); !ok || got != "" {
					t.Fatalf("handing the follow-up to %s answered %v and wrote %q, want a refusal and no task", name, err, got)
				}
				return
			}
			if err != nil || got != doer.String() {
				t.Fatalf("the follow-up is held by %q (err %v), want the chosen colleague %s", got, err, doer)
			}
		})
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
