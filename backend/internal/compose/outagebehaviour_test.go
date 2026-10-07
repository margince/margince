// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/companybrief"
	"github.com/margince/margince/backend/internal/compose/companydossier"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// blockedLane is a model lane whose every provider refuses the call.
type blockedLane struct{}

func (blockedLane) Complete(context.Context, model.Request) (model.Response, error) {
	return model.Response{}, ai.ErrProviderQuota
}

// What the AI tasks screen and the outage page say a task does without a model
// is read from the contract, so each task's declaration is held to what its
// real writer does when the lane fails: a task declared to degrade answers
// from its own facts and says so, and one that is not declared does not.
func TestEachTaskIsDeclaredForWhatItsWriterDoesWithoutAModel(t *testing.T) {
	ctx := context.Background()
	for name, probe := range map[string]struct {
		task        ai.Task
		degradesNow func() bool
	}{
		"company brief (summarize)": {ai.TaskSummarize, func() bool {
			_, by, err := companybrief.Write(ctx, blockedLane{}, "co-1", companybrief.Input{}, "en")
			return err == nil && by == crmcontracts.WrittenByDeterministic
		}},
		"company fit (growth_fit)": {ai.TaskGrowthFit, func() bool {
			_, by, laneFailed := companydossier.WriteGrowthFit(ctx, blockedLane{}, companydossier.Input{}, false, func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }, "en")
			return by == crmcontracts.WrittenByDeterministic && laneFailed
		}},
	} {
		t.Run(name, func(t *testing.T) {
			if got := probe.degradesNow(); got != ai.DegradesOnOutage(probe.task) {
				t.Errorf("%s: the writer degrades=%v but the contract declares degrades=%v, so the screen and the outage page say the wrong thing",
					probe.task, got, ai.DegradesOnOutage(probe.task))
			}
		})
	}
}
