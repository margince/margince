// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package jobfanout

// The rep's standing grant over HTTP, against real migrated Postgres.
//
// Every assertion here turns on something a unit test cannot see: a passport
// row's scopes and revocation, the composite foreign key binding a grant to its
// owner's credential, and the advisory lock that makes two simultaneous answers
// resolve to one live credential rather than two.

import (
	"context"
	"net/http"
	"slices"
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
)

// grantPath is the endpoint these tests answer on.
const grantPath = "/v1/me/agent-grants/morning_brief"

// answerGrant puts one answer as the signed-in rep.
func (re *runnerEnv) answerGrant(t *testing.T, granted bool) int {
	t.Helper()
	return re.Call(t, "PUT", grantPath, integration.AnyMap{"granted": granted}, nil, nil)
}

func TestGrantingMintsTheRepsOwnCredentialAndNothingWider(t *testing.T) {
	re := setupRunner(t)
	if status := re.answerGrant(t, true); status != http.StatusOK {
		t.Fatalf("grant → %d", status)
	}

	var scopes []string
	var selfBound bool
	if err := re.Owner.QueryRow(context.Background(), `
		SELECT p.scopes, p.on_behalf_of = p.granted_by
		  FROM agent_standing_grant g JOIN passport p ON p.id = g.passport_id
		 WHERE g.agent_spec = 'morning_brief'`).Scan(&scopes, &selfBound); err != nil {
		t.Fatalf("reading the minted credential: %v", err)
	}
	// on_behalf_of = granted_by is the invariant the whole feature rests on: a
	// rep can only ever be acted for by a credential they minted themselves.
	if !selfBound {
		t.Error("the minted passport does not act for the user who granted it")
	}
	// WHICH scopes is not asserted here. Whether the set funds the agent's
	// declared tools is a question about the tool specs, and
	// TestEveryGrantFundsTheToolsItsAgentDeclares (backend/agentgrantscopes_test.go)
	// derives it from them — this test carried `read` alone, and one PR later
	// the agent gained annotate_brief and the copy was simply wrong.
	//
	// What THIS test can prove and that one cannot is that a real mint over
	// real Postgres produces a non-empty scope set at all: a credential with no
	// scopes funds nothing and would degrade every run.
	if len(scopes) == 0 {
		t.Error("the minted passport carries no scopes, so every run degrades before its first step")
	}
	// And never wider than the agent could possibly need: send and enrich reach
	// outside the workspace, and no scheduled agent declares a tool requiring
	// either.
	for _, tooWide := range []string{"send", "enrich"} {
		if slices.Contains(scopes, tooWide) {
			t.Errorf("the overnight credential carries %q, which no declared tool requires", tooWide)
		}
	}
}

// TestAGrantMintedBeforeTheAgentGrewReportsItselfShort is the case liveness
// cannot see.
//
// The passport is neither revoked nor expired, so `credential_usable` is true —
// it is perfectly good authority for the job the agent did when the rep
// answered. What it is not is enough for the job the agent does now. Nothing
// fails: the runner degrades the unfunded tools before the first model step and
// the rep's brief is simply not prepared, silently, at 2am. The only thing that
// can tell them is this field.
//
// The passport is narrowed IN PLACE rather than minted narrow, because that is
// what actually happened: the rows were written by an older build and no
// migration or renewal path touches them.
func TestAGrantMintedBeforeTheAgentGrewReportsItselfShort(t *testing.T) {
	re := setupRunner(t)
	if status := re.answerGrant(t, true); status != http.StatusOK {
		t.Fatalf("grant → %d", status)
	}
	if _, err := re.Owner.Exec(context.Background(), `
		UPDATE passport SET scopes = ARRAY['read']
		 WHERE id = (SELECT passport_id FROM agent_standing_grant
		              WHERE agent_spec = 'morning_brief')`); err != nil {
		t.Fatalf("narrowing the credential to what an older build minted: %v", err)
	}

	var body struct {
		Data []struct {
			Spec             string `json:"spec"`
			State            string `json:"state"`
			CredentialUsable bool   `json:"credential_usable"`
			FundsAgent       bool   `json:"credential_funds_agent"`
		} `json:"data"`
	}
	if status := re.Call(t, "GET", "/v1/me/agent-grants", nil, nil, &body); status != http.StatusOK {
		t.Fatalf("listing the rep's grants → %d", status)
	}
	found := false
	for _, grant := range body.Data {
		if grant.Spec != "morning_brief" {
			continue
		}
		found = true
		if grant.State != "granted" {
			t.Errorf("state = %q, want granted: the rep answered and nothing has changed that", grant.State)
		}
		if !grant.CredentialUsable {
			t.Error("credential_usable is false on a passport that is neither revoked nor expired — " +
				"reporting an expiry sends the rep looking for a lapse that never happened")
		}
		if grant.FundsAgent {
			t.Error("credential_funds_agent is true on a passport minted before the agent gained a write tool: " +
				"the run degrades every night and the rep is told nothing is wrong")
		}
	}
	if !found {
		t.Fatal("the rep's own grant list does not carry morning_brief, so this test asserted nothing")
	}
}

func TestWithdrawingEndsTheAuthorityRatherThanTheReference(t *testing.T) {
	re := setupRunner(t)
	if status := re.answerGrant(t, true); status != http.StatusOK {
		t.Fatalf("grant → %d", status)
	}
	if status := re.answerGrant(t, false); status != http.StatusOK {
		t.Fatalf("withdraw → %d", status)
	}

	var live int
	if err := re.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM passport WHERE label = 'overnight brief' AND revoked_at IS NULL`).
		Scan(&live); err != nil {
		t.Fatalf("counting live credentials: %v", err)
	}
	// Dropping the reference without revoking would leave the agent able to act
	// after the rep said stop — the one outcome a withdrawal must not have.
	if live != 0 {
		t.Errorf("%d overnight credential(s) are still live after a withdrawal", live)
	}
	// The decline is REMEMBERED. A rep who said no and one never asked are
	// indistinguishable from the passport table alone, and a product that
	// cannot tell them apart asks the declining rep again every night.
	var state string
	if err := re.Owner.QueryRow(context.Background(),
		`SELECT state FROM agent_standing_grant WHERE agent_spec = 'morning_brief'`).
		Scan(&state); err != nil {
		t.Fatalf("reading the remembered answer: %v", err)
	}
	if state != "declined" {
		t.Errorf("the withdrawal left the answer %q, want declined", state)
	}
}

func TestReGrantingLeavesExactlyOneLiveCredential(t *testing.T) {
	re := setupRunner(t)
	for pass := 0; pass < 3; pass++ {
		if status := re.answerGrant(t, true); status != http.StatusOK {
			t.Fatalf("grant %d → %d", pass+1, status)
		}
	}

	var live int
	var grantCredentialLive bool
	if err := re.Owner.QueryRow(context.Background(), `
		SELECT (SELECT count(*) FROM passport WHERE label = 'overnight brief' AND revoked_at IS NULL),
		       (SELECT p.revoked_at IS NULL FROM agent_standing_grant g
		          JOIN passport p ON p.id = g.passport_id WHERE g.agent_spec = 'morning_brief')`).
		Scan(&live, &grantCredentialLive); err != nil {
		t.Fatalf("reading the credentials: %v", err)
	}
	// A re-grant that minted without revoking would leave the previous
	// credential live and referenced by nothing — standing authority nobody can
	// find in order to end it.
	if live != 1 {
		t.Errorf("%d live overnight credentials after three grants, want exactly 1", live)
	}
	if !grantCredentialLive {
		t.Error("the answer points at a credential that is not live")
	}
}

func TestTwoSimultaneousAnswersStillLeaveOneLiveCredential(t *testing.T) {
	re := setupRunner(t)

	// The race the advisory lock exists for: both calls read "no live
	// credential", both mint, and the upsert keeps one — leaving the other live
	// and unreferenced. Serialized, the second waits and revokes the first's.
	// Released together, so the two requests reach the read at the same moment
	// rather than one landing while the other is still being built — the window
	// is a few statements wide, and two sequential Call setups miss it.
	const answers = 6
	start := make(chan struct{})
	var wg sync.WaitGroup
	statuses := make([]int, answers)
	for i := range statuses {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			<-start
			statuses[slot] = re.Call(t, "PUT", grantPath, integration.AnyMap{"granted": true}, nil, nil)
		}(i)
	}
	close(start)
	wg.Wait()
	for i, status := range statuses {
		if status != http.StatusOK {
			t.Fatalf("concurrent grant %d → %d", i+1, status)
		}
	}

	var live int
	if err := re.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM passport WHERE label = 'overnight brief' AND revoked_at IS NULL`).
		Scan(&live); err != nil {
		t.Fatalf("counting live credentials: %v", err)
	}
	if live != 1 {
		t.Errorf("%d live overnight credentials after simultaneous answers, want 1 — "+
			"the extra one is authority nothing references and nobody can find to revoke", live)
	}
}

func TestAnUnknownAgentIsRefusedRatherThanGranted(t *testing.T) {
	re := setupRunner(t)
	status := re.Call(t, "PUT", "/v1/me/agent-grants/not_an_agent",
		integration.AnyMap{"granted": true}, nil, nil)
	// 422, not 200: minting a credential for an agent this build does not
	// schedule is authority that can never be used and never be found.
	if status != http.StatusUnprocessableEntity && status != http.StatusBadRequest {
		t.Errorf("granting an unknown agent → %d, want a refusal", status)
	}
}
