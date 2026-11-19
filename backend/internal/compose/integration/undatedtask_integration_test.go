// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A task a person writes with no due date waits under Upcoming and backlog.
// It sits in a No due date group after the dated work, and never on Today.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// undatedBacklog is one more undated task than the upcoming lane holds. The
// rest arrive in the lazy batches, and the total is the store's count.
const undatedBacklog = 7

// writeTask writes a task as a person does, or as an automatic source does
// when sourceSystem names one.
func writeTask(t *testing.T, e *Env, contact ids.UUID, subject string, due *time.Time, sourceSystem ...string) ids.UUID {
	t.Helper()
	in := activities.LogActivityInput{
		Kind: "task", Subject: &subject, Source: provenance.RecordSourceManual, DueAt: due,
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	}
	for _, source := range sourceSystem {
		in.SourceSystem, in.SourceID = &source, new(ids.NewV7().String())
	}
	task, _, err := e.Activities.LogActivity(e.Admin(), in)
	if err != nil {
		t.Fatalf("writing %q: %v", subject, err)
	}
	return ids.UUID(task.Id)
}

func TestAnUndatedTaskWaitsUnderUpcomingAndNeverOnToday(t *testing.T) {
	e := Setup(t)
	asOf := clocktest.Now(t)
	contact := e.SeedContact(t, "Renewal Contact", &e.AdminUser)
	dated := writeTask(t, e, contact, "Send the proposal", new(asOf.AddDate(0, 0, 3)))
	undated := map[string]bool{}
	for range undatedBacklog {
		undated[writeTask(t, e, contact, "Call back about the renewal", nil).String()] = true
	}
	owedNow := writeTask(t, e, contact, "Follow up as suggested", nil, provenance.AcceptedRecommendationSource).String()
	worklist := func(scope, filter string) crmcontracts.Worklist {
		page, err := compose.NewAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return asOf }).
			Worklist(e.Admin(), scope, filter, ids.UUID{}, 100, "")
		if err != nil {
			t.Fatal(err)
		}
		return page
	}

	for _, scope := range []string{"mine", "all"} {
		today := false
		for _, row := range worklist(scope, "").Queue {
			if undated[row.Id] {
				t.Errorf("the %s Today queue holds the undated task %s", scope, row.Id)
			}
			today = today || row.Id == owedNow
		}
		if !today {
			t.Errorf("the %s Today queue lost the undated task an accepted suggestion owes now", scope)
		}
		page := worklist(scope, string(crmcontracts.WorklistFilterUpcoming))
		if got := countTotal(page, crmcontracts.WorklistCountCategoryUpcoming); got != undatedBacklog+1 {
			t.Errorf("the %s upcoming total is %d, want the dated task and %d undated ones", scope, got, undatedBacklog)
		}
		var order []string
		for _, row := range page.Queue {
			if row.Id == owedNow {
				t.Errorf("the %s upcoming queue also holds the undated task Today holds", scope)
			}
			if row.Id != dated.String() && !undated[row.Id] {
				continue
			}
			order = append(order, row.Id)
			if undated[row.Id] && (row.DueGroup == nil || *row.DueGroup != crmcontracts.WorklistItemDueGroupWorklistDueGroupNone) {
				t.Errorf("the %s undated task %s carries group %v, want none", scope, row.Id, row.DueGroup)
			}
		}
		if len(order) != undatedBacklog+1 || order[0] != dated.String() {
			t.Errorf("the %s upcoming queue lists %v, want the dated task first and then all %d undated ones",
				scope, order, undatedBacklog)
		}
	}
}
