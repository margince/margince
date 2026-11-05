// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

// nothingSlipsSummary is what the AI activity rail shows under a sweep that
// found none of its member's deals slipping.
var nothingSlipsSummary = langcopy.Phrase{
	En: "None of your open deals is slipping, so nothing was swept tonight.",
	De: "Keiner Ihrer offenen Deals gerät ins Stocken, daher wurde heute Nacht nichts geprüft.",
	Vi: "Không có giao dịch đang mở nào của bạn bị chậm lại, nên đêm nay không có gì cần rà soát.",
}

// closeWhenNothingSlips ends a run without a model call when its goal judges
// the member's slipping deals and that list is empty. runCtx carries the run,
// so the list is the member's own (scheduledRunOwners), read at the window its
// goal names. A list that cannot be read lets the run go ahead.
func (s *RunnerService) closeWhenNothingSlips(runCtx context.Context, spec runner.AgentSpec, runID ids.UUID) (runner.Result, bool) {
	if !spec.JudgesSlippingDeals {
		return runner.Result{}, false
	}
	slipping, err := slippingLister(s.pool)(runCtx, runner.SweepQuietDays)
	if err != nil {
		s.log.Warn("runner: reading the member's slipping deals before the run", "run", runID, "cause", err)
		return runner.Result{}, false
	}
	if len(slipping) > 0 {
		return runner.Result{}, false
	}
	language := langcopy.For(identity.BaseLanguageForPrompt(runCtx, s.pool))
	closed, err := runner.NothingToJudge(language.Say(nothingSlipsSummary))
	if err != nil {
		s.log.Warn("runner: closing a run with nothing to judge", "run", runID, "cause", err)
		return runner.Result{}, false
	}
	return closed, true
}
