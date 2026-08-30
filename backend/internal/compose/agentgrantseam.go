// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The rep's standing overnight answer, joined to the credential it commits.
//
// agents/runner owns the answer; identity owns the passport that makes the
// answer mean something. Neither may import the other, so the edge is injected
// here — identity takes the agentgrant port, and this adapter is the runner
// store wearing it.
//
// The port is deliberately narrower than the store: two methods, both
// transaction-taking, neither able to name a user. What identity can do with a
// grant is exactly answer for the signed-in rep and read back what committed.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/platform/agentgrant"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// agentGrantStore adapts the runner's standing-grant store to the port
// identity holds.
type agentGrantStore struct{ store *runner.Store }

// MyAnswerTx reads the acting rep's own answer. The store's method takes no
// user id, so neither can this.
func (a agentGrantStore) MyAnswerTx(ctx context.Context, tx pgx.Tx, spec string) (agentgrant.Answer, bool, error) {
	grant, found, err := a.store.MyGrantTx(ctx, tx, spec)
	if err != nil || !found {
		return agentgrant.Answer{}, false, err
	}
	return agentgrant.Answer{
		Spec:             grant.Spec,
		State:            grant.State,
		PassportID:       grant.PassportID,
		CredentialUsable: grant.CredentialUsable,
		PassportScopes:   grant.PassportScopes,
		DecidedAt:        grant.DecidedAt,
	}, true, nil
}

// RecordAnswerTx writes the acting rep's answer. The state vocabulary is the
// port's, and it is mapped rather than passed through: the two packages agree
// on the words today, and a silent pass-through would let one of them change
// the vocabulary without the other failing to compile.
func (a agentGrantStore) RecordAnswerTx(
	ctx context.Context, tx pgx.Tx, spec, state string, passportID *ids.PassportID,
) error {
	stored := runner.GrantStateDeclined
	if state == agentgrant.StateGranted {
		stored = runner.GrantStateGranted
	}
	return runner.RecordDecisionTx(ctx, tx, spec, stored, passportID)
}

// grantableAgentNames is the catalog a rep may be asked about, by name.
//
// It reads the JOINED catalog rather than runner.Catalog() directly, so the
// question a rep is asked is about the agents this build actually schedules —
// the same list the nightly fan-out walks. Two spellings would let the product
// ask for authority over an agent that never runs, or run one nobody was asked
// about.
func grantableAgentNames() []string {
	specs := mustScheduledAgents()
	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		names = append(names, spec.Name)
	}
	return names
}
