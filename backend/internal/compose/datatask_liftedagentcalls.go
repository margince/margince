// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"slices"
	"sort"
	"sync"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// liftedAgentCallsDataTask closes the approvals an agent on a colleague's passport
// staged before such an agent could run the call itself.
func liftedAgentCallsDataTask(composed *Extensions) datatask.Task {
	kinds := liftedAgentCallKinds(composed)
	return datatask.Task{
		ID: "core/1792729335_lifted_agent_approvals_withdrawn", Version: 1,
		Title: systemtext.Literal("datatask.title.core.1792729335_lifted_agent_approvals_withdrawn"), Requires: []string{"core/1792562859"},
		Steps: []datatask.Step{{
			Name: "withdraw_lifted_agent_calls", Limit: 200, MinLimit: 10,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: approvals.EstimateLiftedAgentCalls(kinds), Batch: approvals.WithdrawLiftedAgentCallsTx(kinds),
		}},
	}
}

// liftedAgentCallKinds names the registered tools that no longer wait for an
// agent acting as its user; an agent's approval kind is its tool's name. A tool
// marked WaitsEvenForItsUser is left out, so its approvals stay. The
// composition's unit tools are registered beside the core's, so they count.
func liftedAgentCallKinds(composed *Extensions) []string {
	kinds := slices.Clone(coreLiftedAgentCallKinds())
	for _, tool := range composed.composedTools() {
		if spec := tool.Spec(); !spec.WaitsEvenForItsUser {
			kinds = append(kinds, spec.Name)
		}
	}
	sort.Strings(kinds)
	return kinds
}

var coreLiftedAgentCallKinds = sync.OnceValue(func() []string {
	var kinds []string
	for _, spec := range NewRegistry(nil, SendPath{}).Specs() {
		if !spec.WaitsEvenForItsUser {
			kinds = append(kinds, spec.Name)
		}
	}
	return kinds
})
