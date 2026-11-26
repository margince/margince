// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

// The guard that keeps an agent-minted staging away from a server-side
// executor. The kind string alone cannot distinguish them — "enrich" names
// both a compose proposal flow and the tool behind three agent-reachable
// routes — so provenance does: a server-side proposal is staged by the system
// or by a human and carries no passport.
//
// The end-to-end path is exercised over a real database in
// compose/integration; this pins the predicate itself, which is where the
// distinction is actually made.

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestServerProposedTurnsOnThePassport(t *testing.T) {
	minted := ids.From[ids.PassportKind](ids.NewV7())

	if serverProposed(row{Kind: "enrich", PassportID: &minted}) {
		t.Error("a staging carrying a passport was minted by an agent asserting one — " +
			"approving it must not invoke a server-side executor")
	}
	if !serverProposed(row{Kind: "enrich"}) {
		t.Error("a staging with no passport is a server-side proposal — its executor must still run, " +
			"or the provenance check would disable the confirm-first proposal flows it protects")
	}
}

// The other end of the same distinction: what makes a NULL passport_id TRUE.
//
// serverProposed reads the column and answers "the server made this". An agent
// with no passport and no person it acts for would write NULL there too. Its
// proposal would then read as the server's. The self-approval refusal compares
// the two passports, so against a NULL it never fires. The credential would
// release the call it made.
func TestAnAgentStagesOnItsPassportOrForANamedPerson(t *testing.T) {
	minted := ids.From[ids.PassportKind](ids.NewV7())
	for name, tc := range map[string]struct {
		actor   principal.Principal
		refused bool
	}{
		"an agent presenting its passport": {
			actor: principal.Principal{
				Type: principal.PrincipalAgent, ID: "agent:x", OnBehalfOf: ids.NewV7(), PassportID: minted.UUID,
			},
		},
		"an agent presenting none, acting for no one": {
			actor:   principal.Principal{Type: principal.PrincipalAgent, ID: "agent:x"},
			refused: true,
		},
		// The product's own job acting for a named person: the row carries
		// that person in on_behalf_of, and no credential exists to release it.
		"an agent presenting none, acting for a named person": {
			actor: principal.Principal{
				Type: principal.PrincipalAgent, ID: "agent:x", UserID: ids.NewV7(), OnBehalfOf: ids.NewV7(),
			},
		},
		// The case the refusal must not reach. A nightly sweep and a site read
		// propose on nobody's credential, and their NULL is the one this column
		// is read for — refusing it would break the path being protected.
		"the system, which carries no passport by design": {
			actor: principal.Principal{Type: principal.PrincipalSystem, ID: "system:site-read"},
		},
		"a human, who is not a credential at all": {
			actor: principal.Principal{Type: principal.PrincipalHuman, ID: "human:x", UserID: ids.NewV7()},
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := attributableStager(tc.actor)
			if tc.refused && err == nil {
				t.Error("staged a proposal that would read as the server's own, which is the row " +
					"agentMayDecide cannot refuse a self-approval on")
			}
			if !tc.refused && err != nil {
				t.Errorf("refused a legitimate stager: %v", err)
			}
		})
	}
}
