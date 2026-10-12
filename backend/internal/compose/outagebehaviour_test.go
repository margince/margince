// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/companybrief"
	"github.com/margince/margince/backend/internal/compose/companydossier"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
			now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			_, by, laneFailed := companydossier.WriteGrowthFit(ctx, blockedLane{}, judgeableCompany(now), false, func() time.Time { return now }, "en")
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

// judgeableCompany holds enough required inputs for a fit to be judged, so the
// writer asks its lane. Below the floor it abstains without asking.
func judgeableCompany(now time.Time) companydossier.Input {
	read := now.Add(-24 * time.Hour)
	var fields []crmcontracts.CompanyProfileField
	for _, field := range []crmcontracts.CompanyProfileFieldField{
		crmcontracts.CompanyProfileFieldFieldOfferSummary, crmcontracts.CompanyProfileFieldFieldIcp,
		crmcontracts.CompanyProfileFieldFieldIndustry, crmcontracts.CompanyProfileFieldFieldBuyingCenter,
	} {
		id := openapi_types.UUID(ids.NewV7())
		fields = append(fields, crmcontracts.CompanyProfileField{
			Id: &id, Field: field, Value: "recorded",
			Source: crmcontracts.CompanyProfileFieldSourceSiteRead, RetrievedAt: &read, UpdatedAt: read,
		})
	}
	return companydossier.Input{CompanyID: ids.NewV7().String(), ProfileFields: fields}
}
