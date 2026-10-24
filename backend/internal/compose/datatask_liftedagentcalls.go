// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"sort"
	"sync"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/datatask"
)

// liftedAgentCallsDataTask closes the approvals an agent on a colleague's passport
// staged before such an agent could run the call itself.
func liftedAgentCallsDataTask() datatask.Task {
	kinds := liftedAgentCallKinds()
	return datatask.Task{
		ID: "core/1792729335_lifted_agent_approvals_withdrawn", Version: 1,
		Title: "Withdrawing agent approvals that no longer need a human", Requires: []string{"core/1792562859"},
		Steps: []datatask.Step{{
			Name: "withdraw_lifted_agent_calls", Limit: 200, MinLimit: 10,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: approvals.EstimateLiftedAgentCalls(kinds), Batch: approvals.WithdrawLiftedAgentCallsTx(kinds),
		}},
	}
}

// liftedAgentCallKinds names the registered tools that no longer wait for an
// agent acting as its user; an agent's approval kind is its tool's name. A tool
// marked WaitsEvenForItsUser is left out, so its approvals stay.
var liftedAgentCallKinds = sync.OnceValue(func() []string {
	var kinds []string
	for _, spec := range NewRegistry(nil, SendPath{}).Specs() {
		if !spec.WaitsEvenForItsUser {
			kinds = append(kinds, spec.Name)
		}
	}
	sort.Strings(kinds)
	return kinds
})
