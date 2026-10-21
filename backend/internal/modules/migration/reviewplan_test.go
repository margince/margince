// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"strconv"
	"testing"
)

func TestReviewPlanRefusesAHiddenTailOrOverlappingTarget(t *testing.T) {
	t.Run("no partial row census", func(t *testing.T) {
		plan := ReviewPlan{Version: 1, Complete: true}
		for i := range ReviewRowLimit {
			plan.Add(ReviewRow{Object: ObjectLead, ExternalID: strconv.Itoa(i), Outcome: "create"})
		}
		if !plan.Complete || len(plan.Rows) != ReviewRowLimit {
			t.Fatalf("complete bounded plan was lost: complete=%v rows=%d", plan.Complete, len(plan.Rows))
		}
		plan.Add(ReviewRow{Object: ObjectLead, ExternalID: "one-more", Outcome: "create"})
		if plan.Complete || len(plan.Rows) != 0 {
			t.Fatal("an omitted source tail remained an approvable plan")
		}
	})
	t.Run("two updates to one version", func(t *testing.T) {
		plan := ReviewPlan{Version: 1, Complete: true}
		for _, key := range []string{"first", "second"} {
			plan.Add(ReviewRow{Object: ObjectCompany, ExternalID: key, TargetID: "same-company", TargetVersion: 3, Outcome: "update"})
		}
		if plan.Complete {
			t.Fatal("overlapping updates promised two writes against one original version")
		}
	})
	t.Run("duplicate source identity", func(t *testing.T) {
		plan := ReviewPlan{Version: 1, Complete: true}
		plan.Add(ReviewRow{Object: ObjectLead, ExternalID: "same-source", Outcome: "create"})
		plan.Add(ReviewRow{Object: ObjectLead, ExternalID: "same-source", Outcome: "skip"})
		if plan.Complete || len(plan.Rows) != 0 {
			t.Fatal("duplicate source identity remained approvable")
		}
	})
}
