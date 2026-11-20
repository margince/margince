// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// liftedAgentCallsDataTask closes the approvals an agent on a colleague's passport
// staged before such an agent could run the call itself.
func liftedAgentCallsDataTask() datatask.Task {
	kinds := approvals.LiftedAgentCallKinds()
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
