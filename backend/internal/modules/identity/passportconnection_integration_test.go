// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// The principal a connected agent authenticates to has to name the CONNECTION,
// not only the passport. approvals binds three rules to "is this the agent that
// staged the proposal" (sameAgent), and a passport id answers that wrongly the
// moment a client refreshes: the row is retired and a replacement is minted
// under the same grant, so the same agent comes back with a different id.
//
// Held here rather than in approvals because approvals can construct any
// principal it likes; only the authentication path can say what a real one
// carries.

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestARotatedPassportAuthenticatesToTheSameConnection(t *testing.T) {
	e := setupRevocationEnv(t, "passport-connection-rotation")
	fixture := e.connectOAuthFor(t, e.member)

	before := e.rotateIssuing(t, &fixture)
	after := e.rotateIssuing(t, &fixture)

	if before.ID == after.ID {
		t.Fatal("a rotation reissued the same passport row, so this test cannot observe what it exists for")
	}
	if _, err := e.svc.AuthenticateAgent(e.wsCtx(e.member), before.Token); err == nil {
		t.Fatal("the predecessor still authenticates after its successor was minted")
	}
	second, err := e.svc.AuthenticateAgent(e.wsCtx(e.member), after.Token)
	if err != nil {
		t.Fatalf("authenticating the renewed passport: %v", err)
	}
	if second.ConnectionID != fixture.grantID {
		t.Fatalf("the renewed passport authenticated to connection %v, want the grant %v it was minted under",
			second.ConnectionID, fixture.grantID)
	}
	if second.Principal().ConnectionID != fixture.grantID {
		t.Fatal("the connection reaches AgentIdentity but not the principal every store entry point sees")
	}
}

// A passport a human minted directly answers to no connection, which is what
// keeps sameAgent falling back to the passport id for it. A zero here would be
// indistinguishable from "connection not plumbed", so it is asserted rather
// than assumed.
func TestADirectlyMintedPassportCarriesNoConnection(t *testing.T) {
	e := setupRevocationEnv(t, "passport-connection-direct")
	issued, err := e.svc.IssuePassport(e.wsCtx(e.member), e.member, IssuePassportInput{Scopes: []string{"read"}})
	if err != nil {
		t.Fatalf("minting a passport: %v", err)
	}
	agent, err := e.svc.AuthenticateAgent(e.wsCtx(e.member), issued.Token)
	if err != nil {
		t.Fatalf("authenticating the minted passport: %v", err)
	}
	if agent.ConnectionID != ids.Nil {
		t.Fatalf("a directly minted passport named connection %v", agent.ConnectionID)
	}
}
