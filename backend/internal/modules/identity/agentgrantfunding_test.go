// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// Whether a credential minted at some past moment still covers what its agent
// does today.
//
// This is the half liveness cannot see. A passport neither revoked nor expired
// is perfectly good authority — for the job the agent did when the rep answered.
// When the agent gains a tool needing a wider scope, the run does not fail: it
// degrades the unfunded tools away before the first model step and prepares
// nothing. No error, no expiry, no prompt, at 2am.

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/agentgrant"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestACredentialIsShortWhenItsAgentHasGrownSinceItWasMinted(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		spec  string
		held  []string
		funds bool
	}{
		{
			name:  "exactly what the build would mint today",
			spec:  "morning_brief",
			held:  []string{"read", "write"},
			funds: true,
		},
		{
			// The shape that shipped: morning_brief was granted {"read"} and
			// later gained annotate_brief, a write tool.
			name:  "the scopes the agent needed before it gained a tool",
			spec:  "morning_brief",
			held:  []string{"read"},
			funds: false,
		},
		{
			// Wider is not short. A rep whose passport carries more than this
			// agent needs is not owed a renewal prompt, and telling them
			// otherwise sends them to revoke authority that works.
			name:  "more than the agent needs",
			spec:  "morning_brief",
			held:  []string{"read", "write", "admin"},
			funds: true,
		},
		{
			name:  "nothing at all",
			spec:  "morning_brief",
			held:  nil,
			funds: false,
		},
		{
			// There is no scope list to meet, so "covered" would be a claim
			// about a requirement nobody in this build can state.
			name:  "an agent this build cannot spell",
			spec:  "agent_from_another_deployment",
			held:  []string{"read", "write", "admin"},
			funds: false,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := credentialFundsAgent(c.spec, c.held); got != c.funds {
				t.Errorf("credentialFundsAgent(%q, %v) = %v, want %v",
					c.spec, c.held, got, c.funds)
			}
		})
	}
}

// TestTheRenderedGrantReportsBothRenewalCauses holds the two apart on the wire.
//
// A client shows one notice or the other, so a response that conflated them
// would tell a rep their authority expired when nothing did — sending them to
// look for a lapse that never happened.
func TestTheRenderedGrantReportsBothRenewalCauses(t *testing.T) {
	t.Parallel()
	passport := ids.From[ids.PassportKind](ids.NewV7())
	for _, c := range []struct {
		name       string
		answer     agentgrant.Answer
		wantUsable bool
		wantFunds  bool
	}{
		{
			name: "a live credential that still covers the agent",
			answer: agentgrant.Answer{
				State: agentgrant.StateGranted, PassportID: &passport,
				CredentialUsable: true, PassportScopes: []string{"read", "write"},
			},
			wantUsable: true, wantFunds: true,
		},
		{
			name: "a live credential the agent has outgrown",
			answer: agentgrant.Answer{
				State: agentgrant.StateGranted, PassportID: &passport,
				CredentialUsable: true, PassportScopes: []string{"read"},
			},
			wantUsable: true, wantFunds: false,
		},
		{
			// A lapsed passport reports no scopes, so it is short as well as
			// dead. Both are true and the client shows the expiry notice,
			// because that is the thing the rep can act on.
			name: "a lapsed credential",
			answer: agentgrant.Answer{
				State: agentgrant.StateGranted, PassportID: &passport,
				CredentialUsable: false, PassportScopes: nil,
			},
			wantUsable: false, wantFunds: false,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out := renderAgentGrant("morning_brief", c.answer, true)
			if out.CredentialUsable != c.wantUsable {
				t.Errorf("credential_usable = %v, want %v", out.CredentialUsable, c.wantUsable)
			}
			if out.CredentialFundsAgent != c.wantFunds {
				t.Errorf("credential_funds_agent = %v, want %v", out.CredentialFundsAgent, c.wantFunds)
			}
		})
	}
}
