// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func TestOnlyAnOpenUnroutedTaskNobodyHoldsIsTakeable(t *testing.T) {
	task := func(mutate func(*crmcontracts.Activity)) crmcontracts.Activity {
		a := crmcontracts.Activity{Kind: crmcontracts.ActivityKindTask}
		mutate(&a)
		return a
	}
	system := func(a *crmcontracts.Activity, source string) { a.SourceSystem = &source }
	for name, tc := range map[string]struct {
		activity crmcontracts.Activity
		want     bool
	}{
		"a hand-made unassigned task": {task(func(*crmcontracts.Activity) {}), true},
		"a note":                      {crmcontracts.Activity{Kind: crmcontracts.ActivityKindNote}, false},
		"a done task":                 {task(func(a *crmcontracts.Activity) { a.IsDone = new(true) }), false},
		"a privacy request task":      {task(func(a *crmcontracts.Activity) { system(a, provenance.PrivacyRequestTaskSource) }), false},
		"a lead SLA task":             {task(func(a *crmcontracts.Activity) { system(a, provenance.LeadSLATaskSource) }), false},
		"an assurance task":           {task(func(a *crmcontracts.Activity) { system(a, provenance.AssuranceTaskSource) }), false},
		"a quiet-account reminder":    {task(func(a *crmcontracts.Activity) { system(a, provenance.NoActivityReminderSource) }), false},
		"a website inquiry task": {task(func(a *crmcontracts.Activity) {
			system(a, provenance.WebsiteInquirySource)
			a.SourceId = new("submission-1")
		}), false},
		"a task from a caller's own system": {task(func(a *crmcontracts.Activity) { system(a, "crm_import_x") }), true},
	} {
		if got := TakeableTask(tc.activity); got != tc.want {
			t.Errorf("%s: takeable = %v, want %v", name, got, tc.want)
		}
	}
}
