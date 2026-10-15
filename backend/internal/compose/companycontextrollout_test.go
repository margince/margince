// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/config/configtest"
)

// An upgrade keeps today's behaviour until an operator stages it back. So a
// file that says nothing is at onboarding, and a stage is read as written.
func TestTheCompanyContextRolloutDefaultsToOnboardingAndRefusesAnUnknownStage(t *testing.T) {
	if got := config.Val(configtest.MustLoad(t, "version: 1\n", CompanyContextRollout), CompanyContextRollout); got != companyContextRolloutOnboarding {
		t.Errorf("default rollout = %q, want onboarding", got)
	}
	tasks := configtest.MustLoad(t, "company_context:\n  rollout: tasks\n", CompanyContextRollout)
	if !CompanyContextTasks(tasks) || companyContextOnboardingEnabled(config.Val(tasks, CompanyContextRollout)) {
		t.Errorf("rollout tasks: tasks on and onboarding off expected, got %q", config.Val(tasks, CompanyContextRollout))
	}
	if _, err := configtest.Load(t, "company_context:\n  rollout: everything\n", nil, nil, CompanyContextRollout); err == nil {
		t.Error("an unknown rollout stage loaded")
	}
}
